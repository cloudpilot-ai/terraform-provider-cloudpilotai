// Package eks provides resources for managing EKS clusters in the Cloudpilot AI Terraform provider.
package eks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilitaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client/helper"
	awsauth "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/utils/aws"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

// Ensure provider defined types fully satisfy framework interfaces.
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

func (c *Cluster) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	authCfg, err := executionAuthConfigFromModel(ctx, data)
	if err != nil {
		resp.Diagnostics.AddError("failed to decode aws auth configuration", err.Error())
		return
	}

	if err := c.fillMissingParameters(ctx, &data, authCfg); err != nil {
		resp.Diagnostics.AddError(
			"failed to fill missing parameters",
			err.Error(),
		)
		return
	}

	awsEnv, err := awsauth.CommandEnv(ctx, authCfg)
	if err != nil {
		resp.Diagnostics.AddError("failed to prepare aws auth environment", err.Error())
		return
	}

	shellKubeconfig, cleanupShellKubeconfig, err := awsauth.KubeconfigForCommandEnv(data.Kubeconfig.ValueString(), awsEnv)
	if err != nil {
		resp.Diagnostics.AddError("failed to prepare shell kubeconfig", err.Error())
		return
	}
	defer cleanupShellKubeconfig()

	clusterUID := resolveClusterUID(data.ClusterID, data.ClusterID, data.ClusterName, data.Region, data.AccountID)
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
			shellKubeconfig, boolValueOrDefault(data.DisableWorkloadUploading, false), awsEnv); err != nil {
			resp.Diagnostics.AddError(
				"failed to install CloudPilot AI agent component",
				err.Error(),
			)
			return
		}
		tflog.Info(ctx, "installed CloudPilot AI agent component successfully")
	}

	if err := c.waitForClusterReady(ctx, clusterUID); err != nil {
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

	if !boolValueOrDefault(data.OnlyInstallAgent, false) || boolValueOrDefault(data.EnableRebalance, false) {
		if !rebalanceComponentInstalled {
			// 1.2. install cloudpilot ai rebalance component
			tflog.Info(ctx, "installing CloudPilot AI rebalance component")
			if err := helper.InstallCloudpilotAIRebalanceComponent(ctx, c.client,
				clusterUID, shellKubeconfig, data.CustomNodeRole.ValueString(), awsEnv); err != nil {
				resp.Diagnostics.AddError(
					"failed to install CloudPilot AI rebalance component",
					err.Error(),
				)
				return
			}
			tflog.Info(ctx, "installed CloudPilot AI rebalance component successfully")
		}
	}

	if err := c.syncClusterSetting(ctx, &data, clusterUID); err != nil {
		resp.Diagnostics.AddError(
			"failed to sync cluster setting",
			err.Error(),
		)
		return
	}

	upgradeAction := func() error { return nil }
	if boolValueOrDefault(data.EnableUpgrade, false) {
		upgradeAction = func() error {
			upgraded, err := helper.UpgradeCloudpilotAIComponentsIfNeeded(ctx, c.client,
				clusterUID, shellKubeconfig, data.CustomNodeRole.ValueString(), awsEnv)
			if err != nil {
				return err
			}
			if upgraded {
				tflog.Info(ctx, "upgraded CloudPilot AI components successfully")
			} else {
				tflog.Info(ctx, "CloudPilot AI components already up to date, skipping upgrade")
			}
			return nil
		}
	}

	if err := runUpgradeActionAndRefreshClusterSummary(&data, c.client, clusterUID, upgradeAction); err != nil {
		resp.Diagnostics.AddError(
			"failed to read final cluster status after apply",
			err.Error(),
		)
		return
	}

	// 2. sync configurations (no previous state on Create, so pass nil — nothing to delete)
	tflog.Info(ctx, "syncing cluster configuration")
	if err := validateNodeClassDiskFields(ctx, &data); err != nil {
		resp.Diagnostics.AddError("invalid nodeclass disk configuration", err.Error())
		return
	}
	if err := c.syncConfiguration(ctx, &data, clusterUID, nil, nil); err != nil {
		resp.Diagnostics.AddError(
			"failed to sync configuration",
			err.Error(),
		)
		return
	}
	if err := hydratePostWriteState(ctx, c.client, clusterUID, &data); err != nil {
		resp.Diagnostics.AddError(
			"failed to hydrate cluster state after sync",
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

	// Read previous state to know which nodeclasses/nodepools were previously tracked
	var state ClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	previousNCNames := extractResourceNames(ctx, state.NodeClasses, func(m api.EC2NodeClassModel) string {
		return m.Name.ValueString()
	})
	previousNPNames := extractResourceNames(ctx, state.NodePools, func(m api.EC2NodePoolModel) string {
		return m.Name.ValueString()
	})

	authCfg, err := executionAuthConfigFromModel(ctx, data)
	if err != nil {
		resp.Diagnostics.AddError("failed to decode aws auth configuration", err.Error())
		return
	}

	if err := c.fillMissingParameters(ctx, &data, authCfg); err != nil {
		resp.Diagnostics.AddError(
			"failed to fill missing parameters",
			err.Error(),
		)
		return
	}

	awsEnv, err := awsauth.CommandEnv(ctx, authCfg)
	if err != nil {
		resp.Diagnostics.AddError("failed to prepare aws auth environment", err.Error())
		return
	}

	shellKubeconfig, cleanupShellKubeconfig, err := awsauth.KubeconfigForCommandEnv(data.Kubeconfig.ValueString(), awsEnv)
	if err != nil {
		resp.Diagnostics.AddError("failed to prepare shell kubeconfig", err.Error())
		return
	}
	defer cleanupShellKubeconfig()

	clusterUID := resolveClusterUID(data.ClusterID, state.ClusterID, data.ClusterName, data.Region, data.AccountID)
	data.ClusterID = types.StringValue(clusterUID)

	agentExist := true
	if _, err := c.client.GetCluster(clusterUID); err != nil {
		if !errors.Is(err, cloudpilitaiclient.ErrNotFound) {
			resp.Diagnostics.AddError(
				"failed to get cluster",
				err.Error(),
			)
			return
		}
		agentExist = false
	}

	if !agentExist {
		tflog.Info(ctx, "installing CloudPilot AI agent component")
		if err := helper.InstallCloudpilotAIAgentComponent(ctx, c.client,
			shellKubeconfig, boolValueOrDefault(data.DisableWorkloadUploading, false), awsEnv); err != nil {
			resp.Diagnostics.AddError(
				"failed to install CloudPilot AI agent component",
				err.Error(),
			)
			return
		}
		tflog.Info(ctx, "installed CloudPilot AI agent component successfully")
		if err := c.waitForClusterReady(ctx, clusterUID); err != nil {
			resp.Diagnostics.AddError(
				"failed to wait for cloudpilot ai agent component to be ready",
				err.Error(),
			)
			return
		}
	}

	if !boolValueOrDefault(data.OnlyInstallAgent, false) || boolValueOrDefault(data.EnableRebalance, false) {
		rebalanceConfig, err := c.client.GetRebalanceConfiguration(clusterUID)
		if err != nil {
			resp.Diagnostics.AddError(
				"failed to get rebalance configuration",
				err.Error(),
			)
			return
		}

		if rebalanceConfig != nil && rebalanceConfig.LastComponentsActiveTime.IsZero() {
			tflog.Info(ctx, "installing CloudPilot AI rebalance component")
			if err := helper.InstallCloudpilotAIRebalanceComponent(ctx, c.client,
				clusterUID, shellKubeconfig, data.CustomNodeRole.ValueString(), awsEnv); err != nil {
				resp.Diagnostics.AddError(
					"failed to install CloudPilot AI rebalance component",
					err.Error(),
				)
				return
			}
			tflog.Info(ctx, "installed CloudPilot AI rebalance component successfully")
		}
	}

	if err := c.syncClusterSetting(ctx, &data, clusterUID); err != nil {
		resp.Diagnostics.AddError(
			"failed to sync cluster setting",
			err.Error(),
		)
		return
	}

	upgradeAction := func() error { return nil }
	if boolValueOrDefault(data.EnableUpgrade, false) {
		upgradeAction = func() error {
			upgraded, err := helper.UpgradeCloudpilotAIComponentsIfNeeded(ctx, c.client,
				clusterUID, shellKubeconfig, data.CustomNodeRole.ValueString(), awsEnv)
			if err != nil {
				return err
			}
			if upgraded {
				tflog.Info(ctx, "upgraded CloudPilot AI components successfully")
			} else {
				tflog.Info(ctx, "CloudPilot AI components already up to date, skipping upgrade")
			}
			return nil
		}
	}

	if err := runUpgradeActionAndRefreshClusterSummary(&data, c.client, clusterUID, upgradeAction); err != nil {
		resp.Diagnostics.AddError(
			"failed to read final cluster status after apply",
			err.Error(),
		)
		return
	}

	tflog.Info(ctx, "syncing cluster configuration")
	if err := validateNodeClassDiskFields(ctx, &data); err != nil {
		resp.Diagnostics.AddError("invalid nodeclass disk configuration", err.Error())
		return
	}
	if err := c.syncConfiguration(ctx, &data, clusterUID, previousNCNames, previousNPNames); err != nil {
		resp.Diagnostics.AddError(
			"failed to sync configuration",
			err.Error(),
		)
		return
	}
	if err := hydratePostWriteState(ctx, c.client, clusterUID, &data); err != nil {
		resp.Diagnostics.AddError(
			"failed to hydrate cluster state after sync",
			err.Error(),
		)
		return
	}

	tflog.Trace(ctx, "upgraded cluster successfully")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
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
		if err := c.fillMissingParameters(ctx, &data, authCfg); err != nil {
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

	clusterSetting, err := c.client.GetClusterSetting(clusterUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to get cluster setting",
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

	if isImport || !data.EnableRebalance.IsNull() {
		data.EnableRebalance = types.BoolValue(rebalanceConfiguration.Enable)
	}

	if !data.ClusterSetting.IsNull() && !data.ClusterSetting.IsUnknown() {
		data.ClusterSetting = clusterSettingObjectFromAPI(ctx, clusterSetting)
	} else if isImport {
		data.ClusterSetting = clusterSettingObjectFromAPI(ctx, clusterSetting)
	}

	// Always fetch workload configuration from server (supports both normal read and import)
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

	workloadM := lo.SliceToMap(workloadConfiguration.Workloads, func(item api.Workload) (string, api.Workload) {
		return item.Namespace + "/" + item.Type + "/" + item.Name, item
	})

	if !data.Workloads.IsNullOrUnknown() {
		workloadModels, diagnostics := data.Workloads.AsStructSliceT(ctx)
		if diagnostics.HasError() {
			resp.Diagnostics.Append(diagnostics...)
			return
		}

		for wi := range workloadModels {
			k := workloadModels[wi].Namespace.ValueString() + "/" + workloadModels[wi].Type.ValueString() + "/" + workloadModels[wi].Name.ValueString()
			if v, ok := workloadM[k]; ok {
				stateModel := workloadModels[wi]
				workloadModels[wi] = preserveWorkloadStateRepresentation(*v.ToWorkloadModel(), stateModel)
			}
		}
		data.Workloads = customfield.NewObjectListMust(ctx, workloadModels)
	} else if isImport && len(workloadConfiguration.Workloads) > 0 {
		allWorkloads := make([]api.WorkloadModel, 0, len(workloadConfiguration.Workloads))
		for i := range workloadConfiguration.Workloads {
			if m := workloadConfiguration.Workloads[i].ToWorkloadModel(); m != nil {
				allWorkloads = append(allWorkloads, *m)
			}
		}
		data.Workloads = customfield.NewObjectListMust(ctx, allWorkloads)
	}

	// Always fetch nodeclasses from server (supports both normal read and import)
	nodeClassList, err := c.client.ListNodeClasses(clusterUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to list nodeclasses",
			err.Error(),
		)
		return
	}

	ncByName := make(map[string]api.EC2NodeClassModel, len(nodeClassList.EC2NodeClasses))
	for ni := range nodeClassList.EC2NodeClasses {
		ncModel, err := nodeClassList.EC2NodeClasses[ni].ToEC2NodeClassModel(ctx)
		if err != nil {
			resp.Diagnostics.AddError("failed to convert nodeclass", err.Error())
			return
		}
		if ncModel != nil {
			ncByName[nodeClassList.EC2NodeClasses[ni].Name] = *ncModel
		}
	}

	if !data.NodeClasses.IsNullOrUnknown() {
		stateNCs, diags := data.NodeClasses.AsStructSliceT(ctx)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		stateNCByName := make(map[string]api.EC2NodeClassModel, len(stateNCs))
		for _, s := range stateNCs {
			stateNCByName[s.Name.ValueString()] = s
		}
		for name, nc := range ncByName {
			if stateNC, ok := stateNCByName[name]; ok {
				var err error
				nc, err = preserveNodeClassStateRepresentation(ctx, nc, stateNC)
				if err != nil {
					resp.Diagnostics.AddError("failed to preserve nodeclass state", err.Error())
					return
				}
				ncByName[name] = nc
			}
		}

		orderedNCs := orderByStateName(stateNCs, ncByName, func(m api.EC2NodeClassModel) string {
			return m.Name.ValueString()
		})
		nodeClasses, diag := customfield.NewObjectList(ctx, orderedNCs)
		if diag.HasError() {
			resp.Diagnostics.Append(diag...)
			return
		}
		data.NodeClasses = nodeClasses
	} else if isImport && len(ncByName) > 0 {
		allNCs := sortedValuesByName(ncByName, func(m api.EC2NodeClassModel) string {
			return m.Name.ValueString()
		})
		nodeClasses, diag := customfield.NewObjectList(ctx, allNCs)
		if diag.HasError() {
			resp.Diagnostics.Append(diag...)
			return
		}
		data.NodeClasses = nodeClasses
	}

	// Always fetch nodepools from server (supports both normal read and import)
	nodePoolList, err := c.client.ListNodePools(clusterUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to list nodepools",
			err.Error(),
		)
		return
	}

	npByName := make(map[string]api.EC2NodePoolModel, len(nodePoolList.EC2NodePools))
	for ni := range nodePoolList.EC2NodePools {
		npModel, err := nodePoolList.EC2NodePools[ni].ToEC2NodePoolModel()
		if err != nil {
			resp.Diagnostics.AddError("failed to convert nodepool", err.Error())
			return
		}
		if npModel != nil {
			npByName[nodePoolList.EC2NodePools[ni].Name] = *npModel
		}
	}

	if !data.NodePools.IsNullOrUnknown() {
		stateNPs, diags := data.NodePools.AsStructSliceT(ctx)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		stateNPByName := make(map[string]api.EC2NodePoolModel, len(stateNPs))
		for _, s := range stateNPs {
			stateNPByName[s.Name.ValueString()] = s
		}
		for name, np := range npByName {
			if stateNP, ok := stateNPByName[name]; ok {
				np = preserveNodePoolStateRepresentation(ctx, np, stateNP)
				npByName[name] = np
			}
		}

		orderedNPs := orderByStateName(stateNPs, npByName, func(m api.EC2NodePoolModel) string {
			return m.Name.ValueString()
		})
		nodePools, diag := customfield.NewObjectList(ctx, orderedNPs)
		if diag.HasError() {
			resp.Diagnostics.Append(diag...)
			return
		}
		data.NodePools = nodePools
	} else if isImport && len(npByName) > 0 {
		allNPs := sortedValuesByName(npByName, func(m api.EC2NodePoolModel) string {
			return m.Name.ValueString()
		})
		nodePools, diag := customfield.NewObjectList(ctx, allNPs)
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

	authCfg, err := executionAuthConfigFromModel(ctx, data)
	if err != nil {
		resp.Diagnostics.AddError("failed to decode aws auth configuration", err.Error())
		return
	}

	if err := c.fillMissingParameters(ctx, &data, authCfg); err != nil {
		resp.Diagnostics.AddError(
			"failed to fill missing parameters",
			err.Error(),
		)
		return
	}

	awsEnv, err := awsauth.CommandEnv(ctx, authCfg)
	if err != nil {
		resp.Diagnostics.AddError("failed to prepare aws auth environment", err.Error())
		return
	}

	shellKubeconfig, cleanupShellKubeconfig, err := awsauth.KubeconfigForCommandEnv(data.Kubeconfig.ValueString(), awsEnv)
	if err != nil {
		resp.Diagnostics.AddError("failed to prepare shell kubeconfig", err.Error())
		return
	}
	defer cleanupShellKubeconfig()

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

func (c *Cluster) fillMissingParameters(ctx context.Context, data *ClusterModel, authCfg awsauth.ExecutionAuthConfig) error {
	if data.AccountID.IsNull() || data.AccountID.IsUnknown() || data.AccountID.ValueString() == "" {
		accountID, err := awsauth.GetAccountID(ctx, authCfg)
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
		if err := awsauth.UpdateKubeconfig(ctx, data.ClusterName.ValueString(), data.Region.ValueString(), kubeconfigPath, authCfg); err != nil {
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
		if err := c.client.UpdateClusterSetting(clusterUID, setting.ToAPI()); err != nil {
			return fmt.Errorf("failed to update cluster setting: %w", err)
		}
	}
	tflog.Info(ctx, "synced cluster setting successfully")
	return nil
}

func (c *Cluster) syncConfiguration(ctx context.Context, data *ClusterModel, clusterUID string,
	previousNCNames, previousNPNames map[string]struct{},
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
	if err := helper.SyncEC2NodeClassConfiguration(ctx, c.client, clusterUID, data.ClusterName.ValueString(), data.NodeClasses, data.NodeClassTemplates, previousNCNames); err != nil {
		return fmt.Errorf("failed to sync nodeclass configuration: %w", err)
	}
	tflog.Info(ctx, "synced nodeclass configuration successfully")

	// sync nodepool configurations
	tflog.Info(ctx, "syncing nodepool configuration")
	if err := helper.SyncEC2NodePoolConfiguration(ctx, c.client, clusterUID, data.NodePools, data.NodePoolTemplates, previousNPNames); err != nil {
		return fmt.Errorf("failed to sync nodepool configuration: %w", err)
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

	return nil
}

func hydratePostWriteState(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, data *ClusterModel) error {
	var err error
	data.ClusterSetting, err = hydrateClusterSettingPostWrite(ctx, client, clusterUID, data.ClusterSetting)
	if err != nil {
		return err
	}
	data.NodeClasses, err = hydrateNodeClassesPostWrite(ctx, client, clusterUID, data.NodeClasses)
	if err != nil {
		return err
	}
	data.NodeClassTemplates, err = normalizeNodeClassTemplatesPostWrite(ctx, data.NodeClassTemplates)
	if err != nil {
		return err
	}
	data.NodePools, err = hydrateNodePoolsPostWrite(ctx, client, clusterUID, data.NodePools)
	if err != nil {
		return err
	}
	data.NodePoolTemplates, err = normalizeNodePoolTemplatesPostWrite(ctx, data.NodePoolTemplates)
	if err != nil {
		return err
	}
	return nil
}

func hydrateClusterSettingPostWrite(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObject[ClusterSettingModel]) (customfield.NestedObject[ClusterSettingModel], error) {
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

	remote, err := client.GetClusterSetting(clusterUID)
	if err != nil {
		return current, fmt.Errorf("failed to get cluster setting: %w", err)
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
	if remote.EnableNodeRepair != nil {
		setting.EnableNodeRepair = types.BoolValue(*remote.EnableNodeRepair)
	}
	if remote.EnableDiskMonitor != nil {
		setting.EnableDiskMonitor = types.BoolValue(*remote.EnableDiskMonitor)
	}
	if remote.Discount != nil {
		setting.Discount = types.Float64Value(*remote.Discount)
	}
	if remote.PreRunCommand != nil {
		setting.PreRunCommand = types.StringValue(*remote.PreRunCommand)
	}
	if remote.PostRunCommand != nil {
		setting.PostRunCommand = types.StringValue(*remote.PostRunCommand)
	}
}

func hydrateNodeClassesPostWrite(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObjectList[api.EC2NodeClassModel]) (customfield.NestedObjectList[api.EC2NodeClassModel], error) {
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

	remoteByName := make(map[string]api.EC2NodeClassModel, len(remoteList.EC2NodeClasses))
	for i := range remoteList.EC2NodeClasses {
		model, err := remoteList.EC2NodeClasses[i].ToEC2NodeClassModel(ctx)
		if err != nil {
			return current, fmt.Errorf("failed to convert nodeclass %q: %w", remoteList.EC2NodeClasses[i].Name, err)
		}
		if model != nil {
			remoteByName[remoteList.EC2NodeClasses[i].Name] = *model
		}
	}

	hydrated := make([]api.EC2NodeClassModel, 0, len(stateNodeClasses))
	for _, stateNodeClass := range stateNodeClasses {
		if remote, ok := remoteByName[stateNodeClass.Name.ValueString()]; ok {
			preserved, err := preserveNodeClassStateRepresentation(ctx, remote, stateNodeClass)
			if err != nil {
				return current, fmt.Errorf("failed to preserve nodeclass state for %q: %w", stateNodeClass.Name.ValueString(), err)
			}
			hydrated = append(hydrated, preserved)
			continue
		}

		hydrated = append(hydrated, normalizeNodeClassComputedUnknowns(stateNodeClass))
	}

	list, diags := customfield.NewObjectList(ctx, hydrated)
	if diags.HasError() {
		return current, fmt.Errorf("failed to build nodeclasses state: %v", diags)
	}
	return list, nil
}

func normalizeNodeClassComputedUnknowns(model api.EC2NodeClassModel) api.EC2NodeClassModel {
	if model.AmiAlias.IsUnknown() {
		model.AmiAlias = types.StringNull()
	}
	if model.UserData.IsUnknown() {
		model.UserData = types.StringNull()
	}
	return model
}

func normalizeNodeClassTemplatesPostWrite(ctx context.Context, current customfield.NestedObjectList[api.EC2NodeClassTemplateModel]) (customfield.NestedObjectList[api.EC2NodeClassTemplateModel], error) {
	if current.IsNullOrUnknown() {
		return current, nil
	}

	templates, diags := current.AsStructSliceT(ctx)
	if diags.HasError() {
		return current, fmt.Errorf("failed to parse nodeclass_templates: %v", diags)
	}
	for i := range templates {
		if templates[i].AmiAlias.IsUnknown() {
			templates[i].AmiAlias = types.StringNull()
		}
		if templates[i].UserData.IsUnknown() {
			templates[i].UserData = types.StringNull()
		}
	}

	list, diags := customfield.NewObjectList(ctx, templates)
	if diags.HasError() {
		return current, fmt.Errorf("failed to build nodeclass_templates state: %v", diags)
	}
	return list, nil
}

func hydrateNodePoolsPostWrite(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObjectList[api.EC2NodePoolModel]) (customfield.NestedObjectList[api.EC2NodePoolModel], error) {
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

	remoteByName := make(map[string]api.EC2NodePoolModel, len(remoteList.EC2NodePools))
	for i := range remoteList.EC2NodePools {
		model, err := remoteList.EC2NodePools[i].ToEC2NodePoolModel()
		if err != nil {
			return current, fmt.Errorf("failed to convert nodepool %q: %w", remoteList.EC2NodePools[i].Name, err)
		}
		if model != nil {
			remoteByName[remoteList.EC2NodePools[i].Name] = normalizeNodePoolComputedUnknowns(*model)
		}
	}

	hydrated := make([]api.EC2NodePoolModel, 0, len(stateNodePools))
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

func normalizeNodePoolComputedUnknowns(model api.EC2NodePoolModel) api.EC2NodePoolModel {
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

func normalizeNodePoolTemplatesPostWrite(ctx context.Context, current customfield.NestedObjectList[api.EC2NodePoolTemplateModel]) (customfield.NestedObjectList[api.EC2NodePoolTemplateModel], error) {
	if current.IsNullOrUnknown() {
		return current, nil
	}

	templates, diags := current.AsStructSliceT(ctx)
	if diags.HasError() {
		return current, fmt.Errorf("failed to parse nodepool_templates: %v", diags)
	}
	for i := range templates {
		if templates[i].NodeClass.IsUnknown() {
			templates[i].NodeClass = types.StringNull()
		}
		if templates[i].InstanceCPUMIN.IsUnknown() {
			templates[i].InstanceCPUMIN = types.Int64Null()
		}
		if templates[i].InstanceMemoryMIN.IsUnknown() {
			templates[i].InstanceMemoryMIN = types.Int64Null()
		}
	}

	list, diags := customfield.NewObjectList(ctx, templates)
	if diags.HasError() {
		return current, fmt.Errorf("failed to build nodepool_templates state: %v", diags)
	}
	return list, nil
}

func validateNodeClassDiskFields(ctx context.Context, data *ClusterModel) error {
	if err := validateNodeClassDiskFieldsForClasses(ctx, data.NodeClasses); err != nil {
		return err
	}
	return validateNodeClassDiskFieldsForTemplates(ctx, data.NodeClassTemplates)
}

func validateNodeClassDiskFieldsForClasses(ctx context.Context, list customfield.NestedObjectList[api.EC2NodeClassModel]) error {
	if list.IsNullOrUnknown() {
		return nil
	}
	items, diags := list.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("nodeclasses: %v", diags)
	}
	for _, item := range items {
		if hasSystemDiskSize(item.SystemDiskSizeGib) && !item.BlockDeviceMappings.IsNullOrUnknown() {
			return fmt.Errorf("nodeclass %q configures both system_disk_size_gib and block_device_mappings; choose one disk representation", item.Name.ValueString())
		}
	}
	return nil
}

func validateNodeClassDiskFieldsForTemplates(ctx context.Context, list customfield.NestedObjectList[api.EC2NodeClassTemplateModel]) error {
	if list.IsNullOrUnknown() {
		return nil
	}
	items, diags := list.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("nodeclass_templates: %v", diags)
	}
	for _, item := range items {
		if hasSystemDiskSize(item.SystemDiskSizeGib) && !item.BlockDeviceMappings.IsNullOrUnknown() {
			return fmt.Errorf("nodeclass template %q configures both system_disk_size_gib and block_device_mappings; choose one disk representation", item.TemplateName.ValueString())
		}
	}
	return nil
}

func hasSystemDiskSize(value types.Int64) bool {
	return !value.IsNull() && !value.IsUnknown()
}

func preserveNodeClassStateRepresentation(ctx context.Context, remote, state api.EC2NodeClassModel) (api.EC2NodeClassModel, error) {
	remote.TemplateName = state.TemplateName
	remote.Role = preserveManagedString(state.Role, remote.Role)
	remote.EnableImageAccelerator = preserveManagedBool(state.EnableImageAccelerator, remote.EnableImageAccelerator)
	remote.AmiAlias = preserveManagedString(state.AmiAlias, remote.AmiAlias)
	remote.UserData = preserveManagedString(state.UserData, remote.UserData)
	remote.SubnetSelectorTerms = preserveManagedObjectList(ctx, state.SubnetSelectorTerms, remote.SubnetSelectorTerms)
	remote.SecurityGroupSelectorTerms = preserveManagedObjectList(ctx, state.SecurityGroupSelectorTerms, remote.SecurityGroupSelectorTerms)
	remote.InstanceTags = preserveManagedMap(ctx, state.InstanceTags, remote.InstanceTags)
	remote.ExtraCPUAllocationMCore = preserveManagedInt64(state.ExtraCPUAllocationMCore, remote.ExtraCPUAllocationMCore)
	remote.ExtraMemoryAllocationMib = preserveManagedInt64(state.ExtraMemoryAllocationMib, remote.ExtraMemoryAllocationMib)
	remote.OriginNodeClassJSON = preserveManagedString(state.OriginNodeClassJSON, remote.OriginNodeClassJSON)

	if !hasSystemDiskSize(state.SystemDiskSizeGib) {
		remote.SystemDiskSizeGib = preserveManagedInt64(state.SystemDiskSizeGib, remote.SystemDiskSizeGib)
		remote.BlockDeviceMappings = preserveManagedObjectList(ctx, state.BlockDeviceMappings, remote.BlockDeviceMappings)
		return remote, nil
	}

	size, ok, err := systemDiskSizeFromBlockDeviceMappings(ctx, remote.BlockDeviceMappings)
	if err != nil {
		return remote, err
	}
	if ok {
		remote.SystemDiskSizeGib = size
	} else {
		remote.SystemDiskSizeGib = types.Int64Null()
	}
	remote.BlockDeviceMappings = customfield.NullObjectList[api.BlockDeviceMappingModel](ctx)
	return remote, nil
}

func preserveNodePoolStateRepresentation(ctx context.Context, remote, state api.EC2NodePoolModel) api.EC2NodePoolModel {
	remote.TemplateName = state.TemplateName
	remote.Enable = preserveManagedBool(state.Enable, remote.Enable)
	remote.NodeClass = preserveManagedString(state.NodeClass, remote.NodeClass)
	remote.EnableImageAccelerator = preserveManagedBool(state.EnableImageAccelerator, remote.EnableImageAccelerator)
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
	remote.NodeDisruptionLimit = preserveManagedString(state.NodeDisruptionLimit, remote.NodeDisruptionLimit)
	remote.NodeDisruptionDelay = preserveManagedDuration(state.NodeDisruptionDelay, remote.NodeDisruptionDelay)
	remote.Labels = preserveManagedMap(ctx, state.Labels, remote.Labels)
	remote.Taints = preserveManagedObjectList(ctx, state.Taints, remote.Taints)
	remote.OriginNodePoolJSON = preserveManagedString(state.OriginNodePoolJSON, remote.OriginNodePoolJSON)
	return remote
}

func preserveWorkloadStateRepresentation(remote, state api.WorkloadModel) api.WorkloadModel {
	remote.TemplateName = state.TemplateName
	remote.RebalanceAble = preserveManagedBool(state.RebalanceAble, remote.RebalanceAble)
	remote.SpotFriendly = preserveManagedBool(state.SpotFriendly, remote.SpotFriendly)
	remote.MinNonSpotReplicas = preserveManagedInt64(state.MinNonSpotReplicas, remote.MinNonSpotReplicas)
	return remote
}

func systemDiskSizeFromBlockDeviceMappings(ctx context.Context, mappings customfield.NestedObjectList[api.BlockDeviceMappingModel]) (types.Int64, bool, error) {
	if mappings.IsNullOrUnknown() {
		return types.Int64Null(), false, nil
	}
	items, diags := mappings.AsStructSliceT(ctx)
	if diags.HasError() {
		return types.Int64Null(), false, fmt.Errorf("block_device_mappings: %v", diags)
	}
	if len(items) == 0 || items[0].EBS.IsNull() || items[0].EBS.IsUnknown() {
		return types.Int64Null(), false, nil
	}
	ebs, ebsDiags := items[0].EBS.Value(ctx)
	if ebsDiags.HasError() {
		return types.Int64Null(), false, fmt.Errorf("block_device_mappings.ebs: %v", ebsDiags)
	}
	if ebs == nil || ebs.VolumeSize.IsNull() || ebs.VolumeSize.IsUnknown() || ebs.VolumeSize.ValueString() == "" {
		return types.Int64Null(), false, nil
	}
	quantity, err := k8sresource.ParseQuantity(ebs.VolumeSize.ValueString())
	if err != nil {
		return types.Int64Null(), false, fmt.Errorf("block_device_mappings.ebs.volume_size: %w", err)
	}
	return types.Int64Value(quantity.Value() / int64(api.BytesToGiB)), true, nil
}

// orderByStateName returns server items that are tracked in state, preserving
// state order. Items on the server but NOT in state are ignored — Terraform
// only manages resources it declared.
func orderByStateName[T any](stateItems []T, serverByName map[string]T, getName func(T) string) []T {
	result := make([]T, 0, len(stateItems))
	for _, stateItem := range stateItems {
		name := getName(stateItem)
		if serverItem, ok := serverByName[name]; ok {
			result = append(result, serverItem)
		}
	}
	return result
}

// extractResourceNames extracts the set of names from a NestedObjectList in
// Terraform state. Returns nil if the list is null/unknown. This is used to
// determine which resources were previously managed by Terraform so that only
// those can be considered for deletion during sync.
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

func resolveClusterUID(preferred, fallback, clusterName, region, accountID types.String) string {
	if !preferred.IsNull() && !preferred.IsUnknown() && preferred.ValueString() != "" {
		return preferred.ValueString()
	}
	if !fallback.IsNull() && !fallback.IsUnknown() && fallback.ValueString() != "" {
		return fallback.ValueString()
	}
	return api.GenerateClusterUID(api.CloudProviderAWS, clusterName.ValueString(), region.ValueString(), accountID.ValueString())
}

func resolveDeleteClusterUID(clusterID, clusterName, region, accountID types.String) string {
	return resolveClusterUID(clusterID, clusterID, clusterName, region, accountID)
}

// preserveEmptyList keeps an explicit empty slice from state when the server
// returns nil, avoiding false Terraform diffs like `+ instance_family = []`.
func preserveEmptyList(stateVal, serverVal *[]types.String) *[]types.String {
	if serverVal == nil && stateVal != nil && len(*stateVal) == 0 {
		return stateVal
	}
	return serverVal
}

// preserveSemanticDuration keeps the state value when the state and server
// values represent the same duration (e.g. "60m" vs "1h"), avoiding false
// Terraform diffs caused by different textual representations.
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

func isManagedBool(value types.Bool) bool {
	return !value.IsNull()
}

func boolValueOrDefault(value types.Bool, fallback bool) bool {
	if !isManagedBool(value) {
		return fallback
	}
	return value.ValueBool()
}

func int64ValueOrDefault(value types.Int64, fallback int64) int64 {
	if value.IsNull() || value.IsUnknown() {
		return fallback
	}
	return value.ValueInt64()
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
	if remoteVal.IsNull() || remoteVal.IsUnknown() {
		values, diags := stateVal.Value(ctx)
		if !diags.HasError() && len(values) == 0 {
			return stateVal
		}
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
