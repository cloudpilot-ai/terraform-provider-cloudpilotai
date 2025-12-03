package validators

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

func ArchValidators() []validator.List {
	return []validator.List{
		listvalidator.ValueStringsAre(
			stringvalidator.OneOfCaseInsensitive(
				"amd64",
				"arm64",
			),
		),
	}
}
