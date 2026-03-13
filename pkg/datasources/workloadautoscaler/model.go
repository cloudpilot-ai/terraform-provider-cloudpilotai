package workloadautoscaler

import "github.com/hashicorp/terraform-plugin-framework/types"

type WorkloadAutoscalerDataSourceModel struct {
	ClusterID types.String `tfsdk:"cluster_id"`

	Enabled   types.Bool `tfsdk:"enabled"`
	Installed types.Bool `tfsdk:"installed"`
}
