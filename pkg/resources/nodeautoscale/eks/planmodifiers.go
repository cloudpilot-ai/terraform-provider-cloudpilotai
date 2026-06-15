package eks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

func useStateForUnknownInt64() planmodifier.Int64 {
	return useStateForUnknownInt64Modifier{}
}

func useStateForUnknownBool() planmodifier.Bool {
	return useStateForUnknownBoolModifier{}
}

func useStateForUnknownString() planmodifier.String {
	return useStateForUnknownStringModifier{}
}

func useStateForUnknownNonNullString() planmodifier.String {
	return useStateForUnknownNonNullStringModifier{}
}

type useStateForUnknownInt64Modifier struct{}
type useStateForUnknownBoolModifier struct{}
type useStateForUnknownStringModifier struct{}
type useStateForUnknownNonNullStringModifier struct{}

func (m useStateForUnknownInt64Modifier) Description(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownInt64Modifier) MarkdownDescription(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownBoolModifier) Description(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownBoolModifier) MarkdownDescription(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownStringModifier) Description(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownStringModifier) MarkdownDescription(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownNonNullStringModifier) Description(_ context.Context) string {
	return "Preserve the prior non-null state value when the planned value is unknown."
}

func (m useStateForUnknownNonNullStringModifier) MarkdownDescription(_ context.Context) string {
	return "Preserve the prior non-null state value when the planned value is unknown."
}

func (m useStateForUnknownInt64Modifier) PlanModifyInt64(_ context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if req.State.Raw.IsNull() {
		return
	}

	if !req.PlanValue.IsUnknown() {
		return
	}

	if req.ConfigValue.IsUnknown() {
		return
	}

	resp.PlanValue = req.StateValue
}

func (m useStateForUnknownBoolModifier) PlanModifyBool(_ context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if req.State.Raw.IsNull() {
		return
	}

	if !req.PlanValue.IsUnknown() {
		return
	}

	if req.ConfigValue.IsUnknown() {
		return
	}

	resp.PlanValue = req.StateValue
}

func (m useStateForUnknownStringModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() {
		return
	}

	if !req.PlanValue.IsUnknown() {
		return
	}

	if req.ConfigValue.IsUnknown() {
		return
	}

	resp.PlanValue = req.StateValue
}

func (m useStateForUnknownNonNullStringModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() {
		return
	}

	if !req.PlanValue.IsUnknown() {
		return
	}

	if req.ConfigValue.IsUnknown() {
		return
	}

	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}

	resp.PlanValue = req.StateValue
}
