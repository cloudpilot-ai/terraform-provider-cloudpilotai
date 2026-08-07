package schemas

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	commonvalidators "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/validators"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func ScheduledRebalancesSchema(ctx context.Context) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description: "Scheduled rebalance policies managed by Terraform. Omit this attribute to leave all server policies unmanaged; set it to an empty list to remove only policies previously managed by this resource. Policies are matched by name, which must be unique within this list.",
		Optional:    true,
		CustomType:  customfield.NewNestedObjectListType[api.ScheduledRebalancePolicyModel](ctx),
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"name":     schema.StringAttribute{Required: true, Description: "Unique policy name used to create, update, and delete the policy."},
			"cron":     schema.StringAttribute{Optional: true, Description: "Five-field cron expression such as `0 2 * * *`. Required for a new policy; when omitted for an existing policy, Terraform preserves the server value."},
			"timezone": schema.StringAttribute{Optional: true, Description: "IANA timezone such as `America/Los_Angeles`. When omitted for an existing policy, Terraform preserves the server value."},
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
			"force_drain": schema.BoolAttribute{Optional: true, Description: "Whether to continue draining selected nodes when normal eviction is blocked. When omitted for an existing policy, Terraform preserves the server value."},
			"scope": schema.SingleNestedAttribute{
				Description: "Optional node selection scope. `node_names` cannot be combined with `node_pool_name` or `node_selector_terms`. Include filters are evaluated before exclude filters.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectType[api.ScheduledRebalanceScopeModel](ctx),
				Attributes: map[string]schema.Attribute{
					"node_pool_name": schema.StringAttribute{
						Description: "Select nodes from this NodePool. Cannot be combined with `node_names`.",
						Optional:    true,
					},
					"node_names":                  scheduledStringListSchema(ctx, "Explicit Kubernetes node names to include. Cannot be combined with `node_pool_name` or `node_selector_terms`."),
					"node_selector_terms":         scheduledSelectorTermsSchema(ctx, "Kubernetes label selector terms used to include nodes. Terms are ORed; expressions within a term are ANDed. Cannot be combined with `node_names`."),
					"exclude_node_names":          scheduledStringListSchema(ctx, "Explicit Kubernetes node names to exclude after include filters are evaluated."),
					"exclude_node_selector_terms": scheduledSelectorTermsSchema(ctx, "Kubernetes label selector terms used to exclude nodes after include filters are evaluated. Terms are ORed; expressions within a term are ANDed."),
					"capacity_types":              scheduledStringListSchema(ctx, "Provider-supported capacity types to include. Common values are `on-demand` and `spot`; GKE also supports `reserved`."),
				},
			},
			"node_constraints": schema.SingleNestedAttribute{
				Description: "Optional limits applied after the scope is evaluated. All values must be non-negative.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectType[api.ScheduledRebalanceNodeConstraintsModel](ctx),
				Attributes: map[string]schema.Attribute{
					"min_age_seconds": schema.Int64Attribute{
						Description: "Minimum node age in seconds. Nodes younger than this value are not selected. `0` disables the age filter.",
						Optional:    true,
						Validators:  commonvalidators.Int64AtLeast(0),
					},
					"max_nodes": schema.Int64Attribute{
						Description: "Maximum number of matching nodes to select in one run. Omit or set to `0` to apply no maximum.",
						Optional:    true,
						Validators:  commonvalidators.Int64AtLeast(0),
					},
					"min_cluster_size": schema.Int64Attribute{
						Description: "Minimum number of matching nodes to retain. The selection limit is reduced so at least this many matching nodes remain. `0` disables the retention floor.",
						Optional:    true,
						Validators:  commonvalidators.Int64AtLeast(0),
					},
				},
			},
		}},
	}
}

func scheduledStringListSchema(ctx context.Context, description string) schema.ListAttribute {
	return schema.ListAttribute{
		Description: description,
		Optional:    true,
		ElementType: types.StringType,
		CustomType:  customfield.NewListType[types.String](ctx),
	}
}

func scheduledSelectorTermsSchema(ctx context.Context, description string) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description: description,
		Optional:    true,
		CustomType:  customfield.NewNestedObjectListType[api.ScheduledRebalanceNodeSelectorTermModel](ctx),
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"match_expressions": schema.ListNestedAttribute{
				Description: "Label selector requirements that are ANDed within this term.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.ScheduledRebalanceLabelSelectorRequirementModel](ctx),
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"key": schema.StringAttribute{
						Description: "Kubernetes label key to evaluate.",
						Required:    true,
					},
					"operator": schema.StringAttribute{
						Description: "Label selector operator. Allowed values: `In`, `NotIn`, `Exists`, `DoesNotExist`.",
						Required:    true,
						Validators:  commonvalidators.StringOneOf("In", "NotIn", "Exists", "DoesNotExist"),
					},
					"values": scheduledStringListSchema(ctx, "Values used by `In` and `NotIn`. Must be non-empty for those operators and omitted or empty for `Exists` and `DoesNotExist`."),
				}},
			},
		}},
	}
}
