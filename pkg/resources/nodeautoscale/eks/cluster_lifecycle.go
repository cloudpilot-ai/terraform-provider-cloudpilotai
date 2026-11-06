package eks

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilitaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client/helper"
	awsauth "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/utils/aws"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func (c *Cluster) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := c.applyCluster(ctx, &data, ClusterModel{}); err != nil {
		resp.Diagnostics.AddError("failed to create CloudPilot cluster", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *Cluster) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state ClusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := c.applyCluster(ctx, &data, state); err != nil {
		resp.Diagnostics.AddError("failed to update CloudPilot cluster", err.Error())
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

	authCfg, err := executionAuthConfigFromModel(ctx, data)
	if err != nil {
		resp.Diagnostics.AddError("failed to decode aws auth configuration", err.Error())
		return
	}

	isImport := false
	if importFlag, diags := req.Private.GetKey(ctx, "is_import"); !diags.HasError() && string(importFlag) == "true" {
		isImport = true
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, "is_import", []byte("false"))...)
	}

	clusterUID := data.ClusterID.ValueString()
	if clusterUID == "" {
		// cluster_id is not yet known (normal CRUD path); need AWS credentials
		// to derive account_id and kubeconfig.
		if _, err := c.fillMissingParameters(ctx, &data, authCfg, true); err != nil {
			resp.Diagnostics.AddError(
				"failed to fill missing parameters",
				err.Error(),
			)
			return
		}
		clusterUID = api.GenerateClusterUID(api.CloudProviderAWS, data.ClusterName.ValueString(), data.Region.ValueString(), data.AccountID.ValueString())
		data.ClusterID = types.StringValue(clusterUID)
	}

	if _, err := awsauth.CommandEnv(ctx, authCfg); err != nil {
		resp.Diagnostics.AddError("failed to prepare aws auth environment", err.Error())
		return
	}

	summary, err := c.client.GetCluster(clusterUID)
	if err != nil {
		if errors.Is(err, cloudpilitaiclient.ErrNotFound) {
			resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
			return
		}
		resp.Diagnostics.AddError(
			"failed to get cluster",
			err.Error(),
		)
		return
	}
	applyClusterSummaryStatus(&data, summary)

	if !c.readClusterSetting(ctx, &data, clusterUID, isImport, resp) {
		return
	}
	if !c.readWorkloads(ctx, &data, clusterUID, isImport, resp) {
		return
	}
	if !c.readNodeClasses(ctx, &data, clusterUID, isImport, resp) {
		return
	}
	if !c.readNodePools(ctx, &data, clusterUID, isImport, resp) {
		return
	}
	if !data.ScheduledRebalances.IsNullOrUnknown() {
		data.ScheduledRebalances, err = helper.RefreshScheduledRebalances(ctx, c.client, clusterUID, data.ScheduledRebalances)
		if err != nil {
			resp.Diagnostics.AddError("failed to hydrate scheduled rebalances", err.Error())
			return
		}
	} else if isImport {
		data.ScheduledRebalances, err = helper.ImportScheduledRebalances(ctx, c.client, clusterUID)
		if err != nil {
			resp.Diagnostics.AddError("failed to import scheduled rebalances", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *Cluster) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	runtime, err := c.prepareExecution(ctx, &data, true)
	if err != nil {
		resp.Diagnostics.AddError("failed to prepare cluster access", err.Error())
		return
	}
	defer runtime.cleanup()
	shellKubeconfig, awsEnv := runtime.kubeconfig, runtime.awsEnv

	clusterUID := resolveDeleteClusterUID(data.ClusterID, data.ClusterName, data.Region, data.AccountID)

	rebalanceConfig := &api.RebalanceConfig{
		Enable: false,
	}

	if err := c.client.UpdateRebalanceConfiguration(clusterUID, rebalanceConfig); err != nil {
		resp.Diagnostics.AddError(
			"failed to update rebalance configuration",
			err.Error(),
		)
		return
	}

	nodesRestored := false
	if boolValueOrDefault(data.SkipRestore, false) {
		tflog.Info(ctx, "skip_restore is true, skipping node restore step")
	} else if int64ValueOrDefault(data.RestoreNodeNumber, 0) > 0 {
		opNodeNum, err := helper.GetCloudpilotAIOptimizedNodeNumber(ctx, shellKubeconfig, awsEnv)
		if err != nil {
			resp.Diagnostics.AddError(
				"failed to get optimized node number",
				err.Error(),
			)
			return
		}
		if opNodeNum > 0 {
			if err := helper.RestoreCloudpilotAIComponent(ctx, c.client,
				clusterUID, data.ClusterName.ValueString(), data.Region.ValueString(),
				shellKubeconfig, awsEnv, int64ValueOrDefault(data.RestoreNodeNumber, 0)); err != nil {
				resp.Diagnostics.AddError(
					"failed to restore CloudPilot AI component",
					err.Error(),
				)
				return
			}
			nodesRestored = true
		}
	} else {
		tflog.Info(ctx, "restore_node_number is 0, leaving cluster in current optimized state")
	}

	if nodesRestored {
		tflog.Info(ctx, "nodes were restored, running full uninstall flow")
		if err := helper.UninstallCloudpilotAIAgentComponent(ctx, c.client,
			clusterUID, data.ClusterName.ValueString(), api.CloudProviderAWS, data.Region.ValueString(),
			shellKubeconfig, awsEnv); err != nil {
			resp.Diagnostics.AddError(
				"failed to uninstall cloudpilot agent component",
				err.Error(),
			)
			return
		}
	} else {
		tflog.Info(ctx, "nodes were not restored, deleting cloudpilot namespace directly")
		if err := helper.DeleteCloudpilotNamespace(ctx, shellKubeconfig, awsEnv); err != nil {
			resp.Diagnostics.AddError(
				"failed to delete cloudpilot namespace",
				err.Error(),
			)
			return
		}
	}

	if err := c.client.DeleteCluster(clusterUID); err != nil {
		resp.Diagnostics.AddError(
			"failed to delete cluster",
			err.Error(),
		)
		return
	}

	tflog.Trace(ctx, "deleted cluster successfully")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

type clusterExecution struct {
	kubeconfig string
	awsEnv     map[string]string
	cleanup    func()
}

func (c *Cluster) prepareExecution(ctx context.Context, data *ClusterModel, pathFromState bool) (*clusterExecution, error) {
	authCfg, err := executionAuthConfigFromModel(ctx, *data)
	if err != nil {
		return nil, fmt.Errorf("decode AWS authentication: %w", err)
	}
	kubeconfigPath, err := c.fillMissingParameters(ctx, data, authCfg, pathFromState)
	if err != nil {
		return nil, fmt.Errorf("fill cluster access parameters: %w", err)
	}
	awsEnv, err := awsauth.CommandEnv(ctx, authCfg)
	if err != nil {
		return nil, fmt.Errorf("prepare AWS environment: %w", err)
	}
	kubeconfig, cleanup, err := awsauth.KubeconfigForCommandEnv(kubeconfigPath, awsEnv)
	if err != nil {
		return nil, fmt.Errorf("prepare shell kubeconfig: %w", err)
	}
	return &clusterExecution{kubeconfig: kubeconfig, awsEnv: awsEnv, cleanup: cleanup}, nil
}

func (c *Cluster) applyCluster(ctx context.Context, data *ClusterModel, state ClusterModel) error {
	if err := validateClusterSetting(ctx, data.ClusterSetting); err != nil {
		return err
	}
	runtime, err := c.prepareExecution(ctx, data, false)
	if err != nil {
		return err
	}
	defer runtime.cleanup()
	clusterUID := resolveClusterUID(data.ClusterID, state.ClusterID, data.ClusterName, data.Region, data.AccountID)
	data.ClusterID = types.StringValue(clusterUID)
	if err := c.ensureAgentInstalled(ctx, data, clusterUID, runtime); err != nil {
		return err
	}
	previousSchedules := extractResourceNames(ctx, state.ScheduledRebalances, func(m api.ScheduledRebalancePolicyModel) string { return m.Name.ValueString() })
	scheduledPlan, err := helper.PrepareScheduledRebalanceConfiguration(ctx, c.client, clusterUID, data.ScheduledRebalances, previousSchedules)
	if err != nil {
		return fmt.Errorf("invalid scheduled rebalance configuration: %w", err)
	}
	if err := c.ensureNodeAutoscalerInstalled(ctx, data, clusterUID, runtime); err != nil {
		return err
	}
	if err := validateNodeClassDiskFields(ctx, data); err != nil {
		return err
	}
	previousPools := extractResourceNames(ctx, state.NodePools, func(m api.EC2NodePoolModel) string { return m.Name.ValueString() })
	poolPlan, err := helper.PrepareEC2NodePoolConfiguration(ctx, c.client, clusterUID, data.NodePools, data.NodePoolTemplates, previousPools)
	if err != nil {
		return fmt.Errorf("invalid nodepool configuration: %w", err)
	}
	if err := c.syncClusterSetting(ctx, data, clusterUID); err != nil {
		return err
	}
	upgradeAction := func() error {
		if !boolValueOrDefault(data.EnableUpgrade, false) {
			return nil
		}
		_, err := helper.UpgradeCloudpilotAIComponentsIfNeeded(ctx, c.client, clusterUID, api.CloudProviderAWS, runtime.kubeconfig, data.CustomNodeRole.ValueString(), runtime.awsEnv)
		return err
	}
	if err := runUpgradeActionAndRefreshClusterSummary(data, c.client, clusterUID, upgradeAction); err != nil {
		return err
	}
	previousClasses := extractResourceNames(ctx, state.NodeClasses, func(m api.EC2NodeClassModel) string { return m.Name.ValueString() })
	if err := c.syncConfigurationWithScheduledPlan(ctx, data, clusterUID, previousClasses, scheduledPlan, poolPlan); err != nil {
		return err
	}
	return hydratePostWriteState(ctx, c.client, clusterUID, data)
}

func (c *Cluster) ensureAgentInstalled(ctx context.Context, data *ClusterModel, clusterUID string, runtime *clusterExecution) error {
	if _, err := c.client.GetCluster(clusterUID); err != nil {
		if !errors.Is(err, cloudpilitaiclient.ErrNotFound) {
			return fmt.Errorf("get cluster: %w", err)
		}
		if err := helper.InstallCloudpilotAIAgentComponent(ctx, c.client, api.CloudProviderAWS, data.ClusterName.ValueString(), runtime.kubeconfig, boolValueOrDefault(data.DisableWorkloadUploading, false), runtime.awsEnv); err != nil {
			return err
		}
	}
	if err := c.waitForClusterReady(ctx, clusterUID); err != nil {
		return err
	}
	return nil
}

func (c *Cluster) ensureNodeAutoscalerInstalled(ctx context.Context, data *ClusterModel, clusterUID string, runtime *clusterExecution) error {
	if boolValueOrDefault(data.OnlyInstallAgent, false) && !boolValueOrDefault(data.EnableRebalance, false) {
		return nil
	}
	config, err := c.client.GetRebalanceConfiguration(clusterUID)
	if err != nil {
		return fmt.Errorf("get rebalance configuration: %w", err)
	}
	if config != nil && !config.LastComponentsActiveTime.IsZero() {
		return nil
	}
	return helper.InstallCloudpilotAIRebalanceComponent(ctx, c.client, clusterUID, api.CloudProviderAWS, runtime.kubeconfig, data.CustomNodeRole.ValueString(), runtime.awsEnv)
}
