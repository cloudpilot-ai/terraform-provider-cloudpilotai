package api

import (
	"context"
	"reflect"
	"testing"

	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	awscorev1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter/apis/v1"
	"github.com/hashicorp/terraform-plugin-framework/types"
	corev1 "k8s.io/api/core/v1"
)

func awsBoundRequirement(key string, operator corev1.NodeSelectorOperator, value string) awscorev1.NodeSelectorRequirementWithMinValues {
	return awscorev1.NodeSelectorRequirementWithMinValues{NodeSelectorRequirement: corev1.NodeSelectorRequirement{
		Key: key, Operator: operator, Values: []string{value},
	}}
}

func TestEC2NodePoolImportRoundTripPreservesAbsentAndExplicitBounds(t *testing.T) {
	for _, tc := range []struct {
		name         string
		requirements []awscorev1.NodeSelectorRequirementWithMinValues
		cpuMax       types.Int64
		cpuMin       types.Int64
		memoryMax    types.Int64
		memoryMin    types.Int64
	}{
		{
			name:   "unbounded GPU pool",
			cpuMax: types.Int64Null(), cpuMin: types.Int64Null(),
			memoryMax: types.Int64Null(), memoryMin: types.Int64Null(),
		},
		{
			name: "maximums only",
			requirements: []awscorev1.NodeSelectorRequirementWithMinValues{
				awsBoundRequirement(awsproviderv1.LabelInstanceCPU, corev1.NodeSelectorOpLt, "17"),
				awsBoundRequirement(awsproviderv1.LabelInstanceMemory, corev1.NodeSelectorOpLt, "32769"),
			},
			cpuMax: types.Int64Value(17), cpuMin: types.Int64Null(),
			memoryMax: types.Int64Value(32769), memoryMin: types.Int64Null(),
		},
		{
			name: "minimums only",
			requirements: []awscorev1.NodeSelectorRequirementWithMinValues{
				awsBoundRequirement(awsproviderv1.LabelInstanceCPU, corev1.NodeSelectorOpGt, "2"),
				awsBoundRequirement(awsproviderv1.LabelInstanceMemory, corev1.NodeSelectorOpGt, "4096"),
			},
			cpuMax: types.Int64Null(), cpuMin: types.Int64Value(2),
			memoryMax: types.Int64Null(), memoryMin: types.Int64Value(4096),
		},
		{
			name: "explicit zero is not an absent bound",
			requirements: []awscorev1.NodeSelectorRequirementWithMinValues{
				awsBoundRequirement(awsproviderv1.LabelInstanceCPU, corev1.NodeSelectorOpLt, "0"),
				awsBoundRequirement(awsproviderv1.LabelInstanceCPU, corev1.NodeSelectorOpGt, "0"),
				awsBoundRequirement(awsproviderv1.LabelInstanceMemory, corev1.NodeSelectorOpLt, "0"),
				awsBoundRequirement(awsproviderv1.LabelInstanceMemory, corev1.NodeSelectorOpGt, "0"),
			},
			cpuMax: types.Int64Value(0), cpuMin: types.Int64Value(0),
			memoryMax: types.Int64Value(0), memoryMin: types.Int64Value(0),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := EC2NodePool{Name: "cloudpilot-gpu", NodePoolSpec: DefaultGPUEC2NodePoolSpec()}
			remote.NodePoolSpec.Template.Spec.Requirements = append(remote.NodePoolSpec.Template.Spec.Requirements,
				awsBoundRequirement("example.com/untouched", corev1.NodeSelectorOpIn, "keep"))
			remote.NodePoolSpec.Template.Spec.Requirements = append(remote.NodePoolSpec.Template.Spec.Requirements, tc.requirements...)
			original := remote.NodePoolSpec.DeepCopy()
			model, err := remote.ToEC2NodePoolModel()
			if err != nil {
				t.Fatal(err)
			}
			for name, values := range map[string][2]types.Int64{
				"instance_cpu_max":    {model.InstanceCPUMAX, tc.cpuMax},
				"instance_cpu_min":    {model.InstanceCPUMIN, tc.cpuMin},
				"instance_memory_max": {model.InstanceMemoryMAX, tc.memoryMax},
				"instance_memory_min": {model.InstanceMemoryMIN, tc.memoryMin},
			} {
				if !values[0].Equal(values[1]) {
					t.Errorf("imported %s = %s, want %s", name, values[0], values[1])
				}
			}
			updated, err := model.ToEC2NodePool(context.Background(), remote, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(updated.NodePoolSpec.Template.Spec.Requirements, original.Template.Spec.Requirements) {
				t.Fatalf("reapplying imported pool changed requirements:\nbefore: %#v\nafter: %#v", original.Template.Spec.Requirements, updated.NodePoolSpec.Template.Spec.Requirements)
			}
		})
	}
}

func TestEC2NodePoolNullBoundsPreserveConfiguredServerRequirements(t *testing.T) {
	remote := EC2NodePool{Name: "bounded", NodePoolSpec: DefaultGPUEC2NodePoolSpec()}
	remote.NodePoolSpec.Template.Spec.Requirements = append(remote.NodePoolSpec.Template.Spec.Requirements,
		awsBoundRequirement(awsproviderv1.LabelInstanceCPU, corev1.NodeSelectorOpLt, "17"),
		awsBoundRequirement(awsproviderv1.LabelInstanceCPU, corev1.NodeSelectorOpGt, "2"),
		awsBoundRequirement(awsproviderv1.LabelInstanceMemory, corev1.NodeSelectorOpLt, "32769"),
		awsBoundRequirement(awsproviderv1.LabelInstanceMemory, corev1.NodeSelectorOpGt, "4096"),
	)
	original := remote.NodePoolSpec.DeepCopy()
	model := EC2NodePoolModel{
		Name:           types.StringValue("bounded"),
		InstanceCPUMAX: types.Int64Null(), InstanceCPUMIN: types.Int64Null(),
		InstanceMemoryMAX: types.Int64Null(), InstanceMemoryMIN: types.Int64Null(),
	}
	updated, err := model.ToEC2NodePool(context.Background(), remote, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updated.NodePoolSpec.Template.Spec.Requirements, original.Template.Spec.Requirements) {
		t.Fatalf("null bounds changed server requirements: %#v", updated.NodePoolSpec.Template.Spec.Requirements)
	}
}

func TestEC2NodePoolImportRejectsMalformedBounds(t *testing.T) {
	for _, key := range []string{awsproviderv1.LabelInstanceCPU, awsproviderv1.LabelInstanceMemory} {
		for _, operator := range []corev1.NodeSelectorOperator{corev1.NodeSelectorOpLt, corev1.NodeSelectorOpGt} {
			t.Run(key+"/"+string(operator), func(t *testing.T) {
				remote := EC2NodePool{Name: "invalid", NodePoolSpec: DefaultGPUEC2NodePoolSpec()}
				remote.NodePoolSpec.Template.Spec.Requirements = append(remote.NodePoolSpec.Template.Spec.Requirements, awsBoundRequirement(key, operator, "not-an-integer"))
				if _, err := remote.ToEC2NodePoolModel(); err == nil {
					t.Fatal("malformed bound must not become a synthetic zero")
				}
			})
		}
	}
}
