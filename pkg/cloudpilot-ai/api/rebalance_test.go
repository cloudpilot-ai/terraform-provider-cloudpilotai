package api

import (
	"context"
	"testing"

	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
)

func TestEC2NodeClassToModelLeavesExtraAllocationsNullWhenKubeReservedKeysMissing(t *testing.T) {
	model, err := (&EC2NodeClass{
		Name: "cloudpilot",
		NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{
			Kubelet: &awsproviderv1.KubeletConfiguration{
				KubeReserved: map[string]string{
					"ephemeral-storage": "1Gi",
				},
			},
		},
	}).ToEC2NodeClassModel(context.Background())
	if err != nil {
		t.Fatalf("ToEC2NodeClassModel() error = %v", err)
	}

	if !model.ExtraCPUAllocationMCore.IsNull() {
		t.Fatalf("ExtraCPUAllocationMCore should be null when kubeReserved.cpu is missing")
	}

	if !model.ExtraMemoryAllocationMib.IsNull() {
		t.Fatalf("ExtraMemoryAllocationMib should be null when kubeReserved.memory is missing")
	}
}

func TestEC2NodePoolToModelLeavesInstanceMinimumsNullWhenRequirementsMissing(t *testing.T) {
	model, err := (&EC2NodePool{
		Name:         "cloudpilot-general",
		NodePoolSpec: DefaultGeneralEC2NodePoolSpec(),
	}).ToEC2NodePoolModel()
	if err != nil {
		t.Fatalf("ToEC2NodePoolModel() error = %v", err)
	}

	if !model.InstanceCPUMIN.IsNull() {
		t.Fatalf("InstanceCPUMIN should be null when instance-cpu Gt requirement is missing")
	}

	if !model.InstanceMemoryMIN.IsNull() {
		t.Fatalf("InstanceMemoryMIN should be null when instance-memory Gt requirement is missing")
	}
}
