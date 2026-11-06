package api

import (
	"context"
	"strings"
	"testing"

	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestEC2NodeClassToModelParsesRestoredKubeReservedQuantities(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cpu       string
		memory    string
		wantCPU   int64
		wantMemMi int64
	}{
		{name: "existing units", cpu: "100m", memory: "256Mi", wantCPU: 100, wantMemMi: 256},
		{name: "restored standard units", cpu: "1", memory: "1Gi", wantCPU: 1000, wantMemMi: 1024},
		{name: "fractional standard units", cpu: "0.125", memory: "0.5Gi", wantCPU: 125, wantMemMi: 512},
		{name: "alternate exact units", cpu: "100000u", memory: "262144Ki", wantCPU: 100, wantMemMi: 256},
		{name: "scientific CPU and byte memory", cpu: "1e-1", memory: "1073741824", wantCPU: 100, wantMemMi: 1024},
		{name: "zero", cpu: "0", memory: "0", wantCPU: 0, wantMemMi: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := reservedQuantityNodeClass(tc.cpu, tc.memory)
			model, err := remote.ToEC2NodeClassModel(context.Background())
			if err != nil {
				t.Fatalf("ToEC2NodeClassModel() error = %v", err)
			}
			if model.ExtraCPUAllocationMCore.ValueInt64() != tc.wantCPU || model.ExtraMemoryAllocationMib.ValueInt64() != tc.wantMemMi {
				t.Fatalf("reservations = (%v mcore, %v MiB), want (%d mcore, %d MiB)", model.ExtraCPUAllocationMCore, model.ExtraMemoryAllocationMib, tc.wantCPU, tc.wantMemMi)
			}
			updated, err := model.ToEC2NodeClass(context.Background(), "illustrative-cluster", *remote, nil)
			if err != nil {
				t.Fatal(err)
			}
			for key, original := range map[string]string{"cpu": tc.cpu, "memory": tc.memory} {
				want := resource.MustParse(original)
				got := resource.MustParse(updated.NodeClassSpec.Kubelet.KubeReserved[key])
				if want.Cmp(got) != 0 {
					t.Errorf("round-tripped %s quantity = %s, want %s", key, got.String(), want.String())
				}
			}
		})
	}
}

func TestEC2NodeClassToModelRejectsUnrepresentableKubeReservedQuantities(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cpu       string
		memory    string
		wantField string
	}{
		{name: "invalid CPU", cpu: "invalid", memory: "256Mi", wantField: "extra_cpu_allocation_mcore"},
		{name: "CPU trailing data", cpu: "100m-invalid", memory: "256Mi", wantField: "extra_cpu_allocation_mcore"},
		{name: "fractional millicore", cpu: "500u", memory: "256Mi", wantField: "extra_cpu_allocation_mcore"},
		{name: "CPU overflow", cpu: "9223372036854775808m", memory: "256Mi", wantField: "extra_cpu_allocation_mcore"},
		{name: "invalid memory", cpu: "100m", memory: "invalid", wantField: "extra_memory_allocation_mib"},
		{name: "fractional MiB", cpu: "100m", memory: "1G", wantField: "extra_memory_allocation_mib"},
		{name: "memory overflow", cpu: "100m", memory: "9223372036854775808", wantField: "extra_memory_allocation_mib"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reservedQuantityNodeClass(tc.cpu, tc.memory).ToEC2NodeClassModel(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.wantField) {
				t.Fatalf("ToEC2NodeClassModel() error = %v, want %s context", err, tc.wantField)
			}
		})
	}
}

func reservedQuantityNodeClass(cpu, memory string) *EC2NodeClass {
	return &EC2NodeClass{
		Name: "illustrative-nodeclass",
		MemoryExtension: &MemoryExtensionPolicy{
			Enabled: false,
		},
		NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{
			Kubelet: &awsproviderv1.KubeletConfiguration{
				KubeReserved: map[string]string{"cpu": cpu, "memory": memory},
			},
		},
	}
}
