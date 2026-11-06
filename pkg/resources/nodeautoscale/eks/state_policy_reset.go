package eks

import (
	"context"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/statevalue"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func preserveClearedDuration(state, remote types.String) types.String {
	if !state.IsNull() && !state.IsUnknown() && state.ValueString() == "" && remote.IsNull() {
		return state
	}
	return preserveManagedDuration(state, remote)
}

func preserveResetString(state, remote types.String, serverDefault string) types.String {
	if !state.IsNull() && !state.IsUnknown() && state.ValueString() == "" &&
		(remote.IsNull() || (!remote.IsUnknown() && remote.ValueString() == serverDefault)) {
		return state
	}
	return preserveManagedString(state, remote)
}

func jobOptimizationClearsLastField(ctx context.Context, state customfield.NestedObject[api.JobNodePoolOptimizationModel]) bool {
	if state.IsNull() || state.IsUnknown() {
		return false
	}
	value, diags := state.Value(ctx)
	return !diags.HasError() && value != nil && value.MinActiveNodes.IsNull() &&
		!value.ProvisioningDelay.IsNull() && !value.ProvisioningDelay.IsUnknown() && value.ProvisioningDelay.ValueString() == ""
}

func memoryExtensionDisabled(ctx context.Context, remote customfield.NestedObject[api.MemoryExtensionModel]) bool {
	if remote.IsNull() || remote.IsUnknown() {
		return false
	}
	value, diags := remote.Value(ctx)
	return !diags.HasError() && value != nil && !value.Enabled.IsNull() && !value.Enabled.IsUnknown() && !value.Enabled.ValueBool() &&
		value.ReclaimPolicy.IsNull() && (value.ColdMemory.IsNull() || api.IsKnownEmptyObject(value.ColdMemory)) &&
		(value.CachedMemory.IsNull() || api.IsKnownEmptyObject(value.CachedMemory))
}

func preserveNRISwapPolicy(ctx context.Context, remote, state customfield.NestedObject[api.NRISwapPolicyModel]) customfield.NestedObject[api.NRISwapPolicyModel] {
	if api.IsKnownEmptyObject(state) {
		if remote.IsNull() {
			return state
		}
		if !remote.IsUnknown() {
			value, diags := remote.Value(ctx)
			if !diags.HasError() && value != nil && !value.Enabled.IsNull() && !value.Enabled.IsUnknown() && !value.Enabled.ValueBool() &&
				(value.MemoryLimitMultiplier.IsNull() || (!value.MemoryLimitMultiplier.IsUnknown() && value.MemoryLimitMultiplier.ValueFloat64() == 0)) {
				return state
			}
		}
		return remote
	}
	return preservePolicyObject(ctx, remote, state, func(remote, state api.NRISwapPolicyModel) api.NRISwapPolicyModel {
		remote.Enabled = preserveManagedBool(state.Enabled, remote.Enabled)
		if state.MemoryLimitMultiplier.IsNull() {
			remote.MemoryLimitMultiplier = types.Float64Null()
		}
		return remote
	})
}

func preserveEBS(ctx context.Context, remote, state customfield.NestedObject[api.BlockDeviceModel]) customfield.NestedObject[api.BlockDeviceModel] {
	if api.IsKnownEmptyObject(state) && (remote.IsNull() || api.IsKnownEmptyObject(remote)) {
		return state
	}
	return preservePolicyObject(ctx, remote, state, func(remote, state api.BlockDeviceModel) api.BlockDeviceModel {
		remote.Encrypted = preserveManagedBool(state.Encrypted, remote.Encrypted)
		remote.VolumeSize = statevalue.PreserveQuantity(state.VolumeSize, preserveResetString(state.VolumeSize, remote.VolumeSize, ""))
		remote.VolumeType = preserveResetString(state.VolumeType, remote.VolumeType, "")
		remote.VolumeSizePolicy = preserveVolumeSizePolicy(ctx, remote.VolumeSizePolicy, state.VolumeSizePolicy)
		return remote
	})
}

func preserveColdMemory(ctx context.Context, remote, state customfield.NestedObject[api.ColdMemoryModel]) customfield.NestedObject[api.ColdMemoryModel] {
	if remote.IsNull() && coldMemoryClearsLastField(ctx, state) {
		return state
	}
	if api.IsKnownEmptyObject(state) {
		if remote.IsNull() {
			return state
		}
		if !remote.IsUnknown() {
			value, diags := remote.Value(ctx)
			if !diags.HasError() && value != nil && coldMemoryUsesDefaults(*value) {
				return state
			}
		}
		return remote
	}
	return preservePolicyObject(ctx, remote, state, func(remote, state api.ColdMemoryModel) api.ColdMemoryModel {
		remote.SizingMode = preserveResetString(state.SizingMode, remote.SizingMode, "Auto")
		remote.MemoryRatioPercent = preserveManagedInt32(state.MemoryRatioPercent, remote.MemoryRatioPercent)
		remote.MinGiB = preserveManagedInt32(state.MinGiB, remote.MinGiB)
		remote.MaxGiB = preserveManagedInt32(state.MaxGiB, remote.MaxGiB)
		remote.MaxDiskPercent = preserveManagedInt32(state.MaxDiskPercent, remote.MaxDiskPercent)
		return remote
	})
}

func coldMemoryUsesDefaults(value api.ColdMemoryModel) bool {
	return (value.SizingMode.IsNull() || (!value.SizingMode.IsUnknown() && value.SizingMode.ValueString() == "Auto")) &&
		int32IsNullOr(value.MemoryRatioPercent, 30) && int32IsNullOr(value.MinGiB, 2) &&
		int32IsNullOr(value.MaxGiB, 16) && int32IsNullOr(value.MaxDiskPercent, 20)
}

func int32IsNullOr(value types.Int32, expected int32) bool {
	return value.IsNull() || (!value.IsUnknown() && value.ValueInt32() == expected)
}

func preserveCachedMemory(ctx context.Context, remote, state customfield.NestedObject[api.CachedMemoryModel]) customfield.NestedObject[api.CachedMemoryModel] {
	if api.IsKnownEmptyObject(state) {
		if remote.IsNull() {
			return state
		}
		if !remote.IsUnknown() {
			value, diags := remote.Value(ctx)
			if !diags.HasError() && value != nil && int32IsNullOr(value.MaxMemoryPercent, 20) {
				return state
			}
		}
		return remote
	}
	return preservePolicyObject(ctx, remote, state, func(remote, state api.CachedMemoryModel) api.CachedMemoryModel {
		remote.MaxMemoryPercent = preserveManagedInt32(state.MaxMemoryPercent, remote.MaxMemoryPercent)
		return remote
	})
}

func coldMemoryClearsLastField(ctx context.Context, state customfield.NestedObject[api.ColdMemoryModel]) bool {
	if state.IsNull() || state.IsUnknown() {
		return false
	}
	value, diags := state.Value(ctx)
	return !diags.HasError() && value != nil && !value.SizingMode.IsNull() && !value.SizingMode.IsUnknown() &&
		value.SizingMode.ValueString() == "" && value.MemoryRatioPercent.IsNull() && value.MinGiB.IsNull() &&
		value.MaxGiB.IsNull() && value.MaxDiskPercent.IsNull()
}
