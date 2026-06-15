package eks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func Schema(_ context.Context) schema.Schema {
	return schema.Schema{
		Description: "Retrieves information about an existing EKS cluster registered with CloudPilot AI.",
		Attributes: map[string]schema.Attribute{
			"cluster_name": schema.StringAttribute{
				Description: "Name of the EKS cluster.",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "AWS region where the EKS cluster is located.",
				Required:    true,
			},
			"account_id": schema.StringAttribute{
				Description: "AWS account ID. If not provided, it will be detected from the current AWS CLI credentials.",
				Optional:    true,
				Computed:    true,
			},

			"cluster_id": schema.StringAttribute{
				Description: "CloudPilot AI cluster identifier. When provided, this override is used directly and the data source does not derive the default ID from cluster_name, region, and account_id.",
				Optional:    true,
				Computed:    true,
			},
			"cloud_provider": schema.StringAttribute{
				Description: "Cloud provider of the cluster (e.g. 'aws').",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Current status of the cluster: 'online', 'offline', or 'demo'.",
				Computed:    true,
			},
			"agent_version": schema.StringAttribute{
				Description: "Version of the CloudPilot AI agent installed on the cluster.",
				Computed:    true,
			},
			"onboard_manifest_version": schema.StringAttribute{
				Description: "Latest CloudPilot onboard manifest version reported by the service.",
				Computed:    true,
			},
			"need_upgrade": schema.BoolAttribute{
				Description: "Whether CloudPilot currently reports that this cluster needs an upgrade.",
				Computed:    true,
			},
			"rebalance_enable": schema.BoolAttribute{
				Description: "Whether rebalancing is enabled for this cluster.",
				Computed:    true,
			},
		},
	}
}
