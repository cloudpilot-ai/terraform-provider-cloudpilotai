package gke

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type ClusterSettingModel struct {
	EnableNodeRepair  types.Bool    `tfsdk:"enable_node_repair"`
	EnableDiskMonitor types.Bool    `tfsdk:"enable_disk_monitor"`
	Discount          types.Float64 `tfsdk:"discount"`
	PreRunCommand     types.String  `tfsdk:"pre_run_command"`
	PostRunCommand    types.String  `tfsdk:"post_run_command"`
}

type ClusterModel struct {
	Kubeconfig             types.String `tfsdk:"kubeconfig"`
	ClusterID              types.String `tfsdk:"cluster_id"`
	ClusterUID             types.String `tfsdk:"cluster_uid"`
	ClusterName            types.String `tfsdk:"cluster_name"`
	Region                 types.String `tfsdk:"region"`
	ProjectID              types.String `tfsdk:"project_id"`
	ClusterLocation        types.String `tfsdk:"cluster_location"`
	AgentVersion           types.String `tfsdk:"agent_version"`
	OnboardManifestVersion types.String `tfsdk:"onboard_manifest_version"`
	NeedUpgrade            types.Bool   `tfsdk:"need_upgrade"`

	DisableWorkloadUploading types.Bool  `tfsdk:"disable_workload_uploading"`
	OnlyInstallAgent         types.Bool  `tfsdk:"only_install_agent"`
	EnableUpgrade            types.Bool  `tfsdk:"enable_upgrade"`
	EnableRebalance          types.Bool  `tfsdk:"enable_rebalance"`
	SkipRestore              types.Bool  `tfsdk:"skip_restore"`
	RestoreNodeNumber        types.Int64 `tfsdk:"restore_node_number"`
	RestoreDesiredSizes      types.Map   `tfsdk:"restore_desired_sizes"`

	ClusterSetting customfield.NestedObject[ClusterSettingModel]       `tfsdk:"cluster_setting"`
	NodeClasses    customfield.NestedObjectList[api.GCENodeClassModel] `tfsdk:"nodeclasses"`
	NodePools      customfield.NestedObjectList[api.GCENodePoolModel]  `tfsdk:"nodepools"`
}
