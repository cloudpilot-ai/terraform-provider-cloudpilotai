package eks

import (
	"context"
	"testing"

	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	schemaplanmodifier "github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type namedItem struct {
	name string
}

type fakePostWriteStateHydratorClient struct {
	clusterSetting *api.ClusterSetting
	nodeClasses    api.RebalanceNodeClassList
	nodePools      api.RebalanceNodePoolList
}

func (f *fakePostWriteStateHydratorClient) GetClusterSetting(string) (*api.ClusterSetting, error) {
	return f.clusterSetting, nil
}

func (f *fakePostWriteStateHydratorClient) ListNodeClasses(string) (api.RebalanceNodeClassList, error) {
	return f.nodeClasses, nil
}

func (f *fakePostWriteStateHydratorClient) ListNodePools(string) (api.RebalanceNodePoolList, error) {
	return f.nodePools, nil
}

func TestSortedValuesByName(t *testing.T) {
	items := map[string]namedItem{
		"p2p":                {name: "p2p"},
		"cloudpilot-general": {name: "cloudpilot-general"},
		"cloudpilot-gpu":     {name: "cloudpilot-gpu"},
	}

	got := sortedValuesByName(items, func(item namedItem) string {
		return item.name
	})

	want := []string{"cloudpilot-general", "cloudpilot-gpu", "p2p"}
	if len(got) != len(want) {
		t.Fatalf("got %d items, want %d", len(got), len(want))
	}

	for i, item := range got {
		if item.name != want[i] {
			t.Fatalf("got order %v, want %v", []string{got[0].name, got[1].name, got[2].name}, want)
		}
	}
}

func TestUpdatePrefersStateClusterIDOverGeneratedID(t *testing.T) {
	got := resolveClusterUID(
		types.StringUnknown(),
		types.StringValue("server-imported-id"),
		types.StringValue("test-saving-20260601-144407"),
		types.StringValue("us-east-2"),
		types.StringValue("306107317780"),
	)

	if got != "server-imported-id" {
		t.Fatalf("got cluster ID %q, want imported state cluster ID", got)
	}
}

func TestUpdatePrefersConfiguredClusterIDOverStateAndGeneratedID(t *testing.T) {
	got := resolveClusterUID(
		types.StringValue("user-specified-id"),
		types.StringValue("server-imported-id"),
		types.StringValue("test-saving-20260601-144407"),
		types.StringValue("us-east-2"),
		types.StringValue("306107317780"),
	)

	if got != "user-specified-id" {
		t.Fatalf("got cluster ID %q, want user-specified cluster ID", got)
	}
}

func TestSchemaUsesUnifiedUpgradeFlag(t *testing.T) {
	s := Schema(context.Background())

	if _, ok := s.Attributes["enable_upgrade"]; !ok {
		t.Fatalf("eks schema missing enable_upgrade")
	}
	if _, ok := s.Attributes["enable_upgrade_agent"]; ok {
		t.Fatalf("eks schema should not expose enable_upgrade_agent")
	}
	if _, ok := s.Attributes["enable_upgrade_rebalance_component"]; ok {
		t.Fatalf("eks schema should not expose enable_upgrade_rebalance_component")
	}
}

func TestEnableRebalanceHasNoSchemaDefault(t *testing.T) {
	attr, ok := Schema(context.Background()).Attributes["enable_rebalance"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("enable_rebalance attribute has unexpected type %T", Schema(context.Background()).Attributes["enable_rebalance"])
	}
	if attr.BoolDefaultValue() != nil {
		t.Fatalf("enable_rebalance should not have a schema default")
	}
	if attr.IsComputed() {
		t.Fatalf("enable_rebalance should not be computed")
	}
}

func TestDeletePrefersConfiguredClusterIDOverGeneratedID(t *testing.T) {
	got := resolveDeleteClusterUID(
		types.StringValue("user-specified-id"),
		types.StringValue("test-saving-20260601-144407"),
		types.StringValue("us-east-2"),
		types.StringValue("306107317780"),
	)

	if got != "user-specified-id" {
		t.Fatalf("got cluster ID %q, want configured cluster ID during delete", got)
	}
}

func TestWorkloadOptionalFieldsHaveNoDefault(t *testing.T) {
	workloadsAttr, ok := Schema(context.Background()).Attributes["workloads"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("workloads attribute has unexpected type %T", Schema(context.Background()).Attributes["workloads"])
	}

	rebalanceAttr, ok := workloadsAttr.NestedObject.Attributes["rebalance_able"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("rebalance_able attribute has unexpected type %T", workloadsAttr.NestedObject.Attributes["rebalance_able"])
	}
	if rebalanceAttr.BoolDefaultValue() != nil {
		t.Fatalf("rebalance_able should not have a schema default")
	}
	if rebalanceAttr.IsComputed() {
		t.Fatalf("rebalance_able should not be computed")
	}

	spotAttr, ok := workloadsAttr.NestedObject.Attributes["spot_friendly"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("spot_friendly attribute has unexpected type %T", workloadsAttr.NestedObject.Attributes["spot_friendly"])
	}
	if spotAttr.BoolDefaultValue() != nil {
		t.Fatalf("spot_friendly should not have a schema default")
	}
	if spotAttr.IsComputed() {
		t.Fatalf("spot_friendly should not be computed")
	}

	replicasAttr, ok := workloadsAttr.NestedObject.Attributes["min_non_spot_replicas"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("min_non_spot_replicas attribute has unexpected type %T", workloadsAttr.NestedObject.Attributes["min_non_spot_replicas"])
	}
	if replicasAttr.Int64DefaultValue() != nil {
		t.Fatalf("min_non_spot_replicas should not have a schema default")
	}
	if replicasAttr.IsComputed() {
		t.Fatalf("min_non_spot_replicas should not be computed")
	}
}

func TestNodeClassExtraAllocationAttributesHaveNoDefault(t *testing.T) {
	nodeClassesAttr, ok := Schema(context.Background()).Attributes["nodeclasses"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("nodeclasses attribute has unexpected type %T", Schema(context.Background()).Attributes["nodeclasses"])
	}

	cpuAttr, ok := nodeClassesAttr.NestedObject.Attributes["extra_cpu_allocation_mcore"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("extra_cpu_allocation_mcore attribute has unexpected type %T", nodeClassesAttr.NestedObject.Attributes["extra_cpu_allocation_mcore"])
	}
	if cpuAttr.Int64DefaultValue() != nil {
		t.Fatalf("extra_cpu_allocation_mcore should not have a schema default")
	}
	if len(cpuAttr.Int64PlanModifiers()) == 0 {
		t.Fatalf("extra_cpu_allocation_mcore should preserve null state in the plan")
	}

	memoryAttr, ok := nodeClassesAttr.NestedObject.Attributes["extra_memory_allocation_mib"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("extra_memory_allocation_mib attribute has unexpected type %T", nodeClassesAttr.NestedObject.Attributes["extra_memory_allocation_mib"])
	}
	if memoryAttr.Int64DefaultValue() != nil {
		t.Fatalf("extra_memory_allocation_mib should not have a schema default")
	}
	if len(memoryAttr.Int64PlanModifiers()) == 0 {
		t.Fatalf("extra_memory_allocation_mib should preserve null state in the plan")
	}
}

func TestNodePoolMinimumInstanceFilterAttributesHaveNoDefault(t *testing.T) {
	nodePoolsAttr, ok := Schema(context.Background()).Attributes["nodepools"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("nodepools attribute has unexpected type %T", Schema(context.Background()).Attributes["nodepools"])
	}

	cpuMinAttr, ok := nodePoolsAttr.NestedObject.Attributes["instance_cpu_min"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("instance_cpu_min attribute has unexpected type %T", nodePoolsAttr.NestedObject.Attributes["instance_cpu_min"])
	}
	if cpuMinAttr.Int64DefaultValue() != nil {
		t.Fatalf("instance_cpu_min should not have a schema default")
	}
	if len(cpuMinAttr.Int64PlanModifiers()) == 0 {
		t.Fatalf("instance_cpu_min should preserve null state in the plan")
	}

	memoryMinAttr, ok := nodePoolsAttr.NestedObject.Attributes["instance_memory_min"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("instance_memory_min attribute has unexpected type %T", nodePoolsAttr.NestedObject.Attributes["instance_memory_min"])
	}
	if memoryMinAttr.Int64DefaultValue() != nil {
		t.Fatalf("instance_memory_min should not have a schema default")
	}
	if len(memoryMinAttr.Int64PlanModifiers()) == 0 {
		t.Fatalf("instance_memory_min should preserve null state in the plan")
	}
}

func TestUseStateForUnknownInt64PreservesNullState(t *testing.T) {
	resp := &schemaplanmodifier.Int64Response{
		PlanValue: types.Int64Unknown(),
	}

	useStateForUnknownInt64().PlanModifyInt64(context.Background(), schemaplanmodifier.Int64Request{
		State: tfsdk.State{
			Raw: tftypes.NewValue(
				tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"attr": tftypes.Number,
					},
				},
				map[string]tftypes.Value{
					"attr": tftypes.NewValue(tftypes.Number, nil),
				},
			),
		},
		StateValue:  types.Int64Null(),
		PlanValue:   types.Int64Unknown(),
		ConfigValue: types.Int64Null(),
	}, resp)

	if !resp.PlanValue.IsNull() {
		t.Fatalf("plan value should remain null, got %v", resp.PlanValue)
	}
}

func TestUseStateForUnknownStringPreservesNullState(t *testing.T) {
	resp := &schemaplanmodifier.StringResponse{
		PlanValue: types.StringUnknown(),
	}

	useStateForUnknownString().PlanModifyString(context.Background(), schemaplanmodifier.StringRequest{
		State: tfsdk.State{
			Raw: tftypes.NewValue(
				tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"attr": tftypes.String,
					},
				},
				map[string]tftypes.Value{
					"attr": tftypes.NewValue(tftypes.String, nil),
				},
			),
		},
		StateValue:  types.StringNull(),
		PlanValue:   types.StringUnknown(),
		ConfigValue: types.StringNull(),
	}, resp)

	if !resp.PlanValue.IsNull() {
		t.Fatalf("plan value should remain null, got %v", resp.PlanValue)
	}
}

func TestOperationalStringAttributesDoNotPreserveNullState(t *testing.T) {
	tests := []string{"kubeconfig", "account_id"}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			attr, ok := Schema(context.Background()).Attributes[name].(schema.StringAttribute)
			if !ok {
				t.Fatalf("%s attribute has unexpected type %T", name, Schema(context.Background()).Attributes[name])
			}
			modifiers := attr.StringPlanModifiers()
			if len(modifiers) == 0 {
				t.Fatalf("%s should define a string plan modifier", name)
			}

			resp := &schemaplanmodifier.StringResponse{
				PlanValue: types.StringUnknown(),
			}
			modifiers[0].PlanModifyString(context.Background(), schemaplanmodifier.StringRequest{
				State: tfsdk.State{
					Raw: tftypes.NewValue(
						tftypes.Object{
							AttributeTypes: map[string]tftypes.Type{
								name: tftypes.String,
							},
						},
						map[string]tftypes.Value{
							name: tftypes.NewValue(tftypes.String, nil),
						},
					),
				},
				StateValue:  types.StringNull(),
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringNull(),
			}, resp)

			if !resp.PlanValue.IsUnknown() {
				t.Fatalf("%s plan value should stay unknown so apply can backfill it, got %v", name, resp.PlanValue)
			}
		})
	}
}

func TestNodeClassSchemaIncludesFrontendFields(t *testing.T) {
	s := Schema(context.Background())
	nodeClassesAttr := s.Attributes["nodeclasses"].(schema.ListNestedAttribute)
	attrs := nodeClassesAttr.NestedObject.Attributes
	for _, name := range []string{"ami_alias", "user_data", "block_device_mappings"} {
		if _, ok := attrs[name]; !ok {
			t.Fatalf("nodeclasses schema missing %s", name)
		}
	}
}

func TestNodeClassBlockDeviceMappingsSchemaMatchesFrontendSurface(t *testing.T) {
	s := Schema(context.Background())
	nodeClassesAttr := s.Attributes["nodeclasses"].(schema.ListNestedAttribute)
	blockDeviceMappingsAttr, ok := nodeClassesAttr.NestedObject.Attributes["block_device_mappings"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("block_device_mappings attribute has unexpected type %T", nodeClassesAttr.NestedObject.Attributes["block_device_mappings"])
	}
	ebsAttr, ok := blockDeviceMappingsAttr.NestedObject.Attributes["ebs"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("ebs attribute has unexpected type %T", blockDeviceMappingsAttr.NestedObject.Attributes["ebs"])
	}
	for _, name := range []string{"encrypted", "volume_size", "volume_type"} {
		if _, ok := ebsAttr.Attributes[name]; !ok {
			t.Fatalf("ebs schema missing %s", name)
		}
	}
	for _, name := range []string{"delete_on_termination", "iops", "kms_key_id", "snapshot_id", "throughput"} {
		if _, ok := ebsAttr.Attributes[name]; ok {
			t.Fatalf("ebs schema should not expose %s", name)
		}
	}
}

func TestNodeClassFrontendStringFieldsHaveNoDefault(t *testing.T) {
	s := Schema(context.Background())
	nodeClassesAttr := s.Attributes["nodeclasses"].(schema.ListNestedAttribute)
	attrs := nodeClassesAttr.NestedObject.Attributes

	amiAlias, ok := attrs["ami_alias"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("ami_alias attribute has unexpected type %T", attrs["ami_alias"])
	}
	if amiAlias.StringDefaultValue() != nil {
		t.Fatalf("ami_alias should not have a schema default")
	}

	userData, ok := attrs["user_data"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("user_data attribute has unexpected type %T", attrs["user_data"])
	}
	if userData.StringDefaultValue() != nil {
		t.Fatalf("user_data should not have a schema default")
	}
}

func TestPreserveNodeClassStateRepresentationKeepsSystemDiskConvenience(t *testing.T) {
	ctx := context.Background()
	remote := api.EC2NodeClassModel{
		Name: types.StringValue("cloudpilot"),
		BlockDeviceMappings: customfield.NewObjectListMust(ctx, []api.BlockDeviceMappingModel{{
			DeviceName: types.StringValue("/dev/xvda"),
			EBS: customfield.NewObjectMust(ctx, &api.BlockDeviceModel{
				VolumeSize: types.StringValue("64Gi"),
			}),
		}}),
	}
	state := api.EC2NodeClassModel{
		Name:              types.StringValue("cloudpilot"),
		SystemDiskSizeGib: types.Int64Value(20),
	}

	got, err := preserveNodeClassStateRepresentation(ctx, remote, state)
	if err != nil {
		t.Fatalf("preserveNodeClassStateRepresentation() error = %v", err)
	}
	if got.SystemDiskSizeGib.ValueInt64() != 64 {
		t.Fatalf("SystemDiskSizeGib = %d, want 64", got.SystemDiskSizeGib.ValueInt64())
	}
	if !got.BlockDeviceMappings.IsNull() {
		t.Fatalf("BlockDeviceMappings should stay null for system_disk_size_gib representation")
	}
}

func TestPreserveNodeClassStateRepresentationLeavesBlockDeviceMappingsNullWhenOmitted(t *testing.T) {
	ctx := context.Background()
	remote := api.EC2NodeClassModel{
		Name: types.StringValue("cloudpilot"),
		BlockDeviceMappings: customfield.NewObjectListMust(ctx, []api.BlockDeviceMappingModel{{
			DeviceName: types.StringValue("/dev/xvda"),
			EBS: customfield.NewObjectMust(ctx, &api.BlockDeviceModel{
				VolumeSize: types.StringValue("64Gi"),
			}),
		}}),
	}
	state := api.EC2NodeClassModel{
		Name: types.StringValue("cloudpilot"),
	}

	got, err := preserveNodeClassStateRepresentation(ctx, remote, state)
	if err != nil {
		t.Fatalf("preserveNodeClassStateRepresentation() error = %v", err)
	}
	if !got.BlockDeviceMappings.IsNull() {
		t.Fatalf("BlockDeviceMappings should remain null when block_device_mappings is omitted from state")
	}
}

func TestNodePoolSchemaIncludesLabelsAndTaints(t *testing.T) {
	s := Schema(context.Background())
	nodePoolsAttr := s.Attributes["nodepools"].(schema.ListNestedAttribute)
	attrs := nodePoolsAttr.NestedObject.Attributes
	for _, name := range []string{"labels", "taints"} {
		if _, ok := attrs[name]; !ok {
			t.Fatalf("nodepools schema missing %s", name)
		}
	}
}

func TestPreserveNodePoolStateRepresentationLeavesLabelsAndTaintsNullWhenOmitted(t *testing.T) {
	ctx := context.Background()
	remote := api.EC2NodePoolModel{
		Name: types.StringValue("cloudpilot-general"),
		Labels: customfield.NewMapMust[types.String](ctx, map[string]types.String{
			"team": types.StringValue("platform"),
		}),
		Taints: customfield.NewObjectListMust(ctx, []api.TaintModel{{
			Key:    types.StringValue("dedicated"),
			Value:  types.StringValue("wa"),
			Effect: types.StringValue("NoSchedule"),
		}}),
	}
	state := api.EC2NodePoolModel{
		Name: types.StringValue("cloudpilot-general"),
	}

	got := preserveNodePoolStateRepresentation(ctx, remote, state)
	if !got.Labels.IsNull() {
		t.Fatalf("Labels should remain null when labels are omitted from state")
	}
	if !got.Taints.IsNull() {
		t.Fatalf("Taints should remain null when taints are omitted from state")
	}
}

func TestPreserveNodePoolStateRepresentationKeepsEmptyTaintsList(t *testing.T) {
	ctx := context.Background()
	remote := api.EC2NodePoolModel{
		Name:   types.StringValue("cloudpilot-general"),
		Taints: customfield.NullObjectList[api.TaintModel](ctx),
	}
	state := api.EC2NodePoolModel{
		Name:   types.StringValue("cloudpilot-general"),
		Taints: customfield.NewObjectListMust(ctx, []api.TaintModel{}),
	}

	got := preserveNodePoolStateRepresentation(ctx, remote, state)
	if got.Taints.IsNull() {
		t.Fatalf("Taints should preserve an explicit empty list from state")
	}
	taints, diags := got.Taints.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("Taints diagnostics = %v", diags)
	}
	if len(taints) != 0 {
		t.Fatalf("expected empty taints list, got %#v", taints)
	}
}

func TestClusterSettingModelToAPIOnlySendsConfiguredValues(t *testing.T) {
	model := ClusterSettingModel{
		EnableNodeRepair: types.BoolValue(true),
		Discount:         types.Float64Value(0.25),
	}

	got := model.ToAPI()
	if got.EnableNodeRepair == nil || !*got.EnableNodeRepair {
		t.Fatalf("EnableNodeRepair = %#v", got.EnableNodeRepair)
	}
	if got.Discount == nil || *got.Discount != 0.25 {
		t.Fatalf("Discount = %#v", got.Discount)
	}
	if got.EnableDiskMonitor != nil {
		t.Fatalf("EnableDiskMonitor should be omitted, got %#v", got.EnableDiskMonitor)
	}
}

func TestClusterSettingObjectFromAPI(t *testing.T) {
	ctx := context.Background()
	enableRepair := true
	enableDisk := false
	discount := 0.1
	pre := "echo pre"
	post := "echo post"

	got := clusterSettingObjectFromAPI(ctx, &api.ClusterSetting{
		EnableNodeRepair:  &enableRepair,
		EnableDiskMonitor: &enableDisk,
		Discount:          &discount,
		PreRunCommand:     &pre,
		PostRunCommand:    &post,
	})

	value, diags := got.Value(ctx)
	if diags.HasError() {
		t.Fatalf("cluster setting diagnostics = %v", diags)
	}
	if value == nil {
		t.Fatalf("cluster setting should not be nil")
	}
	if !value.EnableNodeRepair.ValueBool() {
		t.Fatalf("EnableNodeRepair should be true")
	}
	if value.PreRunCommand.ValueString() != "echo pre" {
		t.Fatalf("PreRunCommand = %q", value.PreRunCommand.ValueString())
	}
}

func TestClusterSettingSchemaHasExpectedFields(t *testing.T) {
	s := Schema(context.Background())
	clusterSettingAttr, ok := s.Attributes["cluster_setting"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("cluster_setting attribute has unexpected type %T", s.Attributes["cluster_setting"])
	}
	for _, name := range []string{
		"enable_node_repair",
		"enable_disk_monitor",
		"discount",
		"pre_run_command",
		"post_run_command",
	} {
		if _, ok := clusterSettingAttr.Attributes[name]; !ok {
			t.Fatalf("cluster_setting schema missing %s", name)
		}
	}
	if _, ok := clusterSettingAttr.Attributes["maintenance_enabled"]; ok {
		t.Fatalf("cluster_setting schema should not expose maintenance_enabled")
	}
}

func TestClusterSchemaDoesNotExposeEnableDiversityInstanceType(t *testing.T) {
	s := Schema(context.Background())
	if _, ok := s.Attributes["enable_diversity_instance_type"]; ok {
		t.Fatalf("schema should not expose enable_diversity_instance_type")
	}
}

func TestClusterSchemaDoesNotExposeEnableUploadConfig(t *testing.T) {
	s := Schema(context.Background())
	if _, ok := s.Attributes["enable_upload_config"]; ok {
		t.Fatalf("schema should not expose enable_upload_config")
	}
}

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
