package workloadautoscaler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client/helper"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/gkeaccess"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

var (
	_ resource.Resource                = &WorkloadAutoscaler{}
	_ resource.ResourceWithImportState = &WorkloadAutoscaler{}
)

var uninstallWorkloadAutoscaler = helper.UninstallWorkloadAutoscaler

var (
	recommendationPolicyDeleteRetryInterval = 2 * time.Second
	recommendationPolicyDeleteRetryTimeout  = 30 * time.Second
)

type WorkloadAutoscaler struct {
	client cloudpilotaiclient.Interface
}

type postWriteStateHydratorClient interface {
	GetWAConfiguration(clusterID string) (*api.WAConfiguration, error)
	ListRecommendationPolicies(clusterID string) ([]api.RecommendationPolicyResource, error)
	ListAutoscalingPolicies(clusterID string) ([]api.AutoscalingPolicyResource, error)
}

func NewWorkloadAutoscaler() resource.Resource {
	return &WorkloadAutoscaler{}
}

func (w *WorkloadAutoscaler) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workload_autoscaler"
}

func (w *WorkloadAutoscaler) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = Schema(ctx)
}

func (w *WorkloadAutoscaler) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(cloudpilotaiclient.Interface)
	if !ok {
		resp.Diagnostics.AddError(
			"unexpected resource configure type",
			fmt.Sprintf("Expected cloudpilotaiclient.Interface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	w.client = client
}

func (w *WorkloadAutoscaler) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("cluster_id"), req, resp)

	// Mark as import so Read fetches all remote resources instead of only
	// the ones tracked in state. This enables terraform plan
	// -generate-config-out= to produce a complete configuration file.
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "is_import", []byte("true"))...)
}

func (w *WorkloadAutoscaler) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data WorkloadAutoscalerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	kubeconfigPath, err := w.fillMissingParameters(ctx, &data, false)
	if err != nil {
		resp.Diagnostics.AddError("failed to fill missing parameters", err.Error())
		return
	}

	clusterID := data.ClusterID.ValueString()
	if kubeconfigPath == "" {
		resp.Diagnostics.AddError(
			"kubeconfig is required",
			"The kubeconfig attribute must be set for create operations.",
		)
		return
	}
	needInstall := false
	waConfig, err := w.client.GetWAConfiguration(clusterID)
	if err != nil {
		tflog.Warn(ctx, fmt.Sprintf("failed to get Workload Autoscaler configuration before installation, fallback to install path: %v", err))
		needInstall = true
	} else if waConfig == nil || waConfig.WorkloadAutoscalerInstalled == nil || !*waConfig.WorkloadAutoscalerInstalled {
		needInstall = true
	} else {
		tflog.Info(ctx, "Workload Autoscaler is already installed, skipping installation script")
	}

	if needInstall {
		// 1. Install Workload Autoscaler via shell script
		tflog.Info(ctx, "installing CloudPilot AI Workload Autoscaler")
		if err := helper.InstallWorkloadAutoscaler(ctx, w.client,
			kubeconfigPath,
			stringValueOrDefault(data.StorageClass, ""),
			boolValueOrDefault(data.EnableNodeAgent, true),
		); err != nil {
			if helper.IsWorkloadAutoscalerInstallNotReadyError(err) {
				tflog.Warn(ctx, fmt.Sprintf("Workload Autoscaler install script reported not ready after Helm install; continuing with provider readiness checks: %v", err))
				resp.Diagnostics.AddWarning(
					"Workload Autoscaler install is still becoming ready",
					"The install script reported that the Workload Autoscaler was not ready immediately after Helm install. Terraform will continue and use provider-side readiness/configuration checks.",
				)
			} else {
				resp.Diagnostics.AddError(
					"failed to install Workload Autoscaler",
					err.Error(),
				)
				return
			}
		} else {
			tflog.Info(ctx, "installed Workload Autoscaler successfully")
		}
	}

	// 2. Enable WA configuration on the backend
	tflog.Info(ctx, "enabling Workload Autoscaler configuration")
	if err := w.updateWAConfiguration(ctx, clusterID, &data); err != nil {
		resp.Diagnostics.AddError("failed to update Workload Autoscaler configuration", err.Error())
		return
	}

	// 3. Wait for the WA to be ready by polling for configuration
	tflog.Info(ctx, "waiting for Workload Autoscaler to be ready")
	if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 5*time.Minute, true, func(ctx context.Context) (done bool, err error) {
		conf, getErr := w.client.GetWAConfiguration(clusterID)
		if getErr != nil {
			tflog.Info(ctx, "waiting for Workload Autoscaler to be ready")
			return false, nil
		}
		if conf.WorkloadAutoscalerInstalled != nil && *conf.WorkloadAutoscalerInstalled {
			return true, nil
		}
		return false, nil
	}); err != nil {
		tflog.Warn(ctx, fmt.Sprintf("timed out waiting for WA ready status (non-fatal): %v", err))
	}

	// 4. Sync policies (no previous state on Create, so pass nil — nothing to delete)
	if err := w.syncPolicies(ctx, &data, clusterID, nil, nil); err != nil {
		resp.Diagnostics.AddError(
			"failed to sync policies",
			err.Error(),
		)
		return
	}
	if err := hydratePostWriteState(ctx, w.client, clusterID, &data); err != nil {
		resp.Diagnostics.AddError("failed to hydrate Workload Autoscaler state after sync", err.Error())
		return
	}

	tflog.Info(ctx, "created Workload Autoscaler resource successfully")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (w *WorkloadAutoscaler) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data WorkloadAutoscalerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	kubeconfigPath, err := w.fillMissingParameters(ctx, &data, false)
	if err != nil {
		resp.Diagnostics.AddError("failed to fill missing parameters", err.Error())
		return
	}

	// Read previous state to know which resources were previously tracked
	var state WorkloadAutoscalerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	previousRPNames := extractPolicyNames(ctx, state.RecommendationPolicies, func(m api.RecommendationPolicyModel) string {
		return m.Name.ValueString()
	})
	previousAPNames := extractPolicyNames(ctx, state.AutoscalingPolicies, func(m api.AutoscalingPolicyModel) string {
		return m.Name.ValueString()
	})

	clusterID := data.ClusterID.ValueString()

	needReinstall := managedBoolChanged(data.EnableNodeAgent, state.EnableNodeAgent) ||
		managedStringChanged(data.StorageClass, state.StorageClass)
	if !needReinstall && !shouldSkipBackendInstallCheckForStateBackfill(data, state) {
		waConfig, err := w.client.GetWAConfiguration(clusterID)
		if err != nil {
			tflog.Warn(ctx, fmt.Sprintf("failed to get Workload Autoscaler configuration before upgrade, fallback to reinstall path: %v", err))
			needReinstall = true
		} else if waConfig == nil || waConfig.WorkloadAutoscalerInstalled == nil || !*waConfig.WorkloadAutoscalerInstalled {
			needReinstall = true
		}
	}

	if needReinstall {
		if kubeconfigPath == "" {
			resp.Diagnostics.AddError(
				"kubeconfig is required",
				"The kubeconfig attribute must be set when Workload Autoscaler installation is needed.",
			)
			return
		}
		tflog.Info(ctx, "upgrading CloudPilot AI Workload Autoscaler")
		if err := helper.InstallWorkloadAutoscaler(ctx, w.client,
			kubeconfigPath,
			stringValueOrDefault(data.StorageClass, ""),
			boolValueOrDefault(data.EnableNodeAgent, true),
		); err != nil {
			if helper.IsWorkloadAutoscalerInstallNotReadyError(err) {
				tflog.Warn(ctx, fmt.Sprintf("Workload Autoscaler upgrade script reported not ready after Helm install; continuing with provider readiness checks: %v", err))
				resp.Diagnostics.AddWarning(
					"Workload Autoscaler upgrade is still becoming ready",
					"The upgrade script reported that the Workload Autoscaler was not ready immediately after Helm upgrade. Terraform will continue and use provider-side readiness/configuration checks.",
				)
			} else {
				resp.Diagnostics.AddError(
					"failed to upgrade Workload Autoscaler",
					err.Error(),
				)
				return
			}
		} else {
			tflog.Info(ctx, "upgraded Workload Autoscaler successfully")
		}
	} else {
		tflog.Info(ctx, "Workload Autoscaler install settings unchanged and component already installed, skipping upgrade")
	}

	if err := w.updateWAConfiguration(ctx, clusterID, &data); err != nil {
		resp.Diagnostics.AddError("failed to update Workload Autoscaler configuration", err.Error())
		return
	}

	// Sync policies, passing previous state names so only removed policies are deleted
	if err := w.syncPolicies(ctx, &data, clusterID, previousRPNames, previousAPNames); err != nil {
		resp.Diagnostics.AddError(
			"failed to sync policies",
			err.Error(),
		)
		return
	}
	if err := hydratePostWriteState(ctx, w.client, clusterID, &data); err != nil {
		resp.Diagnostics.AddError("failed to hydrate Workload Autoscaler state after sync", err.Error())
		return
	}

	tflog.Info(ctx, "updated Workload Autoscaler resource successfully")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (w *WorkloadAutoscaler) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data WorkloadAutoscalerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	isImport := false
	if importFlag, diags := req.Private.GetKey(ctx, "is_import"); !diags.HasError() && string(importFlag) == "true" {
		isImport = true
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, "is_import", []byte("false"))...)
	}

	clusterID := data.ClusterID.ValueString()

	if conf, err := w.client.GetWAConfiguration(clusterID); err != nil {
		tflog.Warn(ctx, fmt.Sprintf("failed to read Workload Autoscaler configuration: %v", err))
	} else {
		mergeWAConfigurationFromAPI(&data, conf, isImport)
	}

	// Read recommendation policies
	rps, err := w.client.ListRecommendationPolicies(clusterID)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to list recommendation policies",
			err.Error(),
		)
		return
	}

	if !data.RecommendationPolicies.IsNullOrUnknown() {
		stateRPs, diags := data.RecommendationPolicies.AsStructSliceT(ctx)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		rpByName := make(map[string]api.RecommendationPolicyModel, len(rps))
		for i := range rps {
			m := preserveRecommendationPolicyStateRepresentation(
				api.RecommendationPolicyModelFromResource(&rps[i]),
				findRecommendationPolicyStateModel(stateRPs, rps[i].Name),
			)
			rpByName[rps[i].Name] = m
		}

		rpModels := orderByState(stateRPs, rpByName, func(m api.RecommendationPolicyModel) string {
			return m.Name.ValueString()
		})

		rpList, diags := customfield.NewObjectList(ctx, rpModels)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		data.RecommendationPolicies = rpList
	} else if isImport && len(rps) > 0 {
		allRPs := make([]api.RecommendationPolicyModel, 0, len(rps))
		for i := range rps {
			allRPs = append(allRPs, api.RecommendationPolicyModelFromResource(&rps[i]))
		}
		data.RecommendationPolicies = customfield.NewObjectListMust(ctx, allRPs)
	}

	// Read autoscaling policies
	aps, err := w.client.ListAutoscalingPolicies(clusterID)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to list autoscaling policies",
			err.Error(),
		)
		return
	}

	if !data.AutoscalingPolicies.IsNullOrUnknown() {
		if err := mergeAutoscalingPoliciesFromAPI(ctx, &data, aps, isImport); err != nil {
			resp.Diagnostics.AddError("failed to merge autoscaling policies", err.Error())
			return
		}
	} else if isImport {
		if err := mergeAutoscalingPoliciesFromAPI(ctx, &data, aps, true); err != nil {
			resp.Diagnostics.AddError("failed to merge autoscaling policies", err.Error())
			return
		}
	}

	if err := hydratePostWriteState(ctx, w.client, clusterID, &data); err != nil {
		resp.Diagnostics.AddError("failed to hydrate Workload Autoscaler state after read", err.Error())
		return
	}
	if err := w.maybeHydrateExecutionAccess(ctx, &data, isImport); err != nil {
		resp.Diagnostics.AddWarning(
			"skipped GKE kubeconfig auto-discovery",
			fmt.Sprintf("The provider could not auto-generate kubeconfig during read: %s", err),
		)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func hydratePostWriteState(ctx context.Context, client postWriteStateHydratorClient, clusterID string, data *WorkloadAutoscalerModel) error {
	conf, err := client.GetWAConfiguration(clusterID)
	if err != nil {
		return fmt.Errorf("failed to get Workload Autoscaler configuration: %w", err)
	}
	mergeWAConfigurationFromAPI(data, conf, false)

	rps, err := client.ListRecommendationPolicies(clusterID)
	if err != nil {
		return fmt.Errorf("failed to list recommendation policies: %w", err)
	}
	if err := hydrateRecommendationPoliciesPostWrite(ctx, data, rps); err != nil {
		return err
	}

	aps, err := client.ListAutoscalingPolicies(clusterID)
	if err != nil {
		return fmt.Errorf("failed to list autoscaling policies: %w", err)
	}
	if err := hydrateAutoscalingPoliciesPostWrite(ctx, data, aps); err != nil {
		return err
	}

	return nil
}

func hydrateRecommendationPoliciesPostWrite(ctx context.Context, data *WorkloadAutoscalerModel, rps []api.RecommendationPolicyResource) error {
	if data.RecommendationPolicies.IsNullOrUnknown() {
		return nil
	}

	stateRPs, diags := data.RecommendationPolicies.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("recommendation policies diagnostics = %v", diags)
	}

	rpByName := make(map[string]api.RecommendationPolicyModel, len(rps))
	for i := range rps {
		rpByName[rps[i].Name] = preserveRecommendationPolicyStateRepresentation(
			api.RecommendationPolicyModelFromResource(&rps[i]),
			findRecommendationPolicyStateModel(stateRPs, rps[i].Name),
		)
	}

	ordered := orderByState(stateRPs, rpByName, func(m api.RecommendationPolicyModel) string {
		return m.Name.ValueString()
	})
	data.RecommendationPolicies = customfield.NewObjectListMust(ctx, ordered)
	return nil
}

func hydrateAutoscalingPoliciesPostWrite(ctx context.Context, data *WorkloadAutoscalerModel, aps []api.AutoscalingPolicyResource) error {
	return mergeAutoscalingPoliciesFromAPI(ctx, data, aps, false)
}

func findAutoscalingPolicyStateModel(items []api.AutoscalingPolicyModel, name string) api.AutoscalingPolicyModel {
	for _, item := range items {
		if item.Name.ValueString() == name {
			return item
		}
	}
	return api.AutoscalingPolicyModel{}
}

func findRecommendationPolicyStateModel(items []api.RecommendationPolicyModel, name string) api.RecommendationPolicyModel {
	for _, item := range items {
		if item.Name.ValueString() == name {
			return item
		}
	}
	return api.RecommendationPolicyModel{}
}

func mergeAutoscalingPoliciesFromAPI(ctx context.Context, data *WorkloadAutoscalerModel, aps []api.AutoscalingPolicyResource, importAll bool) error {
	if data.AutoscalingPolicies.IsNullOrUnknown() {
		if importAll && len(aps) > 0 {
			allAPs := make([]api.AutoscalingPolicyModel, 0, len(aps))
			for i := range aps {
				allAPs = append(allAPs, api.AutoscalingPolicyModelFromResource(ctx, &aps[i]))
			}
			data.AutoscalingPolicies = customfield.NewObjectListMust(ctx, allAPs)
		}
		return nil
	}

	stateAPs, diags := data.AutoscalingPolicies.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("autoscaling policies diagnostics = %v", diags)
	}

	apByName := make(map[string]api.AutoscalingPolicyModel, len(aps))
	for i := range aps {
		model := api.AutoscalingPolicyModelFromResource(ctx, &aps[i])
		if !importAll {
			model = preserveAutoscalingPolicyStateRepresentation(ctx, model, findAutoscalingPolicyStateModel(stateAPs, aps[i].Name))
		}
		apByName[aps[i].Name] = model
	}

	ordered := orderByState(stateAPs, apByName, func(m api.AutoscalingPolicyModel) string {
		return m.Name.ValueString()
	})
	list, diags := customfield.NewObjectList(ctx, ordered)
	if diags.HasError() {
		return fmt.Errorf("autoscaling policies diagnostics = %v", diags)
	}
	data.AutoscalingPolicies = list
	return nil
}

func preserveRecommendationPolicyStateRepresentation(remote, state api.RecommendationPolicyModel) api.RecommendationPolicyModel {
	remote.StrategyType = preserveManagedString(state.StrategyType, remote.StrategyType)
	remote.PercentileCPU = preserveManagedInt32(state.PercentileCPU, remote.PercentileCPU)
	remote.PercentileMemory = preserveManagedInt32(state.PercentileMemory, remote.PercentileMemory)
	remote.BufferCPU = preserveManagedString(state.BufferCPU, remote.BufferCPU)
	remote.BufferMemory = preserveManagedString(state.BufferMemory, remote.BufferMemory)
	remote.RequestMinCPU = preserveManagedString(state.RequestMinCPU, remote.RequestMinCPU)
	remote.RequestMinMemory = preserveManagedString(state.RequestMinMemory, remote.RequestMinMemory)
	remote.RequestMaxCPU = preserveManagedString(state.RequestMaxCPU, remote.RequestMaxCPU)
	remote.RequestMaxMemory = preserveManagedString(state.RequestMaxMemory, remote.RequestMaxMemory)
	remote.JVMHeapBuffer = preserveManagedString(state.JVMHeapBuffer, remote.JVMHeapBuffer)
	remote.JVMMinHeapXmsRatioOfMemory = preserveManagedString(state.JVMMinHeapXmsRatioOfMemory, remote.JVMMinHeapXmsRatioOfMemory)
	remote.JVMRecentNonHeapWindow = preserveManagedString(state.JVMRecentNonHeapWindow, remote.JVMRecentNonHeapWindow)
	remote.JVMHeapUsedPercentile = preserveManagedInt32(state.JVMHeapUsedPercentile, remote.JVMHeapUsedPercentile)
	return remote
}

func preserveAutoscalingPolicyStateRepresentation(ctx context.Context, remote, state api.AutoscalingPolicyModel) api.AutoscalingPolicyModel {
	remote.Enable = preserveManagedBool(state.Enable, remote.Enable)
	remote.Priority = preserveManagedInt64(state.Priority, remote.Priority)
	remote.DisableRuntimeOptimization = preserveManagedBool(state.DisableRuntimeOptimization, remote.DisableRuntimeOptimization)
	remote.UpdateResources = preserveManagedStringList(state.UpdateResources, remote.UpdateResources)
	remote.DriftThresholdCPU = preserveManagedString(state.DriftThresholdCPU, remote.DriftThresholdCPU)
	remote.DriftThresholdMemory = preserveManagedString(state.DriftThresholdMemory, remote.DriftThresholdMemory)
	remote.OnPolicyRemoval = preserveManagedString(state.OnPolicyRemoval, remote.OnPolicyRemoval)
	remote.TargetRefs = preserveManagedObjectList(ctx, state.TargetRefs, remote.TargetRefs)
	remote.UpdateSchedules = preserveManagedObjectList(ctx, state.UpdateSchedules, remote.UpdateSchedules)
	remote.LimitPolicies = preserveManagedObjectList(ctx, state.LimitPolicies, remote.LimitPolicies)
	remote.StartupBoostEnabled = preserveManagedBool(state.StartupBoostEnabled, remote.StartupBoostEnabled)
	remote.StartupBoostMinBoostDuration = preserveManagedString(state.StartupBoostMinBoostDuration, remote.StartupBoostMinBoostDuration)
	remote.StartupBoostMinReadyDuration = preserveManagedString(state.StartupBoostMinReadyDuration, remote.StartupBoostMinReadyDuration)
	remote.StartupBoostMultiplierCPU = preserveManagedString(state.StartupBoostMultiplierCPU, remote.StartupBoostMultiplierCPU)
	remote.StartupBoostMultiplierMemory = preserveManagedString(state.StartupBoostMultiplierMemory, remote.StartupBoostMultiplierMemory)
	remote.InPlaceFallbackDefaultPolicy = preserveManagedString(state.InPlaceFallbackDefaultPolicy, remote.InPlaceFallbackDefaultPolicy)
	if remote.InPlaceFallbackReasonPolicies.IsNull() || remote.InPlaceFallbackReasonPolicies.IsUnknown() {
		if !state.InPlaceFallbackReasonPolicies.IsNull() && !state.InPlaceFallbackReasonPolicies.IsUnknown() {
			values, diags := state.InPlaceFallbackReasonPolicies.Value(ctx)
			if !diags.HasError() && len(values) == 0 {
				remote.InPlaceFallbackReasonPolicies = state.InPlaceFallbackReasonPolicies
			}
		}
	} else if state.InPlaceFallbackReasonPolicies.IsNull() || state.InPlaceFallbackReasonPolicies.IsUnknown() {
		remote.InPlaceFallbackReasonPolicies = customfield.NullMap[types.String](ctx)
	}
	return remote
}

// orderByState returns server items that are tracked in state, preserving
// state order. Items on the server but NOT in state are ignored — Terraform
// only manages resources it declared.
func orderByState[T any](stateItems []T, serverByName map[string]T, getName func(T) string) []T {
	result := make([]T, 0, len(stateItems))
	for _, stateItem := range stateItems {
		name := getName(stateItem)
		if serverItem, ok := serverByName[name]; ok {
			result = append(result, serverItem)
		}
	}
	return result
}

func deleteRecommendationPolicy(ctx context.Context, client cloudpilotaiclient.Interface, clusterID, name string) error {
	var lastInUseErr error
	err := wait.PollUntilContextTimeout(ctx, recommendationPolicyDeleteRetryInterval, recommendationPolicyDeleteRetryTimeout, true, func(ctx context.Context) (bool, error) {
		err := client.DeleteRecommendationPolicy(clusterID, name)
		if err == nil {
			return true, nil
		}
		if strings.Contains(err.Error(), "not found") {
			return true, nil
		}
		if strings.Contains(err.Error(), "in use") {
			lastInUseErr = err
			tflog.Info(ctx, fmt.Sprintf("recommendation policy %s is still in use, retrying deletion", name))
			return false, nil
		}
		return false, err
	})
	if err != nil && lastInUseErr != nil {
		return lastInUseErr
	}
	return err
}

func (w *WorkloadAutoscaler) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data WorkloadAutoscalerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	kubeconfigPath, err := w.fillMissingParameters(ctx, &data, false)
	if err != nil {
		resp.Diagnostics.AddError("failed to fill missing parameters", err.Error())
		return
	}

	clusterID := data.ClusterID.ValueString()
	var deferredRecommendationPolicies []string

	// 1. Delete only Terraform-tracked autoscaling policies (they reference recommendation policies)
	if !data.AutoscalingPolicies.IsNullOrUnknown() {
		apModels, diags := data.AutoscalingPolicies.AsStructSliceT(ctx)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		for _, ap := range apModels {
			name := ap.Name.ValueString()
			tflog.Info(ctx, fmt.Sprintf("deleting autoscaling policy: %s", name))
			if err := w.client.DeleteAutoscalingPolicy(clusterID, name); err != nil {
				if strings.Contains(err.Error(), "not found") {
					continue
				}
				resp.Diagnostics.AddWarning(
					fmt.Sprintf("skipped deletion of autoscaling policy %s", name),
					err.Error(),
				)
				continue
			}
		}
	}

	// 2. Delete only Terraform-tracked recommendation policies
	if !data.RecommendationPolicies.IsNullOrUnknown() {
		rpModels, diags := data.RecommendationPolicies.AsStructSliceT(ctx)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		for _, rp := range rpModels {
			name := rp.Name.ValueString()
			tflog.Info(ctx, fmt.Sprintf("deleting recommendation policy: %s", name))
			if err := w.client.DeleteRecommendationPolicy(clusterID, name); err != nil {
				if strings.Contains(err.Error(), "not found") {
					continue
				}
				if strings.Contains(err.Error(), "in use") {
					tflog.Info(ctx, fmt.Sprintf("recommendation policy %s is still in use, deferring deletion until after Workload Autoscaler uninstall", name))
					deferredRecommendationPolicies = append(deferredRecommendationPolicies, name)
					continue
				}
				resp.Diagnostics.AddWarning(
					fmt.Sprintf("skipped deletion of recommendation policy %s", name),
					err.Error(),
				)
				continue
			}
		}
	}

	// 3. Disable Workload Autoscaler, wait for resources cleanup, then helm uninstall
	if kubeconfigPath == "" {
		resp.Diagnostics.AddError(
			"kubeconfig is required",
			"The kubeconfig attribute must be set for delete operations.",
		)
		return
	}
	tflog.Info(ctx, "uninstalling Workload Autoscaler")
	if err := uninstallWorkloadAutoscaler(ctx, w.client, clusterID, kubeconfigPath); err != nil {
		resp.Diagnostics.AddError(
			"failed to uninstall Workload Autoscaler",
			err.Error(),
		)
		return
	}

	for _, name := range deferredRecommendationPolicies {
		tflog.Info(ctx, fmt.Sprintf("deleting deferred recommendation policy: %s", name))
		if err := deleteRecommendationPolicy(ctx, w.client, clusterID, name); err != nil {
			if strings.Contains(err.Error(), "not found") {
				continue
			}
			if strings.Contains(err.Error(), "in use") {
				resp.Diagnostics.AddWarning(
					fmt.Sprintf("skipped deletion of recommendation policy %s", name),
					"RecommendationPolicy is still in use by an AutoscalingPolicy. It was not deleted.",
				)
				continue
			}
			resp.Diagnostics.AddWarning(
				fmt.Sprintf("skipped deletion of recommendation policy %s", name),
				err.Error(),
			)
			continue
		}
	}

	// 4. Update backend configuration
	installedFalse := false
	if err := w.client.UpdateWAConfiguration(clusterID, &api.WAConfiguration{
		WorkloadAutoscalerInstalled: &installedFalse,
	}); err != nil {
		tflog.Warn(ctx, fmt.Sprintf("failed to update WA configuration (non-fatal): %v", err))
	}

	tflog.Info(ctx, "deleted Workload Autoscaler resource successfully")
}

func (w *WorkloadAutoscaler) fillMissingParameters(ctx context.Context, data *WorkloadAutoscalerModel, persistGeneratedKubeconfig bool) (string, error) {
	originalKubeconfig := stringValue(data.Kubeconfig)
	info := gkeaccess.AccessInfo{
		ClusterID:  stringValue(data.ClusterID),
		Kubeconfig: originalKubeconfig,
	}

	summary, err := w.client.GetCluster(info.ClusterID)
	if err != nil {
		return "", err
	}
	if summary == nil || summary.CloudProvider != api.CloudProviderGCP {
		return originalKubeconfig, nil
	}

	info.ClusterName = summary.ClusterName
	info.Region = summary.Region
	if err := gkeaccess.EnsureKubeconfigAvailable(ctx, w.client, &info, nil); err != nil {
		return "", err
	}
	if info.Kubeconfig != "" && (persistGeneratedKubeconfig || originalKubeconfig != "") {
		data.Kubeconfig = types.StringValue(info.Kubeconfig)
	}
	return info.Kubeconfig, nil
}

func (w *WorkloadAutoscaler) maybeHydrateExecutionAccess(ctx context.Context, data *WorkloadAutoscalerModel, persistGeneratedKubeconfig bool) error {
	_, err := w.fillMissingParameters(ctx, data, persistGeneratedKubeconfig)
	return err
}

func (w *WorkloadAutoscaler) syncPolicies(ctx context.Context, data *WorkloadAutoscalerModel, clusterID string,
	previousRPNames, previousAPNames map[string]struct{},
) error {
	// Phase 1: Apply all desired RPs (APs may reference them)
	tflog.Info(ctx, "applying recommendation policies")
	rpDesired, err := helper.ApplyRecommendationPolicies(ctx, w.client, clusterID, data.RecommendationPolicies)
	if err != nil {
		return fmt.Errorf("failed to apply recommendation policies: %w", err)
	}

	// Phase 2: Apply APs, then delete stale APs (only those previously tracked in state)
	tflog.Info(ctx, "applying autoscaling policies")
	apDesired, err := helper.ApplyAutoscalingPolicies(ctx, w.client, clusterID, data.AutoscalingPolicies)
	if err != nil {
		return fmt.Errorf("failed to apply autoscaling policies: %w", err)
	}
	if err := helper.DeleteStaleAutoscalingPolicies(ctx, w.client, clusterID, apDesired, previousAPNames); err != nil {
		return fmt.Errorf("failed to delete stale autoscaling policies: %w", err)
	}

	// Phase 3: Delete stale RPs (only those previously tracked in state, now safe)
	tflog.Info(ctx, "deleting stale recommendation policies")
	if err := helper.DeleteStaleRecommendationPolicies(ctx, w.client, clusterID, rpDesired, previousRPNames); err != nil {
		return fmt.Errorf("failed to delete stale recommendation policies: %w", err)
	}

	// Phase 4: Apply proactive update settings
	tflog.Info(ctx, "applying proactive update settings")
	if err := helper.ApplyProactiveUpdates(ctx, w.client, clusterID, data.EnableProactive); err != nil {
		return fmt.Errorf("failed to apply proactive updates: %w", err)
	}

	// Phase 5: Apply disable proactive update settings
	tflog.Info(ctx, "applying disable proactive update settings")
	if err := helper.ApplyDisableProactiveUpdates(ctx, w.client, clusterID, data.DisableProactive); err != nil {
		return fmt.Errorf("failed to apply disable proactive updates: %w", err)
	}

	tflog.Info(ctx, "synced all policies successfully")
	return nil
}

func (w *WorkloadAutoscaler) updateWAConfiguration(ctx context.Context, clusterID string, data *WorkloadAutoscalerModel) error {
	enableTrue := true
	conf := data.ToWAConfiguration()
	conf.EnableWorkloadAutoscaler = &enableTrue
	return w.client.UpdateWAConfiguration(clusterID, conf)
}

// extractPolicyNames extracts the set of names from a NestedObjectList in
// Terraform state. Returns nil if the list is null/unknown. This is used to
// determine which resources were previously managed by Terraform so that only
// those can be considered for deletion during sync.
func extractPolicyNames[T any](ctx context.Context, list customfield.NestedObjectList[T], getName func(T) string) map[string]struct{} {
	if list.IsNullOrUnknown() {
		return nil
	}
	items, diags := list.AsStructSliceT(ctx)
	if diags.HasError() {
		return nil
	}
	names := make(map[string]struct{}, len(items))
	for _, item := range items {
		names[getName(item)] = struct{}{}
	}
	return names
}

func shouldSkipBackendInstallCheckForStateBackfill(data, state WorkloadAutoscalerModel) bool {
	return state.Kubeconfig.ValueString() == "" &&
		data.Kubeconfig.ValueString() != "" &&
		!managedBoolChanged(data.EnableNodeAgent, state.EnableNodeAgent) &&
		!managedStringChanged(data.StorageClass, state.StorageClass)
}

func mergeWAConfigurationFromAPI(data *WorkloadAutoscalerModel, conf *api.WAConfiguration, importAll bool) {
	if conf == nil {
		return
	}

	if importAll || !data.EnableNewWorkloadsProactiveUpdate.IsNull() {
		if conf.EnableNewWorkloadsProactiveUpdate != nil {
			data.EnableNewWorkloadsProactiveUpdate = types.BoolValue(*conf.EnableNewWorkloadsProactiveUpdate)
		} else {
			data.EnableNewWorkloadsProactiveUpdate = types.BoolNull()
		}
	}
	if importAll || !data.LimiterQuotaPerWindow.IsNull() {
		if conf.LimiterQuotaPerWindow != nil {
			data.LimiterQuotaPerWindow = types.Int64Value(int64(*conf.LimiterQuotaPerWindow))
		} else {
			data.LimiterQuotaPerWindow = types.Int64Null()
		}
	}
	if importAll || !data.LimiterBurst.IsNull() {
		if conf.LimiterBurst != nil {
			data.LimiterBurst = types.Int64Value(int64(*conf.LimiterBurst))
		} else {
			data.LimiterBurst = types.Int64Null()
		}
	}
	if importAll || !data.LimiterWindowSeconds.IsNull() {
		if conf.LimiterWindowSeconds != nil {
			data.LimiterWindowSeconds = types.Int64Value(int64(*conf.LimiterWindowSeconds))
		} else {
			data.LimiterWindowSeconds = types.Int64Null()
		}
	}
	if importAll || !data.EnablePreemptedPodGC.IsNull() {
		if conf.EnablePreemptedPodGC != nil {
			data.EnablePreemptedPodGC = types.BoolValue(*conf.EnablePreemptedPodGC)
		} else {
			data.EnablePreemptedPodGC = types.BoolNull()
		}
	}
	if importAll || !data.PreemptedPodGCTTL.IsNull() {
		if conf.PreemptedPodGCTTL != nil {
			data.PreemptedPodGCTTL = types.StringValue(*conf.PreemptedPodGCTTL)
		} else {
			data.PreemptedPodGCTTL = types.StringNull()
		}
	}
	if importAll || !data.EnableInitialOptimizationDataWindowCheck.IsNull() {
		if conf.EnableInitialOptimizationDataWindowCheck != nil {
			data.EnableInitialOptimizationDataWindowCheck = types.BoolValue(*conf.EnableInitialOptimizationDataWindowCheck)
		} else {
			data.EnableInitialOptimizationDataWindowCheck = types.BoolNull()
		}
	}
}

func isManagedBool(value types.Bool) bool {
	return !value.IsNull()
}

func isManagedInt64(value types.Int64) bool {
	return !value.IsNull()
}

func isManagedString(value types.String) bool {
	return !value.IsNull()
}

func boolValueOrDefault(value types.Bool, fallback bool) bool {
	if !isManagedBool(value) {
		return fallback
	}
	return value.ValueBool()
}

func stringValueOrDefault(value types.String, fallback string) string {
	if !isManagedString(value) {
		return fallback
	}
	return value.ValueString()
}

func stringValue(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}

func managedBoolChanged(plan, state types.Bool) bool {
	if !isManagedBool(plan) {
		return false
	}
	if !isManagedBool(state) {
		return true
	}
	return plan.ValueBool() != state.ValueBool()
}

func managedStringChanged(plan, state types.String) bool {
	if !isManagedString(plan) {
		return false
	}
	if !isManagedString(state) {
		return true
	}
	return plan.ValueString() != state.ValueString()
}

func preserveManagedBool(stateVal, remoteVal types.Bool) types.Bool {
	if stateVal.IsNull() {
		return types.BoolNull()
	}
	return remoteVal
}

func preserveManagedInt64(stateVal, remoteVal types.Int64) types.Int64 {
	if stateVal.IsNull() {
		return types.Int64Null()
	}
	return remoteVal
}

func preserveManagedInt32(stateVal, remoteVal types.Int32) types.Int32 {
	if stateVal.IsNull() {
		return types.Int32Null()
	}
	return remoteVal
}

func preserveManagedString(stateVal, remoteVal types.String) types.String {
	if stateVal.IsNull() {
		return types.StringNull()
	}
	return remoteVal
}

func preserveManagedStringList(stateVal, remoteVal *[]types.String) *[]types.String {
	if stateVal == nil {
		return nil
	}
	if remoteVal == nil && len(*stateVal) == 0 {
		return stateVal
	}
	return remoteVal
}

func preserveManagedObjectList[T any](ctx context.Context, stateVal, remoteVal customfield.NestedObjectList[T]) customfield.NestedObjectList[T] {
	if stateVal.IsNull() {
		return customfield.NullObjectList[T](ctx)
	}
	if remoteVal.IsNullOrUnknown() {
		values, diags := stateVal.AsStructSliceT(ctx)
		if !diags.HasError() && len(values) == 0 {
			return stateVal
		}
	}
	return remoteVal
}
