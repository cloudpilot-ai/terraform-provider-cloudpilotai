package gke

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func Schema(_ context.Context) schema.Schema {
	return schema.Schema{
		Description: "Retrieves information about an existing GKE cluster registered with CloudPilot AI.",
		Attributes: map[string]schema.Attribute{
			"cluster_name": schema.StringAttribute{
				Description: "Name of the GKE cluster. Required when cluster_id is not provided; otherwise read from the CloudPilot cluster record.",
				Optional:    true,
				Computed:    true,
			},
			"region": schema.StringAttribute{
				Description: "GCP region where the GKE cluster is located. Required when cluster_id is not provided; otherwise read from the CloudPilot cluster record.",
				Optional:    true,
				Computed:    true,
			},
			"cluster_uid": schema.StringAttribute{
				Description: "Kubernetes cluster UID used to derive the default CloudPilot cluster ID. For GKE, this should match the kube-system namespace UID returned by `kubectl get ns kube-system -o jsonpath='{.metadata.uid}'`. Required when cluster_id is not provided.",
				Optional:    true,
				Computed:    true,
			},
			"cluster_location": schema.StringAttribute{
				Description: "Optional GKE location override for zonal clusters.",
				Optional:    true,
			},
			"cluster_id": schema.StringAttribute{
				Description: "CloudPilot cluster identifier. When provided, this override is used directly and the data source does not derive the default ID from cluster_name, region, and cluster_uid.",
				Optional:    true,
				Computed:    true,
			},
			"cloud_provider": schema.StringAttribute{
				Description: "Cloud provider of the cluster (for example 'gcp').",
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
