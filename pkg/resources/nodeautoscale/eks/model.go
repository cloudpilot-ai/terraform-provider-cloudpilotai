package eks

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type AWSAssumeRoleModel struct {
	RoleARN     types.String `tfsdk:"role_arn"`
	SessionName types.String `tfsdk:"session_name"`
}

type ClusterModel struct {
	Kubeconfig    types.String                                 `tfsdk:"kubeconfig"`
	AWSProfile    types.String                                 `tfsdk:"aws_profile"`
	AWSAssumeRole customfield.NestedObject[AWSAssumeRoleModel] `tfsdk:"aws_assume_role"`
	ClusterID     types.String                                 `tfsdk:"cluster_id"`
	ClusterName   types.String                                 `tfsdk:"cluster_name"`
	Region        types.String                                 `tfsdk:"region"`
	AccountID     types.String                                 `tfsdk:"account_id"`

	ClusterSetting customfield.NestedObject[ClusterSettingModel] `tfsdk:"cluster_setting"`

	// agent configurations
	DisableWorkloadUploading types.Bool `tfsdk:"disable_workload_uploading"`

	OnlyInstallAgent types.Bool `tfsdk:"only_install_agent"`

	EnableUpgrade types.Bool `tfsdk:"enable_upgrade"`

	// rebalance configuration
	EnableRebalance types.Bool `tfsdk:"enable_rebalance"` // if true, OnlyInstallAgent ignored

	CustomNodeRole types.String `tfsdk:"custom_node_role"`

	// destroy configuration
	SkipRestore       types.Bool  `tfsdk:"skip_restore"`
	RestoreNodeNumber types.Int64 `tfsdk:"restore_node_number"`

	// rebalance workload configuration
	WorkloadTemplates customfield.NestedObjectList[api.WorkloadTemplateModel] `tfsdk:"workload_templates"`
	Workloads         customfield.NestedObjectList[api.WorkloadModel]         `tfsdk:"workloads"`

	// rebalance nodeclass configuration
	NodeClassTemplates customfield.NestedObjectList[api.EC2NodeClassTemplateModel] `tfsdk:"nodeclass_templates"`
	NodeClasses        customfield.NestedObjectList[api.EC2NodeClassModel]         `tfsdk:"nodeclasses"`

	// rebalance nodepool configuration
	NodePoolTemplates customfield.NestedObjectList[api.EC2NodePoolTemplateModel] `tfsdk:"nodepool_templates"`
	NodePools         customfield.NestedObjectList[api.EC2NodePoolModel]         `tfsdk:"nodepools"`
}
