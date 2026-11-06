package api

import (
	"context"
	"reflect"
	"testing"

	gcpcorev1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter/apis/v1"
	"github.com/hashicorp/terraform-plugin-framework/types"
	corev1 "k8s.io/api/core/v1"
)

func gcePoolBounds(model *GCENodePoolModel) map[string]types.Int64 {
	return map[string]types.Int64{
		"instance_cpu_min":    model.InstanceCPUMIN,
		"instance_cpu_max":    model.InstanceCPUMAX,
		"instance_memory_min": model.InstanceMemoryMIN,
		"instance_memory_max": model.InstanceMemoryMAX,
	}
}

func TestGCENodePoolAbsentBoundsStayNullThroughImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	remote := GCENodePool{Name: "unrestricted", NodePoolSpec: defaultGCPNodePoolSpec()}
	remote.NodePoolSpec.Template.Spec.Requirements = append(remote.NodePoolSpec.Template.Spec.Requirements,
		gcpcorev1.NodeSelectorRequirementWithMinValues{Key: gceLabelInstanceFamily, Operator: corev1.NodeSelectorOpIn, Values: []string{"n4"}},
	)

	model, err := remote.ToGCENodePoolModel(ctx)
	if err != nil {
		t.Fatalf("ToGCENodePoolModel() error = %v", err)
	}
	for field, bound := range gcePoolBounds(model) {
		if !bound.IsNull() {
			t.Errorf("%s = %s, want null for an absent requirement", field, bound)
		}
	}

	imported, err := model.ToGCENodePool(ctx, GCENodePool{Name: "unrestricted"})
	if err != nil {
		t.Fatalf("ToGCENodePool() error = %v", err)
	}
	for _, requirement := range imported.NodePoolSpec.Template.Spec.Requirements {
		if (requirement.Key == gceLabelInstanceCPU || requirement.Key == gceLabelInstanceMemory) &&
			(requirement.Operator == corev1.NodeSelectorOpLt || requirement.Operator == corev1.NodeSelectorOpGt) {
			t.Errorf("import added an unconfigured CPU or memory bound: %#v", requirement)
		}
	}
	if got := gcpRequirementsToStrings(imported.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceFamily, corev1.NodeSelectorOpIn); got == nil || len(*got) != 1 || (*got)[0].ValueString() != "n4" {
		t.Fatalf("import lost the configured instance family: %#v", imported.NodePoolSpec.Template.Spec.Requirements)
	}
}

func TestGCENodePoolReadsConcreteBoundsWithoutConflatingZeroAndNull(t *testing.T) {
	for _, value := range []string{"0", "17"} {
		t.Run(value, func(t *testing.T) {
			remote := GCENodePool{Name: "bounded", NodePoolSpec: defaultGCPNodePoolSpec()}
			for _, key := range []string{gceLabelInstanceCPU, gceLabelInstanceMemory} {
				for _, operator := range []corev1.NodeSelectorOperator{corev1.NodeSelectorOpLt, corev1.NodeSelectorOpGt} {
					remote.NodePoolSpec.Template.Spec.Requirements = append(remote.NodePoolSpec.Template.Spec.Requirements,
						gcpcorev1.NodeSelectorRequirementWithMinValues{Key: key, Operator: operator, Values: []string{value}},
					)
				}
			}
			model, err := remote.ToGCENodePoolModel(context.Background())
			if err != nil {
				t.Fatalf("ToGCENodePoolModel() error = %v", err)
			}
			for field, bound := range gcePoolBounds(model) {
				if bound.IsNull() || bound.IsUnknown() || bound.String() != value {
					t.Errorf("%s = %s, want concrete %s", field, bound, value)
				}
			}
		})
	}
}

func TestGCENodePoolNullBoundsPreserveServerRequirements(t *testing.T) {
	ctx := context.Background()
	remote := GCENodePool{Name: "bounded", NodePoolSpec: defaultGCPNodePoolSpec()}
	for _, key := range []string{gceLabelInstanceCPU, gceLabelInstanceMemory} {
		remote.NodePoolSpec.Template.Spec.Requirements = append(remote.NodePoolSpec.Template.Spec.Requirements,
			gcpcorev1.NodeSelectorRequirementWithMinValues{Key: key, Operator: corev1.NodeSelectorOpGt, Values: []string{"1"}},
			gcpcorev1.NodeSelectorRequirementWithMinValues{Key: key, Operator: corev1.NodeSelectorOpLt, Values: []string{"64"}},
		)
	}
	want := append([]gcpcorev1.NodeSelectorRequirementWithMinValues(nil), remote.NodePoolSpec.Template.Spec.Requirements...)
	model := GCENodePoolModel{
		Name:              types.StringValue("bounded"),
		InstanceCPUMIN:    types.Int64Null(),
		InstanceCPUMAX:    types.Int64Null(),
		InstanceMemoryMIN: types.Int64Null(),
		InstanceMemoryMAX: types.Int64Null(),
	}
	updated, err := model.ToGCENodePool(ctx, remote)
	if err != nil {
		t.Fatalf("ToGCENodePool() error = %v", err)
	}
	if !reflect.DeepEqual(updated.NodePoolSpec.Template.Spec.Requirements, want) {
		t.Fatalf("null bounds changed requirements: got %#v, want %#v", updated.NodePoolSpec.Template.Spec.Requirements, want)
	}
}

func TestGCENodePoolReadsEmptyAndMalformedBoundValues(t *testing.T) {
	for _, key := range []string{gceLabelInstanceCPU, gceLabelInstanceMemory} {
		for _, operator := range []corev1.NodeSelectorOperator{corev1.NodeSelectorOpLt, corev1.NodeSelectorOpGt} {
			t.Run(key+"/"+string(operator), func(t *testing.T) {
				remote := GCENodePool{Name: "bounded", NodePoolSpec: defaultGCPNodePoolSpec()}
				remote.NodePoolSpec.Template.Spec.Requirements = []gcpcorev1.NodeSelectorRequirementWithMinValues{{Key: key, Operator: operator}}
				model, err := remote.ToGCENodePoolModel(context.Background())
				if err != nil {
					t.Fatalf("empty bound values error = %v", err)
				}
				for field, bound := range gcePoolBounds(model) {
					if !bound.IsNull() {
						t.Errorf("%s = %s, want null when no scalar bound value exists", field, bound)
					}
				}
				remote.NodePoolSpec.Template.Spec.Requirements[0].Values = []string{"invalid"}
				if _, err := remote.ToGCENodePoolModel(context.Background()); err == nil {
					t.Fatal("malformed bound value did not return an error")
				}
			})
		}
	}
}
