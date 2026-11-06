package eks

import (
	"context"
	"errors"
	"reflect"
	"testing"

	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestHydratePostWriteStateRefreshesConfiguredClusterSetting(t *testing.T) {
	ctx := context.Background()
	enableNodeRepair := true
	discount := 0.15
	data := ClusterModel{
		ClusterSetting: customfield.NewObjectMust(ctx, &ClusterSettingModel{
			EnableNodeRepair:  types.BoolUnknown(),
			EnableDiskMonitor: types.BoolUnknown(),
			Discount:          types.Float64Value(discount),
			PreRunCommand:     types.StringUnknown(),
			PostRunCommand:    types.StringUnknown(),
		}),
	}

	err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{
		clusterSetting: &api.ClusterSetting{
			EnableNodeRepair: &enableNodeRepair,
			Discount:         &discount,
		},
	}, "cluster-1", &data)
	if err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	value, diags := data.ClusterSetting.Value(ctx)
	if diags.HasError() {
		t.Fatalf("cluster setting diagnostics = %v", diags)
	}
	if value == nil {
		t.Fatal("cluster setting should not be nil")
	}
	if value.EnableNodeRepair.IsUnknown() || !value.EnableNodeRepair.ValueBool() {
		t.Fatalf("EnableNodeRepair = %#v, want known true", value.EnableNodeRepair)
	}
	if value.EnableDiskMonitor.IsUnknown() {
		t.Fatalf("EnableDiskMonitor should not remain unknown")
	}
	if value.PreRunCommand.IsUnknown() || value.PostRunCommand.IsUnknown() {
		t.Fatalf("cluster setting string siblings should not remain unknown")
	}
}

func TestSyncClusterSettingSendsEmptyPreAndPostCommands(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterSettingUpdateClient{}
	data := ClusterModel{
		ClusterSetting: customfield.NewObjectMust(ctx, &ClusterSettingModel{
			PreRunCommand:  types.StringValue(""),
			PostRunCommand: types.StringValue(""),
		}),
	}

	if err := (&Cluster{client: client}).syncClusterSetting(ctx, &data, "cluster-1"); err != nil {
		t.Fatalf("syncClusterSetting() error = %v", err)
	}

	if client.setting == nil {
		t.Fatalf("UpdateClusterSetting was not called")
	}
	if client.setting.PreRunCommand == nil || *client.setting.PreRunCommand != "" {
		t.Fatalf("PreRunCommand = %#v, want empty string pointer", client.setting.PreRunCommand)
	}
	if client.setting.PostRunCommand == nil || *client.setting.PostRunCommand != "" {
		t.Fatalf("PostRunCommand = %#v, want empty string pointer", client.setting.PostRunCommand)
	}
}

func TestHydratePostWriteStateRefreshesNodeClassesFromServer(t *testing.T) {
	ctx := context.Background()
	userData := "echo existing"
	data := ClusterModel{
		NodeClasses: customfield.NewObjectListMust(ctx, []api.EC2NodeClassModel{{
			Name:         types.StringValue("cloudpilot"),
			TemplateName: types.StringValue("default"),
			AmiAlias:     types.StringUnknown(),
			UserData:     types.StringUnknown(),
		}}),
	}

	err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{
		nodeClasses: api.RebalanceNodeClassList{
			EC2NodeClasses: []api.EC2NodeClass{{
				Name: "cloudpilot",
				NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{
					AMISelectorTerms: []awsproviderv1.AMISelectorTerm{{Alias: "al2023@latest"}},
					UserData:         &userData,
				},
			}},
		},
	}, "cluster-1", &data)
	if err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	nodeClasses, diags := data.NodeClasses.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("nodeclasses diagnostics = %v", diags)
	}
	if len(nodeClasses) != 1 {
		t.Fatalf("got %d nodeclasses, want 1", len(nodeClasses))
	}
	if nodeClasses[0].AmiAlias.IsUnknown() || nodeClasses[0].AmiAlias.ValueString() != "al2023@latest" {
		t.Fatalf("AmiAlias = %#v, want known al2023@latest", nodeClasses[0].AmiAlias)
	}
	if nodeClasses[0].UserData.IsUnknown() || nodeClasses[0].UserData.ValueString() != userData {
		t.Fatalf("UserData = %#v, want known %q", nodeClasses[0].UserData, userData)
	}
	if nodeClasses[0].TemplateName.ValueString() != "default" {
		t.Fatalf("TemplateName = %q, want default", nodeClasses[0].TemplateName.ValueString())
	}
}

func TestHydratePostWriteStateLeavesMissingNodeClassStringsNull(t *testing.T) {
	ctx := context.Background()
	data := ClusterModel{
		NodeClasses: customfield.NewObjectListMust(ctx, []api.EC2NodeClassModel{{
			Name:     types.StringValue("cloudpilot"),
			AmiAlias: types.StringUnknown(),
			UserData: types.StringUnknown(),
		}}),
	}

	err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{
		nodeClasses: api.RebalanceNodeClassList{
			EC2NodeClasses: []api.EC2NodeClass{{
				Name:          "cloudpilot",
				NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{},
			}},
		},
	}, "cluster-1", &data)
	if err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	nodeClasses, diags := data.NodeClasses.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("nodeclasses diagnostics = %v", diags)
	}
	if !nodeClasses[0].AmiAlias.IsNull() {
		t.Fatalf("AmiAlias should be null after hydration, got %#v", nodeClasses[0].AmiAlias)
	}
	if !nodeClasses[0].UserData.IsNull() {
		t.Fatalf("UserData should be null after hydration, got %#v", nodeClasses[0].UserData)
	}
}

func TestHydratePostWriteStateNormalizesUnknownNodeClassTemplateStrings(t *testing.T) {
	ctx := context.Background()
	data := ClusterModel{
		NodeClassTemplates: customfield.NewObjectListMust(ctx, []api.EC2NodeClassTemplateModel{{
			TemplateName: types.StringValue("default"),
			AmiAlias:     types.StringUnknown(),
			UserData:     types.StringUnknown(),
		}}),
	}

	if err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{}, "cluster-1", &data); err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	templates, diags := data.NodeClassTemplates.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("nodeclass templates diagnostics = %v", diags)
	}
	if len(templates) != 1 {
		t.Fatalf("got %d nodeclass templates, want 1", len(templates))
	}
	if !templates[0].AmiAlias.IsNull() {
		t.Fatalf("AmiAlias should be null after hydration, got %#v", templates[0].AmiAlias)
	}
	if !templates[0].UserData.IsNull() {
		t.Fatalf("UserData should be null after hydration, got %#v", templates[0].UserData)
	}
}

func TestHydratePostWriteStateRefreshesNodePoolsFromServer(t *testing.T) {
	ctx := context.Background()
	data := ClusterModel{
		NodePools: customfield.NewObjectListMust(ctx, []api.EC2NodePoolModel{{
			Name:              types.StringValue("cloudpilot-general"),
			TemplateName:      types.StringValue("default"),
			InstanceCPUMIN:    types.Int64Unknown(),
			InstanceMemoryMIN: types.Int64Unknown(),
		}}),
	}

	err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{
		nodePools: api.RebalanceNodePoolList{
			EC2NodePools: []api.EC2NodePool{{
				Name:         "cloudpilot-general",
				NodePoolSpec: api.DefaultGeneralEC2NodePoolSpec(),
			}},
		},
	}, "cluster-1", &data)
	if err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	nodePools, diags := data.NodePools.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("nodepools diagnostics = %v", diags)
	}
	if len(nodePools) != 1 {
		t.Fatalf("got %d nodepools, want 1", len(nodePools))
	}
	if nodePools[0].InstanceCPUMIN.IsUnknown() {
		t.Fatalf("InstanceCPUMIN should not remain unknown")
	}
	if !nodePools[0].InstanceCPUMIN.IsNull() {
		t.Fatalf("InstanceCPUMIN should be null when server omits the minimum filter, got %#v", nodePools[0].InstanceCPUMIN)
	}
	if nodePools[0].InstanceMemoryMIN.IsUnknown() {
		t.Fatalf("InstanceMemoryMIN should not remain unknown")
	}
	if !nodePools[0].InstanceMemoryMIN.IsNull() {
		t.Fatalf("InstanceMemoryMIN should be null when server omits the minimum filter, got %#v", nodePools[0].InstanceMemoryMIN)
	}
	if nodePools[0].TemplateName.ValueString() != "default" {
		t.Fatalf("TemplateName = %q, want default", nodePools[0].TemplateName.ValueString())
	}
}

func TestHydratePostWriteStateNormalizesUnknownNodePoolTemplateMinimums(t *testing.T) {
	ctx := context.Background()
	data := ClusterModel{
		NodePoolTemplates: customfield.NewObjectListMust(ctx, []api.EC2NodePoolTemplateModel{{
			TemplateName:      types.StringValue("default"),
			InstanceCPUMIN:    types.Int64Unknown(),
			InstanceMemoryMIN: types.Int64Unknown(),
		}}),
	}

	if err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{}, "cluster-1", &data); err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	templates, diags := data.NodePoolTemplates.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("nodepool templates diagnostics = %v", diags)
	}
	if len(templates) != 1 {
		t.Fatalf("got %d nodepool templates, want 1", len(templates))
	}
	if !templates[0].InstanceCPUMIN.IsNull() {
		t.Fatalf("InstanceCPUMIN should be null after hydration, got %#v", templates[0].InstanceCPUMIN)
	}
	if !templates[0].InstanceMemoryMIN.IsNull() {
		t.Fatalf("InstanceMemoryMIN should be null after hydration, got %#v", templates[0].InstanceMemoryMIN)
	}
}

func TestClusterSettingObjectPreservingStateLeavesNewUnmanagedFieldsNull(t *testing.T) {
	ctx := context.Background()
	enabled := true
	current := customfield.NewObjectMust(ctx, &ClusterSettingModel{
		EnableNodePoolDecommission: types.BoolNull(),
		EnableWorkloadMinNonSpot:   types.BoolValue(false),
	})
	got, err := clusterSettingObjectPreservingState(ctx, current, &api.ClusterSetting{
		EnableNodePoolDecommission: &enabled,
		EnableWorkloadMinNonSpot:   &enabled,
	})
	if err != nil {
		t.Fatalf("clusterSettingObjectPreservingState() error = %v", err)
	}
	value, diags := got.Value(ctx)
	if diags.HasError() || value == nil {
		t.Fatalf("cluster_setting = %#v, diagnostics = %v", value, diags)
	}
	if !value.EnableNodePoolDecommission.IsNull() {
		t.Fatalf("enable_node_pool_decommission = %#v, want null", value.EnableNodePoolDecommission)
	}
	if value.EnableWorkloadMinNonSpot != types.BoolValue(true) {
		t.Fatalf("enable_workload_min_non_spot = %#v, want managed remote true", value.EnableWorkloadMinNonSpot)
	}
}

func TestClusterSettingObjectPreservingStateReflectsManagedRemoteRemoval(t *testing.T) {
	ctx := context.Background()
	current := customfield.NewObjectMust(ctx, &ClusterSettingModel{
		EnableNodePoolDecommission: types.BoolValue(true),
		PreRunCommand:              types.StringValue("echo before"),
	})

	got, err := clusterSettingObjectPreservingState(ctx, current, &api.ClusterSetting{})
	if err != nil {
		t.Fatalf("clusterSettingObjectPreservingState() error = %v", err)
	}
	value, diags := got.Value(ctx)
	if diags.HasError() || value == nil {
		t.Fatalf("cluster_setting = %#v, diagnostics = %v", value, diags)
	}
	if !value.EnableNodePoolDecommission.IsNull() || !value.PreRunCommand.IsNull() {
		t.Fatalf("managed removed values were preserved: %#v", value)
	}
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
		t.Fatal("successful empty reset must remain an object")
	}
}

func TestClusterSettingsValidateSpotAllocationStrategy(t *testing.T) {
	if err := (ClusterSettingModel{AWSSpotAllocationStrategy: types.StringValue("capacity-optimized")}).Validate(); err == nil {
		t.Fatal("invalid allocation strategy accepted")
	}
	model := ClusterSettingModel{AWSSpotAllocationStrategy: types.StringValue("price-capacity-optimized")}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	value := model.ToAPI()
	if value.AWSSpotAllocationStrategy == nil || *value.AWSSpotAllocationStrategy != "price-capacity-optimized" {
		t.Fatalf("API settings lost allocation strategy: %#v", value)
	}
}

func TestDynamicDiskPolicyPreservesOmittedMultiplier(t *testing.T) {
	ctx := context.Background()
	state := customfield.NewObjectMust(ctx, &api.VolumeSizePolicyModel{MaxSizeGiB: types.Int64Value(512)})
	remote := customfield.NewObjectMust(ctx, &api.VolumeSizePolicyModel{MaxSizeGiB: types.Int64Value(512), PerVCPUGiB: types.Int64Value(0)})
	result := preserveVolumeSizePolicy(ctx, remote, state)
	value, _ := result.Value(ctx)
	if !value.PerVCPUGiB.IsNull() {
		t.Fatal("unmanaged multiplier materialized in state")
	}
}

func TestSyncConfigurationDeletesNodePoolBeforeReferencedNodeClass(t *testing.T) {
	ctx := context.Background()
	client := &fakeConfigurationPreflightClient{nodePools: api.RebalanceNodePoolList{EC2NodePools: []api.EC2NodePool{{Name: "old-pool"}}}}
	cluster := &Cluster{client: client}
	data := ClusterModel{
		NodeClasses: customfield.NewObjectListMust(ctx, []api.EC2NodeClassModel{}),
		NodePools:   customfield.NewObjectListMust(ctx, []api.EC2NodePoolModel{}),
	}
	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", map[string]struct{}{"old-class": {}}, map[string]struct{}{"old-pool": {}}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.operations) != 2 || client.operations[0] != "delete-nodepool" || client.operations[1] != "delete-nodeclass" {
		t.Fatalf("deletion order = %v, want nodepool before nodeclass", client.operations)
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
