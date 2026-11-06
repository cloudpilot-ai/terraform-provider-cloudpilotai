package api

import (
	"context"
	"strings"
	"testing"

	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestMemoryExtensionReservationValidation(t *testing.T) {
	ctx := context.Background()
	enabled := customfield.NewObjectMust(ctx, &MemoryExtensionModel{Enabled: types.BoolValue(true)})
	disabled := customfield.NewObjectMust(ctx, &MemoryExtensionModel{Enabled: types.BoolValue(false)})
	for _, tc := range []struct {
		name     string
		current  EC2NodeClass
		template *EC2NodeClassTemplateModel
		model    EC2NodeClassModel
		wantErr  string
	}{
		{name: "conflicting CPU", model: EC2NodeClassModel{MemoryExtension: enabled, ExtraCPUAllocationMCore: types.Int64Value(200)}, wantErr: "extra_cpu_allocation_mcore"},
		{name: "conflicting memory", model: EC2NodeClassModel{MemoryExtension: enabled, ExtraMemoryAllocationMib: types.Int64Value(512)}, wantErr: "extra_memory_allocation_mib"},
		{name: "matching reservations", model: EC2NodeClassModel{MemoryExtension: enabled, ExtraCPUAllocationMCore: types.Int64Value(100), ExtraMemoryAllocationMib: types.Int64Value(256)}},
		{name: "omitted reservations", model: EC2NodeClassModel{MemoryExtension: enabled}},
		{name: "disabled reservations", model: EC2NodeClassModel{MemoryExtension: disabled, ExtraCPUAllocationMCore: types.Int64Value(200), ExtraMemoryAllocationMib: types.Int64Value(512)}},
		{name: "preserved remote enabled", current: EC2NodeClass{MemoryExtension: &MemoryExtensionPolicy{Enabled: true}}, model: EC2NodeClassModel{ExtraCPUAllocationMCore: types.Int64Value(200)}, wantErr: "extra_cpu_allocation_mcore"},
		{name: "inherited conflict", template: &EC2NodeClassTemplateModel{MemoryExtension: enabled, ExtraCPUAllocationMCore: types.Int64Value(200)}, wantErr: "extra_cpu_allocation_mcore"},
		{name: "class overrides conflict", template: &EC2NodeClassTemplateModel{MemoryExtension: enabled, ExtraCPUAllocationMCore: types.Int64Value(200)}, model: EC2NodeClassModel{ExtraCPUAllocationMCore: types.Int64Value(100)}},
		{name: "class disables policy", template: &EC2NodeClassTemplateModel{MemoryExtension: enabled, ExtraCPUAllocationMCore: types.Int64Value(200)}, model: EC2NodeClassModel{MemoryExtension: disabled}},
		{name: "unknown reservation", template: &EC2NodeClassTemplateModel{MemoryExtension: enabled, ExtraCPUAllocationMCore: types.Int64Value(200)}, model: EC2NodeClassModel{ExtraCPUAllocationMCore: types.Int64Unknown()}},
		{name: "unknown enabled", template: &EC2NodeClassTemplateModel{MemoryExtension: enabled, ExtraCPUAllocationMCore: types.Int64Value(200)}, model: EC2NodeClassModel{MemoryExtension: customfield.NewObjectMust(ctx, &MemoryExtensionModel{Enabled: types.BoolUnknown()})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.model.ToEC2NodeClass(ctx, "example", tc.current, tc.template)
			if tc.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestMemoryExtensionReservationDisableValidation(t *testing.T) {
	ctx := context.Background()
	for _, policy := range []struct {
		name  string
		value customfield.NestedObject[MemoryExtensionModel]
	}{
		{name: "disable", value: customfield.NewObjectMust(ctx, &MemoryExtensionModel{Enabled: types.BoolValue(false)})},
		{name: "reset", value: customfield.NewObjectMust(ctx, &MemoryExtensionModel{})},
	} {
		for _, tc := range []struct {
			name    string
			cpu     string
			memory  string
			model   EC2NodeClassModel
			wantErr bool
		}{
			{name: "restored differing CPU", cpu: "200m", model: EC2NodeClassModel{ExtraCPUAllocationMCore: types.Int64Value(100)}, wantErr: true},
			{name: "restored differing memory", memory: "512Mi", model: EC2NodeClassModel{ExtraMemoryAllocationMib: types.Int64Value(256)}, wantErr: true},
			{name: "restored unset", cpu: "<unset>", model: EC2NodeClassModel{ExtraCPUAllocationMCore: types.Int64Value(100)}, wantErr: true},
			{name: "matching restored values", cpu: "200m", memory: "512Mi", model: EC2NodeClassModel{ExtraCPUAllocationMCore: types.Int64Value(200), ExtraMemoryAllocationMib: types.Int64Value(512)}},
			{name: "equivalent restored values", cpu: "1", memory: "1Gi", model: EC2NodeClassModel{ExtraCPUAllocationMCore: types.Int64Value(1000), ExtraMemoryAllocationMib: types.Int64Value(1024)}},
			{name: "omitted reservations", cpu: "200m", memory: "512Mi"},
		} {
			t.Run(policy.name+"/"+tc.name, func(t *testing.T) {
				annotations := map[string]string{}
				if tc.cpu != "" {
					annotations["cloudpilot.ai/memory-extension-original-kube-reserved-cpu"] = tc.cpu
				}
				if tc.memory != "" {
					annotations["cloudpilot.ai/memory-extension-original-kube-reserved-memory"] = tc.memory
				}
				current := EC2NodeClass{MemoryExtension: &MemoryExtensionPolicy{Enabled: true}, NodeClassAnnotation: annotations}
				tc.model.MemoryExtension = policy.value
				_, err := tc.model.ToEC2NodeClass(ctx, "example", current, nil)
				if (err != nil) != tc.wantErr {
					t.Fatalf("error = %v, want error = %v", err, tc.wantErr)
				}
			})
		}
	}
}
