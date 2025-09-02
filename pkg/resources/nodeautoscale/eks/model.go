package eks

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type ClusterModel struct {
	Kubeconfig  types.String `tfsdk:"kubeconfig"`
	ClusterID   types.String `tfsdk:"cluster_id"`
	ClusterName types.String `tfsdk:"cluster_name"`
	Region      types.String `tfsdk:"region"`
	AccountID   types.String `tfsdk:"account_id"`

	// agent configurations
	DisableWorkloadUploading types.Bool `tfsdk:"disable_workload_uploading"`

	OnlyInstallAgent types.Bool `tfsdk:"only_install_agent"`

	// upgrade configurations
	EnableUpgradeAgent              types.Bool `tfsdk:"enable_upgrade_agent"`
	EnableUpgradeRebalanceComponent types.Bool `tfsdk:"enable_upgrade_rebalance_component"` // if true, OnlyInstallAgent ignored

	// rebalance configuration
	EnableRebalance             types.Bool `tfsdk:"enable_rebalance"` // if true, OnlyInstallAgent ignored
	EnableUploadConfig          types.Bool `tfsdk:"enable_upload_config"`
	EnableDiversityInstanceType types.Bool `tfsdk:"enable_diversity_instance_type"`

	// restore configuration
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
