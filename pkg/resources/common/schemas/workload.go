// Package schemas provides common schema definitions for CloudPilot AI Terraform provider resources.
package schemas

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/samber/lo"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	commonvalidators "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/validators"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func WorkloadSchema(ctx context.Context) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description: "Workload optimization settings managed by Terraform. Omit to leave workloads unmanaged. An empty list stops managing all workloads but does not reset settings previously applied to the server.",
		Optional:    true,
		CustomType:  customfield.NewNestedObjectListType[api.WorkloadModel](ctx),
		NestedObject: schema.NestedAttributeObject{
			Attributes: lo.Assign(map[string]schema.Attribute{
				"name": schema.StringAttribute{
					Description: "Kubernetes workload name.",
					Required:    true,
				},
				"type": schema.StringAttribute{
					Description: "Kubernetes workload type as recognized by CloudPilot, for example `deployment` or `statefulset`.",
					Required:    true,
				},
				"namespace": schema.StringAttribute{
					Description: "Kubernetes namespace containing the workload.",
					Required:    true,
				},

				"template_name": schema.StringAttribute{
					Description:        "Deprecated provider-side workload template name to merge before applying this workload.",
					Optional:           true,
					DeprecationMessage: ProviderSideTemplateDeprecationMessage,
				},
			}, workloadTemplateSchema()),
		},
	}
}

func WorkloadTemplateSchema(ctx context.Context) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description:        "Deprecated provider-side workload templates. Omit to leave provider-side templates unmanaged. Use the EKS module for reusable template composition.",
		Optional:           true,
		DeprecationMessage: ProviderSideTemplateDeprecationMessage,
		CustomType:         customfield.NewNestedObjectListType[api.WorkloadTemplateModel](ctx),
		NestedObject: schema.NestedAttributeObject{
			Attributes: lo.Assign(map[string]schema.Attribute{
				"template_name": schema.StringAttribute{
					Description:        "Unique provider-side workload template name.",
					Required:           true,
					DeprecationMessage: ProviderSideTemplateDeprecationMessage,
				},
			}, workloadTemplateSchema()),
		},
	}
}

func workloadTemplateSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"rebalance_able": schema.BoolAttribute{
			Description: "Whether CloudPilot may move this workload during node rebalancing. When omitted, Terraform preserves the server value.",
			Optional:    true,
		},
		"spot_friendly": schema.BoolAttribute{
			Description: "Whether CloudPilot may place this workload on Spot instances. When omitted, Terraform preserves the server value.",
			Optional:    true,
		},
		"min_non_spot_replicas": schema.Int64Attribute{
			Description: "Minimum replicas to keep on non-Spot capacity. Must be non-negative. When omitted, Terraform preserves the server value.",
			Optional:    true,
			Validators:  commonvalidators.Int64AtLeast(0),
		},
	}
}
