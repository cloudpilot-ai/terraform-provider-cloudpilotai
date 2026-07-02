package gke

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

var (
	_ resource.Resource                = &Cluster{}
	_ resource.ResourceWithImportState = &Cluster{}
	_ resource.ResourceWithModifyPlan  = &Cluster{}
)

const clusterReadyPollTimeout = 5 * time.Minute

var uninstallCloudpilotAIAgentComponent = helper.UninstallCloudpilotAIAgentComponent
var restoreCloudpilotAIAfterUninstall = helper.RestoreCloudpilotAIRebalanceComponentWithEnv
var installCloudpilotAIAgentComponent = helper.InstallCloudpilotAIAgentComponent

type Cluster struct {
	client cloudpilotaiclient.Interface
}

type postWriteStateHydratorClient interface {
	GetClusterSetting(clusterID string) (*api.ClusterSetting, error)
	GetRebalanceConfiguration(clusterID string) (*api.RebalanceConfig, error)
	ListNodeClasses(clusterID string) (api.RebalanceNodeClassList, error)
	ListNodePools(clusterID string) (api.RebalanceNodePoolList, error)
}

type clusterSummaryReader interface {
	GetCluster(clusterID string) (*api.ClusterCostsSummary, error)
}

func NewCluster() resource.Resource {
	return &Cluster{}
}

func (c *Cluster) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gke_cluster"
}

func (c *Cluster) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = Schema(ctx)
}

func (c *Cluster) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(cloudpilotaiclient.Interface)
	if !ok {
		resp.Diagnostics.AddError(
			"unexpected resource configure type",
			fmt.Sprintf("Expected cloudpilotai cloudpilotaiclient.Interface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	c.client = client
}

func (c *Cluster) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var plan ClusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := resolveClusterUID(plan.ClusterID, state.ClusterID, plan.ClusterName, plan.Region, state.ClusterUID)
	if clusterID == "" {
		return
	}

	summary, err := c.client.GetCluster(clusterID)
	if err != nil {
		if errors.Is(err, cloudpilotaiclient.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("failed to refresh upgrade status during plan", err.Error())
		return
	}

	applyClusterSummaryStatus(&plan, summary)
	if planMayUpgradeCluster(plan, summary) {
		markClusterSummaryStatusUnknown(&plan)
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

func (c *Cluster) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterID := req.ID

	clusterInfo, err := c.client.GetCluster(clusterID)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to import cluster",
			fmt.Sprintf("Could not retrieve cluster %q: %s", clusterID, err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_name"), clusterInfo.ClusterName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("region"), clusterInfo.Region)...)
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "is_import", []byte("true"))...)
}

func (c *Cluster) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := c.fillMissingParameters(ctx, &data); err != nil {
		resp.Diagnostics.AddError("failed to fill missing parameters", err.Error())
		return
	}
	if err := validateClusterIdentity(&data); err != nil {
		resp.Diagnostics.AddError("missing required gke identity fields", err.Error())
		return
	}

	clusterID := resolveClusterUID(data.ClusterID, types.StringNull(), data.ClusterName, data.Region, data.ClusterUID)
	data.ClusterID = types.StringValue(clusterID)

	if err := c.ensureAgentInstalled(ctx, &data, clusterID); err != nil {
		resp.Diagnostics.AddError("failed to install cloudpilot ai agent", err.Error())
		return
	}
	if err := c.ensureNodeAutoscalerInstalled(ctx, &data, clusterID); err != nil {
		resp.Diagnostics.AddError("failed to install gke node autoscaler component", err.Error())
		return
	}

	if err := c.syncClusterSetting(ctx, &data, clusterID); err != nil {
		resp.Diagnostics.AddError("failed to sync cluster setting", err.Error())
		return
	}

	if err := c.upgradeComponentsIfNeeded(ctx, &data, clusterID); err != nil {
		resp.Diagnostics.AddError("failed to upgrade cloudpilot ai components", err.Error())
		return
	}
	if err := c.syncConfiguration(ctx, &data, clusterID, nil, nil); err != nil {
		resp.Diagnostics.AddError("failed to sync gke node autoscaler configuration", err.Error())
		return
	}

	if err := hydrateClusterPostWriteState(ctx, c.client, clusterID, &data); err != nil {
		resp.Diagnostics.AddError("failed to hydrate cluster state after sync", err.Error())
		return
	}

	if err := c.refreshClusterSummaryStatus(&data, clusterID); err != nil {
		resp.Diagnostics.AddError("failed to refresh cluster summary", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *Cluster) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ClusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := c.fillMissingParameters(ctx, &data); err != nil {
		resp.Diagnostics.AddError("failed to fill missing parameters", err.Error())
		return
	}
	if err := validateClusterIdentity(&data); err != nil {
		resp.Diagnostics.AddError("missing required gke identity fields", err.Error())
		return
	}

	clusterID := resolveClusterUID(data.ClusterID, state.ClusterID, data.ClusterName, data.Region, data.ClusterUID)
	data.ClusterID = types.StringValue(clusterID)

	if err := c.ensureAgentInstalled(ctx, &data, clusterID); err != nil {
		resp.Diagnostics.AddError("failed to install cloudpilot ai agent", err.Error())
		return
	}
	if err := c.ensureNodeAutoscalerInstalled(ctx, &data, clusterID); err != nil {
		resp.Diagnostics.AddError("failed to install gke node autoscaler component", err.Error())
		return
	}

	if err := c.syncClusterSetting(ctx, &data, clusterID); err != nil {
		resp.Diagnostics.AddError("failed to sync cluster setting", err.Error())
		return
	}

	if err := c.upgradeComponentsIfNeeded(ctx, &data, clusterID); err != nil {
		resp.Diagnostics.AddError("failed to upgrade cloudpilot ai components", err.Error())
		return
	}
	previousNCNames := extractResourceNames(ctx, state.NodeClasses, func(m api.GCENodeClassModel) string {
		return m.Name.ValueString()
	})
	previousNPNames := extractResourceNames(ctx, state.NodePools, func(m api.GCENodePoolModel) string {
		return m.Name.ValueString()
	})
	if err := c.syncConfiguration(ctx, &data, clusterID, previousNCNames, previousNPNames); err != nil {
		resp.Diagnostics.AddError("failed to sync gke node autoscaler configuration", err.Error())
		return
	}

	if err := hydrateClusterPostWriteState(ctx, c.client, clusterID, &data); err != nil {
		resp.Diagnostics.AddError("failed to hydrate cluster state after sync", err.Error())
		return
	}

	if err := c.refreshClusterSummaryStatus(&data, clusterID); err != nil {
		resp.Diagnostics.AddError("failed to refresh cluster summary", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *Cluster) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	isImport := false
	if importFlag, diags := req.Private.GetKey(ctx, "is_import"); !diags.HasError() && string(importFlag) == "true" {
		isImport = true
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, "is_import", []byte("false"))...)
	}

	clusterID := resolveClusterUID(data.ClusterID, types.StringNull(), data.ClusterName, data.Region, data.ClusterUID)
	if clusterID == "" {
		resp.Diagnostics.AddError("failed to resolve cluster id", "cluster UID could not be derived from the configured GKE identity")
		return
	}
	data.ClusterID = types.StringValue(clusterID)

	summary, err := c.client.GetCluster(clusterID)
	if err != nil {
		if errors.Is(err, cloudpilotaiclient.ErrNotFound) {
			resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
			return
		}
		resp.Diagnostics.AddError("failed to get cluster", err.Error())
		return
	}
	mergeClusterIdentityFromSummary(&data, summary)
	applyClusterSummaryStatus(&data, summary)
	if err := c.readClusterManagementState(ctx, &data, clusterID, isImport); err != nil {
		resp.Diagnostics.AddError("failed to hydrate gke management state", err.Error())
		return
	}
	if err := c.maybeHydrateExecutionAccess(ctx, &data); err != nil {
		resp.Diagnostics.AddWarning(
			"skipped GKE kubeconfig auto-discovery",
			fmt.Sprintf("The provider could not auto-generate kubeconfig during read: %s", err),
		)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *Cluster) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := c.fillMissingParameters(ctx, &data); err != nil {
		resp.Diagnostics.AddError("failed to fill missing parameters", err.Error())
		return
	}

	clusterID := resolveDeleteClusterUID(data.ClusterID, data.ClusterName, data.Region, data.ClusterUID)
	if err := c.deleteCluster(ctx, &data, clusterID); err != nil {
		var warning warningOnlyError
		if errors.As(err, &warning) {
			resp.Diagnostics.AddWarning(warning.summary, warning.detail)
		} else {
			resp.Diagnostics.AddError("failed to delete cluster", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *Cluster) ensureAgentInstalled(ctx context.Context, data *ClusterModel, clusterUID string) error {
	agentInstalled := false
	if _, err := c.client.GetCluster(clusterUID); err == nil {
		agentInstalled = true
	} else if !errors.Is(err, cloudpilotaiclient.ErrNotFound) {
		return err
	}

	if !agentInstalled {
		if err := c.fillMissingParameters(ctx, data); err != nil {
			return err
		}
		if err := installCloudpilotAIAgentComponent(
			ctx,
			c.client,
			api.CloudProviderGCP,
			data.ClusterName.ValueString(),
			data.Kubeconfig.ValueString(),
			boolValueOrDefault(data.DisableWorkloadUploading, false),
			nil,
		); err != nil {
			return err
		}
		if err := c.waitForClusterReady(ctx, clusterUID); err != nil {
			return err
		}
	}

	return nil
}

func (c *Cluster) upgradeComponentsIfNeeded(ctx context.Context, data *ClusterModel, clusterUID string) error {
	if data.EnableUpgrade.IsNull() || data.EnableUpgrade.IsUnknown() || !data.EnableUpgrade.ValueBool() {
		return nil
	}

	if err := c.fillMissingParameters(ctx, data); err != nil {
		return err
	}

	_, err := helper.UpgradeCloudpilotAIComponentsIfNeeded(
		ctx,
		c.client,
		clusterUID,
		api.CloudProviderGCP,
		data.Kubeconfig.ValueString(),
		"",
		nil,
	)
	return err
}

func (c *Cluster) ensureNodeAutoscalerInstalled(ctx context.Context, data *ClusterModel, clusterUID string) error {
	if !shouldManageNodeAutoscaler(*data) {
		return nil
	}

	rebalanceConfig, err := c.client.GetRebalanceConfiguration(clusterUID)
	needInstall := false
	if err != nil {
		if !errors.Is(err, cloudpilotaiclient.ErrNotFound) {
			return err
		}
		needInstall = true
	} else if rebalanceConfig == nil || rebalanceConfig.LastComponentsActiveTime.IsZero() {
		needInstall = true
	}

	if !needInstall {
		return nil
	}

	if err := c.fillMissingParameters(ctx, data); err != nil {
		return err
	}
	if data.Kubeconfig.ValueString() == "" {
		return fmt.Errorf("kubeconfig is required when the provider needs to install the gke node autoscaler component")
	}

	return installCloudpilotAIRebalanceComponent(
		ctx,
		c.client,
		clusterUID,
		api.CloudProviderGCP,
		data.Kubeconfig.ValueString(),
		"",
		nil,
	)
}

func (c *Cluster) waitForClusterReady(ctx context.Context, clusterUID string) error {
	return wait.PollUntilContextTimeout(ctx, 10*time.Second, clusterReadyPollTimeout, true, func(ctx context.Context) (bool, error) {
		_, err := c.client.GetCluster(clusterUID)
		if err != nil {
			if errors.Is(err, cloudpilotaiclient.ErrNotFound) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func validateClusterIdentity(data *ClusterModel) error {
	if data.ClusterID.IsNull() || data.ClusterID.IsUnknown() || data.ClusterID.ValueString() == "" {
		if data.ClusterUID.IsNull() || data.ClusterUID.IsUnknown() || data.ClusterUID.ValueString() == "" {
			return fmt.Errorf("set cluster_uid when cluster_id is unset")
		}
	}
	return nil
}

func (c *Cluster) fillMissingParameters(ctx context.Context, data *ClusterModel) error {
	projectHints, err := gkeaccess.ProjectIDCandidatesFromNodeClassModels(ctx, data.NodeClasses)
	if err != nil {
		return err
	}

	info := gkeaccess.AccessInfo{
		ClusterID:       stringValue(data.ClusterID),
		ClusterName:     stringValue(data.ClusterName),
		Region:          stringValue(data.Region),
		ClusterLocation: stringValue(data.ClusterLocation),
		ProjectID:       stringValue(data.ProjectID),
		Kubeconfig:      stringValue(data.Kubeconfig),
	}
	if err := gkeaccess.EnsureKubeconfigAvailable(ctx, c.client, &info, projectHints); err != nil {
		return err
	}

	if info.ClusterName != "" {
		data.ClusterName = types.StringValue(info.ClusterName)
	}
	if info.Region != "" {
		data.Region = types.StringValue(info.Region)
	}
	setOptionalComputedString(&data.ProjectID, info.ProjectID)
	if info.Kubeconfig != "" {
		data.Kubeconfig = types.StringValue(info.Kubeconfig)
	}
	if stringValue(data.ClusterUID) == "" && info.Kubeconfig != "" {
		clusterUID, err := gkeaccess.RunKubectlGetClusterUID(ctx, info.Kubeconfig)
		if err != nil {
			return err
		}
		if clusterUID != "" {
			data.ClusterUID = types.StringValue(clusterUID)
		}
	}
	return nil
}

func (c *Cluster) maybeHydrateExecutionAccess(ctx context.Context, data *ClusterModel) error {
	projectHints, err := gkeaccess.ProjectIDCandidatesFromNodeClassModels(ctx, data.NodeClasses)
	if err != nil {
		return err
	}

	info := gkeaccess.AccessInfo{
		ClusterID:       stringValue(data.ClusterID),
		ClusterName:     stringValue(data.ClusterName),
		Region:          stringValue(data.Region),
		ClusterLocation: stringValue(data.ClusterLocation),
		ProjectID:       stringValue(data.ProjectID),
		Kubeconfig:      stringValue(data.Kubeconfig),
	}
	if err := gkeaccess.EnsureKubeconfigAvailable(ctx, c.client, &info, projectHints); err != nil {
		return err
	}
	setOptionalComputedString(&data.ProjectID, info.ProjectID)
	if info.Kubeconfig != "" {
		data.Kubeconfig = types.StringValue(info.Kubeconfig)
	}
	if stringValue(data.ClusterUID) == "" && info.Kubeconfig != "" {
		clusterUID, err := gkeaccess.RunKubectlGetClusterUID(ctx, info.Kubeconfig)
		if err != nil {
			return err
		}
		if clusterUID != "" {
			data.ClusterUID = types.StringValue(clusterUID)
		}
	}
	return nil
}

func shouldManageNodeAutoscaler(data ClusterModel) bool {
	if !data.NodeClasses.IsNullOrUnknown() || !data.NodePools.IsNullOrUnknown() {
		return true
	}
	if !data.EnableRebalance.IsNull() && !data.EnableRebalance.IsUnknown() {
		return data.EnableRebalance.ValueBool()
	}
	if !data.OnlyInstallAgent.IsNull() && !data.OnlyInstallAgent.IsUnknown() {
		return !data.OnlyInstallAgent.ValueBool()
	}
	return true
}

func shouldSkipRebalanceDisableSync(data ClusterModel) bool {
	return boolValueOrDefault(data.OnlyInstallAgent, false) &&
		data.NodeClasses.IsNullOrUnknown() &&
		data.NodePools.IsNullOrUnknown()
}

func stringValue(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}

func setOptionalComputedString(value *types.String, inferred string) {
	if inferred != "" {
		*value = types.StringValue(inferred)
		return
	}
	if value.IsNull() || value.IsUnknown() || strings.TrimSpace(value.ValueString()) == "" {
		*value = types.StringNull()
	}
}

func boolValueOrDefault(value types.Bool, fallback bool) bool {
	if value.IsNull() || value.IsUnknown() {
		return fallback
	}
	return value.ValueBool()
}

func restoreEnvFromClusterModel(ctx context.Context, data ClusterModel) (map[string]string, error) {
	env := map[string]string{}

	if !data.RestoreNodeNumber.IsNull() && !data.RestoreNodeNumber.IsUnknown() {
		if data.RestoreNodeNumber.ValueInt64() < 0 {
			return nil, fmt.Errorf("restore_node_number must be non-negative")
		}
		if data.RestoreNodeNumber.ValueInt64() > 0 {
			env["RESTORE_DESIRED_SIZE"] = fmt.Sprintf("%d", data.RestoreNodeNumber.ValueInt64())
		}
	}

	if data.RestoreDesiredSizes.IsNull() || data.RestoreDesiredSizes.IsUnknown() {
		return env, nil
	}

	restoreSizes := map[string]types.Int64{}
	diags := data.RestoreDesiredSizes.ElementsAs(ctx, &restoreSizes, false)
	if diags.HasError() {
		return nil, fmt.Errorf("restore_desired_sizes: %v", diags)
	}

	keys := make([]string, 0, len(restoreSizes))
	for key := range restoreSizes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := restoreSizes[key]
		if value.IsNull() || value.IsUnknown() {
			continue
		}
		if value.ValueInt64() < 0 {
			return nil, fmt.Errorf("restore_desired_sizes[%q] must be non-negative", key)
		}
		env["RESTORE_DESIRED_SIZE_"+sanitizeRestoreNodePoolEnvSuffix(key)] = fmt.Sprintf("%d", value.ValueInt64())
	}

	return env, nil
}

func (c *Cluster) syncClusterSetting(ctx context.Context, data *ClusterModel, clusterUID string) error {
	if data.ClusterSetting.IsNull() || data.ClusterSetting.IsUnknown() {
		return nil
	}

	setting, diags := data.ClusterSetting.Value(ctx)
	if diags.HasError() {
		return fmt.Errorf("failed to parse cluster_setting: %v", diags)
	}
	if setting == nil {
		return nil
	}

	if err := c.client.UpdateClusterSetting(clusterUID, setting.ToAPI()); err != nil {
		return fmt.Errorf("failed to update cluster setting: %w", err)
	}

	return nil
}

func (c *Cluster) syncConfiguration(
	ctx context.Context,
	data *ClusterModel,
	clusterID string,
	previousNCNames, previousNPNames map[string]struct{},
) error {
	enableRebalanceManaged := !data.EnableRebalance.IsNull() && !data.EnableRebalance.IsUnknown()
	if enableRebalanceManaged && !data.EnableRebalance.ValueBool() {
		if shouldSkipRebalanceDisableSync(*data) {
			tflog.Info(ctx, "gke node autoscaler is unmanaged, skipping rebalance disable sync")
		} else {
			tflog.Info(ctx, "disabling gke rebalance configuration")
			if err := helper.SyncRebalanceConfiguration(ctx, c.client, clusterID, false); err != nil {
				if errors.Is(err, cloudpilotaiclient.ErrNotFound) {
					tflog.Info(ctx, "gke rebalance configuration is absent, treating disable as no-op")
				} else {
					return fmt.Errorf("failed to sync rebalance configuration: %w", err)
				}
			}
		}
	}

	tflog.Info(ctx, "syncing gke nodeclass configuration")
	nodeClassNames, err := helper.ApplyGCENodeClassConfiguration(ctx, c.client, clusterID, data.NodeClasses)
	if err != nil {
		return fmt.Errorf("failed to sync nodeclass configuration: %w", err)
	}

	tflog.Info(ctx, "syncing gke nodepool configuration")
	if err := helper.SyncGCENodePoolConfiguration(ctx, c.client, clusterID, data.NodePools, previousNPNames); err != nil {
		return fmt.Errorf("failed to sync nodepool configuration: %w", err)
	}

	if err := helper.DeleteStaleGCENodeClasses(ctx, c.client, clusterID, nodeClassNames, previousNCNames); err != nil {
		return fmt.Errorf("failed to delete stale nodeclass configuration: %w", err)
	}

	if enableRebalanceManaged && data.EnableRebalance.ValueBool() {
		tflog.Info(ctx, "enabling gke rebalance configuration")
		if err := helper.SyncRebalanceConfiguration(ctx, c.client, clusterID, true); err != nil {
			return fmt.Errorf("failed to sync rebalance configuration: %w", err)
		}
	}

	return nil
}

func (c *Cluster) deleteCluster(ctx context.Context, data *ClusterModel, clusterUID string) error {
	warnings := make([]string, 0, 5)
	if err := c.client.UpdateRebalanceConfiguration(clusterUID, &api.RebalanceConfig{Enable: false}); err != nil {
		warnings = append(warnings, fmt.Sprintf("skipped disabling remote rebalance configuration: %s", err))
	}
	if stringValue(data.ClusterID) == "" {
		data.ClusterID = types.StringValue(clusterUID)
	}

	if err := c.fillMissingParameters(ctx, data); err != nil {
		return err
	}

	if err := uninstallCloudpilotAIAgentComponent(
		ctx,
		c.client,
		clusterUID,
		data.ClusterName.ValueString(),
		api.CloudProviderGCP,
		data.Region.ValueString(),
		data.Kubeconfig.ValueString(),
		nil,
	); err != nil {
		return fmt.Errorf("failed to uninstall cloudpilot agent component: %w", err)
	}

	restoreEnv, err := restoreEnvFromClusterModel(ctx, *data)
	if err != nil {
		return fmt.Errorf("failed to parse gke restore configuration: %w", err)
	}
	switch {
	case boolValue(data.SkipRestore):
		tflog.Info(ctx, "skip_restore is true, skipping gke node pool restore step")
	case len(restoreEnv) == 0:
		tflog.Info(ctx, "restore_node_number is 0 and no per-pool restore sizes are configured, leaving cluster in its current optimized state")
	default:
		tflog.Info(ctx, "restoring regular gke node pools after CloudPilot uninstall")
		if err := restoreCloudpilotAIAfterUninstall(ctx, c.client, clusterUID, api.CloudProviderGCP, data.Kubeconfig.ValueString(), restoreEnv, nil); err != nil {
			return fmt.Errorf("failed to restore gke node pools after uninstall: %w", err)
		}
	}

	if err := c.client.DeleteCluster(clusterUID); err != nil {
		warnings = append(warnings, fmt.Sprintf("skipped deletion of remote cluster record: %s", err))
	}

	if len(warnings) > 0 {
		return warningOnlyError{
			summary: "Remote GKE cleanup skipped",
			detail:  strings.Join(warnings, "\n"),
		}
	}
	return nil
}

type warningOnlyError struct {
	summary string
	detail  string
}

func (w warningOnlyError) Error() string {
	return w.detail
}

func planMayUpgradeCluster(data ClusterModel, summary *api.ClusterCostsSummary) bool {
	return boolValue(data.EnableUpgrade) && summary != nil && summary.NeedUpgrade
}

func markClusterSummaryStatusUnknown(data *ClusterModel) {
	data.AgentVersion = types.StringUnknown()
	data.OnboardManifestVersion = types.StringUnknown()
	data.NeedUpgrade = types.BoolUnknown()
}

func (c *Cluster) refreshClusterSummaryStatus(data *ClusterModel, clusterID string) error {
	summary, err := c.client.GetCluster(clusterID)
	if err != nil {
		return err
	}
	applyClusterSummaryStatus(data, summary)
	return nil
}

func hydrateClusterPostWriteState(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, data *ClusterModel) error {
	var err error
	data.ClusterSetting, err = hydrateClusterSettingPostWrite(ctx, client, clusterUID, data.ClusterSetting)
	if err != nil {
		return err
	}
	data.NodeClasses, err = hydrateNodeClassesPostWrite(ctx, client, clusterUID, data.NodeClasses)
	if err != nil {
		return err
	}
	data.NodePools, err = hydrateNodePoolsPostWrite(ctx, client, clusterUID, data.NodePools)
	if err != nil {
		return err
	}

	if !data.EnableRebalance.IsNull() && !data.EnableRebalance.IsUnknown() {
		rebalanceConfig, err := client.GetRebalanceConfiguration(clusterUID)
		if err != nil && !errors.Is(err, cloudpilotaiclient.ErrNotFound) {
			return err
		}
		if rebalanceConfig != nil {
			data.EnableRebalance = types.BoolValue(rebalanceConfig.Enable)
		}
	}

	return nil
}

func hydrateClusterSettingPostWrite(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObject[ClusterSettingModel]) (customfield.NestedObject[ClusterSettingModel], error) {
	remote, err := client.GetClusterSetting(clusterUID)
	if err != nil {
		return current, fmt.Errorf("failed to get cluster setting: %w", err)
	}
	return clusterSettingObjectPreservingState(ctx, current, remote)
}

func clusterSettingObjectPreservingState(ctx context.Context, current customfield.NestedObject[ClusterSettingModel], remote *api.ClusterSetting) (customfield.NestedObject[ClusterSettingModel], error) {
	if current.IsNull() || current.IsUnknown() {
		return current, nil
	}

	value, diags := current.Value(ctx)
	if diags.HasError() {
		return current, fmt.Errorf("failed to parse cluster_setting: %v", diags)
	}

	hydrated := ClusterSettingModel{}
	if value != nil {
		hydrated = *value
		normalizeClusterSettingUnknowns(&hydrated)
	}
	mergeClusterSettingFromAPI(&hydrated, remote)

	object, diags := customfield.NewObject(ctx, &hydrated)
	if diags.HasError() {
		return current, fmt.Errorf("failed to build cluster_setting state: %v", diags)
	}
	return object, nil
}

func normalizeClusterSettingUnknowns(setting *ClusterSettingModel) {
	if setting.EnableNodeRepair.IsUnknown() {
		setting.EnableNodeRepair = types.BoolNull()
	}
	if setting.EnableDiskMonitor.IsUnknown() {
		setting.EnableDiskMonitor = types.BoolNull()
	}
	if setting.Discount.IsUnknown() {
		setting.Discount = types.Float64Null()
	}
	if setting.PreRunCommand.IsUnknown() {
		setting.PreRunCommand = types.StringNull()
	}
	if setting.PostRunCommand.IsUnknown() {
		setting.PostRunCommand = types.StringNull()
	}
}

func mergeClusterSettingFromAPI(setting *ClusterSettingModel, remote *api.ClusterSetting) {
	if remote == nil {
		return
	}
	if remote.EnableNodeRepair != nil && !setting.EnableNodeRepair.IsNull() {
		setting.EnableNodeRepair = types.BoolValue(*remote.EnableNodeRepair)
	}
	if remote.EnableDiskMonitor != nil && !setting.EnableDiskMonitor.IsNull() {
		setting.EnableDiskMonitor = types.BoolValue(*remote.EnableDiskMonitor)
	}
	if remote.Discount != nil && !setting.Discount.IsNull() {
		setting.Discount = types.Float64Value(*remote.Discount)
	}
	if remote.PreRunCommand != nil && !setting.PreRunCommand.IsNull() {
		setting.PreRunCommand = types.StringValue(*remote.PreRunCommand)
	}
	if remote.PostRunCommand != nil && !setting.PostRunCommand.IsNull() {
		setting.PostRunCommand = types.StringValue(*remote.PostRunCommand)
	}
}

func mergeClusterIdentityFromSummary(data *ClusterModel, summary *api.ClusterCostsSummary) {
	if summary == nil {
		return
	}
	if summary.ClusterName != "" {
		data.ClusterName = types.StringValue(summary.ClusterName)
	}
	if summary.Region != "" {
		data.Region = types.StringValue(summary.Region)
	}
}

func (c *Cluster) readClusterManagementState(ctx context.Context, data *ClusterModel, clusterID string, isImport bool) error {
	clusterSetting, err := c.client.GetClusterSetting(clusterID)
	if err != nil {
		return fmt.Errorf("failed to get cluster setting: %w", err)
	}
	if !data.ClusterSetting.IsNull() && !data.ClusterSetting.IsUnknown() {
		data.ClusterSetting, err = clusterSettingObjectPreservingState(ctx, data.ClusterSetting, clusterSetting)
		if err != nil {
			return err
		}
	} else if isImport {
		data.ClusterSetting = clusterSettingObjectFromAPI(ctx, clusterSetting)
	}

	rebalanceConfig, err := c.client.GetRebalanceConfiguration(clusterID)
	if err != nil && !errors.Is(err, cloudpilotaiclient.ErrNotFound) {
		return fmt.Errorf("failed to get rebalance configuration: %w", err)
	}
	if rebalanceConfig != nil && (isImport || !data.EnableRebalance.IsNull()) {
		data.EnableRebalance = types.BoolValue(rebalanceConfig.Enable)
	} else if isImport && rebalanceConfig == nil {
		data.EnableRebalance = types.BoolValue(false)
	}

	if !data.NodeClasses.IsNullOrUnknown() {
		data.NodeClasses, err = hydrateNodeClassesPostWrite(ctx, c.client, clusterID, data.NodeClasses)
		if err != nil {
			return err
		}
	} else if isImport {
		importedNodeClasses, err := hydrateAllNodeClasses(ctx, c.client, clusterID)
		if err != nil {
			return err
		}
		importedNodeClassesSlice, diags := importedNodeClasses.AsStructSliceT(ctx)
		if diags.HasError() {
			return fmt.Errorf("failed to parse imported nodeclasses: %v", diags)
		}
		if len(importedNodeClassesSlice) > 0 {
			data.NodeClasses = importedNodeClasses
		}
	}

	if !data.NodePools.IsNullOrUnknown() {
		data.NodePools, err = hydrateNodePoolsPostWrite(ctx, c.client, clusterID, data.NodePools)
		if err != nil {
			return err
		}
	} else if isImport {
		importedNodePools, err := hydrateAllNodePools(ctx, c.client, clusterID)
		if err != nil {
			return err
		}
		importedNodePoolsSlice, diags := importedNodePools.AsStructSliceT(ctx)
		if diags.HasError() {
			return fmt.Errorf("failed to parse imported nodepools: %v", diags)
		}
		if len(importedNodePoolsSlice) > 0 {
			data.NodePools = importedNodePools
		}
	}

	return nil
}

func hydrateNodeClassesPostWrite(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObjectList[api.GCENodeClassModel]) (customfield.NestedObjectList[api.GCENodeClassModel], error) {
	if current.IsNullOrUnknown() {
		return current, nil
	}

	stateNodeClasses, diags := current.AsStructSliceT(ctx)
	if diags.HasError() {
		return current, fmt.Errorf("failed to parse nodeclasses: %v", diags)
	}

	remoteList, err := client.ListNodeClasses(clusterUID)
	if err != nil {
		return current, fmt.Errorf("failed to list nodeclasses: %w", err)
	}

	remoteByName := make(map[string]api.GCENodeClassModel, len(remoteList.GCENodeClasses))
	for i := range remoteList.GCENodeClasses {
		model, err := remoteList.GCENodeClasses[i].ToGCENodeClassModel(ctx)
		if err != nil {
			return current, fmt.Errorf("failed to convert nodeclass %q: %w", remoteList.GCENodeClasses[i].Name, err)
		}
		if model != nil {
			remoteByName[remoteList.GCENodeClasses[i].Name] = *model
		}
	}

	hydrated := make([]api.GCENodeClassModel, 0, len(stateNodeClasses))
	for _, stateNodeClass := range stateNodeClasses {
		if remote, ok := remoteByName[stateNodeClass.Name.ValueString()]; ok {
			hydrated = append(hydrated, preserveNodeClassStateRepresentation(ctx, remote, stateNodeClass))
			continue
		}
		hydrated = append(hydrated, stateNodeClass)
	}

	list, diags := customfield.NewObjectList(ctx, hydrated)
	if diags.HasError() {
		return current, fmt.Errorf("failed to build nodeclasses state: %v", diags)
	}
	return list, nil
}

func hydrateNodePoolsPostWrite(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObjectList[api.GCENodePoolModel]) (customfield.NestedObjectList[api.GCENodePoolModel], error) {
	if current.IsNullOrUnknown() {
		return current, nil
	}

	stateNodePools, diags := current.AsStructSliceT(ctx)
	if diags.HasError() {
		return current, fmt.Errorf("failed to parse nodepools: %v", diags)
	}

	remoteList, err := client.ListNodePools(clusterUID)
	if err != nil {
		return current, fmt.Errorf("failed to list nodepools: %w", err)
	}

	remoteByName := make(map[string]api.GCENodePoolModel, len(remoteList.GCENodePools))
	for i := range remoteList.GCENodePools {
		model, err := remoteList.GCENodePools[i].ToGCENodePoolModel(ctx)
		if err != nil {
			return current, fmt.Errorf("failed to convert nodepool %q: %w", remoteList.GCENodePools[i].Name, err)
		}
		if model != nil {
			remoteByName[remoteList.GCENodePools[i].Name] = normalizeNodePoolComputedUnknowns(*model)
		}
	}

	hydrated := make([]api.GCENodePoolModel, 0, len(stateNodePools))
	for _, stateNodePool := range stateNodePools {
		if remote, ok := remoteByName[stateNodePool.Name.ValueString()]; ok {
			hydrated = append(hydrated, normalizeNodePoolComputedUnknowns(preserveNodePoolStateRepresentation(ctx, remote, stateNodePool)))
			continue
		}
		hydrated = append(hydrated, normalizeNodePoolComputedUnknowns(stateNodePool))
	}

	list, diags := customfield.NewObjectList(ctx, hydrated)
	if diags.HasError() {
		return current, fmt.Errorf("failed to build nodepools state: %v", diags)
	}
	return list, nil
}

func preserveNodeClassStateRepresentation(ctx context.Context, remote, state api.GCENodeClassModel) api.GCENodeClassModel {
	remote.EnableImageAccelerator = preserveManagedBool(state.EnableImageAccelerator, remote.EnableImageAccelerator)
	remote.ServiceAccount = preserveManagedString(state.ServiceAccount, remote.ServiceAccount)
	remote.Disks = preserveGCEDiskStateRepresentation(ctx, remote.Disks, state.Disks)
	remote.ImageSelectorTerms = preserveGCEImageSelectorTermStateRepresentation(ctx, remote.ImageSelectorTerms, state.ImageSelectorTerms)
	remote.SubnetRangeName = preserveManagedString(state.SubnetRangeName, remote.SubnetRangeName)
	remote.KubeletConfiguration = preserveGCEKubeletConfigurationStateRepresentation(ctx, remote.KubeletConfiguration, state.KubeletConfiguration)
	remote.Labels = preserveManagedMap(ctx, state.Labels, remote.Labels)
	remote.Metadata = preserveManagedMap(ctx, state.Metadata, remote.Metadata)
	remote.NetworkTags = preserveManagedStringSlice(state.NetworkTags, remote.NetworkTags)
	remote.ConfidentialInstanceType = preserveManagedString(state.ConfidentialInstanceType, remote.ConfidentialInstanceType)
	remote.NetworkConfig = preserveGCENetworkConfigStateRepresentation(ctx, remote.NetworkConfig, state.NetworkConfig)
	remote.AutoGPUTaint = preserveManagedBool(state.AutoGPUTaint, remote.AutoGPUTaint)
	remote.GPUDriverVersion = preserveManagedString(state.GPUDriverVersion, remote.GPUDriverVersion)
	remote.OriginNodeClassJSON = preserveStateStringWhenRemoteNull(state.OriginNodeClassJSON, remote.OriginNodeClassJSON)
	return remote
}

func preserveNodePoolStateRepresentation(ctx context.Context, remote, state api.GCENodePoolModel) api.GCENodePoolModel {
	remote.Enable = preserveManagedBool(state.Enable, remote.Enable)
	remote.EnableImageAccelerator = preserveManagedBool(state.EnableImageAccelerator, remote.EnableImageAccelerator)
	remote.NodeClass = preserveManagedString(state.NodeClass, remote.NodeClass)
	remote.EnableGPU = preserveManagedBool(state.EnableGPU, remote.EnableGPU)
	remote.ProvisionPriority = preserveManagedInt32(state.ProvisionPriority, remote.ProvisionPriority)
	remote.InstanceFamily = preserveManagedList(state.InstanceFamily, remote.InstanceFamily)
	remote.InstanceArch = preserveManagedList(state.InstanceArch, remote.InstanceArch)
	remote.CapacityType = preserveManagedList(state.CapacityType, remote.CapacityType)
	remote.Zone = preserveManagedList(state.Zone, remote.Zone)
	remote.InstanceCPUMAX = preserveManagedInt64(state.InstanceCPUMAX, remote.InstanceCPUMAX)
	remote.InstanceCPUMIN = preserveManagedInt64(state.InstanceCPUMIN, remote.InstanceCPUMIN)
	remote.InstanceMemoryMAX = preserveManagedInt64(state.InstanceMemoryMAX, remote.InstanceMemoryMAX)
	remote.InstanceMemoryMIN = preserveManagedInt64(state.InstanceMemoryMIN, remote.InstanceMemoryMIN)
	remote.Labels = preserveManagedMap(ctx, state.Labels, remote.Labels)
	remote.Taints = preserveTaintStateRepresentation(ctx, remote.Taints, state.Taints)
	remote.NodeDisruptionLimit = preserveManagedString(state.NodeDisruptionLimit, remote.NodeDisruptionLimit)
	remote.NodeDisruptionDelay = preserveManagedDuration(state.NodeDisruptionDelay, remote.NodeDisruptionDelay)
	remote.OriginNodePoolJSON = preserveStateStringWhenRemoteNull(state.OriginNodePoolJSON, remote.OriginNodePoolJSON)
	return remote
}

func normalizeNodePoolComputedUnknowns(model api.GCENodePoolModel) api.GCENodePoolModel {
	if model.NodeClass.IsUnknown() {
		model.NodeClass = types.StringNull()
	}
	if model.InstanceCPUMIN.IsUnknown() {
		model.InstanceCPUMIN = types.Int64Null()
	}
	if model.InstanceMemoryMIN.IsUnknown() {
		model.InstanceMemoryMIN = types.Int64Null()
	}
	return model
}

func extractResourceNames[T any](ctx context.Context, list customfield.NestedObjectList[T], getName func(T) string) map[string]struct{} {
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

func sortedValuesByName[T any](values map[string]T, getName func(T) string) []T {
	result := make([]T, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return getName(result[i]) < getName(result[j])
	})

	return result
}

func resolveDeleteClusterUID(clusterID, clusterName, region, clusterUID types.String) string {
	return resolveClusterUID(clusterID, clusterID, clusterName, region, clusterUID)
}

func applyClusterSummaryStatus(data *ClusterModel, summary *api.ClusterCostsSummary) {
	if summary == nil {
		return
	}

	data.AgentVersion = types.StringValue(summary.AgentVersion)
	data.OnboardManifestVersion = types.StringValue(summary.OnboardManifestVersion)
	data.NeedUpgrade = types.BoolValue(summary.NeedUpgrade)
}

func preserveSemanticDuration(stateVal, serverVal types.String) types.String {
	if stateVal.IsNull() || stateVal.IsUnknown() || serverVal.IsNull() || serverVal.IsUnknown() {
		return serverVal
	}
	stateD, err1 := time.ParseDuration(stateVal.ValueString())
	serverD, err2 := time.ParseDuration(serverVal.ValueString())
	if err1 == nil && err2 == nil && stateD == serverD {
		return stateVal
	}
	return serverVal
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

func preserveGCEDiskStateRepresentation(ctx context.Context, remote, state customfield.NestedObjectList[api.GCEDiskModel]) customfield.NestedObjectList[api.GCEDiskModel] {
	return preserveManagedObjectListByIndex(ctx, state, remote, func(remoteDisk, stateDisk api.GCEDiskModel) api.GCEDiskModel {
		remoteDisk.SizeGiB = preserveManagedInt64(stateDisk.SizeGiB, remoteDisk.SizeGiB)
		remoteDisk.Category = preserveManagedString(stateDisk.Category, remoteDisk.Category)
		remoteDisk.Boot = preserveManagedBool(stateDisk.Boot, remoteDisk.Boot)
		return remoteDisk
	})
}

func preserveGCEImageSelectorTermStateRepresentation(ctx context.Context, remote, state customfield.NestedObjectList[api.GCEImageSelectorTermModel]) customfield.NestedObjectList[api.GCEImageSelectorTermModel] {
	return preserveManagedObjectListByIndex(ctx, state, remote, func(remoteTerm, stateTerm api.GCEImageSelectorTermModel) api.GCEImageSelectorTermModel {
		remoteTerm.ID = preserveManagedString(stateTerm.ID, remoteTerm.ID)
		remoteTerm.Family = preserveManagedString(stateTerm.Family, remoteTerm.Family)
		remoteTerm.Channel = preserveManagedString(stateTerm.Channel, remoteTerm.Channel)
		remoteTerm.Version = preserveManagedString(stateTerm.Version, remoteTerm.Version)
		return remoteTerm
	})
}

func preserveGCEKubeletConfigurationStateRepresentation(ctx context.Context, remote, state customfield.NestedObject[api.GCEKubeletConfigurationModel]) customfield.NestedObject[api.GCEKubeletConfigurationModel] {
	if state.IsNull() {
		return customfield.NullObject[api.GCEKubeletConfigurationModel](ctx)
	}
	if remote.IsNull() || remote.IsUnknown() {
		return remote
	}

	stateValue, diags := state.Value(ctx)
	if diags.HasError() || stateValue == nil {
		return remote
	}
	remoteValue, diags := remote.Value(ctx)
	if diags.HasError() || remoteValue == nil {
		return remote
	}

	remoteValue.KubeReserved = preserveManagedMap(ctx, stateValue.KubeReserved, remoteValue.KubeReserved)
	remoteValue.SystemReserved = preserveManagedMap(ctx, stateValue.SystemReserved, remoteValue.SystemReserved)
	remoteValue.EvictionHard = preserveManagedMap(ctx, stateValue.EvictionHard, remoteValue.EvictionHard)
	remoteValue.EvictionSoft = preserveManagedMap(ctx, stateValue.EvictionSoft, remoteValue.EvictionSoft)

	object, diags := customfield.NewObject(ctx, remoteValue)
	if diags.HasError() {
		return remote
	}
	return object
}

func preserveGCENetworkConfigStateRepresentation(ctx context.Context, remote, state customfield.NestedObject[api.GCENetworkConfigModel]) customfield.NestedObject[api.GCENetworkConfigModel] {
	if state.IsNull() {
		return customfield.NullObject[api.GCENetworkConfigModel](ctx)
	}
	if remote.IsNull() || remote.IsUnknown() {
		return remote
	}

	stateValue, diags := state.Value(ctx)
	if diags.HasError() || stateValue == nil {
		return remote
	}
	remoteValue, diags := remote.Value(ctx)
	if diags.HasError() || remoteValue == nil {
		return remote
	}

	remoteValue.EnablePrivateNodes = preserveManagedBool(stateValue.EnablePrivateNodes, remoteValue.EnablePrivateNodes)
	remoteValue.Subnetwork = preserveManagedString(stateValue.Subnetwork, remoteValue.Subnetwork)
	remoteValue.AdditionalNetworkInterfaces = preserveGCEAdditionalNetworkInterfaceStateRepresentation(ctx, remoteValue.AdditionalNetworkInterfaces, stateValue.AdditionalNetworkInterfaces)

	object, diags := customfield.NewObject(ctx, remoteValue)
	if diags.HasError() {
		return remote
	}
	return object
}

func preserveGCEAdditionalNetworkInterfaceStateRepresentation(ctx context.Context, remote, state customfield.NestedObjectList[api.GCEAdditionalNetworkInterfaceModel]) customfield.NestedObjectList[api.GCEAdditionalNetworkInterfaceModel] {
	return preserveManagedObjectListByIndex(ctx, state, remote, func(remoteNIC, stateNIC api.GCEAdditionalNetworkInterfaceModel) api.GCEAdditionalNetworkInterfaceModel {
		remoteNIC.Network = preserveManagedString(stateNIC.Network, remoteNIC.Network)
		remoteNIC.Subnetwork = preserveManagedString(stateNIC.Subnetwork, remoteNIC.Subnetwork)
		return remoteNIC
	})
}

func preserveTaintStateRepresentation(ctx context.Context, remote, state customfield.NestedObjectList[api.TaintModel]) customfield.NestedObjectList[api.TaintModel] {
	return preserveManagedObjectListByIndex(ctx, state, remote, func(remoteTaint, stateTaint api.TaintModel) api.TaintModel {
		remoteTaint.Key = preserveManagedString(stateTaint.Key, remoteTaint.Key)
		remoteTaint.Value = preserveManagedString(stateTaint.Value, remoteTaint.Value)
		remoteTaint.Effect = preserveManagedString(stateTaint.Effect, remoteTaint.Effect)
		return remoteTaint
	})
}

func preserveStateStringWhenRemoteNull(stateVal, remoteVal types.String) types.String {
	if stateVal.IsNull() {
		return types.StringNull()
	}
	if remoteVal.IsNull() || remoteVal.IsUnknown() {
		return stateVal
	}
	return remoteVal
}

func preserveManagedDuration(stateVal, remoteVal types.String) types.String {
	if stateVal.IsNull() || stateVal.IsUnknown() {
		return types.StringNull()
	}
	return preserveSemanticDuration(stateVal, remoteVal)
}

func preserveManagedList(stateVal, remoteVal *[]types.String) *[]types.String {
	if stateVal == nil {
		return nil
	}
	if remoteVal == nil && len(*stateVal) == 0 {
		return stateVal
	}
	return remoteVal
}

func preserveManagedMap(ctx context.Context, stateVal, remoteVal customfield.Map[types.String]) customfield.Map[types.String] {
	if stateVal.IsNull() {
		return customfield.NullMap[types.String](ctx)
	}

	stateValues, diags := stateVal.Value(ctx)
	if diags.HasError() {
		return remoteVal
	}
	if remoteVal.IsNull() || remoteVal.IsUnknown() {
		if len(stateValues) == 0 {
			return stateVal
		}
		return remoteVal
	}

	remoteValues, diags := remoteVal.Value(ctx)
	if diags.HasError() {
		return remoteVal
	}

	filtered := make(map[string]types.String, len(stateValues))
	for key, stateValue := range stateValues {
		if remoteValue, ok := remoteValues[key]; ok {
			filtered[key] = remoteValue
			continue
		}
		filtered[key] = stateValue
	}

	return customfield.NewMapMust[types.String](ctx, filtered)
}

func preserveManagedObject[T any](ctx context.Context, stateVal, remoteVal customfield.NestedObject[T]) customfield.NestedObject[T] {
	if stateVal.IsNull() {
		return customfield.NullObject[T](ctx)
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

func preserveManagedObjectListByIndex[T any](ctx context.Context, stateVal, remoteVal customfield.NestedObjectList[T], preserve func(remote, state T) T) customfield.NestedObjectList[T] {
	base := preserveManagedObjectList(ctx, stateVal, remoteVal)
	if stateVal.IsNull() || base.IsNullOrUnknown() {
		return base
	}

	stateItems, diags := stateVal.AsStructSliceT(ctx)
	if diags.HasError() {
		return base
	}
	remoteItems, diags := base.AsStructSliceT(ctx)
	if diags.HasError() {
		return base
	}

	for i := range remoteItems {
		if i >= len(stateItems) {
			break
		}
		remoteItems[i] = preserve(remoteItems[i], stateItems[i])
	}

	list, diags := customfield.NewObjectList(ctx, remoteItems)
	if diags.HasError() {
		return base
	}
	return list
}

func preserveManagedStringSlice(stateVal, remoteVal []types.String) []types.String {
	if stateVal == nil {
		return nil
	}
	if remoteVal == nil && len(stateVal) == 0 {
		return stateVal
	}
	return remoteVal
}
