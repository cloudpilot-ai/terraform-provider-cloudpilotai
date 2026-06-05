package workloadautoscaler

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type fakePostWriteStateHydratorClient struct {
	waConfig               *api.WAConfiguration
	recommendationPolicies []api.RecommendationPolicyResource
	autoscalingPolicies    []api.AutoscalingPolicyResource
}

func (f *fakePostWriteStateHydratorClient) GetWAConfiguration(string) (*api.WAConfiguration, error) {
	return f.waConfig, nil
}

func (f *fakePostWriteStateHydratorClient) ListRecommendationPolicies(string) ([]api.RecommendationPolicyResource, error) {
	return f.recommendationPolicies, nil
}

func (f *fakePostWriteStateHydratorClient) ListAutoscalingPolicies(string) ([]api.AutoscalingPolicyResource, error) {
	return f.autoscalingPolicies, nil
}

func autoscalingPolicyWithServerDefaults(name, recommendationPolicyName string) api.AutoscalingPolicyResource {
	removeLimit := true
	autoHeadroom := "2"

	return api.AutoscalingPolicyResource{
		Name:   name,
		Enable: true,
		Spec: api.AutoscalingPolicySpec{
			RecommendationPolicyName:   recommendationPolicyName,
			Priority:                   0,
			DisableRuntimeOptimization: false,
			UpdateResources:            []string{"cpu", "memory"},
			DriftThresholds: map[string]string{
				"cpu":    "5%",
				"memory": "5%",
			},
			OnPolicyRemoval: "off",
			TargetRefs: []api.TypedObjectReference{{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			}},
			UpdateSchedule: []api.UpdateScheduleItem{{
				Name: "default",
				Mode: "off",
			}},
			LimitPolicies: map[string]api.ResourceLimitPolicy{
				"cpu": {
					RemoveLimit: &removeLimit,
				},
				"memory": {
					AutoHeadroom: &autoHeadroom,
				},
			},
			InPlaceFallback: &api.InPlaceFallback{
				ReasonPolicies: map[string]string{},
			},
		},
	}
}

func TestWorkloadAutoscalerModelToWAConfigurationIncludesAdvancedFields(t *testing.T) {
	model := WorkloadAutoscalerModel{
		EnableNewWorkloadsProactiveUpdate:        types.BoolValue(true),
		LimiterQuotaPerWindow:                    types.Int64Value(7),
		LimiterBurst:                             types.Int64Value(11),
		LimiterWindowSeconds:                     types.Int64Value(31),
		EnablePreemptedPodGC:                     types.BoolValue(false),
		PreemptedPodGCTTL:                        types.StringValue("45m"),
		EnableInitialOptimizationDataWindowCheck: types.BoolValue(false),
	}

	got := model.ToWAConfiguration()
	if got.EnableNewWorkloadsProactiveUpdate == nil || !*got.EnableNewWorkloadsProactiveUpdate {
		t.Fatalf("EnableNewWorkloadsProactiveUpdate = %#v", got.EnableNewWorkloadsProactiveUpdate)
	}
	if got.LimiterQuotaPerWindow == nil || *got.LimiterQuotaPerWindow != 7 {
		t.Fatalf("LimiterQuotaPerWindow = %#v", got.LimiterQuotaPerWindow)
	}
	if got.EnablePreemptedPodGC == nil || *got.EnablePreemptedPodGC {
		t.Fatalf("EnablePreemptedPodGC = %#v", got.EnablePreemptedPodGC)
	}
}

func TestSchemaRemovesEnableUpgrade(t *testing.T) {
	s := Schema(context.Background())

	if _, ok := s.Attributes["enable_upgrade"]; ok {
		t.Fatalf("workload autoscaler schema should not expose enable_upgrade")
	}
	if _, ok := s.Attributes["enable_node_agent"]; !ok {
		t.Fatalf("workload autoscaler schema missing enable_node_agent")
	}

	if _, ok := s.Attributes["cluster_id"].(schema.StringAttribute); !ok {
		t.Fatalf("cluster_id attribute has unexpected type %T", s.Attributes["cluster_id"])
	}
}

func TestTopLevelOptionalConfigFieldsHaveNoSchemaDefaults(t *testing.T) {
	s := Schema(context.Background())

	for _, name := range []string{
		"storage_class",
		"enable_node_agent",
		"enable_new_workloads_proactive_update",
		"limiter_quota_per_window",
		"limiter_burst",
		"limiter_window_seconds",
		"enable_preempted_pod_gc",
		"preempted_pod_gc_ttl",
		"enable_initial_optimization_data_window_check",
	} {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Fatalf("schema missing %s", name)
		}
		switch typed := attr.(type) {
		case schema.StringAttribute:
			if typed.StringDefaultValue() != nil {
				t.Fatalf("%s should not have a schema default", name)
			}
			if typed.IsComputed() {
				t.Fatalf("%s should not be computed", name)
			}
		case schema.BoolAttribute:
			if typed.BoolDefaultValue() != nil {
				t.Fatalf("%s should not have a schema default", name)
			}
			if typed.IsComputed() {
				t.Fatalf("%s should not be computed", name)
			}
		case schema.Int64Attribute:
			if typed.Int64DefaultValue() != nil {
				t.Fatalf("%s should not have a schema default", name)
			}
			if typed.IsComputed() {
				t.Fatalf("%s should not be computed", name)
			}
		default:
			t.Fatalf("%s has unexpected type %T", name, attr)
		}
	}
}

func TestHydratePostWriteStateRefreshesBackendManagedFields(t *testing.T) {
	ctx := context.Background()
	quota := 7
	proactive := true

	data := WorkloadAutoscalerModel{
		ClusterID:                         types.StringValue("cluster-1"),
		Kubeconfig:                        types.StringValue("/tmp/kubeconfig"),
		StorageClass:                      types.StringValue(""),
		EnableNodeAgent:                   types.BoolValue(true),
		EnableNewWorkloadsProactiveUpdate: types.BoolUnknown(),
		LimiterQuotaPerWindow:             types.Int64Unknown(),
		RecommendationPolicies: customfield.NewObjectListMust(ctx, []api.RecommendationPolicyModel{{
			Name:                types.StringValue("java"),
			HistoryWindowCPU:    types.StringValue("24h"),
			HistoryWindowMemory: types.StringValue("24h"),
			EvaluationPeriod:    types.StringValue("1h"),
		}}),
		AutoscalingPolicies: customfield.NewObjectListMust(ctx, []api.AutoscalingPolicyModel{{
			Name:                     types.StringValue("default-ap"),
			RecommendationPolicyName: types.StringValue("java"),
		}}),
	}

	err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{
		waConfig: &api.WAConfiguration{
			EnableNewWorkloadsProactiveUpdate: &proactive,
			LimiterQuotaPerWindow:             &quota,
		},
		recommendationPolicies: []api.RecommendationPolicyResource{{
			Name: "java",
			Spec: api.RecommendationPolicySpec{
				StrategyType: "percentile",
				HistoryWindowDuration: api.WindowDuration{
					CPU:    "24h",
					Memory: "24h",
				},
				EvaluationPeriod: "1h",
			},
		}},
		autoscalingPolicies: []api.AutoscalingPolicyResource{{
			Name:   "default-ap",
			Enable: true,
			Spec: api.AutoscalingPolicySpec{
				RecommendationPolicyName: "java",
				OnPolicyRemoval:          "off",
			},
		}},
	}, "cluster-1", &data)
	if err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	if data.Kubeconfig.ValueString() != "/tmp/kubeconfig" {
		t.Fatalf("Kubeconfig = %q, want /tmp/kubeconfig", data.Kubeconfig.ValueString())
	}
	if data.EnableNewWorkloadsProactiveUpdate.IsUnknown() || !data.EnableNewWorkloadsProactiveUpdate.ValueBool() {
		t.Fatalf("EnableNewWorkloadsProactiveUpdate = %#v, want known true", data.EnableNewWorkloadsProactiveUpdate)
	}
	if data.LimiterQuotaPerWindow.IsUnknown() || data.LimiterQuotaPerWindow.ValueInt64() != 7 {
		t.Fatalf("LimiterQuotaPerWindow = %#v, want known 7", data.LimiterQuotaPerWindow)
	}
}

func TestMergeAutoscalingPoliciesFromAPIForReadPreservesOmittedDefaults(t *testing.T) {
	ctx := context.Background()

	data := WorkloadAutoscalerModel{
		AutoscalingPolicies: customfield.NewObjectListMust(ctx, []api.AutoscalingPolicyModel{{
			Name:                     types.StringValue("readonly"),
			Enable:                   types.BoolValue(true),
			RecommendationPolicyName: types.StringValue("cost-savings"),
			Priority:                 types.Int64Value(0),
			TargetRefs: customfield.NewObjectListMust(ctx, []api.TargetRefModel{{
				APIVersion: types.StringValue("apps/v1"),
				Kind:       types.StringValue("Deployment"),
			}}),
			UpdateSchedules: customfield.NewObjectListMust(ctx, []api.UpdateScheduleModel{{
				Name: types.StringValue("default"),
				Mode: types.StringValue("off"),
			}}),
		}}),
	}

	err := mergeAutoscalingPoliciesFromAPI(ctx, &data, []api.AutoscalingPolicyResource{
		autoscalingPolicyWithServerDefaults("readonly", "cost-savings"),
	}, false)
	if err != nil {
		t.Fatalf("mergeAutoscalingPoliciesFromAPI() error = %v", err)
	}

	policies, diags := data.AutoscalingPolicies.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("AutoscalingPolicies diagnostics = %v", diags)
	}
	if len(policies) != 1 {
		t.Fatalf("AutoscalingPolicies length = %d, want 1", len(policies))
	}

	got := policies[0]
	if !got.DisableRuntimeOptimization.IsNull() {
		t.Fatalf("DisableRuntimeOptimization should remain null for omitted config, got %#v", got.DisableRuntimeOptimization)
	}
	if got.UpdateResources != nil {
		t.Fatalf("UpdateResources should remain nil for omitted config, got %#v", got.UpdateResources)
	}
	if !got.DriftThresholdCPU.IsNull() {
		t.Fatalf("DriftThresholdCPU should remain null for omitted config, got %#v", got.DriftThresholdCPU)
	}
	if !got.DriftThresholdMemory.IsNull() {
		t.Fatalf("DriftThresholdMemory should remain null for omitted config, got %#v", got.DriftThresholdMemory)
	}
	if !got.OnPolicyRemoval.IsNull() {
		t.Fatalf("OnPolicyRemoval should remain null for omitted config, got %#v", got.OnPolicyRemoval)
	}
	if !got.LimitPolicies.IsNull() {
		t.Fatalf("LimitPolicies should remain null for omitted config, got %#v", got.LimitPolicies)
	}
	if !got.InPlaceFallbackReasonPolicies.IsNull() {
		t.Fatalf("InPlaceFallbackReasonPolicies should remain null for omitted config, got %#v", got.InPlaceFallbackReasonPolicies)
	}
}

func TestMergeAutoscalingPoliciesFromAPIForImportKeepsRemoteDefaults(t *testing.T) {
	ctx := context.Background()
	data := WorkloadAutoscalerModel{}

	err := mergeAutoscalingPoliciesFromAPI(ctx, &data, []api.AutoscalingPolicyResource{
		autoscalingPolicyWithServerDefaults("readonly", "cost-savings"),
	}, true)
	if err != nil {
		t.Fatalf("mergeAutoscalingPoliciesFromAPI() error = %v", err)
	}

	policies, diags := data.AutoscalingPolicies.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("AutoscalingPolicies diagnostics = %v", diags)
	}
	if len(policies) != 1 {
		t.Fatalf("AutoscalingPolicies length = %d, want 1", len(policies))
	}

	got := policies[0]
	if got.OnPolicyRemoval.ValueString() != "off" {
		t.Fatalf("OnPolicyRemoval = %#v, want off", got.OnPolicyRemoval)
	}
	if got.UpdateResources == nil || len(*got.UpdateResources) != 2 {
		t.Fatalf("UpdateResources = %#v, want cpu and memory", got.UpdateResources)
	}
	if got.DriftThresholdCPU.ValueString() != "5%" {
		t.Fatalf("DriftThresholdCPU = %#v, want 5%%", got.DriftThresholdCPU)
	}
	if got.LimitPolicies.IsNull() {
		t.Fatalf("LimitPolicies should be populated during import")
	}
}

func TestPreserveAutoscalingPolicyStateRepresentationKeepsEmptyReasonPolicyMap(t *testing.T) {
	ctx := context.Background()
	remote := api.AutoscalingPolicyModel{
		Name:                          types.StringValue("default-ap"),
		InPlaceFallbackReasonPolicies: customfield.NullMap[types.String](ctx),
	}
	state := api.AutoscalingPolicyModel{
		Name:                          types.StringValue("default-ap"),
		InPlaceFallbackReasonPolicies: customfield.NewMapMust[types.String](ctx, map[string]types.String{}),
	}

	got := preserveAutoscalingPolicyStateRepresentation(ctx, remote, state)
	if got.InPlaceFallbackReasonPolicies.IsNull() {
		t.Fatalf("InPlaceFallbackReasonPolicies should preserve an explicit empty map from state")
	}
	values, diags := got.InPlaceFallbackReasonPolicies.Value(ctx)
	if diags.HasError() {
		t.Fatalf("InPlaceFallbackReasonPolicies diagnostics = %v", diags)
	}
	if len(values) != 0 {
		t.Fatalf("expected empty reason policy map, got %#v", values)
	}
}

func TestShouldSkipBackendInstallCheckForStateBackfill(t *testing.T) {
	data := WorkloadAutoscalerModel{
		Kubeconfig:      types.StringValue("/tmp/kubeconfig"),
		StorageClass:    types.StringValue(""),
		EnableNodeAgent: types.BoolValue(true),
	}
	state := WorkloadAutoscalerModel{
		Kubeconfig:      types.StringValue(""),
		StorageClass:    types.StringValue(""),
		EnableNodeAgent: types.BoolValue(true),
	}

	if !shouldSkipBackendInstallCheckForStateBackfill(data, state) {
		t.Fatalf("expected kubeconfig-only state backfill to skip backend install check")
	}
}
