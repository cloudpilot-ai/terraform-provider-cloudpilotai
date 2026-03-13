package workloadautoscaler

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type WorkloadAutoscalerModel struct {
	ClusterID  types.String `tfsdk:"cluster_id"`
	Kubeconfig types.String `tfsdk:"kubeconfig"`

	StorageClass    types.String `tfsdk:"storage_class"`
	EnableNodeAgent types.Bool   `tfsdk:"enable_node_agent"`

	RecommendationPolicies customfield.NestedObjectList[api.RecommendationPolicyModel] `tfsdk:"recommendation_policies"`
	AutoscalingPolicies    customfield.NestedObjectList[api.AutoscalingPolicyModel]    `tfsdk:"autoscaling_policies"`
	EnableProactive        customfield.NestedObjectList[api.EnableProactiveModel]      `tfsdk:"enable_proactive"`
	DisableProactive       customfield.NestedObjectList[api.DisableProactiveModel]     `tfsdk:"disable_proactive"`
}
