package api

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRecommendationPolicyUpdatesPreserveModernAndUnknownFields(t *testing.T) {
	raw := `{"name":"java","spec":{"strategyType":"percentile","historyWindowDuration":{"cpu":"24h","memory":"24h"},"evaluationPeriod":"1m","jvm":{"minHeapXms":"25%","maxHeapXms":"1Gi","minHeapXmx":"512Mi","maxHeapXmx":"4Gi","futureFloor":{"enabled":false,"count":0}},"futurePolicy":null}}`
	var base RecommendationPolicyResource
	if err := json.Unmarshal([]byte(raw), &base); err != nil {
		t.Fatal(err)
	}
	model := RecommendationPolicyModelFromResource(&base)
	if model.JVMMaxHeapXms.ValueString() != "1Gi" || model.JVMMinHeapXmx.ValueString() != "512Mi" || model.JVMMaxHeapXmx.ValueString() != "4Gi" {
		t.Fatalf("heap fields missing in read: %#v", model)
	}
	model.BufferCPU = types.StringValue("10%")
	model.JVMMinHeapXmx = types.StringValue("")
	updated := model.ToResourceFromBase(&base)
	encoded, err := json.Marshal(updated)
	if err != nil {
		t.Fatal(err)
	}
	spec := decodePolicySpec(t, encoded)
	jvm := spec["jvm"].(map[string]any)
	if jvm["maxHeapXms"] != "1Gi" || jvm["maxHeapXmx"] != "4Gi" || jvm["minHeapXms"] != "25%" {
		t.Fatalf("modern heap fields lost: %s", encoded)
	}
	if _, ok := jvm["minHeapXmx"]; ok {
		t.Fatalf("cleared known field restored: %s", encoded)
	}
	if !reflect.DeepEqual(jvm["futureFloor"], map[string]any{"enabled": false, "count": float64(0)}) {
		t.Fatalf("future JVM fields lost: %s", encoded)
	}
	if value, ok := spec["futurePolicy"]; !ok || value != nil {
		t.Fatalf("unknown null lost: %s", encoded)
	}
	if base.Spec.JVM.MinHeapXmx == nil {
		t.Fatal("update mutated base policy")
	}
}

func TestAutoscalingPolicyUpdatesPreserveStartupPolicyAndTargetFields(t *testing.T) {
	ctx := context.Background()
	raw := `{"name":"application","enable":true,"spec":{"recommendationPolicyName":"steady","priority":9,"disableRuntimeOptimization":true,"targetRefs":[{"apiVersion":"apps/v1","kind":"Deployment","name":"app","namespace":"test","futureTarget":true}],"resourceStartupBoost":{"enabled":true,"recommendationPolicyName":"startup","updateResources":["memory"],"minBoostDuration":"3m","futureBoost":false},"futureSpec":0}}`
	var base AutoscalingPolicyResource
	if err := json.Unmarshal([]byte(raw), &base); err != nil {
		t.Fatal(err)
	}
	model := AutoscalingPolicyModelFromResource(ctx, &base)
	if model.StartupBoostRecommendationPolicyName.ValueString() != "startup" || model.StartupBoostUpdateResources == nil {
		t.Fatal("startup fields missing in read")
	}
	model.Priority = types.Int64Value(0)
	model.DisableRuntimeOptimization = types.BoolValue(false)
	refs, diags := model.TargetRefs.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatal(diags)
	}
	model.TargetRefs = customfield.NewObjectListMust(ctx, refs)
	updated, err := model.ToResourceFromBase(ctx, &base)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(updated)
	if err != nil {
		t.Fatal(err)
	}
	spec := decodePolicySpec(t, encoded)
	boost := spec["resourceStartupBoost"].(map[string]any)
	if boost["recommendationPolicyName"] != "startup" || !reflect.DeepEqual(boost["updateResources"], []any{"memory"}) || boost["futureBoost"] != false {
		t.Fatalf("startup behavior lost: %s", encoded)
	}
	ref := spec["targetRefs"].([]any)[0].(map[string]any)
	if ref["futureTarget"] != true {
		t.Fatalf("target fields lost: %s", encoded)
	}
	if _, ok := spec["priority"]; ok {
		t.Fatalf("zero priority resurrected: %s", encoded)
	}
	if _, ok := spec["disableRuntimeOptimization"]; ok {
		t.Fatalf("false flag resurrected: %s", encoded)
	}
	if spec["futureSpec"] != float64(0) {
		t.Fatalf("future zero lost: %s", encoded)
	}
}

func TestStartupBoostCanBeUpdatedWithoutEnablingOrResettingOtherFields(t *testing.T) {
	ctx := context.Background()
	base := &AutoscalingPolicyResource{Name: "app", Spec: AutoscalingPolicySpec{ResourceStartupBoost: &WorkloadStartupResourceBoost{Enabled: true, RecommendationPolicyName: "startup", UpdateResources: []string{"memory"}, MinBoostDuration: "3m"}}}
	model := AutoscalingPolicyModel{Name: types.StringValue("app"), RecommendationPolicyName: types.StringValue("steady"), StartupBoostMinReadyDuration: types.StringValue("30s")}
	updated, err := model.ToResourceFromBase(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	boost := updated.Spec.ResourceStartupBoost
	if !boost.Enabled || boost.RecommendationPolicyName != "startup" || boost.MinBoostDuration != "3m" || boost.MinReadyDuration != "30s" {
		t.Fatalf("partial startup update reset fields: %#v", boost)
	}
	model.StartupBoostEnabled = types.BoolValue(false)
	updated, err = model.ToResourceFromBase(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Spec.ResourceStartupBoost.Enabled || updated.Spec.ResourceStartupBoost.RecommendationPolicyName != "startup" {
		t.Fatalf("disable lost configuration: %#v", updated.Spec.ResourceStartupBoost)
	}
	if !base.Spec.ResourceStartupBoost.Enabled {
		t.Fatal("update mutated base startup config")
	}
}

func TestProactiveStartupBoostFilterIncludesExplicitFalse(t *testing.T) {
	model := EnableProactiveModel{StartupBoostEnabled: types.BoolValue(false)}
	request := model.ToRequest()
	if request.ListFilter.StartupBoostEnabled == nil || *request.ListFilter.StartupBoostEnabled {
		t.Fatal("explicit false startup filter was dropped")
	}
	disabled := DisableProactiveModel{StartupBoostEnabled: model.StartupBoostEnabled}
	if !reflect.DeepEqual(request.ListFilter, disabled.ToRequest().ListFilter) {
		t.Fatal("enable/disable filter mappings differ")
	}
}

func decodePolicySpec(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	return object["spec"].(map[string]any)
}

func TestRecommendationPolicyRetainsUnknownOnlyJVMConfiguration(t *testing.T) {
	var base RecommendationPolicyResource
	if err := json.Unmarshal([]byte(`{"name":"java","spec":{"jvm":{"futureKnob":false}}}`), &base); err != nil {
		t.Fatal(err)
	}
	model := RecommendationPolicyModel{Name: types.StringValue("java")}
	encoded, err := json.Marshal(model.ToResourceFromBase(&base))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodePolicySpec(t, encoded)["jvm"], map[string]any{"futureKnob": false}) {
		t.Fatalf("unknown-only JVM configuration was erased: %s", encoded)
	}
}

func TestAutoscalingPolicyRawJSONRetainsUnknownFieldsAcrossSelectorReordering(t *testing.T) {
	ctx := context.Background()
	raw := `{"name":"application","enable":true,"spec":{"recommendationPolicyName":"steady","targetRefs":[{"apiVersion":"apps/v1","kind":"Deployment","labelSelector":{"matchLabels":{"app":"a"}},"futureTarget":"a"},{"apiVersion":"apps/v1","kind":"Deployment","labelSelector":{"matchLabels":{"app":"b"}},"futureTarget":"b"}]}}`
	var base AutoscalingPolicyResource
	if err := json.Unmarshal([]byte(raw), &base); err != nil {
		t.Fatal(err)
	}
	model := AutoscalingPolicyModelFromResource(ctx, &base)
	refs, diags := model.TargetRefs.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatal(diags)
	}
	model.TargetRefs = customfield.NewObjectListMust(ctx, []TargetRefModel{refs[1], refs[0]})
	updated, err := model.ToResourceFromBase(ctx, &base)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(updated)
	if err != nil {
		t.Fatal(err)
	}
	targets := decodePolicySpec(t, encoded)["targetRefs"].([]any)
	if targets[0].(map[string]any)["futureTarget"] != "b" || targets[1].(map[string]any)["futureTarget"] != "a" {
		t.Fatalf("target fields attached to the wrong selector: %s", encoded)
	}
}
