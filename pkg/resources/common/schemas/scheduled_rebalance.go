package schemas

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func ScheduledRebalancesSchema(ctx context.Context) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description: "Scheduled rebalance policies managed by Terraform. Omit this attribute to leave all server policies unmanaged; set it to an empty list to remove only policies previously managed by this resource.",
		Optional:    true,
		CustomType:  customfield.NewNestedObjectListType[api.ScheduledRebalancePolicyModel](ctx),
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"name":     schema.StringAttribute{Required: true, Description: "Unique policy name."},
			"cron":     schema.StringAttribute{Optional: true, Description: "Five-field cron expression. Required for a new policy; when omitted for an existing policy, Terraform preserves the server value."},
			"timezone": schema.StringAttribute{Optional: true, Description: "IANA timezone. When omitted, Terraform preserves the server value."},
			"enabled":  schema.BoolAttribute{Optional: true, Description: "Whether the policy is enabled. When omitted, Terraform preserves the server value."},
			"selection_order": schema.StringAttribute{
				Optional:    true,
				Description: "Node selection order: oldest_first, newest_first, name_asc, or lowest_utilization_first.",
				Validators: []validator.String{stringvalidator.OneOf(
					api.ScheduledRebalanceSelectionOrderOldestFirst,
					api.ScheduledRebalanceSelectionOrderNewestFirst,
					api.ScheduledRebalanceSelectionOrderNameAsc,
					api.ScheduledRebalanceSelectionOrderLowestUtilizationFirst,
				)},
			},
			"force_drain": schema.BoolAttribute{Optional: true, Description: "Whether to force drain selected nodes."},
			"scope": schema.SingleNestedAttribute{
				Optional:   true,
				CustomType: customfield.NewNestedObjectType[api.ScheduledRebalanceScopeModel](ctx),
				Attributes: map[string]schema.Attribute{
					"node_pool_name":              schema.StringAttribute{Optional: true},
					"node_names":                  scheduledStringListSchema(ctx),
					"node_selector_terms":         scheduledSelectorTermsSchema(ctx),
					"exclude_node_names":          scheduledStringListSchema(ctx),
					"exclude_node_selector_terms": scheduledSelectorTermsSchema(ctx),
					"capacity_types":              scheduledStringListSchema(ctx),
				},
			},
			"node_constraints": schema.SingleNestedAttribute{
				Optional:   true,
				CustomType: customfield.NewNestedObjectType[api.ScheduledRebalanceNodeConstraintsModel](ctx),
				Attributes: map[string]schema.Attribute{
					"min_age_seconds":  schema.Int64Attribute{Optional: true},
					"max_nodes":        schema.Int64Attribute{Optional: true},
					"min_cluster_size": schema.Int64Attribute{Optional: true},
				},
			},
		}},
	}
}

func scheduledStringListSchema(ctx context.Context) schema.ListAttribute {
	return schema.ListAttribute{Optional: true, ElementType: types.StringType, CustomType: customfield.NewListType[types.String](ctx)}
}

func scheduledSelectorTermsSchema(ctx context.Context) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional:   true,
		CustomType: customfield.NewNestedObjectListType[api.ScheduledRebalanceNodeSelectorTermModel](ctx),
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"match_expressions": schema.ListNestedAttribute{
				Optional:   true,
				CustomType: customfield.NewNestedObjectListType[api.ScheduledRebalanceLabelSelectorRequirementModel](ctx),
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"key":      schema.StringAttribute{Required: true},
					"operator": schema.StringAttribute{Required: true},
					"values":   scheduledStringListSchema(ctx),
				}},
			},
		}},
	}
}
