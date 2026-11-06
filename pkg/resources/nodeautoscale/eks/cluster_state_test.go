package eks

import (
	"context"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	schemaplanmodifier "github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestRunUpgradeActionAndRefreshClusterSummaryRefreshesAfterUpgrade(t *testing.T) {
	calls := make([]string, 0, 2)
	data := &ClusterModel{}
	client := &fakeClusterSummaryClient{
		summary: &api.ClusterCostsSummary{
			AgentVersion:           "v1.18.6",
			OnboardManifestVersion: "v1.18.7",
			NeedUpgrade:            true,
		},
		onGetCluster: func() {
			calls = append(calls, "refresh")
		},
	}

	applyClusterSummaryStatus(data, client.summary)

	upgradeAction := func() error {
		calls = append(calls, "upgrade")
		client.summary = &api.ClusterCostsSummary{
			AgentVersion:           "v1.18.7",
			OnboardManifestVersion: "v1.18.7",
			NeedUpgrade:            false,
		}
		return nil
	}

	if err := runUpgradeActionAndRefreshClusterSummary(data, client, "cluster-1", upgradeAction); err != nil {
		t.Fatalf("runUpgradeActionAndRefreshClusterSummary() error = %v", err)
	}

	if client.getClusterCalls != 1 {
		t.Fatalf("GetCluster calls = %d, want 1", client.getClusterCalls)
	}
	if len(calls) != 2 || calls[0] != "upgrade" || calls[1] != "refresh" {
		t.Fatalf("call order = %v, want [upgrade refresh]", calls)
	}
	if data.AgentVersion != types.StringValue("v1.18.7") {
		t.Fatalf("AgentVersion = %#v, want v1.18.7", data.AgentVersion)
	}
	if data.OnboardManifestVersion != types.StringValue("v1.18.7") {
		t.Fatalf("OnboardManifestVersion = %#v, want v1.18.7", data.OnboardManifestVersion)
	}
	if data.NeedUpgrade != types.BoolValue(false) {
		t.Fatalf("NeedUpgrade = %#v, want false", data.NeedUpgrade)
	}
}

func TestModifyPlanRefreshesUpgradeStatusFromRemoteSummary(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterSummaryClient{
		summary: &api.ClusterCostsSummary{
			AgentVersion:           "v1.18.6",
			OnboardManifestVersion: "v1.18.7",
			NeedUpgrade:            true,
		},
	}

	plan := tfsdk.Plan{Schema: Schema(ctx)}
	planDiags := plan.Set(ctx, &ClusterModel{
		ClusterID:              types.StringUnknown(),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringNull(),
		OnboardManifestVersion: types.StringNull(),
		NeedUpgrade:            types.BoolNull(),
	})
	if planDiags.HasError() {
		t.Fatalf("plan.Set() diagnostics = %v", planDiags)
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	stateDiags := state.Set(ctx, &ClusterModel{
		ClusterID:              types.StringValue("server-imported-id"),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringValue("123456789012"),
		OnboardManifestVersion: types.StringValue("old-manifest"),
		NeedUpgrade:            types.BoolValue(false),
	})
	if stateDiags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", stateDiags)
	}

	resp := &resource.ModifyPlanResponse{
		Plan: tfsdk.Plan{Schema: Schema(ctx)},
	}
	(&Cluster{client: client}).ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:  plan,
		State: state,
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan() diagnostics = %v", resp.Diagnostics)
	}
	if client.getClusterCalls != 1 {
		t.Fatalf("GetCluster calls = %d, want 1", client.getClusterCalls)
	}

	var got ClusterModel
	getDiags := resp.Plan.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.Plan.Get() diagnostics = %v", getDiags)
	}
	if got.OnboardManifestVersion != types.StringValue("v1.18.7") {
		t.Fatalf("OnboardManifestVersion = %#v, want v1.18.7", got.OnboardManifestVersion)
	}
	if got.NeedUpgrade != types.BoolValue(true) {
		t.Fatalf("NeedUpgrade = %#v, want true", got.NeedUpgrade)
	}
}

func TestModifyPlanLeavesUpgradeStatusUnknownWhenUpgradeWillRun(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterSummaryClient{
		summary: &api.ClusterCostsSummary{
			AgentVersion:           "v1.20.0",
			OnboardManifestVersion: "v1.20.1",
			NeedUpgrade:            true,
		},
	}

	plan := tfsdk.Plan{Schema: Schema(ctx)}
	planDiags := plan.Set(ctx, &ClusterModel{
		ClusterID:              types.StringUnknown(),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringNull(),
		EnableUpgrade:          types.BoolValue(true),
		AgentVersion:           types.StringValue("v1.20.0"),
		OnboardManifestVersion: types.StringValue("v1.20.1"),
		NeedUpgrade:            types.BoolValue(true),
	})
	if planDiags.HasError() {
		t.Fatalf("plan.Set() diagnostics = %v", planDiags)
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	stateDiags := state.Set(ctx, &ClusterModel{
		ClusterID:              types.StringValue("server-imported-id"),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringValue("123456789012"),
		EnableUpgrade:          types.BoolValue(true),
		AgentVersion:           types.StringValue("v1.20.0"),
		OnboardManifestVersion: types.StringValue("v1.20.1"),
		NeedUpgrade:            types.BoolValue(true),
	})
	if stateDiags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", stateDiags)
	}

	resp := &resource.ModifyPlanResponse{
		Plan: tfsdk.Plan{Schema: Schema(ctx)},
	}
	(&Cluster{client: client}).ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:  plan,
		State: state,
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan() diagnostics = %v", resp.Diagnostics)
	}

	var got ClusterModel
	getDiags := resp.Plan.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.Plan.Get() diagnostics = %v", getDiags)
	}
	if !got.AgentVersion.IsUnknown() {
		t.Fatalf("AgentVersion = %#v, want unknown", got.AgentVersion)
	}
	if !got.OnboardManifestVersion.IsUnknown() {
		t.Fatalf("OnboardManifestVersion = %#v, want unknown", got.OnboardManifestVersion)
	}
	if !got.NeedUpgrade.IsUnknown() {
		t.Fatalf("NeedUpgrade = %#v, want unknown", got.NeedUpgrade)
	}
}

func TestModifyPlanSkipsRefreshWhenClusterNotFound(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterSummaryClient{
		err: cloudpilotaiclient.ErrNotFound,
	}

	plan := tfsdk.Plan{Schema: Schema(ctx)}
	planDiags := plan.Set(ctx, &ClusterModel{
		ClusterID:              types.StringUnknown(),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringNull(),
		AgentVersion:           types.StringValue("planned-agent"),
		OnboardManifestVersion: types.StringValue("planned-manifest"),
		NeedUpgrade:            types.BoolValue(true),
	})
	if planDiags.HasError() {
		t.Fatalf("plan.Set() diagnostics = %v", planDiags)
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	stateDiags := state.Set(ctx, &ClusterModel{
		ClusterID:              types.StringValue("server-imported-id"),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringValue("123456789012"),
		AgentVersion:           types.StringValue("state-agent"),
		OnboardManifestVersion: types.StringValue("state-manifest"),
		NeedUpgrade:            types.BoolValue(false),
	})
	if stateDiags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", stateDiags)
	}

	resp := &resource.ModifyPlanResponse{
		Plan: plan,
	}
	(&Cluster{client: client}).ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:  plan,
		State: state,
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan() diagnostics = %v", resp.Diagnostics)
	}
	if client.getClusterCalls != 1 {
		t.Fatalf("GetCluster calls = %d, want 1", client.getClusterCalls)
	}

	var got ClusterModel
	getDiags := resp.Plan.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.Plan.Get() diagnostics = %v", getDiags)
	}
	if got.AgentVersion != types.StringValue("planned-agent") {
		t.Fatalf("AgentVersion = %#v, want planned-agent", got.AgentVersion)
	}
	if got.OnboardManifestVersion != types.StringValue("planned-manifest") {
		t.Fatalf("OnboardManifestVersion = %#v, want planned-manifest", got.OnboardManifestVersion)
	}
	if got.NeedUpgrade != types.BoolValue(true) {
		t.Fatalf("NeedUpgrade = %#v, want true", got.NeedUpgrade)
	}
}

func TestReadRefreshesUpgradeStatusFromRemoteSummary(t *testing.T) {
	ctx := context.Background()
	client := &fakeReadClusterClient{
		summary: &api.ClusterCostsSummary{
			AgentVersion:           "v1.18.6",
			OnboardManifestVersion: "v1.18.7",
			NeedUpgrade:            true,
		},
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	diags := state.Set(ctx, &ClusterModel{
		ClusterID:              types.StringValue("cluster-1"),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringValue("123456789012"),
		Kubeconfig:             types.StringValue("/tmp/kubeconfig"),
		AgentVersion:           types.StringValue("old-agent"),
		OnboardManifestVersion: types.StringValue("old-manifest"),
		NeedUpgrade:            types.BoolValue(false),
	})
	if diags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", diags)
	}

	resp := &resource.ReadResponse{
		State: tfsdk.State{Schema: Schema(ctx)},
	}
	(&Cluster{client: client}).Read(ctx, resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics = %v", resp.Diagnostics)
	}
	if client.getClusterCalls != 1 {
		t.Fatalf("GetCluster calls = %d, want 1", client.getClusterCalls)
	}

	var got ClusterModel
	getDiags := resp.State.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.State.Get() diagnostics = %v", getDiags)
	}
	if got.AgentVersion != types.StringValue("v1.18.6") {
		t.Fatalf("AgentVersion = %#v, want v1.18.6", got.AgentVersion)
	}
	if got.OnboardManifestVersion != types.StringValue("v1.18.7") {
		t.Fatalf("OnboardManifestVersion = %#v, want v1.18.7", got.OnboardManifestVersion)
	}
	if got.NeedUpgrade != types.BoolValue(true) {
		t.Fatalf("NeedUpgrade = %#v, want true", got.NeedUpgrade)
	}
}

func TestReadKeepsStateWhenClusterNotFound(t *testing.T) {
	ctx := context.Background()
	client := &fakeReadClusterClient{
		getClusterErr: cloudpilotaiclient.ErrNotFound,
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	diags := state.Set(ctx, &ClusterModel{
		ClusterID:              types.StringValue("cluster-1"),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringValue("123456789012"),
		Kubeconfig:             types.StringValue("/tmp/kubeconfig"),
		AgentVersion:           types.StringValue("state-agent"),
		OnboardManifestVersion: types.StringValue("state-manifest"),
		NeedUpgrade:            types.BoolValue(true),
	})
	if diags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", diags)
	}

	resp := &resource.ReadResponse{
		State: tfsdk.State{Schema: Schema(ctx)},
	}
	(&Cluster{client: client}).Read(ctx, resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics = %v", resp.Diagnostics)
	}
	if client.getClusterCalls != 1 {
		t.Fatalf("GetCluster calls = %d, want 1", client.getClusterCalls)
	}

	var got ClusterModel
	getDiags := resp.State.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.State.Get() diagnostics = %v", getDiags)
	}
	if got.ClusterID != types.StringValue("cluster-1") {
		t.Fatalf("ClusterID = %#v, want cluster-1", got.ClusterID)
	}
	if got.AgentVersion != types.StringValue("state-agent") {
		t.Fatalf("AgentVersion = %#v, want state-agent", got.AgentVersion)
	}
	if got.OnboardManifestVersion != types.StringValue("state-manifest") {
		t.Fatalf("OnboardManifestVersion = %#v, want state-manifest", got.OnboardManifestVersion)
	}
	if got.NeedUpgrade != types.BoolValue(true) {
		t.Fatalf("NeedUpgrade = %#v, want true", got.NeedUpgrade)
	}
}

func TestUpgradeStatusSchemaModifiersPreservePriorState(t *testing.T) {
	s := Schema(context.Background())

	needUpgradeAttr, ok := s.Attributes["need_upgrade"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("need_upgrade attribute has unexpected type %T", s.Attributes["need_upgrade"])
	}
	boolModifiers := needUpgradeAttr.BoolPlanModifiers()
	if len(boolModifiers) == 0 {
		t.Fatalf("need_upgrade should have a bool plan modifier")
	}

	boolResp := &schemaplanmodifier.BoolResponse{
		PlanValue: types.BoolUnknown(),
	}
	boolModifiers[0].PlanModifyBool(context.Background(), schemaplanmodifier.BoolRequest{
		State: tfsdk.State{
			Raw: tftypes.NewValue(
				tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"need_upgrade": tftypes.Bool,
					},
				},
				map[string]tftypes.Value{
					"need_upgrade": tftypes.NewValue(tftypes.Bool, true),
				},
			),
		},
		ConfigValue: types.BoolNull(),
		StateValue:  types.BoolValue(true),
		PlanValue:   types.BoolUnknown(),
	}, boolResp)
	if boolResp.PlanValue != types.BoolValue(true) {
		t.Fatalf("need_upgrade PlanValue = %#v, want prior state true", boolResp.PlanValue)
	}

	stringTests := []struct {
		name  string
		value string
	}{
		{name: "agent_version", value: "v1.2.3"},
		{name: "onboard_manifest_version", value: "manifest-2026-06-15"},
	}

	for _, tt := range stringTests {
		t.Run(tt.name, func(t *testing.T) {
			attr, ok := s.Attributes[tt.name].(schema.StringAttribute)
			if !ok {
				t.Fatalf("%s attribute has unexpected type %T", tt.name, s.Attributes[tt.name])
			}
			modifiers := attr.StringPlanModifiers()
			if len(modifiers) == 0 {
				t.Fatalf("%s should have a string plan modifier", tt.name)
			}

			resp := &schemaplanmodifier.StringResponse{
				PlanValue: types.StringUnknown(),
			}
			modifiers[0].PlanModifyString(context.Background(), schemaplanmodifier.StringRequest{
				State: tfsdk.State{
					Raw: tftypes.NewValue(
						tftypes.Object{
							AttributeTypes: map[string]tftypes.Type{
								tt.name: tftypes.String,
							},
						},
						map[string]tftypes.Value{
							tt.name: tftypes.NewValue(tftypes.String, tt.value),
						},
					),
				},
				ConfigValue: types.StringNull(),
				StateValue:  types.StringValue(tt.value),
				PlanValue:   types.StringUnknown(),
			}, resp)

			if resp.PlanValue != types.StringValue(tt.value) {
				t.Fatalf("%s PlanValue = %#v, want prior state %q", tt.name, resp.PlanValue, tt.value)
			}
		})
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
	kubeconfigAttr, ok := Schema(context.Background()).Attributes["kubeconfig"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("kubeconfig attribute has unexpected type %T", Schema(context.Background()).Attributes["kubeconfig"])
	}
	if !kubeconfigAttr.IsOptional() || kubeconfigAttr.IsComputed() {
		t.Fatalf("kubeconfig must be optional-only, got optional=%v computed=%v", kubeconfigAttr.IsOptional(), kubeconfigAttr.IsComputed())
	}
	if len(kubeconfigAttr.StringPlanModifiers()) != 0 {
		t.Fatal("kubeconfig must not preserve an execution-local path from prior state")
	}

	tests := []string{"account_id"}

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
		NodeDisruptionBudgets: customfield.NewObjectListMust(ctx, []api.DisruptionBudgetModel{{
			Nodes: types.StringValue("10%"),
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
	if !got.NodeDisruptionBudgets.IsNull() {
		t.Fatalf("NodeDisruptionBudgets should remain null when budgets are omitted from state")
	}
}

func TestPreserveNodePoolStateRepresentationKeepsBudgetDurationRepresentation(t *testing.T) {
	ctx := context.Background()
	state := api.EC2NodePoolModel{
		NodeDisruptionBudgets: customfield.NewObjectListMust(ctx, []api.DisruptionBudgetModel{{
			Nodes:    types.StringValue("10%"),
			Duration: types.StringValue("60m"),
		}}),
	}
	remote := api.EC2NodePoolModel{
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
