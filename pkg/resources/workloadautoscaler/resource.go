package workloadautoscaler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client/helper"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

var _ resource.Resource = &WorkloadAutoscaler{}

type WorkloadAutoscaler struct {
	client cloudpilotaiclient.Interface
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

func (w *WorkloadAutoscaler) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data WorkloadAutoscalerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := data.ClusterID.ValueString()
	kubeconfigPath := data.Kubeconfig.ValueString()

	// 1. Install Workload Autoscaler via shell script
	tflog.Info(ctx, "installing CloudPilot AI Workload Autoscaler")
	if err := helper.InstallWorkloadAutoscaler(ctx, w.client,
		kubeconfigPath,
		data.StorageClass.ValueString(),
		data.EnableNodeAgent.ValueBool(),
	); err != nil {
		resp.Diagnostics.AddError(
			"failed to install Workload Autoscaler",
			err.Error(),
		)
		return
	}
	tflog.Info(ctx, "installed Workload Autoscaler successfully")

	// 2. Enable WA configuration on the backend
	tflog.Info(ctx, "enabling Workload Autoscaler configuration")
	enableTrue := true
	if err := w.client.UpdateWAConfiguration(clusterID, &api.WAConfiguration{
		EnableWorkloadAutoscaler: &enableTrue,
	}); err != nil {
		tflog.Warn(ctx, fmt.Sprintf("failed to update WA configuration (non-fatal): %v", err))
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

	// Re-install WA to pick up any configuration changes (storage_class, enable_node_agent)
	tflog.Info(ctx, "upgrading CloudPilot AI Workload Autoscaler")
	if err := helper.InstallWorkloadAutoscaler(ctx, w.client,
		data.Kubeconfig.ValueString(),
		data.StorageClass.ValueString(),
		data.EnableNodeAgent.ValueBool(),
	); err != nil {
		resp.Diagnostics.AddError(
			"failed to upgrade Workload Autoscaler",
			err.Error(),
		)
		return
	}
	tflog.Info(ctx, "upgraded Workload Autoscaler successfully")

	// Sync policies, passing previous state names so only removed policies are deleted
	if err := w.syncPolicies(ctx, &data, clusterID, previousRPNames, previousAPNames); err != nil {
		resp.Diagnostics.AddError(
			"failed to sync policies",
			err.Error(),
		)
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

	clusterID := data.ClusterID.ValueString()

	// Read recommendation policies
	if !data.RecommendationPolicies.IsNullOrUnknown() {
		rps, err := w.client.ListRecommendationPolicies(clusterID)
		if err != nil {
			resp.Diagnostics.AddError(
				"failed to list recommendation policies",
				err.Error(),
			)
			return
		}

		rpByName := make(map[string]api.RecommendationPolicyModel, len(rps))
		for i := range rps {
			m := api.RecommendationPolicyModelFromResource(&rps[i])
			rpByName[rps[i].Name] = m
		}

		stateRPs, diags := data.RecommendationPolicies.AsStructSliceT(ctx)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
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
	}

	// Read autoscaling policies
	if !data.AutoscalingPolicies.IsNullOrUnknown() {
		aps, err := w.client.ListAutoscalingPolicies(clusterID)
		if err != nil {
			resp.Diagnostics.AddError(
				"failed to list autoscaling policies",
				err.Error(),
			)
			return
		}

		apByName := make(map[string]api.AutoscalingPolicyModel, len(aps))
		for i := range aps {
			m := api.AutoscalingPolicyModelFromResource(ctx, &aps[i])
			apByName[aps[i].Name] = m
		}

		stateAPs, diags := data.AutoscalingPolicies.AsStructSliceT(ctx)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		apModels := orderByState(stateAPs, apByName, func(m api.AutoscalingPolicyModel) string {
			return m.Name.ValueString()
		})

		apList, diags := customfield.NewObjectList(ctx, apModels)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		data.AutoscalingPolicies = apList
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
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

func (w *WorkloadAutoscaler) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data WorkloadAutoscalerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := data.ClusterID.ValueString()

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
				resp.Diagnostics.AddError(
					fmt.Sprintf("failed to delete autoscaling policy %s", name),
					err.Error(),
				)
				return
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
					resp.Diagnostics.AddWarning(
						fmt.Sprintf("skipped deletion of recommendation policy %s", name),
						"RecommendationPolicy is still in use by an AutoscalingPolicy. It was not deleted.",
					)
					continue
				}
				resp.Diagnostics.AddError(
					fmt.Sprintf("failed to delete recommendation policy %s", name),
					err.Error(),
				)
				return
			}
		}
	}

	// 3. Disable Workload Autoscaler, wait for resources cleanup, then helm uninstall
	tflog.Info(ctx, "uninstalling Workload Autoscaler")
	if err := helper.UninstallWorkloadAutoscaler(ctx, w.client, clusterID, data.Kubeconfig.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"failed to uninstall Workload Autoscaler",
			err.Error(),
		)
		return
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
