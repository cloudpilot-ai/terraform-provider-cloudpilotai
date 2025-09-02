// Package schemas provides common schema definitions for CloudPilot AI Terraform provider resources.
package schemas

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/samber/lo"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func WorkloadSchema(ctx context.Context) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description: "Workloads configuration (no change if not set)",
		Optional:    true,
		CustomType:  customfield.NewNestedObjectListType[api.WorkloadModel](ctx),
		NestedObject: schema.NestedAttributeObject{
			Attributes: lo.Assign(map[string]schema.Attribute{
				"name": schema.StringAttribute{
					Description: "Name",
					Required:    true,
				},
				"type": schema.StringAttribute{
					Description: "Type",
					Required:    true,
				},
				"namespace": schema.StringAttribute{
					Description: "Namespace",
					Required:    true,
				},

				"template_name": schema.StringAttribute{
					Description: "Workload Template Name",
					Optional:    true,
				},
			}, workloadTemplateSchema()),
		},
	}
}

func WorkloadTemplateSchema(ctx context.Context) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description: "Workload templates configuration (no change if not set)",
		Optional:    true,
		CustomType:  customfield.NewNestedObjectListType[api.WorkloadTemplateModel](ctx),
		NestedObject: schema.NestedAttributeObject{
			Attributes: lo.Assign(map[string]schema.Attribute{
				"template_name": schema.StringAttribute{
					Description: "Workload Template Name",
					Required:    true,
				},
			}, workloadTemplateSchema()),
		},
	}
}

func workloadTemplateSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"rebalance_able": schema.BoolAttribute{
			Description: "Rebalance able",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(true),
		},
		"spot_friendly": schema.BoolAttribute{
			Description: "Spot friendly",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(true),
		},
		"min_non_spot_replicas": schema.Int64Attribute{
			Description: "Min non spot replicas",
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(0),
		},
	}
}
