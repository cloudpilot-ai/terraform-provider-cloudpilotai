package workloadautoscaler

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func Schema(_ context.Context) schema.Schema {
	return schema.Schema{
		Description: "Retrieves the Workload Autoscaler configuration for a given cluster.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Description: "The CloudPilot AI cluster ID.",
				Required:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the Workload Autoscaler is enabled on this cluster.",
				Computed:    true,
			},
			"installed": schema.BoolAttribute{
				Description: "Whether the Workload Autoscaler is installed on this cluster.",
				Computed:    true,
			},
		},
	}
}
