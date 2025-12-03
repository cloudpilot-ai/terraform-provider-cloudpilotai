package defaults

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// capacityTypeDefaultValue implements defaults.List for capacity_type default values
type capacityTypeDefaultValue struct{}

func (d capacityTypeDefaultValue) Description(ctx context.Context) string {
	return "Default capacity types: on-demand, spot"
}

func (d capacityTypeDefaultValue) MarkdownDescription(ctx context.Context) string {
	return "Default capacity types: `on-demand`, `spot`"
}

func (d capacityTypeDefaultValue) DefaultList(ctx context.Context, req defaults.ListRequest, resp *defaults.ListResponse) {
	elements := []attr.Value{
		types.StringValue("on-demand"),
		types.StringValue("spot"),
	}

	resp.PlanValue = types.ListValueMust(types.StringType, elements)
}

func CapacityTypeDefault() defaults.List {
	return capacityTypeDefaultValue{}
}
