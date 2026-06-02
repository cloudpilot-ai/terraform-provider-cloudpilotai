package eks

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	schemaplanmodifier "github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type namedItem struct {
	name string
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
