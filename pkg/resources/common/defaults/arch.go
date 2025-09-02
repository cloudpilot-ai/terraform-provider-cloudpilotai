// Package defaults provides default value implementations for Terraform resource schemas.
package defaults

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type archDefaultValue struct{}

func (d archDefaultValue) Description(ctx context.Context) string {
	return "Default architecture: amd64, arm64"
}

func (d archDefaultValue) MarkdownDescription(ctx context.Context) string {
	return "Default architecture: `amd64`, `arm64`"
}

func (d archDefaultValue) DefaultList(ctx context.Context, req defaults.ListRequest, resp *defaults.ListResponse) {
	elements := []attr.Value{
		types.StringValue("amd64"),
		types.StringValue("arm64"),
	}

	resp.PlanValue = types.ListValueMust(types.StringType, elements)
}

func ArchDefault() defaults.List {
	return archDefaultValue{}
}
