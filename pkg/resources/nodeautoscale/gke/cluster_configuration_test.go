package gke

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClusterSettingObjectPreservingStateLeavesUnmanagedRemoteValuesNull(t *testing.T) {
	ctx := context.Background()
	enableNodeRepair := false
	enableDiskMonitor := false
	enableNodePoolDecommission := true
	enableWorkloadMinNonSpot := true
	discount := 0.2
	preRunCommand := "remote-pre"
	postRunCommand := "remote-post"
	current := customfield.NewObjectMust(ctx, &ClusterSettingModel{
		EnableNodeRepair:           types.BoolNull(),
		EnableDiskMonitor:          types.BoolValue(true),
		EnableNodePoolDecommission: types.BoolNull(),
		EnableWorkloadMinNonSpot:   types.BoolValue(false),
		Discount:                   types.Float64Null(),
		PreRunCommand:              types.StringNull(),
		PostRunCommand:             types.StringValue("configured-post"),
	})

	got, err := clusterSettingObjectPreservingState(ctx, current, &api.ClusterSetting{
		EnableNodeRepair:           &enableNodeRepair,
		EnableDiskMonitor:          &enableDiskMonitor,
		EnableNodePoolDecommission: &enableNodePoolDecommission,
		EnableWorkloadMinNonSpot:   &enableWorkloadMinNonSpot,
		Discount:                   &discount,
		PreRunCommand:              &preRunCommand,
		PostRunCommand:             &postRunCommand,
	})
	if err != nil {
		t.Fatalf("clusterSettingObjectPreservingState() error = %v", err)
	}
	value, diags := got.Value(ctx)
	if diags.HasError() {
		t.Fatalf("cluster_setting diagnostics = %v", diags)
	}
	if value == nil {
		t.Fatal("cluster_setting value is nil")
	}
	if !value.EnableNodeRepair.IsNull() {
		t.Fatalf("enable_node_repair = %#v, want null", value.EnableNodeRepair)
	}
	if value.EnableDiskMonitor != types.BoolValue(false) {
		t.Fatalf("enable_disk_monitor = %#v, want managed remote false", value.EnableDiskMonitor)
	}
	if !value.EnableNodePoolDecommission.IsNull() {
		t.Fatalf("enable_node_pool_decommission = %#v, want null", value.EnableNodePoolDecommission)
	}
	if value.EnableWorkloadMinNonSpot != types.BoolValue(true) {
		t.Fatalf("enable_workload_min_non_spot = %#v, want managed remote true", value.EnableWorkloadMinNonSpot)
	}
	if !value.Discount.IsNull() {
		t.Fatalf("discount = %#v, want null", value.Discount)
	}
	if !value.PreRunCommand.IsNull() {
		t.Fatalf("pre_run_command = %#v, want null", value.PreRunCommand)
	}
	if value.PostRunCommand != types.StringValue("remote-post") {
		t.Fatalf("post_run_command = %#v, want managed remote value", value.PostRunCommand)
	}
}

func TestClusterSettingObjectPreservingStateReflectsManagedRemoteRemoval(t *testing.T) {
	ctx := context.Background()
	current := customfield.NewObjectMust(ctx, &ClusterSettingModel{
		EnableNodePoolDecommission: types.BoolValue(true),
		EnableWorkloadMinNonSpot:   types.BoolValue(false),
	})

	got, err := clusterSettingObjectPreservingState(ctx, current, &api.ClusterSetting{})
	if err != nil {
		t.Fatalf("clusterSettingObjectPreservingState() error = %v", err)
	}
	value, diags := got.Value(ctx)
	if diags.HasError() || value == nil {
		t.Fatalf("cluster_setting = %#v, diagnostics = %v", value, diags)
	}
	if !value.EnableNodePoolDecommission.IsNull() || !value.EnableWorkloadMinNonSpot.IsNull() {
		t.Fatalf("managed removed values were preserved: %#v", value)
	}
}

func TestPreserveNodePoolStateRepresentationDropsUnmanagedRemoteLabels(t *testing.T) {
	ctx := context.Background()
	state := api.GCENodePoolModel{
		Name:   types.StringValue("cloudpilot-general"),
		Labels: customfield.NewMapMust[types.String](ctx, map[string]types.String{"codex-test": types.StringValue("update-1")}),
	}
	remote := api.GCENodePoolModel{
		Name: types.StringValue("cloudpilot-general"),
		Labels: customfield.NewMapMust[types.String](ctx, map[string]types.String{
			"codex-test":                 types.StringValue("update-1"),
			"node.cloudpilot.ai/managed": types.StringValue("true"),
		}),
		NodeDisruptionBudgets: customfield.NewObjectListMust(ctx, []api.DisruptionBudgetModel{{
			Nodes: types.StringValue("10%"),
		}}),
	}

	got := preserveNodePoolStateRepresentation(ctx, remote, state)
	values, diags := got.Labels.Value(ctx)
	if diags.HasError() {
		t.Fatalf("labels diagnostics = %v", diags)
	}
	if len(values) != 1 {
		t.Fatalf("labels = %#v, want only the user-managed label", values)
	}
	if _, ok := values["node.cloudpilot.ai/managed"]; ok {
		t.Fatalf("labels should not keep unmanaged remote default: %#v", values)
	}
	if !got.NodeDisruptionBudgets.IsNull() {
		t.Fatalf("NodeDisruptionBudgets should remain null when budgets are omitted from state")
	}
}

func TestPreserveNodePoolStateRepresentationKeepsBudgetDurationRepresentation(t *testing.T) {
	ctx := context.Background()
	state := api.GCENodePoolModel{
		NodeDisruptionBudgets: customfield.NewObjectListMust(ctx, []api.DisruptionBudgetModel{{
			Nodes:    types.StringValue("10%"),
			Duration: types.StringValue("60m"),
		}}),
	}
	remote := api.GCENodePoolModel{
		NodeDisruptionBudgets: customfield.NewObjectListMust(ctx, []api.DisruptionBudgetModel{{
			Nodes:    types.StringValue("10%"),
			Reasons:  nil,
			Duration: types.StringValue("1h"),
		}}),
	}

	got := preserveNodePoolStateRepresentation(ctx, remote, state)
	budgets, diags := got.NodeDisruptionBudgets.AsStructSliceT(ctx)
	if diags.HasError() || len(budgets) != 1 {
		t.Fatalf("budgets = %#v, diagnostics = %v", budgets, diags)
	}
	if budgets[0].Duration != types.StringValue("60m") {
		t.Fatalf("duration = %#v, want preserved 60m", budgets[0].Duration)
	}
}

func TestPreserveNodeClassStateRepresentationLeavesUnmanagedNetworkConfigFieldsNull(t *testing.T) {
	ctx := context.Background()
	state := api.GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		NetworkConfig: customfield.NewObjectMust(ctx, &api.GCENetworkConfigModel{
			EnablePrivateNodes:          types.BoolNull(),
			Subnetwork:                  types.StringValue("projects/test/regions/us-central1/subnetworks/default"),
			AdditionalNetworkInterfaces: customfield.NullObjectList[api.GCEAdditionalNetworkInterfaceModel](ctx),
		}),
	}
	remote := api.GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		NetworkConfig: customfield.NewObjectMust(ctx, &api.GCENetworkConfigModel{
			EnablePrivateNodes:          types.BoolValue(false),
			Subnetwork:                  types.StringValue("projects/test/regions/us-central1/subnetworks/default"),
			AdditionalNetworkInterfaces: customfield.NullObjectList[api.GCEAdditionalNetworkInterfaceModel](ctx),
		}),
	}

	got := preserveNodeClassStateRepresentation(ctx, remote, state)
	networkConfig, diags := got.NetworkConfig.Value(ctx)
	if diags.HasError() {
		t.Fatalf("network_config diagnostics = %v", diags)
	}
	if networkConfig == nil {
		t.Fatal("network_config is nil")
	}
	if !networkConfig.EnablePrivateNodes.IsNull() {
		t.Fatalf("enable_private_nodes = %#v, want null for unmanaged field", networkConfig.EnablePrivateNodes)
	}
	if networkConfig.Subnetwork.ValueString() != "projects/test/regions/us-central1/subnetworks/default" {
		t.Fatalf("subnetwork = %#v, want configured subnetwork", networkConfig.Subnetwork)
	}
}

func TestPreserveNodeClassStateRepresentationKeepsManagedNetworkConfigFields(t *testing.T) {
	ctx := context.Background()
	state := api.GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		NetworkConfig: customfield.NewObjectMust(ctx, &api.GCENetworkConfigModel{
			EnablePrivateNodes:          types.BoolValue(false),
			Subnetwork:                  types.StringValue("projects/test/regions/us-central1/subnetworks/default"),
			AdditionalNetworkInterfaces: customfield.NullObjectList[api.GCEAdditionalNetworkInterfaceModel](ctx),
		}),
	}
	remote := api.GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		NetworkConfig: customfield.NewObjectMust(ctx, &api.GCENetworkConfigModel{
			EnablePrivateNodes:          types.BoolValue(true),
			Subnetwork:                  types.StringValue("projects/test/regions/us-central1/subnetworks/updated"),
			AdditionalNetworkInterfaces: customfield.NullObjectList[api.GCEAdditionalNetworkInterfaceModel](ctx),
		}),
	}

	got := preserveNodeClassStateRepresentation(ctx, remote, state)
	networkConfig, diags := got.NetworkConfig.Value(ctx)
	if diags.HasError() {
		t.Fatalf("network_config diagnostics = %v", diags)
	}
	if networkConfig == nil {
		t.Fatal("network_config is nil")
	}
	if !networkConfig.EnablePrivateNodes.ValueBool() {
		t.Fatalf("enable_private_nodes = %#v, want remote managed value true", networkConfig.EnablePrivateNodes)
	}
	if networkConfig.Subnetwork.ValueString() != "projects/test/regions/us-central1/subnetworks/updated" {
		t.Fatalf("subnetwork = %#v, want remote managed value", networkConfig.Subnetwork)
	}
}

func TestPreserveNodeClassStateRepresentationLeavesUnmanagedNestedFieldsNull(t *testing.T) {
	ctx := context.Background()
	state := api.GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		Disks: customfield.NewObjectListMust(ctx, []api.GCEDiskModel{{
			SizeGiB:  types.Int64Null(),
			Category: types.StringValue("pd-balanced"),
			Boot:     types.BoolNull(),
		}}),
		ImageSelectorTerms: customfield.NewObjectListMust(ctx, []api.GCEImageSelectorTermModel{{
			ID:      types.StringNull(),
			Family:  types.StringValue("ContainerOptimizedOS"),
			Channel: types.StringNull(),
			Version: types.StringValue("1.2.3.4"),
		}}),
		KubeletConfiguration: customfield.NewObjectMust(ctx, &api.GCEKubeletConfigurationModel{
			KubeReserved: customfield.NewMapMust[types.String](ctx, map[string]types.String{
				"cpu": types.StringValue("100m"),
			}),
			SystemReserved: customfield.NullMap[types.String](ctx),
			EvictionHard:   customfield.NewMapMust[types.String](ctx, map[string]types.String{}),
			EvictionSoft:   customfield.NullMap[types.String](ctx),
		}),
		NetworkConfig: customfield.NewObjectMust(ctx, &api.GCENetworkConfigModel{
			AdditionalNetworkInterfaces: customfield.NewObjectListMust(ctx, []api.GCEAdditionalNetworkInterfaceModel{{
				Network:    types.StringNull(),
				Subnetwork: types.StringValue("projects/test/regions/us-central1/subnetworks/secondary"),
			}}),
		}),
	}
	remote := api.GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		Disks: customfield.NewObjectListMust(ctx, []api.GCEDiskModel{{
			SizeGiB:  types.Int64Value(100),
			Category: types.StringValue("pd-ssd"),
			Boot:     types.BoolValue(false),
		}}),
		ImageSelectorTerms: customfield.NewObjectListMust(ctx, []api.GCEImageSelectorTermModel{{
			ID:      types.StringValue("projects/test/global/images/cos"),
			Family:  types.StringValue("ContainerOptimizedOS"),
			Channel: types.StringValue("regular"),
			Version: types.StringValue("2.3.4.5"),
		}}),
		KubeletConfiguration: customfield.NewObjectMust(ctx, &api.GCEKubeletConfigurationModel{
			KubeReserved: customfield.NewMapMust[types.String](ctx, map[string]types.String{
				"cpu":    types.StringValue("200m"),
				"memory": types.StringValue("512Mi"),
			}),
			SystemReserved: customfield.NewMapMust[types.String](ctx, map[string]types.String{
				"cpu": types.StringValue("100m"),
			}),
			EvictionHard: customfield.NewMapMust[types.String](ctx, map[string]types.String{
				"memory.available": types.StringValue("100Mi"),
			}),
			EvictionSoft: customfield.NewMapMust[types.String](ctx, map[string]types.String{
				"memory.available": types.StringValue("200Mi"),
			}),
		}),
		NetworkConfig: customfield.NewObjectMust(ctx, &api.GCENetworkConfigModel{
			AdditionalNetworkInterfaces: customfield.NewObjectListMust(ctx, []api.GCEAdditionalNetworkInterfaceModel{{
				Network:    types.StringValue("projects/test/global/networks/default"),
				Subnetwork: types.StringValue("projects/test/regions/us-central1/subnetworks/updated"),
			}}),
		}),
	}

	got := preserveNodeClassStateRepresentation(ctx, remote, state)
	disks, diskDiags := got.Disks.AsStructSliceT(ctx)
	if diskDiags.HasError() {
		t.Fatalf("disks diagnostics = %v", diskDiags)
	}
	if len(disks) != 1 {
		t.Fatalf("disks length = %d, want 1", len(disks))
	}
	if !disks[0].SizeGiB.IsNull() {
		t.Fatalf("disks[0].size_gib = %#v, want null", disks[0].SizeGiB)
	}
	if disks[0].Category != types.StringValue("pd-ssd") {
		t.Fatalf("disks[0].category = %#v, want remote managed value", disks[0].Category)
	}
	if !disks[0].Boot.IsNull() {
		t.Fatalf("disks[0].boot = %#v, want null", disks[0].Boot)
	}

	terms, termDiags := got.ImageSelectorTerms.AsStructSliceT(ctx)
	if termDiags.HasError() {
		t.Fatalf("image_selector_terms diagnostics = %v", termDiags)
	}
	if !terms[0].ID.IsNull() {
		t.Fatalf("image_selector_terms[0].id = %#v, want null", terms[0].ID)
	}
	if terms[0].Family != types.StringValue("ContainerOptimizedOS") {
		t.Fatalf("image_selector_terms[0].family = %#v, want managed remote value", terms[0].Family)
	}
	if !terms[0].Channel.IsNull() {
		t.Fatalf("image_selector_terms[0].channel = %#v, want null", terms[0].Channel)
	}
	if terms[0].Version != types.StringValue("2.3.4.5") {
		t.Fatalf("image_selector_terms[0].version = %#v, want managed remote value", terms[0].Version)
	}

	kubeletConfig, kubeletDiags := got.KubeletConfiguration.Value(ctx)
	if kubeletDiags.HasError() {
		t.Fatalf("kubelet_configuration diagnostics = %v", kubeletDiags)
	}
	kubeReserved, kubeReservedDiags := kubeletConfig.KubeReserved.Value(ctx)
	if kubeReservedDiags.HasError() {
		t.Fatalf("kube_reserved diagnostics = %v", kubeReservedDiags)
	}
	if len(kubeReserved) != 1 || kubeReserved["cpu"] != types.StringValue("200m") {
		t.Fatalf("kube_reserved = %#v, want only managed cpu from remote", kubeReserved)
	}
	if !kubeletConfig.SystemReserved.IsNull() {
		t.Fatalf("system_reserved = %#v, want null", kubeletConfig.SystemReserved)
	}
	evictionHard, evictionHardDiags := kubeletConfig.EvictionHard.Value(ctx)
	if evictionHardDiags.HasError() {
		t.Fatalf("eviction_hard diagnostics = %v", evictionHardDiags)
	}
	if len(evictionHard) != 0 {
		t.Fatalf("eviction_hard = %#v, want explicit empty map", evictionHard)
	}
	if !kubeletConfig.EvictionSoft.IsNull() {
		t.Fatalf("eviction_soft = %#v, want null", kubeletConfig.EvictionSoft)
	}

	networkConfig, networkConfigDiags := got.NetworkConfig.Value(ctx)
	if networkConfigDiags.HasError() {
		t.Fatalf("network_config diagnostics = %v", networkConfigDiags)
	}
	nics, nicDiags := networkConfig.AdditionalNetworkInterfaces.AsStructSliceT(ctx)
	if nicDiags.HasError() {
		t.Fatalf("additional_network_interfaces diagnostics = %v", nicDiags)
	}
	if !nics[0].Network.IsNull() {
		t.Fatalf("additional_network_interfaces[0].network = %#v, want null", nics[0].Network)
	}
	if nics[0].Subnetwork != types.StringValue("projects/test/regions/us-central1/subnetworks/updated") {
		t.Fatalf("additional_network_interfaces[0].subnetwork = %#v, want managed remote value", nics[0].Subnetwork)
	}
}

func TestPreserveNodePoolStateRepresentationLeavesUnmanagedTaintValueNull(t *testing.T) {
	ctx := context.Background()
	state := api.GCENodePoolModel{
		Name: types.StringValue("cloudpilot-general"),
		Taints: customfield.NewObjectListMust(ctx, []api.TaintModel{{
			Key:    types.StringValue("dedicated"),
			Value:  types.StringNull(),
			Effect: types.StringValue("NoSchedule"),
		}}),
	}
	remote := api.GCENodePoolModel{
		Name: types.StringValue("cloudpilot-general"),
		Taints: customfield.NewObjectListMust(ctx, []api.TaintModel{{
			Key:    types.StringValue("dedicated"),
			Value:  types.StringValue("remote-default"),
			Effect: types.StringValue("NoSchedule"),
		}}),
	}

	got := preserveNodePoolStateRepresentation(ctx, remote, state)
	taints, diags := got.Taints.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("taints diagnostics = %v", diags)
	}
	if len(taints) != 1 {
		t.Fatalf("taints length = %d, want 1", len(taints))
	}
	if !taints[0].Value.IsNull() {
		t.Fatalf("taints[0].value = %#v, want null", taints[0].Value)
	}
}

func testNodeClassModel(ctx context.Context, name string) api.GCENodeClassModel {
	return api.GCENodeClassModel{
		Name: types.StringValue(name),
		ImageSelectorTerms: customfield.NewObjectListMust(ctx, []api.GCEImageSelectorTermModel{
			{
				ID: types.StringValue("projects/demo/global/images/container-optimized-os"),
			},
		}),
	}
}

func testNodePoolModel(name, nodeClass string, instanceFamily *[]types.String, disruptionDelay string) api.GCENodePoolModel {
	return api.GCENodePoolModel{
		Name:                types.StringValue(name),
		NodeClass:           types.StringValue(nodeClass),
		InstanceFamily:      instanceFamily,
		NodeDisruptionDelay: types.StringValue(disruptionDelay),
	}
}

func testRemoteNodeClass(name string) api.GCENodeClass {
	return api.GCENodeClass{
		Name:                   name,
		EnableImageAccelerator: true,
		NodeClassSpec: &api.GCENodeClassSpec{
			ImageSelectorTerms: []api.GCEImageSelectorTerm{
				{
					ID: "projects/demo/global/images/container-optimized-os",
				},
			},
		},
	}
}

func testRemoteNodePool(t *testing.T, ctx context.Context, name, nodeClass string, instanceFamily *[]types.String, disruptionDelay string) api.GCENodePool {
	t.Helper()

	model := testNodePoolModel(name, nodeClass, instanceFamily, disruptionDelay)
	model.EnableImageAccelerator = types.BoolValue(true)
	nodePool, err := model.ToGCENodePool(ctx, api.GCENodePool{Name: name})
	if err != nil {
		t.Fatalf("ToGCENodePool() error = %v", err)
	}
	return *nodePool
}

func testManagedRemoteNodePool(t *testing.T, ctx context.Context, name, nodeClass string) api.GCENodePool {
	t.Helper()

	nodePool := testRemoteNodePool(t, ctx, name, nodeClass, nil, "60m")
	if nodePool.NodePoolSpec == nil {
		t.Fatalf("testRemoteNodePool() returned nil NodePoolSpec")
	}
	if nodePool.NodePoolSpec.Template.ObjectMeta.Labels == nil {
		nodePool.NodePoolSpec.Template.ObjectMeta.Labels = map[string]string{}
	}
	nodePool.NodePoolSpec.Template.ObjectMeta.Labels["node.cloudpilot.ai/managed"] = "true"
	return nodePool
}
func TestClusterSettingsParityPreservesConfiguredRepresentation(t *testing.T) {
	ctx := context.Background()
	cooldown := "30m0s"
	current := customfield.NewObjectMust(ctx, &ClusterSettingModel{
		DisruptionCooldown: types.StringValue("30m"),
	})
	updated, err := clusterSettingObjectPreservingState(ctx, current, &api.ClusterSetting{
		DisruptionCooldown: &cooldown,
	})
	if err != nil {
		t.Fatal(err)
	}
	value, diags := updated.Value(ctx)
	if diags.HasError() || value == nil {
		t.Fatalf("settings diagnostics: %v", diags)
	}
	if value.DisruptionCooldown.ValueString() != "30m" || !value.Discount.IsNull() {
		t.Fatalf("configured representation changed: %#v", value)
	}
}

func TestClusterSettingsValidateCooldown(t *testing.T) {
	for _, value := range []string{"-1s", "1ms", "forever", "999999999999999999h"} {
		if err := (ClusterSettingModel{DisruptionCooldown: types.StringValue(value)}).Validate(); err == nil {
			t.Fatalf("invalid cooldown %q accepted", value)
		}
	}
	if err := (ClusterSettingModel{DisruptionCooldown: types.StringValue("1h30m")}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyJobOptimizationRetainsResetState(t *testing.T) {
	ctx := context.Background()
	state := customfield.NewObjectMust(ctx, &api.JobNodePoolOptimizationModel{})
	remote := customfield.NullObject[api.JobNodePoolOptimizationModel](ctx)
	if result := preserveJobOptimization(ctx, remote, state); result.IsNull() {
		t.Fatal("successful empty policy reset must remain an object")
	}
}

func (f *fakeClusterClient) GetWorkloadRebalanceConfiguration(string) (*api.ClusterWorkloadSpec, error) {
	return &api.ClusterWorkloadSpec{}, nil
}

type workloadReadClient struct{ config *api.ClusterWorkloadSpec }

func (f workloadReadClient) GetWorkloadRebalanceConfiguration(string) (*api.ClusterWorkloadSpec, error) {
	return f.config, nil
}

func TestAgentOnlyIgnoresModuleDefaultEmptyLists(t *testing.T) {
	ctx := context.Background()
	model := ClusterModel{
		OnlyInstallAgent: types.BoolValue(true), EnableRebalance: types.BoolValue(false),
		NodeClasses: customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{}),
		NodePools:   customfield.NewObjectListMust(ctx, []api.GCENodePoolModel{}),
	}
	if shouldManageNodeAutoscaler(model) {
		t.Fatal("default empty lists must not install Node Autoscaler in agent-only mode")
	}
	if !shouldSkipRebalanceDisableSync(model) {
		t.Fatal("agent-only mode must leave rebalance unmanaged")
	}
	model.EnableRebalance = types.BoolValue(true)
	if !shouldManageNodeAutoscaler(model) {
		t.Fatal("explicit rebalance must override agent-only mode")
	}
}

func TestGKEWorkloadStatePreservesUnmanagedFields(t *testing.T) {
	ctx := context.Background()
	client := workloadReadClient{config: &api.ClusterWorkloadSpec{Workloads: []api.Workload{{Name: "app", Type: "deployment", Namespace: "default", RebalanceAble: false, SpotFriendly: true, MinNonSpotReplicas: 2}}}}
	current := customfield.NewObjectListMust(ctx, []api.WorkloadModel{{Name: types.StringValue("app"), Type: types.StringValue("deployment"), Namespace: types.StringValue("default"), RebalanceAble: types.BoolValue(true)}})
	updated, err := hydrateWorkloadsState(ctx, client, "cluster-1", current, false)
	if err != nil {
		t.Fatal(err)
	}
	models, _ := updated.AsStructSliceT(ctx)
	if len(models) != 1 || models[0].RebalanceAble.ValueBool() || !models[0].SpotFriendly.IsNull() || !models[0].MinNonSpotReplicas.IsNull() {
		t.Fatalf("unexpected state: %#v", models)
	}
	imported, err := hydrateWorkloadsState(ctx, client, "cluster-1", customfield.NullObjectList[api.WorkloadModel](ctx), true)
	if err != nil {
		t.Fatal(err)
	}
	importedModels, _ := imported.AsStructSliceT(ctx)
	if len(importedModels) != 1 || !importedModels[0].SpotFriendly.ValueBool() || importedModels[0].MinNonSpotReplicas.ValueInt64() != 2 {
		t.Fatalf("workload import lost fields: %#v", importedModels)
	}
}

type fakeClusterSettingSyncClient struct {
	cloudpilotaiclient.Interface
	settings         []*api.ClusterSetting
	settingErr       error
	maintenanceCalls int
}

func (f *fakeClusterSettingSyncClient) UpdateClusterSetting(_ string, setting *api.ClusterSetting) error {
	f.settings = append(f.settings, setting)
	return f.settingErr
}

func (f *fakeClusterSettingSyncClient) UpdateClusterMaintenanceStatus(_ string, _ *api.ClusterMaintenanceStatus) error {
	f.maintenanceCalls++
	return nil
}

func TestSyncClusterSettingWritesOnlyManagedFields(t *testing.T) {
	ctx := context.Background()
	discount, enabled := 0.5, false
	for _, tc := range []struct {
		name  string
		value customfield.NestedObject[ClusterSettingModel]
		want  *api.ClusterSetting
	}{
		{name: "null block", value: customfield.NullObject[ClusterSettingModel](ctx)},
		{name: "unknown block", value: customfield.UnknownObject[ClusterSettingModel](ctx)},
		{name: "empty block", value: customfield.NewObjectMust(ctx, &ClusterSettingModel{})},
		{name: "unknown fields", value: customfield.NewObjectMust(ctx, &ClusterSettingModel{EnableNodeRepair: types.BoolUnknown(), Discount: types.Float64Unknown(), DisruptionCooldown: types.StringUnknown()})},
		{name: "partial block", value: customfield.NewObjectMust(ctx, &ClusterSettingModel{EnableNodeRepair: types.BoolValue(false), Discount: types.Float64Value(discount)}), want: &api.ClusterSetting{EnableNodeRepair: &enabled, Discount: &discount}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeClusterSettingSyncClient{}
			cluster := &Cluster{client: client}
			if err := cluster.syncClusterSetting(ctx, &ClusterModel{ClusterSetting: tc.value}, "cluster-1"); err != nil {
				t.Fatal(err)
			}
			wantCalls := 0
			if tc.want != nil {
				wantCalls = 1
			}
			if len(client.settings) != wantCalls {
				t.Fatalf("settings calls = %d, want %d", len(client.settings), wantCalls)
			}
			if tc.want != nil && !reflect.DeepEqual(client.settings[0], tc.want) {
				t.Fatalf("settings payload = %#v, want %#v", client.settings[0], tc.want)
			}
			if client.maintenanceCalls != 0 {
				t.Fatal("Terraform must not update maintenance status")
			}
		})
	}
}

func TestSyncClusterSettingPropagatesUpdateFailure(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("settings update failed")
	client := &fakeClusterSettingSyncClient{settingErr: failure}
	cluster := &Cluster{client: client}
	data := ClusterModel{ClusterSetting: customfield.NewObjectMust(ctx, &ClusterSettingModel{Discount: types.Float64Value(0.5)})}
	if err := cluster.syncClusterSetting(ctx, &data, "cluster-1"); !errors.Is(err, failure) {
		t.Fatalf("settings failure = %v", err)
	}
}

func TestClusterSettingSchemaExcludesUnsupportedFields(t *testing.T) {
	attributes := schemaAttributesClusterSetting(context.Background())["cluster_setting"].(schema.SingleNestedAttribute).Attributes
	for _, name := range []string{"maintenance_enabled", "memory_request_multiplier", "zero_memory_request_mib", "saving_report_baseline_start_at", "saving_report_baseline_end_at"} {
		if _, ok := attributes[name]; ok {
			t.Fatalf("cluster_setting must not expose %s", name)
		}
	}
}
