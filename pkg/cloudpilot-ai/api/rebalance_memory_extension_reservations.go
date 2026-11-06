package api

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ValidateMemoryExtensionReservations rejects explicit reservations that the
// server would replace while Memory Extension is enabled. Later models override
// earlier models, matching the NodeClass template application order.
func ValidateMemoryExtensionReservations(ctx context.Context, current *EC2NodeClass, models ...*EC2NodeClassTemplateModel) error {
	enabled := current != nil && current.MemoryExtension != nil && current.MemoryExtension.Enabled
	enabledKnown := true
	cpu, memory := types.Int64Null(), types.Int64Null()
	for _, model := range models {
		if model == nil {
			continue
		}
		if model.MemoryExtension.IsUnknown() {
			enabledKnown = false
		} else if IsKnownEmptyObject(model.MemoryExtension) {
			enabled, enabledKnown = false, true
		} else if !model.MemoryExtension.IsNull() {
			policy, diags := model.MemoryExtension.Value(ctx)
			if diags.HasError() {
				return fmt.Errorf("memory_extension: %v", diags)
			}
			if policy != nil && !policy.Enabled.IsNull() {
				enabledKnown = !policy.Enabled.IsUnknown()
				if enabledKnown {
					enabled = policy.Enabled.ValueBool()
				}
			}
		}
		if !model.ExtraCPUAllocationMCore.IsNull() {
			cpu = model.ExtraCPUAllocationMCore
		}
		if !model.ExtraMemoryAllocationMib.IsNull() {
			memory = model.ExtraMemoryAllocationMib
		}
	}
	if !enabledKnown {
		return nil
	}
	if !enabled {
		if err := validateRestoredMemoryExtensionReservation(current, cpu, "extra_cpu_allocation_mcore", "m", "cpu"); err != nil {
			return err
		}
		return validateRestoredMemoryExtensionReservation(current, memory, "extra_memory_allocation_mib", "Mi", "memory")
	}
	if !cpu.IsNull() && !cpu.IsUnknown() && cpu.ValueInt64() != 100 {
		return fmt.Errorf("memory_extension requires extra_cpu_allocation_mcore to be omitted or 100 because the server manages kubeReserved CPU while enabled")
	}
	if !memory.IsNull() && !memory.IsUnknown() && memory.ValueInt64() != 256 {
		return fmt.Errorf("memory_extension requires extra_memory_allocation_mib to be omitted or 256 because the server manages kubeReserved memory while enabled")
	}
	return nil
}

func validateRestoredMemoryExtensionReservation(current *EC2NodeClass, configured types.Int64, field, unit, key string) error {
	if current == nil || configured.IsNull() || configured.IsUnknown() {
		return nil
	}
	original, ok := current.NodeClassAnnotation["cloudpilot.ai/memory-extension-original-kube-reserved-"+key]
	if !ok {
		return nil
	}
	want := resource.MustParse(fmt.Sprintf("%d%s", configured.ValueInt64(), unit))
	restored, err := resource.ParseQuantity(original)
	if err == nil && restored.Cmp(want) == 0 {
		return nil
	}
	return fmt.Errorf("disabling memory_extension restores a saved kubeReserved %s value that differs from %s; omit or set %s to null for this apply, then configure it after Memory Extension is disabled", key, field, field)
}
