package gke

import "github.com/hashicorp/terraform-plugin-framework/types"

type ClusterDataSourceModel struct {
	ClusterName     types.String `tfsdk:"cluster_name"`
	Region          types.String `tfsdk:"region"`
	ClusterUID      types.String `tfsdk:"cluster_uid"`
	ClusterID       types.String `tfsdk:"cluster_id"`
	ClusterLocation types.String `tfsdk:"cluster_location"`

	CloudProvider          types.String `tfsdk:"cloud_provider"`
	Status                 types.String `tfsdk:"status"`
	AgentVersion           types.String `tfsdk:"agent_version"`
	OnboardManifestVersion types.String `tfsdk:"onboard_manifest_version"`
	NeedUpgrade            types.Bool   `tfsdk:"need_upgrade"`
	RebalanceEnable        types.Bool   `tfsdk:"rebalance_enable"`
}
