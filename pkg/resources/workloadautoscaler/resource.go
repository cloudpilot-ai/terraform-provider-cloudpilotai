package workloadautoscaler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client/helper"
	awsauth "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/utils/aws"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/gkeaccess"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"k8s.io/apimachinery/pkg/util/wait"
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
		kubeconfigPath, err := w.fillMissingParameters(ctx, &data, false)
		if err != nil {
			resp.Diagnostics.AddError("failed to fill missing parameters", err.Error())
			return
		}
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
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
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
	kubeconfigPath, err := w.fillMissingParameters(ctx, &data, true)
	if err != nil {
		resp.Diagnostics.AddError("failed to fill missing parameters", err.Error())
		return
	}

	clusterID := data.ClusterID.ValueString()

	if !w.deleteTrackedAutoscalingPolicies(ctx, clusterID, data, resp) {
		return
	}
	deferredRecommendationPolicies, ok := w.deleteTrackedRecommendationPolicies(ctx, clusterID, data, resp)
	if !ok {
		return
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

func (w *WorkloadAutoscaler) deleteTrackedAutoscalingPolicies(ctx context.Context, clusterID string, data WorkloadAutoscalerModel, resp *resource.DeleteResponse) bool {

	// 1. Delete only Terraform-tracked autoscaling policies (they reference recommendation policies)
	if !data.AutoscalingPolicies.IsNullOrUnknown() {
		apModels, diags := data.AutoscalingPolicies.AsStructSliceT(ctx)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return false
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

	return true
}

func (w *WorkloadAutoscaler) deleteTrackedRecommendationPolicies(ctx context.Context, clusterID string, data WorkloadAutoscalerModel, resp *resource.DeleteResponse) ([]string, bool) {
	var deferredRecommendationPolicies []string

	// 2. Delete only Terraform-tracked recommendation policies
	if !data.RecommendationPolicies.IsNullOrUnknown() {
		rpModels, diags := data.RecommendationPolicies.AsStructSliceT(ctx)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return nil, false
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

	return deferredRecommendationPolicies, true
}

func (w *WorkloadAutoscaler) fillMissingParameters(ctx context.Context, data *WorkloadAutoscalerModel, pathFromState bool) (string, error) {
	originalKubeconfig := stringValue(data.Kubeconfig)
	clusterID := stringValue(data.ClusterID)
	summary, err := w.client.GetCluster(clusterID)
	if err != nil {
		return "", err
	}
	if summary == nil {
		return originalKubeconfig, nil
	}

	switch summary.CloudProvider {
	case api.CloudProviderGCP:
		info := gkeaccess.AccessInfo{
			ClusterID:       clusterID,
			ClusterName:     summary.ClusterName,
			Region:          summary.Region,
			ClusterLocation: stringValue(data.GCPLocation),
			ProjectID:       stringValue(data.GCPProjectID),
			Kubeconfig:      originalKubeconfig,
		}
		ensureKubeconfig := gkeaccess.EnsureKubeconfigAvailable
		if pathFromState {
			ensureKubeconfig = gkeaccess.EnsureKubeconfigAvailableFromState
		}
		if err := ensureKubeconfig(ctx, w.client, &info, nil); err != nil {
			return "", err
		}
		return info.Kubeconfig, nil
	case api.CloudProviderAWS:
		authCfg, err := executionAuthConfigFromModel(ctx, *data)
		if err != nil {
			return "", err
		}
		if pathFromState {
			return awsauth.EnsureKubeconfigAvailableFromState(ctx, summary.ClusterName, summary.Region, originalKubeconfig, authCfg)
		}
		return awsauth.EnsureKubeconfigAvailable(ctx, summary.ClusterName, summary.Region, originalKubeconfig, authCfg)
	default:
		return originalKubeconfig, nil
	}
}

func executionAuthConfigFromModel(ctx context.Context, data WorkloadAutoscalerModel) (awsauth.ExecutionAuthConfig, error) {
	cfg := awsauth.ExecutionAuthConfig{}
	if !data.AWSProfile.IsNull() && !data.AWSProfile.IsUnknown() {
		cfg.Profile = data.AWSProfile.ValueString()
	}
	if data.AWSAssumeRole.IsNull() || data.AWSAssumeRole.IsUnknown() {
		return cfg, nil
	}

	value, diags := data.AWSAssumeRole.Value(ctx)
	if diags.HasError() {
		return cfg, fmt.Errorf("failed to decode aws_assume_role: %v", diags)
	}
	if value == nil {
		return cfg, nil
	}
	if !value.RoleARN.IsNull() && !value.RoleARN.IsUnknown() {
		cfg.AssumeRoleARN = value.RoleARN.ValueString()
	}
	if !value.SessionName.IsNull() && !value.SessionName.IsUnknown() {
		cfg.AssumeRoleSessionName = value.SessionName.ValueString()
	}
	return cfg, nil
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
	if !data.AutoscalingPolicies.IsNullOrUnknown() {
		if err := helper.DeleteStaleAutoscalingPolicies(ctx, w.client, clusterID, apDesired, previousAPNames); err != nil {
			return fmt.Errorf("failed to delete stale autoscaling policies: %w", err)
		}
	}

	// Phase 3: Delete stale RPs (only those previously tracked in state, now safe)
	tflog.Info(ctx, "deleting stale recommendation policies")
	if !data.RecommendationPolicies.IsNullOrUnknown() {
		if err := helper.DeleteStaleRecommendationPolicies(ctx, w.client, clusterID, rpDesired, previousRPNames); err != nil {
			return fmt.Errorf("failed to delete stale recommendation policies: %w", err)
		}
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
