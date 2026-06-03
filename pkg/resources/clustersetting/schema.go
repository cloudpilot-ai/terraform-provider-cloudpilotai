package clustersetting

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func Schema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Description: "CloudPilot AI cluster-level settings for an existing cluster.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Description: "CloudPilot AI cluster ID.",
				Required:    true,
			},
			"enable_node_repair": schema.BoolAttribute{
				Description: "Enable CloudPilot AI node repair for this cluster.",
				Optional:    true,
				Computed:    true,
			},
			"enable_disk_monitor": schema.BoolAttribute{
				Description: "Enable disk monitor for this cluster.",
				Optional:    true,
				Computed:    true,
			},
			"maintenance_enabled": schema.BoolAttribute{
				Description: "Enable maintenance mode for this cluster.",
				Optional:    true,
				Computed:    true,
			},
			"discount": schema.Float64Attribute{
				Description: "Cluster-level discount ratio used by cost calculations.",
				Optional:    true,
				Computed:    true,
			},
			"pre_run_command": schema.StringAttribute{
				Description: "Command run before maintenance or repair actions.",
				Optional:    true,
				Computed:    true,
			},
			"post_run_command": schema.StringAttribute{
				Description: "Command run after maintenance or repair actions.",
				Optional:    true,
				Computed:    true,
			},
		},
	}
}
