package api

import (
	"context"
	"fmt"

	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ColdMemoryPolicy struct {
	SizingMode         *string `json:"sizingMode,omitempty"`
	MemoryRatioPercent *int32  `json:"memoryRatioPercent,omitempty"`
	MinGiB             *int32  `json:"minGiB,omitempty"`
	MaxGiB             *int32  `json:"maxGiB,omitempty"`
	MaxDiskPercent     *int32  `json:"maxDiskPercent,omitempty"`
}

type CachedMemoryPolicy struct {
	MaxMemoryPercent *int32 `json:"maxMemoryPercent,omitempty"`
}

type MemoryExtensionPolicy struct {
	Enabled       bool               `json:"enabled"`
	ColdMemory    ColdMemoryPolicy   `json:"coldMemory,omitempty"`
	CachedMemory  CachedMemoryPolicy `json:"cachedMemory,omitempty"`
	ReclaimPolicy *string            `json:"reclaimPolicy,omitempty"`
}

type ColdMemoryModel struct {
	SizingMode         types.String `tfsdk:"sizing_mode"`
	MemoryRatioPercent types.Int32  `tfsdk:"memory_ratio_percent"`
	MinGiB             types.Int32  `tfsdk:"min_gib"`
	MaxGiB             types.Int32  `tfsdk:"max_gib"`
	MaxDiskPercent     types.Int32  `tfsdk:"max_disk_percent"`
}

type CachedMemoryModel struct {
	MaxMemoryPercent types.Int32 `tfsdk:"max_memory_percent"`
}

type MemoryExtensionModel struct {
	Enabled       types.Bool                                  `tfsdk:"enabled"`
	ColdMemory    customfield.NestedObject[ColdMemoryModel]   `tfsdk:"cold_memory"`
	CachedMemory  customfield.NestedObject[CachedMemoryModel] `tfsdk:"cached_memory"`
	ReclaimPolicy types.String                                `tfsdk:"reclaim_policy"`
}

func optionalInt32(value *int32) types.Int32 {
	if value == nil {
		return types.Int32Null()
	}
	return types.Int32Value(*value)
}

func optionalString(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func memoryExtensionModelFromAPI(ctx context.Context, in *MemoryExtensionPolicy) customfield.NestedObject[MemoryExtensionModel] {
	if in == nil {
		return customfield.NullObject[MemoryExtensionModel](ctx)
	}
	out := MemoryExtensionModel{Enabled: types.BoolValue(in.Enabled), ReclaimPolicy: optionalString(in.ReclaimPolicy), ColdMemory: customfield.NullObject[ColdMemoryModel](ctx), CachedMemory: customfield.NullObject[CachedMemoryModel](ctx)}
	cold := in.ColdMemory
	if cold.SizingMode != nil || cold.MemoryRatioPercent != nil || cold.MinGiB != nil || cold.MaxGiB != nil || cold.MaxDiskPercent != nil {
		out.ColdMemory = customfield.NewObjectMust(ctx, &ColdMemoryModel{SizingMode: optionalString(cold.SizingMode), MemoryRatioPercent: optionalInt32(cold.MemoryRatioPercent), MinGiB: optionalInt32(cold.MinGiB), MaxGiB: optionalInt32(cold.MaxGiB), MaxDiskPercent: optionalInt32(cold.MaxDiskPercent)})
	}
	if in.CachedMemory.MaxMemoryPercent != nil {
		out.CachedMemory = customfield.NewObjectMust(ctx, &CachedMemoryModel{MaxMemoryPercent: optionalInt32(in.CachedMemory.MaxMemoryPercent)})
	}
	return customfield.NewObjectMust(ctx, &out)
}

func (m MemoryExtensionModel) ToAPI(ctx context.Context, current *MemoryExtensionPolicy) (*MemoryExtensionPolicy, error) {
	if IsKnownEmptyObject(customfield.NewObjectMust(ctx, &m)) {
		return &MemoryExtensionPolicy{Enabled: false}, nil
	}
	out := MemoryExtensionPolicy{}
	if current != nil {
		out = *current
	}
	if !m.Enabled.IsNull() && !m.Enabled.IsUnknown() {
		out.Enabled = m.Enabled.ValueBool()
	}
	if !m.ReclaimPolicy.IsNull() && !m.ReclaimPolicy.IsUnknown() {
		v := m.ReclaimPolicy.ValueString()
		out.ReclaimPolicy = nil
		if v != "" {
			out.ReclaimPolicy = &v
		}
	}
	if !m.ColdMemory.IsNull() && !m.ColdMemory.IsUnknown() {
		cold, diags := m.ColdMemory.Value(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("memory_extension.cold_memory: %v", diags)
		}
		if IsKnownEmptyObject(m.ColdMemory) {
			out.ColdMemory = ColdMemoryPolicy{}
		} else if cold != nil {
			applyColdMemoryModel(&out.ColdMemory, *cold)
		}
	}
	if !m.CachedMemory.IsNull() && !m.CachedMemory.IsUnknown() {
		cached, diags := m.CachedMemory.Value(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("memory_extension.cached_memory: %v", diags)
		}
		if IsKnownEmptyObject(m.CachedMemory) {
			out.CachedMemory = CachedMemoryPolicy{}
		} else if cached != nil {
			setOptionalInt32(&out.CachedMemory.MaxMemoryPercent, cached.MaxMemoryPercent)
		}
	}
	hasReset := memoryExtensionHasReset(ctx, m)
	if !hasReset && current == nil && (m.Enabled.IsNull() || m.Enabled.IsUnknown()) && out.ReclaimPolicy == nil &&
		out.ColdMemory == (ColdMemoryPolicy{}) && out.CachedMemory.MaxMemoryPercent == nil {
		return nil, nil
	}
	if err := validateMemoryExtensionPolicy(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func setOptionalInt32(out **int32, value types.Int32) {
	if !value.IsNull() && !value.IsUnknown() {
		v := value.ValueInt32()
		*out = &v
	}
}

func applyColdMemoryModel(out *ColdMemoryPolicy, model ColdMemoryModel) {
	if !model.SizingMode.IsNull() && !model.SizingMode.IsUnknown() {
		v := model.SizingMode.ValueString()
		out.SizingMode = nil
		if v != "" {
			out.SizingMode = &v
		}
	}
	setOptionalInt32(&out.MemoryRatioPercent, model.MemoryRatioPercent)
	setOptionalInt32(&out.MinGiB, model.MinGiB)
	setOptionalInt32(&out.MaxGiB, model.MaxGiB)
	setOptionalInt32(&out.MaxDiskPercent, model.MaxDiskPercent)
}

func int32OrDefault(value *int32, fallback int32) int32 {
	if value == nil {
		return fallback
	}
	return *value
}

func validateMemoryExtensionPolicy(policy *MemoryExtensionPolicy) error {
	if policy == nil || !policy.Enabled {
		return nil
	}
	cold := policy.ColdMemory
	if cold.SizingMode != nil && *cold.SizingMode != "Auto" {
		return fmt.Errorf("memory_extension.cold_memory.sizing_mode must be Auto")
	}
	if policy.ReclaimPolicy != nil && *policy.ReclaimPolicy != "Conservative" {
		return fmt.Errorf("memory_extension.reclaim_policy must be Conservative")
	}
	min, max := int32OrDefault(cold.MinGiB, 2), int32OrDefault(cold.MaxGiB, 16)
	if min <= 0 || max <= 0 || min > max || max > 64 {
		return fmt.Errorf("memory_extension cold memory requires 1 <= min_gib <= max_gib <= 64")
	}
	for _, check := range []struct {
		name          string
		value         *int32
		fallback, max int32
	}{
		{"cold_memory.memory_ratio_percent", cold.MemoryRatioPercent, 30, 100},
		{"cold_memory.max_disk_percent", cold.MaxDiskPercent, 20, 50},
		{"cached_memory.max_memory_percent", policy.CachedMemory.MaxMemoryPercent, 20, 40},
	} {
		value := int32OrDefault(check.value, check.fallback)
		if value < 1 || value > check.max {
			return fmt.Errorf("memory_extension.%s must be between 1 and %d", check.name, check.max)
		}
	}
	return nil
}

func memoryExtensionHasReset(ctx context.Context, model MemoryExtensionModel) bool {
	if IsKnownEmptyObject(model.ColdMemory) || IsKnownEmptyObject(model.CachedMemory) ||
		(!model.ReclaimPolicy.IsNull() && !model.ReclaimPolicy.IsUnknown() && model.ReclaimPolicy.ValueString() == "") {
		return true
	}
	if model.ColdMemory.IsNull() || model.ColdMemory.IsUnknown() {
		return false
	}
	cold, diags := model.ColdMemory.Value(ctx)
	return !diags.HasError() && cold != nil && !cold.SizingMode.IsNull() && !cold.SizingMode.IsUnknown() && cold.SizingMode.ValueString() == ""
}
