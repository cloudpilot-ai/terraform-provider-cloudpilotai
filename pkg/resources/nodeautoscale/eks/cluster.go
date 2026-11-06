package eks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilitaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client/helper"
	awsauth "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/utils/aws"
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

type Cluster struct {
	client cloudpilitaiclient.Interface
}

type postWriteStateHydratorClient interface {
	GetClusterSetting(clusterID string) (*api.ClusterSetting, error)
	ListNodeClasses(clusterID string) (api.RebalanceNodeClassList, error)
	ListNodePools(clusterID string) (api.RebalanceNodePoolList, error)
	ListScheduledRebalances(clusterID string) (api.ScheduledRebalancePolicyList, error)
}

type clusterSummaryReader interface {
	GetCluster(clusterID string) (*api.ClusterCostsSummary, error)
}

func applyClusterSummaryStatus(data *ClusterModel, summary *api.ClusterCostsSummary) {
	if summary == nil {
		return
	}

	data.AgentVersion = types.StringValue(summary.AgentVersion)
	data.OnboardManifestVersion = types.StringValue(summary.OnboardManifestVersion)
	data.NeedUpgrade = types.BoolValue(summary.NeedUpgrade)
}

func markClusterSummaryStatusUnknown(data *ClusterModel) {
	data.AgentVersion = types.StringUnknown()
	data.OnboardManifestVersion = types.StringUnknown()
	data.NeedUpgrade = types.BoolUnknown()
}

func planMayUpgradeCluster(data ClusterModel, summary *api.ClusterCostsSummary) bool {
	return boolValueOrDefault(data.EnableUpgrade, false) && summary != nil && summary.NeedUpgrade
}

func refreshClusterSummaryStatus(data *ClusterModel, reader clusterSummaryReader, clusterID string) error {
	summary, err := reader.GetCluster(clusterID)
	if err != nil {
		return err
	}

	applyClusterSummaryStatus(data, summary)
	return nil
}

func runUpgradeActionAndRefreshClusterSummary(
	data *ClusterModel,
	reader clusterSummaryReader,
	clusterID string,
	upgradeAction func() error,
) error {
	if upgradeAction != nil {
		if err := upgradeAction(); err != nil {
			return fmt.Errorf("upgrade action failed: %w", err)
		}
	}

	if err := refreshClusterSummaryStatus(data, reader, clusterID); err != nil {
		return fmt.Errorf("refresh final cluster summary failed: %w", err)
	}

	return nil
}

const clusterReadyPollTimeout = 5 * time.Minute

func NewCluster() resource.Resource {
	return &Cluster{}
}

func (c *Cluster) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_eks_cluster"
}

func (c *Cluster) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = Schema(ctx)
}

func (c *Cluster) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(cloudpilitaiclient.Interface)
	if !ok {
		resp.Diagnostics.AddError(
			"unexpected resource configure type",
			fmt.Sprintf("Expected cloudpilotai cloudpilitaiclient.Interface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
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
	if err := validateMemoryExtensionReservations(ctx, plan); err != nil {
		resp.Diagnostics.AddError("invalid Memory Extension reservation configuration", err.Error())
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

	clusterUID := resolveClusterUID(plan.ClusterID, state.ClusterID, plan.ClusterName, plan.Region, state.AccountID)
	if clusterUID == "" {
		return
	}

	summary, err := c.client.GetCluster(clusterUID)
	if err != nil {
		if errors.Is(err, cloudpilitaiclient.ErrNotFound) {
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

	// Mark this as an import so that Read fetches all remote resources
	// (nodeclasses, nodepools, workloads) instead of only the ones already
	// tracked in state. This enables terraform plan -generate-config-out=
	// to produce a complete configuration file.
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "is_import", []byte("true"))...)
}

func (c *Cluster) waitForClusterReady(ctx context.Context, clusterUID string) error {
	tflog.Info(ctx, "waiting for cloudpilot ai agent component to be ready")
	return wait.PollUntilContextTimeout(ctx, 10*time.Second, clusterReadyPollTimeout, true, func(ctx context.Context) (done bool, err error) {
		_, err = c.client.GetCluster(clusterUID)
		if err != nil {
			if errors.Is(err, cloudpilitaiclient.ErrNotFound) {
				tflog.Info(ctx, "waiting for cloudpilot ai agent component to be ready")
				return false, nil
			}

			return false, err
		}
		return true, nil
	})
}

func executionAuthConfigFromModel(ctx context.Context, data ClusterModel) (awsauth.ExecutionAuthConfig, error) {
	cfg := awsauth.ExecutionAuthConfig{}

	if !data.AWSProfile.IsNull() && !data.AWSProfile.IsUnknown() {
		cfg.Profile = data.AWSProfile.ValueString()
	}

	if !data.AWSAssumeRole.IsNull() && !data.AWSAssumeRole.IsUnknown() {
		value, diags := data.AWSAssumeRole.Value(ctx)
		if diags.HasError() {
			return cfg, fmt.Errorf("failed to decode aws_assume_role: %v", diags)
		}
		if value != nil {
			if !value.RoleARN.IsNull() && !value.RoleARN.IsUnknown() {
				cfg.AssumeRoleARN = value.RoleARN.ValueString()
			}
			if !value.SessionName.IsNull() && !value.SessionName.IsUnknown() {
				cfg.AssumeRoleSessionName = value.SessionName.ValueString()
			}
		}
	}

	return cfg, nil
}

func (c *Cluster) fillMissingParameters(ctx context.Context, data *ClusterModel, authCfg awsauth.ExecutionAuthConfig, pathFromState bool) (string, error) {
	if data.AccountID.IsNull() || data.AccountID.IsUnknown() || data.AccountID.ValueString() == "" {
		accountID, err := awsauth.GetAccountID(ctx, authCfg)
		if err != nil {
			return "", err
		}

		data.AccountID = types.StringValue(accountID)
	}

	configuredPath := ""
	if !data.Kubeconfig.IsNull() && !data.Kubeconfig.IsUnknown() {
		configuredPath = data.Kubeconfig.ValueString()
	}

	ensureKubeconfig := awsauth.EnsureKubeconfigAvailable
	if pathFromState {
		ensureKubeconfig = awsauth.EnsureKubeconfigAvailableFromState
	}
	return ensureKubeconfig(
		ctx,
		data.ClusterName.ValueString(),
		data.Region.ValueString(),
		configuredPath,
		authCfg,
	)
}

func (c *Cluster) syncClusterSetting(ctx context.Context, data *ClusterModel, clusterUID string) error {
	// sync cluster setting only when it is configured on the Terraform resource.
	if data.ClusterSetting.IsNull() || data.ClusterSetting.IsUnknown() {
		return nil
	}

	tflog.Info(ctx, "syncing cluster setting")
	setting, diags := data.ClusterSetting.Value(ctx)
	if diags.HasError() {
		return fmt.Errorf("failed to parse cluster_setting: %v", diags)
	}
	if setting != nil {
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
	}
	tflog.Info(ctx, "synced cluster setting successfully")
	return nil
}

func (c *Cluster) syncConfiguration(ctx context.Context, data *ClusterModel, clusterUID string,
	previousNCNames, previousNPNames, previousScheduledRebalanceNames map[string]struct{},
) error {
	scheduledRebalancePlan, err := helper.PrepareScheduledRebalanceConfiguration(ctx, c.client, clusterUID, data.ScheduledRebalances, previousScheduledRebalanceNames)
	if err != nil {
		return fmt.Errorf("failed to prepare scheduled rebalance configuration: %w", err)
	}
	if err := validateNodeClassDiskFields(ctx, data); err != nil {
		return fmt.Errorf("failed to validate nodeclass disk configuration: %w", err)
	}
	nodePoolPlan, err := helper.PrepareEC2NodePoolConfiguration(ctx, c.client, clusterUID, data.NodePools, data.NodePoolTemplates, previousNPNames)
	if err != nil {
		return fmt.Errorf("failed to prepare nodepool configuration: %w", err)
	}
	return c.syncConfigurationWithScheduledPlan(ctx, data, clusterUID, previousNCNames, scheduledRebalancePlan, nodePoolPlan)
}

func (c *Cluster) syncConfigurationWithScheduledPlan(ctx context.Context, data *ClusterModel, clusterUID string,
	previousNCNames map[string]struct{}, scheduledRebalancePlan *helper.ScheduledRebalancePlan, nodePoolPlan *helper.EC2NodePoolPlan,
) error {
	// sync workload configurations
	tflog.Info(ctx, "syncing workload configuration")
	if err := helper.SyncWorkloadConfiguration(ctx, c.client, clusterUID, data.Workloads, data.WorkloadTemplates); err != nil {
		return fmt.Errorf("failed to sync workload configuration: %w", err)
	}
	tflog.Info(ctx, "synced workload configuration successfully")

	// sync nodeclass configurations first, since nodepools may depend on
	// nodeclass settings (e.g. enable_image_accelerator).
	tflog.Info(ctx, "syncing nodeclass configuration")
	if err := helper.SyncEC2NodeClassConfiguration(ctx, c.client, clusterUID, data.ClusterName.ValueString(), data.NodeClasses, data.NodeClassTemplates, nil); err != nil {
		return fmt.Errorf("failed to sync nodeclass configuration: %w", err)
	}
	tflog.Info(ctx, "synced nodeclass configuration successfully")

	// sync nodepool configurations
	tflog.Info(ctx, "syncing nodepool configuration")
	if err := helper.ApplyEC2NodePoolConfiguration(c.client, nodePoolPlan); err != nil {
		return fmt.Errorf("failed to sync nodepool configuration: %w", err)
	}
	if err := helper.DeleteStaleEC2NodeClasses(ctx, c.client, clusterUID, data.NodeClasses, previousNCNames); err != nil {
		return fmt.Errorf("failed to delete stale nodeclasses: %w", err)
	}

	tflog.Info(ctx, "synced nodepool configuration successfully")

	// sync rebalance configuration
	tflog.Info(ctx, "syncing rebalance configuration")
	if isManagedBool(data.EnableRebalance) {
		if err := helper.SyncRebalanceConfiguration(ctx, c.client, clusterUID, data.EnableRebalance.ValueBool()); err != nil {
			return fmt.Errorf("failed to sync rebalance configuration: %w", err)
		}
		tflog.Info(ctx, "synced rebalance configuration successfully")
	} else {
		tflog.Info(ctx, "enable_rebalance is unmanaged, skipping rebalance configuration sync")
	}

	if err := c.syncClusterSetting(ctx, data, clusterUID); err != nil {
		return err
	}

	if err := helper.ApplyScheduledRebalanceConfiguration(c.client, scheduledRebalancePlan); err != nil {
		return fmt.Errorf("failed to sync scheduled rebalance configuration: %w", err)
	}

	return nil
}
