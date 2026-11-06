package gke

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client/helper"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/gkeaccess"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	_ resource.Resource                = &Cluster{}
	_ resource.ResourceWithImportState = &Cluster{}
	_ resource.ResourceWithModifyPlan  = &Cluster{}
)

const clusterReadyPollTimeout = 5 * time.Minute

var uninstallCloudpilotAIAgentComponent = helper.UninstallCloudpilotAIAgentComponent
var restoreCloudpilotAIRebalanceComponent = helper.RestoreCloudpilotAIRebalanceComponentWithEnv
var installCloudpilotAIAgentComponent = helper.InstallCloudpilotAIAgentComponent
var uninstallWorkloadAutoscaler = helper.UninstallWorkloadAutoscaler
var deleteCloudpilotNamespace = helper.DeleteCloudpilotNamespace

type Cluster struct {
	client cloudpilotaiclient.Interface
}

type postWriteStateHydratorClient interface {
	GetWorkloadRebalanceConfiguration(clusterID string) (*api.ClusterWorkloadSpec, error)
	GetClusterSetting(clusterID string) (*api.ClusterSetting, error)
	GetRebalanceConfiguration(clusterID string) (*api.RebalanceConfig, error)
	ListNodeClasses(clusterID string) (api.RebalanceNodeClassList, error)
	ListNodePools(clusterID string) (api.RebalanceNodePoolList, error)
	ListScheduledRebalances(clusterID string) (api.ScheduledRebalancePolicyList, error)
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
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan ClusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateClusterSetting(ctx, plan.ClusterSetting); err != nil {
		resp.Diagnostics.AddError("invalid cluster_setting", err.Error())
		return
	}
	if req.State.Raw.IsNull() {
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

	kubeconfigPath, err := c.fillMissingParameters(ctx, &data, false)
	if err != nil {
		resp.Diagnostics.AddError("failed to fill missing parameters", err.Error())
		return
	}
	if err := validateClusterIdentity(&data); err != nil {
		resp.Diagnostics.AddError("missing required gke identity fields", err.Error())
		return
	}

	clusterID := resolveClusterUID(data.ClusterID, types.StringNull(), data.ClusterName, data.Region, data.ClusterUID)
	data.ClusterID = types.StringValue(clusterID)

	if err := c.ensureAgentInstalled(ctx, &data, clusterID, kubeconfigPath); err != nil {
		resp.Diagnostics.AddError("failed to install cloudpilot ai agent", err.Error())
		return
	}
	scheduledRebalancePlan, err := helper.PrepareScheduledRebalanceConfiguration(ctx, c.client, clusterID, data.ScheduledRebalances, nil)
	if err != nil {
		resp.Diagnostics.AddError("invalid scheduled rebalance configuration", err.Error())
		return
	}
	if err := c.ensureNodeAutoscalerInstalled(ctx, &data, clusterID, kubeconfigPath); err != nil {
		resp.Diagnostics.AddError("failed to install gke node autoscaler component", err.Error())
		return
	}
	nodePoolPlan, err := helper.PrepareGCENodePoolConfiguration(ctx, c.client, clusterID, data.NodePools, nil)
	if err != nil {
		resp.Diagnostics.AddError("invalid gke nodepool configuration", err.Error())
		return
	}
	nodeClassPlan, err := helper.PrepareGCENodeClassConfiguration(ctx, c.client, clusterID, data.NodeClasses)
	if err != nil {
		resp.Diagnostics.AddError("invalid gke nodeclass configuration", err.Error())
		return
	}

	if err := c.syncClusterSetting(ctx, &data, clusterID); err != nil {
		resp.Diagnostics.AddError("failed to sync cluster setting", err.Error())
		return
	}

	if err := c.upgradeComponentsIfNeeded(ctx, &data, clusterID, kubeconfigPath); err != nil {
		resp.Diagnostics.AddError("failed to upgrade cloudpilot ai components", err.Error())
		return
	}
	if err := c.syncConfigurationWithScheduledPlan(ctx, &data, clusterID, nil, scheduledRebalancePlan, nodeClassPlan, nodePoolPlan); err != nil {
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

	kubeconfigPath, err := c.fillMissingParameters(ctx, &data, false)
	if err != nil {
		resp.Diagnostics.AddError("failed to fill missing parameters", err.Error())
		return
	}
	if err := validateClusterIdentity(&data); err != nil {
		resp.Diagnostics.AddError("missing required gke identity fields", err.Error())
		return
	}

	clusterID := resolveClusterUID(data.ClusterID, state.ClusterID, data.ClusterName, data.Region, data.ClusterUID)
	data.ClusterID = types.StringValue(clusterID)
	previousNCNames := extractResourceNames(ctx, state.NodeClasses, func(m api.GCENodeClassModel) string {
		return m.Name.ValueString()
	})
	previousNPNames := extractResourceNames(ctx, state.NodePools, func(m api.GCENodePoolModel) string {
		return m.Name.ValueString()
	})
	previousScheduledRebalanceNames := extractResourceNames(ctx, state.ScheduledRebalances, func(m api.ScheduledRebalancePolicyModel) string {
		return m.Name.ValueString()
	})

	if err := c.ensureAgentInstalled(ctx, &data, clusterID, kubeconfigPath); err != nil {
		resp.Diagnostics.AddError("failed to install cloudpilot ai agent", err.Error())
		return
	}
	scheduledRebalancePlan, err := helper.PrepareScheduledRebalanceConfiguration(ctx, c.client, clusterID, data.ScheduledRebalances, previousScheduledRebalanceNames)
	if err != nil {
		resp.Diagnostics.AddError("invalid scheduled rebalance configuration", err.Error())
		return
	}
	if err := c.ensureNodeAutoscalerInstalled(ctx, &data, clusterID, kubeconfigPath); err != nil {
		resp.Diagnostics.AddError("failed to install gke node autoscaler component", err.Error())
		return
	}
	nodePoolPlan, err := helper.PrepareGCENodePoolConfiguration(ctx, c.client, clusterID, data.NodePools, previousNPNames)
	if err != nil {
		resp.Diagnostics.AddError("invalid gke nodepool configuration", err.Error())
		return
	}
	nodeClassPlan, err := helper.PrepareGCENodeClassConfiguration(ctx, c.client, clusterID, data.NodeClasses)
	if err != nil {
		resp.Diagnostics.AddError("invalid gke nodeclass configuration", err.Error())
		return
	}

	if err := c.syncClusterSetting(ctx, &data, clusterID); err != nil {
		resp.Diagnostics.AddError("failed to sync cluster setting", err.Error())
		return
	}

	if err := c.upgradeComponentsIfNeeded(ctx, &data, clusterID, kubeconfigPath); err != nil {
		resp.Diagnostics.AddError("failed to upgrade cloudpilot ai components", err.Error())
		return
	}
	if err := c.syncConfigurationWithScheduledPlan(ctx, &data, clusterID, previousNCNames, scheduledRebalancePlan, nodeClassPlan, nodePoolPlan); err != nil {
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

	clusterID := resolveDeleteClusterUID(data.ClusterID, data.ClusterName, data.Region, data.ClusterUID)
	if stringValue(data.ClusterID) == "" {
		data.ClusterID = types.StringValue(clusterID)
	}

	kubeconfigPath, err := c.fillMissingParameters(ctx, &data, true)
	if err != nil {
		resp.Diagnostics.AddError("failed to fill missing parameters", err.Error())
		return
	}

	if err := c.deleteCluster(ctx, &data, clusterID, kubeconfigPath); err != nil {
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

func (c *Cluster) ensureAgentInstalled(ctx context.Context, data *ClusterModel, clusterUID, kubeconfigPath string) error {
	agentInstalled := false
	if _, err := c.client.GetCluster(clusterUID); err == nil {
		agentInstalled = true
	} else if !errors.Is(err, cloudpilotaiclient.ErrNotFound) {
		return err
	}

	if !agentInstalled {
		if err := installCloudpilotAIAgentComponent(
			ctx,
			c.client,
			api.CloudProviderGCP,
			data.ClusterName.ValueString(),
			kubeconfigPath,
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

func (c *Cluster) upgradeComponentsIfNeeded(ctx context.Context, data *ClusterModel, clusterUID, kubeconfigPath string) error {
	if data.EnableUpgrade.IsNull() || data.EnableUpgrade.IsUnknown() || !data.EnableUpgrade.ValueBool() {
		return nil
	}

	_, err := helper.UpgradeCloudpilotAIComponentsIfNeeded(
		ctx,
		c.client,
		clusterUID,
		api.CloudProviderGCP,
		kubeconfigPath,
		"",
		nil,
	)
	return err
}

func (c *Cluster) ensureNodeAutoscalerInstalled(ctx context.Context, data *ClusterModel, clusterUID, kubeconfigPath string) error {
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

	if kubeconfigPath == "" {
		return fmt.Errorf("kubeconfig is required when the provider needs to install the gke node autoscaler component")
	}

	return installCloudpilotAIRebalanceComponent(
		ctx,
		c.client,
		clusterUID,
		api.CloudProviderGCP,
		kubeconfigPath,
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

func (c *Cluster) fillMissingParameters(ctx context.Context, data *ClusterModel, pathFromState bool) (string, error) {
	projectHints, err := gkeaccess.ProjectIDCandidatesFromNodeClassModels(ctx, data.NodeClasses)
	if err != nil {
		return "", err
	}

	info := gkeaccess.AccessInfo{
		ClusterID:       stringValue(data.ClusterID),
		ClusterName:     stringValue(data.ClusterName),
		Region:          stringValue(data.Region),
		ClusterLocation: stringValue(data.ClusterLocation),
		ProjectID:       stringValue(data.ProjectID),
		Kubeconfig:      stringValue(data.Kubeconfig),
	}
	ensureKubeconfig := gkeaccess.EnsureKubeconfigAvailable
	if pathFromState {
		ensureKubeconfig = gkeaccess.EnsureKubeconfigAvailableFromState
	}
	if err := ensureKubeconfig(ctx, c.client, &info, projectHints); err != nil {
		return "", err
	}

	if info.ClusterName != "" {
		data.ClusterName = types.StringValue(info.ClusterName)
	}
	if info.Region != "" {
		data.Region = types.StringValue(info.Region)
	}
	setOptionalComputedString(&data.ProjectID, info.ProjectID)
	if stringValue(data.ClusterUID) == "" && info.Kubeconfig != "" {
		clusterUID, err := gkeaccess.RunKubectlGetClusterUID(ctx, info.Kubeconfig)
		if err != nil {
			return "", err
		}
		if clusterUID != "" {
			data.ClusterUID = types.StringValue(clusterUID)
		}
	}
	return info.Kubeconfig, nil
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
	if err := gkeaccess.EnsureKubeconfigAvailableFromState(ctx, c.client, &info, projectHints); err != nil {
		return err
	}
	setOptionalComputedString(&data.ProjectID, info.ProjectID)
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
	if boolValueOrDefault(data.EnableRebalance, false) {
		return true
	}
	if boolValueOrDefault(data.OnlyInstallAgent, false) {
		return false
	}
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
	return boolValueOrDefault(data.OnlyInstallAgent, false) && !boolValueOrDefault(data.EnableRebalance, false)
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

	if err := setting.Validate(); err != nil {
		return err
	}
	payload := setting.ToAPI()
	if *payload == (api.ClusterSetting{}) {
		return nil
	}
	if err := c.client.UpdateClusterSetting(clusterUID, payload); err != nil {
		return fmt.Errorf("failed to update cluster setting: %w", err)
	}

	return nil
}

func (c *Cluster) syncConfiguration(
	ctx context.Context,
	data *ClusterModel,
	clusterID string,
	previousNCNames, previousNPNames map[string]struct{},
	previousScheduledRebalanceNames map[string]struct{},
) error {
	scheduledRebalancePlan, err := helper.PrepareScheduledRebalanceConfiguration(ctx, c.client, clusterID, data.ScheduledRebalances, previousScheduledRebalanceNames)
	if err != nil {
		return fmt.Errorf("failed to prepare scheduled rebalance configuration: %w", err)
	}
	nodePoolPlan, err := helper.PrepareGCENodePoolConfiguration(ctx, c.client, clusterID, data.NodePools, previousNPNames)
	if err != nil {
		return fmt.Errorf("failed to prepare nodepool configuration: %w", err)
	}
	nodeClassPlan, err := helper.PrepareGCENodeClassConfiguration(ctx, c.client, clusterID, data.NodeClasses)
	if err != nil {
		return fmt.Errorf("failed to prepare nodeclass configuration: %w", err)
	}
	return c.syncConfigurationWithScheduledPlan(ctx, data, clusterID, previousNCNames, scheduledRebalancePlan, nodeClassPlan, nodePoolPlan)
}

func (c *Cluster) syncConfigurationWithScheduledPlan(
	ctx context.Context,
	data *ClusterModel,
	clusterID string,
	previousNCNames map[string]struct{},
	scheduledRebalancePlan *helper.ScheduledRebalancePlan,
	nodeClassPlan *helper.GCENodeClassPlan,
	nodePoolPlan *helper.GCENodePoolPlan,
) error {
	if err := helper.SyncWorkloadConfiguration(ctx, c.client, clusterID, data.Workloads, customfield.NullObjectList[api.WorkloadTemplateModel](ctx)); err != nil {
		return fmt.Errorf("failed to sync workload configuration: %w", err)
	}
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
	nodeClassNames, err := helper.ApplyGCENodeClassConfiguration(c.client, nodeClassPlan)
	if err != nil {
		return fmt.Errorf("failed to sync nodeclass configuration: %w", err)
	}

	tflog.Info(ctx, "syncing gke nodepool configuration")
	if err := helper.ApplyGCENodePoolConfiguration(c.client, nodePoolPlan); err != nil {
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
	if err := helper.ApplyScheduledRebalanceConfiguration(c.client, scheduledRebalancePlan); err != nil {
		return fmt.Errorf("failed to sync scheduled rebalance configuration: %w", err)
	}

	return nil
}

func (c *Cluster) deleteCluster(ctx context.Context, data *ClusterModel, clusterUID, kubeconfigPath string) error {
	warnings := make([]string, 0, 5)
	if err := c.client.UpdateRebalanceConfiguration(clusterUID, &api.RebalanceConfig{Enable: false}); err != nil {
		warnings = append(warnings, fmt.Sprintf("skipped disabling remote rebalance configuration: %s", err))
	}
	if stringValue(data.ClusterID) == "" {
		data.ClusterID = types.StringValue(clusterUID)
	}

	restoreEnv, err := restoreEnvFromClusterModel(ctx, *data)
	if err != nil {
		return fmt.Errorf("failed to parse gke restore configuration: %w", err)
	}

	nodesRestored := false
	switch {
	case boolValue(data.SkipRestore):
		tflog.Info(ctx, "skip_restore is true, skipping gke node pool restore step")
	case len(restoreEnv) == 0:
		tflog.Info(ctx, "restore_node_number is 0 and no per-pool restore sizes are configured, leaving cluster in its current optimized state")
	default:
		tflog.Info(ctx, "restoring regular gke node pools before CloudPilot uninstall")
		if err := restoreCloudpilotAIRebalanceComponent(ctx, c.client, clusterUID, api.CloudProviderGCP, kubeconfigPath, restoreEnv, nil); err != nil {
			return fmt.Errorf("failed to restore gke node pools before uninstall: %w", err)
		}

		tflog.Info(ctx, "gke node pools were restored, running full uninstall flow")
		if err := uninstallCloudpilotAIAgentComponent(
			ctx,
			c.client,
			clusterUID,
			data.ClusterName.ValueString(),
			api.CloudProviderGCP,
			data.Region.ValueString(),
			kubeconfigPath,
			nil,
		); err != nil {
			return fmt.Errorf("failed to uninstall cloudpilot agent component: %w", err)
		}
		nodesRestored = true
	}

	if !nodesRestored {
		tflog.Info(ctx, "gke node pools were not restored, uninstalling Workload Autoscaler and deleting cloudpilot namespace")
		if err := uninstallWorkloadAutoscaler(ctx, c.client, clusterUID, kubeconfigPath); err != nil {
			return fmt.Errorf("failed to uninstall workload autoscaler: %w", err)
		}
		if err := deleteCloudpilotNamespace(ctx, kubeconfigPath, nil); err != nil {
			return fmt.Errorf("failed to delete cloudpilot namespace: %w", err)
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
