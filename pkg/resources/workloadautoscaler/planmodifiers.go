package workloadautoscaler

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

func useStateForUnknownString() planmodifier.String {
	return useStateForUnknownStringModifier{}
}

type useStateForUnknownStringModifier struct{}

func (m useStateForUnknownStringModifier) Description(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownStringModifier) MarkdownDescription(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
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
