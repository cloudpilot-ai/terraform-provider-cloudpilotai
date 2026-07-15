package workloadautoscaler

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type WorkloadAutoscalerModel struct {
	ClusterID     types.String                                 `tfsdk:"cluster_id"`
	Kubeconfig    types.String                                 `tfsdk:"kubeconfig"`
	AWSProfile    types.String                                 `tfsdk:"aws_profile"`
	AWSAssumeRole customfield.NestedObject[AWSAssumeRoleModel] `tfsdk:"aws_assume_role"`
	GCPProjectID  types.String                                 `tfsdk:"gcp_project_id"`
	GCPLocation   types.String                                 `tfsdk:"gcp_cluster_location"`

	StorageClass    types.String `tfsdk:"storage_class"`
	EnableNodeAgent types.Bool   `tfsdk:"enable_node_agent"`

	EnableNewWorkloadsProactiveUpdate        types.Bool   `tfsdk:"enable_new_workloads_proactive_update"`
	LimiterQuotaPerWindow                    types.Int64  `tfsdk:"limiter_quota_per_window"`
	LimiterBurst                             types.Int64  `tfsdk:"limiter_burst"`
	LimiterWindowSeconds                     types.Int64  `tfsdk:"limiter_window_seconds"`
	EnablePreemptedPodGC                     types.Bool   `tfsdk:"enable_preempted_pod_gc"`
	PreemptedPodGCTTL                        types.String `tfsdk:"preempted_pod_gc_ttl"`
	EnableInitialOptimizationDataWindowCheck types.Bool   `tfsdk:"enable_initial_optimization_data_window_check"`

	RecommendationPolicies customfield.NestedObjectList[api.RecommendationPolicyModel] `tfsdk:"recommendation_policies"`
	AutoscalingPolicies    customfield.NestedObjectList[api.AutoscalingPolicyModel]    `tfsdk:"autoscaling_policies"`
	EnableProactive        customfield.NestedObjectList[api.EnableProactiveModel]      `tfsdk:"enable_proactive"`
	DisableProactive       customfield.NestedObjectList[api.DisableProactiveModel]     `tfsdk:"disable_proactive"`
}

type AWSAssumeRoleModel struct {
	RoleARN     types.String `tfsdk:"role_arn"`
	SessionName types.String `tfsdk:"session_name"`
}

func (m WorkloadAutoscalerModel) ToWAConfiguration() *api.WAConfiguration {
	conf := &api.WAConfiguration{}
	if !m.EnableNewWorkloadsProactiveUpdate.IsNull() && !m.EnableNewWorkloadsProactiveUpdate.IsUnknown() {
		v := m.EnableNewWorkloadsProactiveUpdate.ValueBool()
		conf.EnableNewWorkloadsProactiveUpdate = &v
	}
	if !m.LimiterQuotaPerWindow.IsNull() && !m.LimiterQuotaPerWindow.IsUnknown() {
		v := int(m.LimiterQuotaPerWindow.ValueInt64())
		conf.LimiterQuotaPerWindow = &v
	}
	if !m.LimiterBurst.IsNull() && !m.LimiterBurst.IsUnknown() {
		v := int(m.LimiterBurst.ValueInt64())
		conf.LimiterBurst = &v
	}
	if !m.LimiterWindowSeconds.IsNull() && !m.LimiterWindowSeconds.IsUnknown() {
		v := int(m.LimiterWindowSeconds.ValueInt64())
		conf.LimiterWindowSeconds = &v
	}
	if !m.EnablePreemptedPodGC.IsNull() && !m.EnablePreemptedPodGC.IsUnknown() {
		v := m.EnablePreemptedPodGC.ValueBool()
		conf.EnablePreemptedPodGC = &v
	}
	if !m.PreemptedPodGCTTL.IsNull() && !m.PreemptedPodGCTTL.IsUnknown() && m.PreemptedPodGCTTL.ValueString() != "" {
		v := m.PreemptedPodGCTTL.ValueString()
		conf.PreemptedPodGCTTL = &v
	}
	if !m.EnableInitialOptimizationDataWindowCheck.IsNull() && !m.EnableInitialOptimizationDataWindowCheck.IsUnknown() {
		v := m.EnableInitialOptimizationDataWindowCheck.ValueBool()
		conf.EnableInitialOptimizationDataWindowCheck = &v
	}
	return conf
}

func (m *WorkloadAutoscalerModel) ApplyWAConfiguration(conf *api.WAConfiguration) {
	if conf == nil {
		return
	}
	if conf.EnableNewWorkloadsProactiveUpdate != nil {
		m.EnableNewWorkloadsProactiveUpdate = types.BoolValue(*conf.EnableNewWorkloadsProactiveUpdate)
	}
	if conf.LimiterQuotaPerWindow != nil {
		m.LimiterQuotaPerWindow = types.Int64Value(int64(*conf.LimiterQuotaPerWindow))
	}
	if conf.LimiterBurst != nil {
		m.LimiterBurst = types.Int64Value(int64(*conf.LimiterBurst))
	}
	if conf.LimiterWindowSeconds != nil {
		m.LimiterWindowSeconds = types.Int64Value(int64(*conf.LimiterWindowSeconds))
	}
	if conf.EnablePreemptedPodGC != nil {
		m.EnablePreemptedPodGC = types.BoolValue(*conf.EnablePreemptedPodGC)
	}
	if conf.PreemptedPodGCTTL != nil {
		m.PreemptedPodGCTTL = types.StringValue(*conf.PreemptedPodGCTTL)
	}
	if conf.EnableInitialOptimizationDataWindowCheck != nil {
		m.EnableInitialOptimizationDataWindowCheck = types.BoolValue(*conf.EnableInitialOptimizationDataWindowCheck)
	}
}
