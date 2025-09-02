package validators

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

func CapacityTypeValidators() []validator.List {
	return []validator.List{
		listvalidator.ValueStringsAre(
			stringvalidator.OneOfCaseInsensitive(
				"on-demand",
				"spot",
			),
		),
	}
}
