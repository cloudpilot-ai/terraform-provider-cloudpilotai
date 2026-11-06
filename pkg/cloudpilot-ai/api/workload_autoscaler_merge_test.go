package api

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestAutoscalingPolicyPartialUpdatesKeepUnmanagedChildren(t *testing.T) {
	ctx := context.Background()
	var base AutoscalingPolicyResource
	raw := `{"name":"app","enable":true,"spec":{"recommendationPolicyName":"steady","driftThresholds":{"cpu":"10%","memory":"20%"},"targetRefs":[{"apiVersion":"apps/v1","kind":"Deployment","name":"api","namespace":"production","labelSelector":{"matchLabels":{"app":"api"},"matchExpressions":[{"key":"tier","operator":"In","values":["backend"]}]}}],"updateSchedule":[{"name":"business","schedule":"0 9 * * 1-5","duration":"8h","mode":"inplace"}],"limitPolicies":{"cpu":{"keepLimit":false,"multiplier":"2"}},"inPlaceFallback":{"defaultPolicy":"recreate","reasonPolicies":{"JVMHeapDrift":"hold"}}}}`
	if err := json.Unmarshal([]byte(raw), &base); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(base)
	model := AutoscalingPolicyModel{
		Name: types.StringValue("app"), DriftThresholdCPU: types.StringValue("15%"),
		TargetRefs: customfield.NewObjectListMust(ctx, []TargetRefModel{{
			APIVersion: types.StringValue("apps/v1"), Kind: types.StringValue("Deployment"),
			LabelSelector: customfield.NullObject[LabelSelectorModel](ctx),
		}}),
		UpdateSchedules:              customfield.NewObjectListMust(ctx, []UpdateScheduleModel{{Name: types.StringValue("business"), Mode: types.StringValue("off")}}),
		LimitPolicies:                customfield.NewObjectListMust(ctx, []LimitPolicyModel{{Resource: types.StringValue("cpu"), KeepLimit: types.BoolValue(false)}}),
		InPlaceFallbackDefaultPolicy: types.StringValue("hold"),
	}
	updated, err := model.ToResourceFromBase(ctx, &base)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Spec.DriftThresholds["cpu"] != "15%" || updated.Spec.DriftThresholds["memory"] != "20%" {
		t.Fatalf("drift thresholds = %#v", updated.Spec.DriftThresholds)
	}
	if !reflect.DeepEqual(updated.Spec.TargetRefs, base.Spec.TargetRefs) {
		t.Fatalf("null target children modified remote: %#v", updated.Spec.TargetRefs)
	}
	item := updated.Spec.UpdateSchedule[0]
	if item.Mode != "off" || item.Schedule != "0 9 * * 1-5" || item.Duration != "8h" {
		t.Fatalf("schedule children lost: %#v", item)
	}
	policy := updated.Spec.LimitPolicies["cpu"]
	if policy.Multiplier == nil || *policy.Multiplier != "2" || policy.KeepLimit != nil {
		t.Fatalf("limit children lost: %#v", policy)
	}
	fallback := updated.Spec.InPlaceFallback
	if fallback.DefaultPolicy != "hold" || fallback.ReasonPolicies["JVMHeapDrift"] != "hold" {
		t.Fatalf("fallback children lost: %#v", fallback)
	}
	after, _ := json.Marshal(base)
	if string(before) != string(after) {
		t.Fatal("partial update mutated the base policy")
	}
}

func TestLabelSelectorPartialUpdateKeepsNullCollectionsAndValues(t *testing.T) {
	ctx := context.Background()
	base := &LabelSelector{MatchLabels: map[string]string{"app": "api"}, MatchExpressions: []LabelSelectorRequirement{{Key: "tier", Operator: "In", Values: []string{"backend"}}}}
	model := customfield.NewObjectMust(ctx, &LabelSelectorModel{
		MatchExpressions: customfield.NewObjectListMust(ctx, []LabelSelectorRequirementModel{{Key: types.StringValue("tier"), Operator: types.StringValue("In")}}),
	})
	got, err := labelSelectorModelToAPIFromBase(ctx, model, base)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("null selector children modified remote: %#v", got)
	}
	model = customfield.NewObjectMust(ctx, &LabelSelectorModel{MatchLabels: customfield.NewMapMust[types.String](ctx, map[string]types.String{"app": types.StringValue("new")})})
	got, err = labelSelectorModelToAPIFromBase(ctx, model, base)
	if err != nil {
		t.Fatal(err)
	}
	if got.MatchLabels["app"] != "new" || !reflect.DeepEqual(got.MatchExpressions, base.MatchExpressions) || base.MatchLabels["app"] != "api" {
		t.Fatalf("partial selector update lost expressions or mutated base: %#v", got)
	}
}

func TestFallbackReasonUpdatesKeepNullDefaultAndClearExplicitEmptyMap(t *testing.T) {
	ctx := context.Background()
	base := &AutoscalingPolicyResource{Name: "app", Spec: AutoscalingPolicySpec{InPlaceFallback: &InPlaceFallback{DefaultPolicy: "recreate", ReasonPolicies: map[string]string{"JVMHeapDrift": "hold"}}}}
	model := AutoscalingPolicyModel{Name: types.StringValue("app"), InPlaceFallbackReasonPolicies: customfield.NewMapMust[types.String](ctx, map[string]types.String{})}
	got, err := model.ToResourceFromBase(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.InPlaceFallback.DefaultPolicy != "recreate" || len(got.Spec.InPlaceFallback.ReasonPolicies) != 0 {
		t.Fatalf("fallback = %#v", got.Spec.InPlaceFallback)
	}
}
