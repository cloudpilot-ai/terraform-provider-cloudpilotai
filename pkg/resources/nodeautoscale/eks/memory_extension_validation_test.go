package eks

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestValidateMemoryExtensionReservationsUsesEffectiveTemplate(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		class   api.EC2NodeClassModel
		wantErr bool
	}{
		{name: "inherited conflict", wantErr: true},
		{name: "null preserves template", class: api.EC2NodeClassModel{ExtraCPUAllocationMCore: types.Int64Null()}, wantErr: true},
		{name: "class matches forced CPU", class: api.EC2NodeClassModel{ExtraCPUAllocationMCore: types.Int64Value(100)}},
		{name: "class CPU unknown", class: api.EC2NodeClassModel{ExtraCPUAllocationMCore: types.Int64Unknown()}},
		{name: "class policy unknown", class: api.EC2NodeClassModel{MemoryExtension: customfield.UnknownObject[api.MemoryExtensionModel](ctx)}},
		{name: "raw JSON unknown", class: api.EC2NodeClassModel{OriginNodeClassJSON: types.StringUnknown()}},
		{name: "raw JSON bypasses typed values", class: api.EC2NodeClassModel{OriginNodeClassJSON: types.StringValue(`{"name":"example"}`)}},
		{name: "class disables policy", class: api.EC2NodeClassModel{MemoryExtension: customfield.NewObjectMust(ctx, &api.MemoryExtensionModel{Enabled: types.BoolValue(false)})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.class.Name = types.StringValue("example")
			tc.class.TemplateName = types.StringValue("template")
			data := ClusterModel{
				NodeClassTemplates: customfield.NewObjectListMust(ctx, []api.EC2NodeClassTemplateModel{{
					TemplateName:            types.StringValue("template"),
					MemoryExtension:         customfield.NewObjectMust(ctx, &api.MemoryExtensionModel{Enabled: types.BoolValue(true)}),
					ExtraCPUAllocationMCore: types.Int64Value(200),
				}}),
				NodeClasses: customfield.NewObjectListMust(ctx, []api.EC2NodeClassModel{tc.class}),
			}
			if err := validateMemoryExtensionReservations(ctx, data); (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error = %v", err, tc.wantErr)
			}
		})
	}
}

func TestModifyPlanRejectsConflictingMemoryExtensionReservations(t *testing.T) {
	ctx := context.Background()
	plan := tfsdk.Plan{Schema: Schema(ctx)}
	diags := plan.Set(ctx, &ClusterModel{
		ClusterName: types.StringValue("example"), Region: types.StringValue("us-east-2"),
		NodeClasses: customfield.NewObjectListMust(ctx, []api.EC2NodeClassModel{{
			Name:                     types.StringValue("example"),
			MemoryExtension:          customfield.NewObjectMust(ctx, &api.MemoryExtensionModel{Enabled: types.BoolValue(true)}),
			ExtraMemoryAllocationMib: types.Int64Value(512),
		}}),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	response := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: Schema(ctx)}}
	(&Cluster{}).ModifyPlan(ctx, resource.ModifyPlanRequest{Plan: plan}, response)
	if !response.Diagnostics.HasError() || !strings.Contains(response.Diagnostics[0].Detail(), "extra_memory_allocation_mib") {
		t.Fatalf("diagnostics = %v, want reservation conflict before remote access", response.Diagnostics)
	}
}
