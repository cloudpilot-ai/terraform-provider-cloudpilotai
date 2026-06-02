package eks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

func useStateForUnknownInt64() planmodifier.Int64 {
	return useStateForUnknownInt64Modifier{}
}

type useStateForUnknownInt64Modifier struct{}

func (m useStateForUnknownInt64Modifier) Description(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownInt64Modifier) MarkdownDescription(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
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
