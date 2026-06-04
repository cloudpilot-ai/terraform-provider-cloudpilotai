package api

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func TestRecommendationPolicyModelJVMRoundTrip(t *testing.T) {
	model := RecommendationPolicyModel{
		Name:                       types.StringValue("java"),
		StrategyType:               types.StringValue("percentile"),
		PercentileCPU:              types.Int32Value(95),
		PercentileMemory:           types.Int32Value(99),
		HistoryWindowCPU:           types.StringValue("24h"),
		HistoryWindowMemory:        types.StringValue("48h"),
		EvaluationPeriod:           types.StringValue("1m"),
		JVMHeapBuffer:              types.StringValue("300Mi"),
		JVMMinHeapXmsRatioOfMemory: types.StringValue("0.5"),
		JVMRecentNonHeapWindow:     types.StringValue("2h"),
		JVMHeapUsedPercentile:      types.Int32Value(30),
	}

	resource := model.ToResource()
	if resource.Spec.JVM == nil {
		t.Fatalf("Spec.JVM is nil")
	}
	if resource.Spec.JVM.HeapBuffer == nil || *resource.Spec.JVM.HeapBuffer != "300Mi" {
		t.Fatalf("HeapBuffer = %#v", resource.Spec.JVM.HeapBuffer)
	}
	if resource.Spec.JVM.MinHeapXmsRatioOfMemory == nil || *resource.Spec.JVM.MinHeapXmsRatioOfMemory != "0.5" {
		t.Fatalf("MinHeapXmsRatioOfMemory = %#v", resource.Spec.JVM.MinHeapXmsRatioOfMemory)
	}

	roundTrip := RecommendationPolicyModelFromResource(resource)
	if roundTrip.JVMHeapBuffer.ValueString() != "300Mi" {
		t.Fatalf("JVMHeapBuffer = %q", roundTrip.JVMHeapBuffer.ValueString())
	}
	if roundTrip.JVMHeapUsedPercentile.ValueInt32() != 30 {
		t.Fatalf("JVMHeapUsedPercentile = %d", roundTrip.JVMHeapUsedPercentile.ValueInt32())
	}
}

func TestAutoscalingPolicyModelRoundTripsFrontendFields(t *testing.T) {
	ctx := context.Background()
	model := AutoscalingPolicyModel{
		Name:                       types.StringValue("default-ap"),
		Enable:                     types.BoolValue(true),
		RecommendationPolicyName:   types.StringValue("balanced"),
		Priority:                   types.Int64Value(10),
		DisableRuntimeOptimization: types.BoolValue(true),
		TargetRefs: customfield.NewObjectListMust(ctx, []TargetRefModel{{
			APIVersion: types.StringValue("apps/v1"),
			Kind:       types.StringValue("Deployment"),
			LabelSelector: customfield.NewObjectMust(ctx, &LabelSelectorModel{
				MatchLabels: customfield.NewMapMust[types.String](ctx, map[string]types.String{
					"app": types.StringValue("api"),
				}),
				MatchExpressions: customfield.NewObjectListMust(ctx, []LabelSelectorRequirementModel{{
					Key:      types.StringValue("tier"),
					Operator: types.StringValue("In"),
					Values:   &[]types.String{types.StringValue("backend")},
				}}),
			}),
		}}),
		InPlaceFallbackDefaultPolicy: types.StringValue("recreate"),
		InPlaceFallbackReasonPolicies: customfield.NewMapMust[types.String](ctx, map[string]types.String{
			"JVMHeapDrift": types.StringValue("hold"),
		}),
	}

	resource, err := model.ToResource(ctx)
	if err != nil {
		t.Fatalf("ToResource() error = %v", err)
	}
	if !resource.Spec.DisableRuntimeOptimization {
		t.Fatalf("DisableRuntimeOptimization should be true")
	}
	if len(resource.Spec.TargetRefs) != 1 || resource.Spec.TargetRefs[0].LabelSelector == nil {
		t.Fatalf("TargetRefs = %#v", resource.Spec.TargetRefs)
	}
	if resource.Spec.TargetRefs[0].LabelSelector.MatchLabels["app"] != "api" {
		t.Fatalf("MatchLabels = %#v", resource.Spec.TargetRefs[0].LabelSelector.MatchLabels)
	}
	if resource.Spec.InPlaceFallback == nil || resource.Spec.InPlaceFallback.ReasonPolicies["JVMHeapDrift"] != "hold" {
		t.Fatalf("InPlaceFallback = %#v", resource.Spec.InPlaceFallback)
	}

	roundTrip := AutoscalingPolicyModelFromResource(ctx, resource)
	if !roundTrip.DisableRuntimeOptimization.ValueBool() {
		t.Fatalf("DisableRuntimeOptimization did not round-trip")
	}
	reasonPolicies, diags := roundTrip.InPlaceFallbackReasonPolicies.Value(ctx)
	if diags.HasError() {
		t.Fatalf("ReasonPolicies diagnostics = %v", diags)
	}
	if reasonPolicies["JVMHeapDrift"].ValueString() != "hold" {
		t.Fatalf("ReasonPolicies = %#v", reasonPolicies)
	}
}

func TestAutoscalingPolicyModelFromResourceLeavesAbsentReasonPoliciesNull(t *testing.T) {
	model := AutoscalingPolicyModelFromResource(context.Background(), &AutoscalingPolicyResource{
		Name:   "default-ap",
		Enable: true,
		Spec: AutoscalingPolicySpec{
			RecommendationPolicyName: "balanced",
			OnPolicyRemoval:          "off",
		},
	})

	if !model.InPlaceFallbackReasonPolicies.IsNull() {
		t.Fatalf("InPlaceFallbackReasonPolicies should be null when the backend does not return any fallback policies")
	}
}
