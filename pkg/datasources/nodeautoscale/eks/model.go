package eks

import "github.com/hashicorp/terraform-plugin-framework/types"

type ClusterDataSourceModel struct {
	ClusterName types.String `tfsdk:"cluster_name"`
	Region      types.String `tfsdk:"region"`
	AccountID   types.String `tfsdk:"account_id"`

	ClusterID       types.String `tfsdk:"cluster_id"`
	CloudProvider   types.String `tfsdk:"cloud_provider"`
	Status          types.String `tfsdk:"status"`
	AgentVersion    types.String `tfsdk:"agent_version"`
	RebalanceEnable types.Bool   `tfsdk:"rebalance_enable"`
}
