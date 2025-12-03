// Package eks provides resources for managing EKS clusters in the Cloudpilot AI Terraform provider.
package eks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilitaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client/helper"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/utils/aws"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &Cluster{}

type Cluster struct {
	client cloudpilitaiclient.Interface
}

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

func (c *Cluster) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := c.fillMissingParameters(&data); err != nil {
		resp.Diagnostics.AddError(
			"failed to fill missing parameters",
			err.Error(),
		)
		return
	}

	clusterUID := api.GenerateClusterUID(api.CloudProviderAWS, data.ClusterName.ValueString(), data.Region.ValueString(), data.AccountID.ValueString())
	data.ClusterID = types.StringValue(clusterUID)

	agentInstalled := false
	if _, err := c.client.GetCluster(clusterUID); err == nil {
		agentInstalled = true
	} else if !errors.Is(err, cloudpilitaiclient.ErrNotFound) {
		resp.Diagnostics.AddError(
			"failed to install CloudPilot AI agent component",
			err.Error(),
		)
		return
	}

	if !agentInstalled {
		// 1. install cloudpilot ai agent component
		tflog.Info(ctx, "installing CloudPilot AI agent component")
		if err := helper.InstallCloudpilotAIAgentComponent(ctx, c.client,
			data.Kubeconfig.ValueString(), data.DisableWorkloadUploading.ValueBool()); err != nil {
			resp.Diagnostics.AddError(
				"failed to install CloudPilot AI agent component",
				err.Error(),
			)
			return
		}
		tflog.Info(ctx, "installed CloudPilot AI agent component successfully")
	}

	tflog.Info(ctx, "waiting for cloudpilot ai agent component to be ready")
	if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, 5*time.Minute, true, func(ctx context.Context) (done bool, err error) {
		_, err = c.client.GetCluster(clusterUID)
		if err != nil {
			if errors.Is(err, cloudpilitaiclient.ErrNotFound) {
				tflog.Info(ctx, "waiting for cloudpilot ai agent component to be ready")
				return false, nil
			}

			return false, err
		}
		return true, nil
	}); err != nil {
		resp.Diagnostics.AddError(
			"failed to wait for cloudpilot ai agent component to be ready",
			err.Error(),
		)
		return
	}

	rebalanceConfig, err := c.client.GetRebalanceConfiguration(clusterUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to get rebalance configuration",
			err.Error(),
		)
		return
	}

	rebalanceComponentInstalled := !rebalanceConfig.LastComponentsActiveTime.IsZero()

	if !data.OnlyInstallAgent.ValueBool() ||
		data.EnableRebalance.ValueBool() ||
		data.EnableUpgradeRebalanceComponent.ValueBool() {
		if !rebalanceComponentInstalled {
			// 1.2. install cloudpilot ai rebalance component
			tflog.Info(ctx, "installing CloudPilot AI rebalance component")
			if err := helper.InstallCloudpilotAIRebalanceComponent(ctx, c.client,
				clusterUID, data.Kubeconfig.ValueString()); err != nil {
				resp.Diagnostics.AddError(
					"failed to install CloudPilot AI rebalance component",
					err.Error(),
				)
				return
			}
			tflog.Info(ctx, "installed CloudPilot AI rebalance component successfully")
		}
	}

	// 2. sync configurations
	tflog.Info(ctx, "syncing cluster configuration")
	if err := c.syncConfiguration(ctx, &data, clusterUID); err != nil {
		resp.Diagnostics.AddError(
			"failed to sync configuration",
			err.Error(),
		)
		return
	}

	tflog.Trace(ctx, "registered cluster successfully")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *Cluster) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ClusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := c.fillMissingParameters(&data); err != nil {
		resp.Diagnostics.AddError(
			"failed to fill missing parameters",
			err.Error(),
		)
		return
	}

	clusterUID := api.GenerateClusterUID(api.CloudProviderAWS, data.ClusterName.ValueString(), data.Region.ValueString(), data.AccountID.ValueString())
	data.ClusterID = types.StringValue(clusterUID)

	upgradeAgentComponent := data.EnableUpgradeAgent.ValueBool()
	agentExist := true
	if !upgradeAgentComponent {
		_, err := c.client.GetCluster(clusterUID)
		if err != nil {
			if !errors.Is(err, cloudpilitaiclient.ErrNotFound) {
				resp.Diagnostics.AddError(
					"failed to get cluster",
					err.Error(),
				)
				return
			}
			upgradeAgentComponent = true
			agentExist = false
		}
	}

	upgradedAgent := false
	// If the agent does not exist when upgrading, install the agent first,
	// otherwise you should upgrade the rebalance component first, then the agent component.
	if upgradeAgentComponent && !agentExist {
		// upgrade cloudpilot ai agent component
		tflog.Info(ctx, "upgrading CloudPilot AI agent component")
		if err := helper.InstallCloudpilotAIAgentComponent(ctx, c.client,
			data.Kubeconfig.ValueString(), data.DisableWorkloadUploading.ValueBool()); err != nil {
			resp.Diagnostics.AddError(
				"failed to upgrade CloudPilot AI agent component",
				err.Error(),
			)
			return
		}

		upgradedAgent = true
		tflog.Info(ctx, "upgraded CloudPilot AI agent component successfully")
	}

	upgradeRebalanceComponent := data.EnableUpgradeRebalanceComponent.ValueBool()
	if !upgradeRebalanceComponent && data.EnableRebalance.ValueBool() {
		rebalanceConfig, err := c.client.GetRebalanceConfiguration(clusterUID)
		if err != nil {
			resp.Diagnostics.AddError(
				"failed to get rebalance configuration",
				err.Error(),
			)
			return
		}

		if rebalanceConfig != nil && rebalanceConfig.LastComponentsActiveTime.IsZero() {
			upgradeRebalanceComponent = true
		}
	}

	if upgradeRebalanceComponent {
		// upgrade cloudpilot ai rebalance component
		tflog.Info(ctx, "upgrading CloudPilot AI rebalance component")
		if err := helper.InstallCloudpilotAIRebalanceComponent(ctx, c.client,
			clusterUID, data.Kubeconfig.ValueString()); err != nil {
			resp.Diagnostics.AddError(
				"failed to upgrade CloudPilot AI rebalance component",
				err.Error(),
			)
			return
		}
		tflog.Info(ctx, "upgraded CloudPilot AI rebalance component successfully")
	}

	if !upgradedAgent && agentExist {
		// upgrade cloudpilot ai agent component
		tflog.Info(ctx, "upgrading CloudPilot AI agent component")
		if err := helper.InstallCloudpilotAIAgentComponent(ctx, c.client,
			data.Kubeconfig.ValueString(), data.DisableWorkloadUploading.ValueBool()); err != nil {
			resp.Diagnostics.AddError(
				"failed to upgrade CloudPilot AI agent component",
				err.Error(),
			)
			return
		}
		tflog.Info(ctx, "upgraded CloudPilot AI agent component successfully")
	}

	tflog.Info(ctx, "syncing cluster configuration")
	if err := c.syncConfiguration(ctx, &data, clusterUID); err != nil {
		resp.Diagnostics.AddError(
			"failed to sync configuration",
			err.Error(),
		)
		return
	}

	tflog.Trace(ctx, "upgraded cluster successfully")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *Cluster) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := c.fillMissingParameters(&data); err != nil {
		resp.Diagnostics.AddError(
			"failed to fill missing parameters",
			err.Error(),
		)
		return
	}

	clusterUID := api.GenerateClusterUID(api.CloudProviderAWS, data.ClusterName.ValueString(), data.Region.ValueString(), data.AccountID.ValueString())
	data.ClusterID = types.StringValue(clusterUID)

	if _, err := c.client.GetCluster(clusterUID); err != nil {
		resp.Diagnostics.AddError(
			"failed to get cluster",
			err.Error(),
		)
		return
	}

	rebalanceConfiguration, err := c.client.GetRebalanceConfiguration(clusterUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to get rebalance configuration",
			err.Error(),
		)
		return
	}

	data.EnableRebalance = types.BoolValue(rebalanceConfiguration.Enable)
	data.EnableUploadConfig = types.BoolValue(rebalanceConfiguration.UploadConfig)
	data.EnableDiversityInstanceType = types.BoolValue(rebalanceConfiguration.EnableDiversityInstanceType)

	if !data.Workloads.IsNullOrUnknown() {
		// read workload configuration
		workloadConfiguration, err := c.client.GetWorkloadRebalanceConfiguration(clusterUID)
		if err != nil {
			resp.Diagnostics.AddError(
				"failed to get workload rebalance configuration",
				err.Error(),
			)
			return
		}
		if workloadConfiguration == nil {
			workloadConfiguration = &api.ClusterWorkloadSpec{}
		}

		workloadModels, diagnostics := data.Workloads.AsStructSliceT(ctx)
		if diagnostics.HasError() {
			resp.Diagnostics.Append(diagnostics...)
			return
		}
		workloadM := lo.SliceToMap(workloadConfiguration.Workloads, func(item api.Workload) (string, api.Workload) {
			return item.Namespace + item.Type + item.Name, item
		})

		for wi := range workloadModels {
			k := workloadConfiguration.Workloads[wi].Namespace + workloadConfiguration.Workloads[wi].Type + workloadConfiguration.Workloads[wi].Name
			if v, ok := workloadM[k]; ok {
				workloadModels[wi] = *v.ToWorkloadModel()
			}
		}
		data.Workloads = customfield.NewObjectListMust(ctx, workloadModels)
	}

	if !data.NodeClasses.IsNullOrUnknown() {
		// read nodeclass configuration
		nodeClassList, err := c.client.ListNodeClasses(clusterUID)
		if err != nil {
			resp.Diagnostics.AddError(
				"failed to list nodeclasses",
				err.Error(),
			)
			return
		}

		nodeClassModels := make([]api.EC2NodeClassModel, 0, len(nodeClassList.EC2NodeClasses))
		for ni := range nodeClassList.EC2NodeClasses {
			nodeClassModel, err := nodeClassList.EC2NodeClasses[ni].ToEC2NodeClassModel(ctx)
			if err != nil {
				resp.Diagnostics.AddError(
					"failed to convert nodeclass",
					err.Error(),
				)
				return
			}
			if nodeClassModel == nil {
				continue
			}

			nodeClassModels = append(nodeClassModels, *nodeClassModel)
		}

		nodeClasses, diag := customfield.NewObjectList(ctx, nodeClassModels)
		if diag.HasError() {
			resp.Diagnostics.Append(diag...)
			return
		}
		data.NodeClasses = nodeClasses
	}

	if !data.NodePools.IsNullOrUnknown() {
		// read nodepool configuration
		nodePoolList, err := c.client.ListNodePools(clusterUID)
		if err != nil {
			resp.Diagnostics.AddError(
				"failed to list nodepools",
				err.Error(),
			)
			return
		}

		nodePoolModels := make([]api.EC2NodePoolModel, 0, len(nodePoolList.EC2NodePools))
		for ni := range nodePoolList.EC2NodePools {
			nodePoolModel, err := nodePoolList.EC2NodePools[ni].ToEC2NodePoolModel()
			if err != nil {
				resp.Diagnostics.AddError(
					"failed to convert nodepool",
					err.Error(),
				)
				return
			}
			if nodePoolModel == nil {
				continue
			}

			nodePoolModels = append(nodePoolModels, *nodePoolModel)
		}

		nodePools, diag := customfield.NewObjectList(ctx, nodePoolModels)
		if diag.HasError() {
			resp.Diagnostics.Append(diag...)
			return
		}
		data.NodePools = nodePools
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *Cluster) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := c.fillMissingParameters(&data); err != nil {
		resp.Diagnostics.AddError(
			"failed to fill missing parameters",
			err.Error(),
		)
		return
	}

	clusterUID := api.GenerateClusterUID(api.CloudProviderAWS, data.ClusterName.ValueString(), data.Region.ValueString(), data.AccountID.ValueString())

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

	opNodeNum, err := helper.GetCloudpilotAIOptimizedNodeNumber(ctx, data.Kubeconfig.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to get optimized node number",
			err.Error(),
		)
		return
	}
	if opNodeNum > 0 {
		if data.RestoreNodeNumber.ValueInt64() == 0 {
			resp.Diagnostics.AddError(
				"restore_node_number is required",
				"Please set restore_node_number to a positive integer to restore the original node number before deleting the cluster.",
			)
			return
		}

		if err := helper.RestoreCloudpilotAIComponent(ctx, c.client,
			clusterUID, data.ClusterName.ValueString(), data.Region.ValueString(),
			data.Kubeconfig.ValueString(),
			data.RestoreNodeNumber.ValueInt64()); err != nil {
			resp.Diagnostics.AddError(
				"failed to restore CloudPilot AI component",
				err.Error(),
			)
			return
		}
	}

	if err := helper.UninstallCloudpilotAIAgentComponent(ctx, c.client,
		clusterUID, data.ClusterName.ValueString(), api.CloudProviderAWS, data.Region.ValueString(),
		data.Kubeconfig.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"failed to uninstall cloudpilot agent component",
			err.Error(),
		)
		return
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

func (c *Cluster) fillMissingParameters(data *ClusterModel) error {
	if data.AccountID.IsNull() || data.AccountID.IsUnknown() || data.AccountID.ValueString() == "" {
		accountID, err := aws.GetAccountID()
		if err != nil {
			return err
		}

		data.AccountID = types.StringValue(accountID)
	}

	kubeconfigPath := ""
	if !data.Kubeconfig.IsNull() && !data.Kubeconfig.IsUnknown() && data.Kubeconfig.ValueString() != "" {
		kubeconfigPath = data.Kubeconfig.ValueString()
		_, err := os.Stat(kubeconfigPath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}

		if os.IsNotExist(err) {
			kubeconfigPath = ""
		}
	}

	if kubeconfigPath == "" {
		kubeconfigPath = strings.Join([]string{data.Region.ValueString(), data.ClusterName.ValueString(), "kubeconfig"}, "_")
		if err := aws.UpdateKubeconfig(data.ClusterName.ValueString(), data.Region.ValueString(), kubeconfigPath); err != nil {
			return err
		}
	}

	fp, err := filepath.Abs(kubeconfigPath)
	if err != nil {
		return err
	}

	data.Kubeconfig = types.StringValue(fp)

	return nil
}

func (c *Cluster) syncConfiguration(ctx context.Context, data *ClusterModel, clusterUID string) error {
	// sync workload configurations
	tflog.Info(ctx, "syncing workload configuration")
	if err := helper.SyncWorkloadConfiguration(ctx, c.client, clusterUID, data.Workloads, data.WorkloadTemplates); err != nil {
		return fmt.Errorf("failed to sync workload configuration: %w", err)
	}
	tflog.Info(ctx, "synced workload configuration successfully")

	// sync nodepool configurations
	tflog.Info(ctx, "syncing nodepool configuration")
	if err := helper.SyncEC2NodePoolConfiguration(ctx, c.client, clusterUID, data.NodePools, data.NodePoolTemplates); err != nil {
		return fmt.Errorf("failed to sync nodepool configuration: %w", err)
	}
	tflog.Info(ctx, "synced nodepool configuration successfully")

	// sync nodeclass configurations
	tflog.Info(ctx, "syncing nodeclass configuration")
	if err := helper.SyncEC2NodeClassConfiguration(ctx, c.client, clusterUID, data.ClusterName.ValueString(), data.NodeClasses, data.NodeClassTemplates); err != nil {
		return fmt.Errorf("failed to sync nodeclass configuration: %w", err)
	}
	tflog.Info(ctx, "synced nodeclass configuration successfully")

	// sync rebalance configuration
	tflog.Info(ctx, "syncing rebalance configuration")
	if err := helper.SyncRebalanceConfiguration(ctx, c.client, clusterUID, data.EnableRebalance.ValueBool(), data.EnableUploadConfig.ValueBool(), data.EnableDiversityInstanceType.ValueBool()); err != nil {
		return fmt.Errorf("failed to sync rebalance configuration: %w", err)
	}
	tflog.Info(ctx, "synced rebalance configuration successfully")

	return nil
}
