package workloadautoscaler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRecommendationPolicyClearsSurviveWriteReadAndReapply(t *testing.T) {
	ctx := context.Background()
	var base api.RecommendationPolicyResource
	if err := json.Unmarshal([]byte(`{"name":"java","spec":{"strategyType":"percentile","historyWindowDuration":{"cpu":"24h","memory":"24h"},"evaluationPeriod":"1m","buffer":{"cpu":"10%","memory":"20%"},"limits":{"requestMin":{"cpu":"100m","memory":"128Mi"},"requestMax":{"cpu":"2","memory":"1Gi"}},"jvm":{"heapBuffer":"25%","minHeapXms":"128Mi","maxHeapXms":"1Gi","minHeapXmx":"256Mi","maxHeapXmx":"2Gi","minHeapXmsRatioOfMemory":"0.2","recentNonHeapWindow":"2h"}}}`), &base); err != nil {
		t.Fatal(err)
	}
	clear := types.StringValue("")
	configured := api.RecommendationPolicyModel{
		Name: types.StringValue("java"), HistoryWindowCPU: types.StringValue("24h"),
		HistoryWindowMemory: types.StringValue("24h"), EvaluationPeriod: types.StringValue("1m"),
		BufferCPU: clear, BufferMemory: clear,
		RequestMinCPU: clear, RequestMinMemory: clear, RequestMaxCPU: clear, RequestMaxMemory: clear,
		JVMHeapBuffer: clear, JVMMinHeapXms: clear, JVMMaxHeapXms: clear, JVMMinHeapXmx: clear, JVMMaxHeapXmx: clear,
		JVMMinHeapXmsRatioOfMemory: clear, JVMRecentNonHeapWindow: clear,
	}
	updated := configured.ToResourceFromBase(&base)
	var remote api.RecommendationPolicyResource
	payload := policyJSON(t, updated)
	if err := json.Unmarshal(payload, &remote); err != nil {
		t.Fatal(err)
	}
	data := WorkloadAutoscalerModel{RecommendationPolicies: customfield.NewObjectListMust(ctx, []api.RecommendationPolicyModel{configured})}
	if err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{recommendationPolicies: []api.RecommendationPolicyResource{remote}}, "cluster", &data); err != nil {
		t.Fatal(err)
	}
	policies, diags := data.RecommendationPolicies.AsStructSliceT(ctx)
	if diags.HasError() || len(policies) != 1 {
		t.Fatalf("recommendation policies=%#v diagnostics=%v", policies, diags)
	}
	got := policies[0]
	for name, value := range map[string]types.String{
		"buffer CPU": got.BufferCPU, "buffer memory": got.BufferMemory,
		"request min CPU": got.RequestMinCPU, "request min memory": got.RequestMinMemory,
		"request max CPU": got.RequestMaxCPU, "request max memory": got.RequestMaxMemory,
		"heap buffer": got.JVMHeapBuffer, "min Xms": got.JVMMinHeapXms, "max Xms": got.JVMMaxHeapXms,
		"min Xmx": got.JVMMinHeapXmx, "max Xmx": got.JVMMaxHeapXmx,
		"heap ratio": got.JVMMinHeapXmsRatioOfMemory, "non-heap window": got.JVMRecentNonHeapWindow,
	} {
		if !value.Equal(clear) {
			t.Errorf("%s=%v, want explicit empty string", name, value)
		}
	}
	reapplied := got.ToResourceFromBase(&remote)
	if reapplied.Spec.Buffer != nil || reapplied.Spec.Limits.RequestMin != nil || reapplied.Spec.Limits.RequestMax != nil {
		t.Fatalf("reapply resurrected a resource bound: %s", policyJSON(t, reapplied))
	}
	jvm := reapplied.Spec.JVM
	if jvm == nil || jvm.HeapBuffer != nil || jvm.MinHeapXms != nil || jvm.MaxHeapXms != nil || jvm.MinHeapXmx != nil || jvm.MaxHeapXmx != nil || jvm.MinHeapXmsRatioOfMemory != nil || jvm.RecentNonHeapWindow != nil {
		t.Fatalf("reapply resurrected a JVM setting: %s", policyJSON(t, reapplied))
	}
}

func TestAutoscalingPolicyClearsAndFalseValuesSurviveWriteReadAndReapply(t *testing.T) {
	ctx := context.Background()
	var base api.AutoscalingPolicyResource
	if err := json.Unmarshal([]byte(`{"name":"app","enable":true,"spec":{"recommendationPolicyName":"steady","priority":9,"disableRuntimeOptimization":true,"updateResources":["cpu","memory"],"driftThresholds":{"cpu":"10%","memory":"20%"},"targetRefs":[{"apiVersion":"apps/v1","kind":"Deployment","name":"api","labelSelector":{"matchLabels":{"app":"api"}}}],"updateSchedule":[{"name":"window","schedule":"0 9 * * *","duration":"8h","mode":"inplace"}],"limitPolicies":{"cpu":{"removeLimit":true}},"resourceStartupBoost":{"enabled":true,"recommendationPolicyName":"startup","minBoostDuration":"3m","minReadyDuration":"1m","resourceMultipliers":{"cpu":"2","memory":"2"}},"inPlaceFallback":{"defaultPolicy":"recreate","reasonPolicies":{"JVMHeapDrift":"hold"}}}}`), &base); err != nil {
		t.Fatal(err)
	}
	clear := types.StringValue("")
	configured := api.AutoscalingPolicyModel{
		Name: types.StringValue("app"), RecommendationPolicyName: types.StringValue("steady"),
		Enable: types.BoolValue(false), Priority: types.Int64Value(0), DisableRuntimeOptimization: types.BoolValue(false),
		UpdateResources: &[]types.String{}, DriftThresholdCPU: clear, DriftThresholdMemory: clear,
		TargetRefs: customfield.NewObjectListMust(ctx, []api.TargetRefModel{{
			APIVersion: types.StringValue("apps/v1"), Kind: types.StringValue("Deployment"), Name: types.StringValue("api"),
			LabelSelector: customfield.NewObjectMust(ctx, &api.LabelSelectorModel{}),
		}}),
		UpdateSchedules: customfield.NewObjectListMust(ctx, []api.UpdateScheduleModel{{
			Name: types.StringValue("window"), Schedule: clear, Duration: clear, Mode: types.StringValue("inplace"),
		}}),
		LimitPolicies: customfield.NewObjectListMust(ctx, []api.LimitPolicyModel{{
			Resource: types.StringValue("cpu"), RemoveLimit: types.BoolValue(false), KeepLimit: types.BoolValue(false),
		}}),
		StartupBoostEnabled: types.BoolValue(false), StartupBoostRecommendationPolicyName: clear,
		StartupBoostMinBoostDuration: clear, StartupBoostMinReadyDuration: clear,
		StartupBoostMultiplierCPU: clear, StartupBoostMultiplierMemory: clear,
		InPlaceFallbackReasonPolicies: customfield.NewMapMust[types.String](ctx, map[string]types.String{}),
	}
	updated, err := configured.ToResourceFromBase(ctx, &base)
	if err != nil {
		t.Fatal(err)
	}
	payload := policyJSON(t, updated)
	var remote api.AutoscalingPolicyResource
	if err := json.Unmarshal(payload, &remote); err != nil {
		t.Fatal(err)
	}
	// The server uses metav1.Duration and materializes omitted durations as 0s.
	remote.Spec.ResourceStartupBoost.MinBoostDuration = "0s"
	remote.Spec.ResourceStartupBoost.MinReadyDuration = "0s"
	remote.Spec.UpdateSchedule[0].Duration = "0s"
	data := WorkloadAutoscalerModel{AutoscalingPolicies: customfield.NewObjectListMust(ctx, []api.AutoscalingPolicyModel{configured})}
	if err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{autoscalingPolicies: []api.AutoscalingPolicyResource{remote}}, "cluster", &data); err != nil {
		t.Fatal(err)
	}
	policies, diags := data.AutoscalingPolicies.AsStructSliceT(ctx)
	if diags.HasError() || len(policies) != 1 {
		t.Fatalf("autoscaling policies=%#v diagnostics=%v", policies, diags)
	}
	got := policies[0]
	for name, value := range map[string]types.String{
		"CPU drift": got.DriftThresholdCPU, "memory drift": got.DriftThresholdMemory,
		"startup RP":         got.StartupBoostRecommendationPolicyName,
		"min boost duration": got.StartupBoostMinBoostDuration, "min ready duration": got.StartupBoostMinReadyDuration,
		"CPU multiplier": got.StartupBoostMultiplierCPU, "memory multiplier": got.StartupBoostMultiplierMemory,
	} {
		if !value.Equal(clear) {
			t.Errorf("%s=%v, want explicit empty string", name, value)
		}
	}
	if got.UpdateResources == nil || len(*got.UpdateResources) != 0 || !got.Priority.Equal(types.Int64Value(0)) || !got.Enable.Equal(types.BoolValue(false)) || !got.DisableRuntimeOptimization.Equal(types.BoolValue(false)) || !got.StartupBoostEnabled.Equal(types.BoolValue(false)) {
		t.Fatalf("empty scope, zero or false was lost: %#v", got)
	}
	schedules, diags := got.UpdateSchedules.AsStructSliceT(ctx)
	if diags.HasError() || len(schedules) != 1 || !schedules[0].Schedule.Equal(clear) || !schedules[0].Duration.Equal(clear) {
		t.Fatalf("schedule clear lost: %#v diagnostics=%v", schedules, diags)
	}
	limits, diags := got.LimitPolicies.AsStructSliceT(ctx)
	if diags.HasError() || len(limits) != 1 || !limits[0].RemoveLimit.Equal(types.BoolValue(false)) || !limits[0].KeepLimit.Equal(types.BoolValue(false)) {
		t.Fatalf("explicit false action state lost: %#v diagnostics=%v", limits, diags)
	}
	refs, diags := got.TargetRefs.AsStructSliceT(ctx)
	if diags.HasError() || len(refs) != 1 || refs[0].LabelSelector.IsNull() {
		t.Fatalf("explicit empty selector lost: %#v diagnostics=%v", refs, diags)
	}
	reapplied, err := got.ToResourceFromBase(ctx, &remote)
	if err != nil {
		t.Fatal(err)
	}
	if reapplied.Spec.UpdateResources == nil || len(reapplied.Spec.UpdateResources) != 0 || reapplied.Spec.TargetRefs[0].LabelSelector != nil || len(reapplied.Spec.InPlaceFallback.ReasonPolicies) != 0 || len(reapplied.Spec.DriftThresholds) != 0 || len(reapplied.Spec.ResourceStartupBoost.ResourceMultipliers) != 0 {
		t.Fatalf("reapply resurrected cleared fields: %s", policyJSON(t, reapplied))
	}
	policy := reapplied.Spec.LimitPolicies["cpu"]
	if policy.RemoveLimit != nil || policy.KeepLimit != nil {
		t.Fatalf("explicit false actions must stay omitted for the server CRD: %s", policyJSON(t, reapplied))
	}
}

func TestStartupResourceScopeRejectsEmptyWhileManagedScopeAllowsIt(t *testing.T) {
	ctx := context.Background()
	attrs := autoscalingPolicyAttributes(ctx)
	for _, tc := range []struct {
		name      string
		wantError bool
	}{
		{name: "update_resources"},
		{name: "startup_boost_update_resources", wantError: true},
	} {
		attribute := attrs[tc.name]
		var response validator.ListResponse
		for _, check := range attribute.(interface{ ListValidators() []validator.List }).ListValidators() {
			check.ValidateList(ctx, validator.ListRequest{ConfigValue: types.ListValueMust(types.StringType, nil)}, &response)
		}
		if response.Diagnostics.HasError() != tc.wantError {
			t.Errorf("%s empty-list diagnostics=%v want error=%v", tc.name, response.Diagnostics, tc.wantError)
		}
	}
}

func TestNullMapValuesRemainUnmanagedThroughPolicyHydration(t *testing.T) {
	ctx := context.Background()
	configured := api.AutoscalingPolicyModel{
		Name: types.StringValue("app"),
		TargetRefs: customfield.NewObjectListMust(ctx, []api.TargetRefModel{{
			APIVersion: types.StringValue("apps/v1"), Kind: types.StringValue("Deployment"), Name: types.StringValue("api"),
			LabelSelector: customfield.NewObjectMust(ctx, &api.LabelSelectorModel{
				MatchLabels: customfield.NewMapMust[types.String](ctx, map[string]types.String{
					"app": types.StringNull(), "absent": types.StringNull(), "team": types.StringValue("platform"),
				}),
			}),
		}}),
		InPlaceFallbackReasonPolicies: customfield.NewMapMust[types.String](ctx, map[string]types.String{
			"JVMHeapDrift": types.StringNull(), "QoSChangeForbidden": types.StringNull(), "PodResizePending": types.StringValue("recreate"),
		}),
	}
	base := api.AutoscalingPolicyResource{Name: "app", Spec: api.AutoscalingPolicySpec{
		TargetRefs: []api.TypedObjectReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", LabelSelector: &api.LabelSelector{
			MatchLabels: map[string]string{"app": "api"},
		}}},
		InPlaceFallback: &api.InPlaceFallback{DefaultPolicy: "recreate", ReasonPolicies: map[string]string{"JVMHeapDrift": "hold"}},
	}}
	remote, err := configured.ToResourceFromBase(ctx, &base)
	if err != nil {
		t.Fatal(err)
	}
	data := WorkloadAutoscalerModel{AutoscalingPolicies: customfield.NewObjectListMust(ctx, []api.AutoscalingPolicyModel{configured})}
	if err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{autoscalingPolicies: []api.AutoscalingPolicyResource{*remote}}, "cluster", &data); err != nil {
		t.Fatal(err)
	}
	policies, diags := data.AutoscalingPolicies.AsStructSliceT(ctx)
	if diags.HasError() || len(policies) != 1 {
		t.Fatalf("policies=%#v diagnostics=%v", policies, diags)
	}
	refs, diags := policies[0].TargetRefs.AsStructSliceT(ctx)
	if diags.HasError() || len(refs) != 1 {
		t.Fatalf("refs=%#v diagnostics=%v", refs, diags)
	}
	selector, diags := refs[0].LabelSelector.Value(ctx)
	if diags.HasError() || selector == nil {
		t.Fatalf("selector=%#v diagnostics=%v", selector, diags)
	}
	labels, diags := selector.MatchLabels.Value(ctx)
	if diags.HasError() || !labels["app"].IsNull() || !labels["absent"].IsNull() || labels["team"].ValueString() != "platform" {
		t.Fatalf("label map=%#v diagnostics=%v", labels, diags)
	}
	reasons, diags := policies[0].InPlaceFallbackReasonPolicies.Value(ctx)
	if diags.HasError() || !reasons["JVMHeapDrift"].IsNull() || !reasons["QoSChangeForbidden"].IsNull() || reasons["PodResizePending"].ValueString() != "recreate" {
		t.Fatalf("reason map=%#v diagnostics=%v", reasons, diags)
	}
	reapplied, err := policies[0].ToResourceFromBase(ctx, remote)
	if err != nil {
		t.Fatal(err)
	}
	if reapplied.Spec.TargetRefs[0].LabelSelector.MatchLabels["app"] != "api" || reapplied.Spec.InPlaceFallback.ReasonPolicies["JVMHeapDrift"] != "hold" {
		t.Fatalf("unmanaged existing map value changed: %s", policyJSON(t, reapplied))
	}
	if _, present := reapplied.Spec.TargetRefs[0].LabelSelector.MatchLabels["absent"]; present {
		t.Fatal("an unmanaged absent label was created")
	}
	if _, present := reapplied.Spec.InPlaceFallback.ReasonPolicies["QoSChangeForbidden"]; present {
		t.Fatal("an unmanaged absent reason policy was created")
	}
}

func policyJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
