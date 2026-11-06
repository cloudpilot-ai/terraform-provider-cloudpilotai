package api

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func limitPolicyFromModel(policy ResourceLimitPolicy, model LimitPolicyModel) (ResourceLimitPolicy, error) {
	selected := 0
	for _, active := range []bool{
		model.RemoveLimit.ValueBool(), model.KeepLimit.ValueBool(),
		model.Multiplier.ValueString() != "", model.AutoHeadroom.ValueString() != "",
	} {
		if active {
			selected++
		}
	}
	if selected > 1 {
		return ResourceLimitPolicy{}, fmt.Errorf("at most one of remove_limit, keep_limit, multiplier, or auto_headroom may be selected")
	}
	if selected == 1 {
		// Selecting a mode replaces inherited actions; null-only updates preserve them.
		policy = ResourceLimitPolicy{}
	}
	applyLimitAction(&policy.RemoveLimit, model.RemoveLimit)
	applyLimitAction(&policy.KeepLimit, model.KeepLimit)
	applyOptionalString(&policy.Multiplier, model.Multiplier)
	applyOptionalString(&policy.AutoHeadroom, model.AutoHeadroom)
	return policy, nil
}

func applyLimitAction(target **bool, value types.Bool) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	// The server CRD permits only true; false disables the action by omitting it.
	*target = nil
	if value.ValueBool() {
		enabled := true
		*target = &enabled
	}
}
