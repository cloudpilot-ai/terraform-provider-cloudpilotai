package gke

import (
	"context"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func preserveEmptyPolicyReset[T any](ctx context.Context, remote, state customfield.NestedObject[T]) (customfield.NestedObject[T], bool) {
	if !api.IsKnownEmptyObject(state) {
		return remote, false
	}
	if remote.IsNull() || api.IsKnownEmptyObject(remote) {
		return state, true
	}
	return remote, true
}

func preserveResetDuration(state, remote types.String) types.String {
	if !state.IsNull() && !state.IsUnknown() && state.ValueString() == "" && remote.IsNull() {
		return state
	}
	return preserveManagedDuration(state, remote)
}

func isJobDelayReset(ctx context.Context, state customfield.NestedObject[api.JobNodePoolOptimizationModel]) bool {
	if state.IsNull() || state.IsUnknown() {
		return false
	}
	value, diags := state.Value(ctx)
	return !diags.HasError() && value != nil && value.MinActiveNodes.IsNull() &&
		!value.ProvisioningDelay.IsNull() && !value.ProvisioningDelay.IsUnknown() && value.ProvisioningDelay.ValueString() == ""
}

func preserveEmptyLocalSSDReset(ctx context.Context, remote, state customfield.NestedObject[api.GCEEphemeralStorageLocalSSDModel]) customfield.NestedObject[api.GCEEphemeralStorageLocalSSDModel] {
	if remote.IsNull() {
		return state
	}
	value, diags := remote.Value(ctx)
	if !diags.HasError() && value != nil && value.Count.IsNull() && !value.AutoCount.IsUnknown() && !value.AutoCount.ValueBool() {
		return state
	}
	return remote
}
