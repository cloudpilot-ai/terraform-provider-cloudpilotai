package workloadautoscaler

import (
	"context"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

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

func TestPreserveAutoscalingPolicyStateRepresentationKeepsNestedManagedValues(t *testing.T) {
	ctx := context.Background()
	state := api.AutoscalingPolicyModel{
		Name: types.StringValue("default-ap"),
		UpdateSchedules: customfield.NewObjectListMust(ctx, []api.UpdateScheduleModel{{
			Name:     types.StringValue("business-hours"),
			Schedule: types.StringValue("0 9 * * 1-5"),
			Duration: types.StringValue("8h"),
			Mode:     types.StringValue("inplace"),
		}}),
		LimitPolicies: customfield.NewObjectListMust(ctx, []api.LimitPolicyModel{{
			Resource:     types.StringValue("cpu"),
			RemoveLimit:  types.BoolValue(false),
			KeepLimit:    types.BoolValue(false),
			Multiplier:   types.StringValue("2.0"),
			AutoHeadroom: types.StringValue("1.2"),
		}}),
		StartupBoostEnabled: types.BoolValue(false),
	}
	remote := api.AutoscalingPolicyModel{
		Name: types.StringValue("default-ap"),
		UpdateSchedules: customfield.NewObjectListMust(ctx, []api.UpdateScheduleModel{
			{
				Name:     types.StringValue("business-hours"),
				Schedule: types.StringValue("0 9 * * 1-5"),
				Duration: types.StringValue("8h0m0s"),
				Mode:     types.StringValue("inplace"),
			},
			{
				Name:     types.StringValue("server-only"),
				Schedule: types.StringValue("0 0 * * *"),
				Duration: types.StringValue("1h0m0s"),
				Mode:     types.StringValue("off"),
			},
		}),
		LimitPolicies: customfield.NewObjectListMust(ctx, []api.LimitPolicyModel{
			{
				Resource:     types.StringValue("cpu"),
				RemoveLimit:  types.BoolNull(),
				KeepLimit:    types.BoolNull(),
				Multiplier:   types.StringValue("2.0"),
				AutoHeadroom: types.StringValue("1.2"),
			},
			{
				Resource:    types.StringValue("memory"),
				RemoveLimit: types.BoolValue(true),
			},
		}),
		StartupBoostEnabled: types.BoolNull(),
	}

	got := preserveAutoscalingPolicyStateRepresentation(ctx, remote, state)
	schedules, diags := got.UpdateSchedules.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("UpdateSchedules diagnostics = %v", diags)
	}
	if len(schedules) != 2 || schedules[0].Duration.ValueString() != "8h" || schedules[1].Name.ValueString() != "server-only" {
		t.Fatalf("UpdateSchedules = %#v, want preserved state item followed by unmatched remote item", schedules)
	}

	policies, diags := got.LimitPolicies.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("LimitPolicies diagnostics = %v", diags)
	}
	if len(policies) != 2 || policies[1].Resource.ValueString() != "memory" {
		t.Fatalf("LimitPolicies = %#v, want preserved state item followed by unmatched remote item", policies)
	}
	if policies[0].RemoveLimit.IsNull() || policies[0].RemoveLimit.ValueBool() {
		t.Fatalf("RemoveLimit = %#v, want explicit false", policies[0].RemoveLimit)
	}
	if policies[0].KeepLimit.IsNull() || policies[0].KeepLimit.ValueBool() {
		t.Fatalf("KeepLimit = %#v, want explicit false", policies[0].KeepLimit)
	}
	if got.StartupBoostEnabled.IsNull() || got.StartupBoostEnabled.ValueBool() {
		t.Fatalf("StartupBoostEnabled = %#v, want explicit false", got.StartupBoostEnabled)
	}
}

func TestPreserveAutoscalingPolicyStateRepresentationKeepsTargetRefNestedCollections(t *testing.T) {
	ctx := context.Background()
	emptyValues := []types.String{}
	state := api.AutoscalingPolicyModel{
		TargetRefs: customfield.NewObjectListMust(ctx, []api.TargetRefModel{
			{
				APIVersion: types.StringValue("apps/v1"),
				Kind:       types.StringValue("Deployment"),
				Namespace:  types.StringValue("demo"),
				LabelSelector: customfield.NewObjectMust(ctx, &api.LabelSelectorModel{
					MatchLabels: customfield.NewMapMust[types.String](ctx, map[string]types.String{
						"autoscaling": types.StringValue("cloudpilot"),
					}),
					MatchExpressions: customfield.NullObjectList[api.LabelSelectorRequirementModel](ctx),
				}),
			},
			{
				APIVersion: types.StringValue("apps/v1"),
				Kind:       types.StringValue("StatefulSet"),
				Name:       types.StringValue("empty-expressions"),
				Namespace:  types.StringValue("demo"),
				LabelSelector: customfield.NewObjectMust(ctx, &api.LabelSelectorModel{
					MatchLabels:      customfield.NullMap[types.String](ctx),
					MatchExpressions: customfield.NewObjectListMust(ctx, []api.LabelSelectorRequirementModel{}),
				}),
			},
			{
				APIVersion: types.StringValue("apps/v1"),
				Kind:       types.StringValue("StatefulSet"),
				Name:       types.StringValue("empty-values"),
				Namespace:  types.StringValue("demo"),
				LabelSelector: customfield.NewObjectMust(ctx, &api.LabelSelectorModel{
					MatchLabels: customfield.NullMap[types.String](ctx),
					MatchExpressions: customfield.NewObjectListMust(ctx, []api.LabelSelectorRequirementModel{{
						Key:      types.StringValue("autoscaling"),
						Operator: types.StringValue("Exists"),
						Values:   &emptyValues,
					}}),
				}),
			},
		}),
	}
	remote := api.AutoscalingPolicyModel{
		TargetRefs: customfield.NewObjectListMust(ctx, []api.TargetRefModel{
			{
				APIVersion: types.StringValue("apps/v1"),
				Kind:       types.StringValue("Deployment"),
				Namespace:  types.StringValue("demo"),
				LabelSelector: customfield.NewObjectMust(ctx, &api.LabelSelectorModel{
					MatchLabels: customfield.NewMapMust[types.String](ctx, map[string]types.String{
						"autoscaling": types.StringValue("cloudpilot"),
					}),
					MatchExpressions: customfield.NewObjectListMust(ctx, []api.LabelSelectorRequirementModel{}),
				}),
			},
			{
				APIVersion:    types.StringValue("apps/v1"),
				Kind:          types.StringValue("StatefulSet"),
				Name:          types.StringValue("empty-expressions"),
				Namespace:     types.StringValue("demo"),
				LabelSelector: customfield.NullObject[api.LabelSelectorModel](ctx),
			},
			{
				APIVersion: types.StringValue("apps/v1"),
				Kind:       types.StringValue("StatefulSet"),
				Name:       types.StringValue("empty-values"),
				Namespace:  types.StringValue("demo"),
				LabelSelector: customfield.NewObjectMust(ctx, &api.LabelSelectorModel{
					MatchLabels: customfield.NullMap[types.String](ctx),
					MatchExpressions: customfield.NewObjectListMust(ctx, []api.LabelSelectorRequirementModel{{
						Key:      types.StringValue("autoscaling"),
						Operator: types.StringValue("Exists"),
					}}),
				}),
			},
			{
				APIVersion: types.StringValue("apps/v1"),
				Kind:       types.StringValue("Deployment"),
				Name:       types.StringValue("server-only"),
				Namespace:  types.StringValue("demo"),
			},
		}),
	}

	got := preserveAutoscalingPolicyStateRepresentation(ctx, remote, state)
	targetRefs, diags := got.TargetRefs.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("TargetRefs diagnostics = %v", diags)
	}
	if len(targetRefs) != 4 || targetRefs[3].Name.ValueString() != "server-only" {
		t.Fatalf("TargetRefs = %#v, want three state refs followed by unmatched remote ref", targetRefs)
	}

	selector, diags := targetRefs[0].LabelSelector.Value(ctx)
	if diags.HasError() || selector == nil {
		t.Fatalf("first LabelSelector diagnostics = %v, value = %#v", diags, selector)
	}
	if !selector.MatchExpressions.IsNull() {
		t.Fatalf("first MatchExpressions = %#v, want null", selector.MatchExpressions)
	}

	selector, diags = targetRefs[1].LabelSelector.Value(ctx)
	if diags.HasError() || selector == nil {
		t.Fatalf("second LabelSelector diagnostics = %v, value = %#v", diags, selector)
	}
	if selector.MatchExpressions.IsNull() {
		t.Fatalf("second MatchExpressions should preserve explicit empty list")
	}

	selector, diags = targetRefs[2].LabelSelector.Value(ctx)
	if diags.HasError() || selector == nil {
		t.Fatalf("third LabelSelector diagnostics = %v, value = %#v", diags, selector)
	}
	expressions, diags := selector.MatchExpressions.AsStructSliceT(ctx)
	if diags.HasError() || len(expressions) != 1 {
		t.Fatalf("third MatchExpressions diagnostics = %v, value = %#v", diags, expressions)
	}
	if expressions[0].Values == nil || len(*expressions[0].Values) != 0 {
		t.Fatalf("Values = %#v, want explicit empty list", expressions[0].Values)
	}
}

func TestPreserveAutoscalingPolicyStateRepresentationKeepsEquivalentStartupBoostDurations(t *testing.T) {
	state := api.AutoscalingPolicyModel{
		StartupBoostEnabled:          types.BoolValue(true),
		StartupBoostMinBoostDuration: types.StringValue("5m"),
		StartupBoostMinReadyDuration: types.StringValue("3m"),
	}
	remote := api.AutoscalingPolicyModel{
		StartupBoostEnabled:          types.BoolValue(true),
		StartupBoostMinBoostDuration: types.StringValue("5m0s"),
		StartupBoostMinReadyDuration: types.StringValue("3m0s"),
	}

	got := preserveAutoscalingPolicyStateRepresentation(context.Background(), remote, state)
	if got.StartupBoostMinBoostDuration.ValueString() != "5m" {
		t.Fatalf("StartupBoostMinBoostDuration = %#v, want 5m", got.StartupBoostMinBoostDuration)
	}
	if got.StartupBoostMinReadyDuration.ValueString() != "3m" {
		t.Fatalf("StartupBoostMinReadyDuration = %#v, want 3m", got.StartupBoostMinReadyDuration)
	}
}

func TestPreserveRecommendationPolicyStateRepresentationKeepsExplicitEmptyJVMMinHeapXms(t *testing.T) {
	state := api.RecommendationPolicyModel{JVMMinHeapXms: types.StringValue("")}
	remote := api.RecommendationPolicyModel{JVMMinHeapXms: types.StringNull()}

	got := preserveRecommendationPolicyStateRepresentation(remote, state)
	if got.JVMMinHeapXms != types.StringValue("") {
		t.Fatalf("JVMMinHeapXms = %#v, want explicit empty string", got.JVMMinHeapXms)
	}
}

func TestStateRepresentationPreservesManagedParityFields(t *testing.T) {
	ctx := context.Background()
	state := api.AutoscalingPolicyModel{StartupBoostRecommendationPolicyName: types.StringValue(""), StartupBoostUpdateResources: &[]types.String{types.StringValue("memory")}, TargetRefs: customfield.NewObjectListMust(ctx, []api.TargetRefModel{{APIVersion: types.StringValue("apps/v1"), Kind: types.StringValue("Deployment"), Name: types.StringValue("app")}})}
	remote := api.AutoscalingPolicyModel{StartupBoostRecommendationPolicyName: types.StringNull(), StartupBoostUpdateResources: &[]types.String{types.StringValue("memory")}, TargetRefs: customfield.NewObjectListMust(ctx, []api.TargetRefModel{{APIVersion: types.StringValue("apps/v1"), Kind: types.StringValue("Deployment"), Name: types.StringValue("app")}})}
	got := preserveAutoscalingPolicyStateRepresentation(ctx, remote, state)
	if got.StartupBoostRecommendationPolicyName != state.StartupBoostRecommendationPolicyName || got.StartupBoostUpdateResources == nil {
		t.Fatal("managed startup representation lost")
	}
	recommendation := api.RecommendationPolicyModel{JVMMaxHeapXms: types.StringValue(""), JVMMinHeapXmx: types.StringValue(""), JVMMaxHeapXmx: types.StringValue("")}
	gotRecommendation := preserveRecommendationPolicyStateRepresentation(api.RecommendationPolicyModel{}, recommendation)
	if gotRecommendation.JVMMaxHeapXms.IsNull() || gotRecommendation.JVMMinHeapXmx.IsNull() || gotRecommendation.JVMMaxHeapXmx.IsNull() {
		t.Fatal("explicit JVM bound removals lost")
	}
	unmanaged := preserveRecommendationPolicyStateRepresentation(api.RecommendationPolicyModel{JVMMaxHeapXmx: types.StringValue("4Gi")}, api.RecommendationPolicyModel{})
	if !unmanaged.JVMMaxHeapXmx.IsNull() {
		t.Fatal("unmanaged JVM bound leaked into state")
	}
}

func TestHydratePostWriteStatePreservesClearedLimitPolicyStrings(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		policy api.LimitPolicyModel
	}{
		{name: "multiplier", policy: api.LimitPolicyModel{Multiplier: types.StringValue("")}},
		{name: "auto headroom", policy: api.LimitPolicyModel{AutoHeadroom: types.StringValue("")}},
		{name: "both", policy: api.LimitPolicyModel{Multiplier: types.StringValue(""), AutoHeadroom: types.StringValue("")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.policy.Resource = types.StringValue("cpu")
			data := WorkloadAutoscalerModel{
				AutoscalingPolicies: customfield.NewObjectListMust(ctx, []api.AutoscalingPolicyModel{{
					Name:          types.StringValue("app"),
					LimitPolicies: customfield.NewObjectListMust(ctx, []api.LimitPolicyModel{tc.policy}),
				}}),
			}
			remote := api.AutoscalingPolicyResource{Name: "app", Spec: api.AutoscalingPolicySpec{
				LimitPolicies: map[string]api.ResourceLimitPolicy{"cpu": {}},
			}}
			client := &fakePostWriteStateHydratorClient{autoscalingPolicies: []api.AutoscalingPolicyResource{remote}}
			if err := hydratePostWriteState(ctx, client, "cluster", &data); err != nil {
				t.Fatal(err)
			}
			policies, diags := data.AutoscalingPolicies.AsStructSliceT(ctx)
			if diags.HasError() || len(policies) != 1 {
				t.Fatalf("autoscaling policies = %#v, diagnostics = %v", policies, diags)
			}
			limits, diags := policies[0].LimitPolicies.AsStructSliceT(ctx)
			if diags.HasError() || len(limits) != 1 {
				t.Fatalf("limit policies = %#v, diagnostics = %v", limits, diags)
			}
			if limits[0].Multiplier != tc.policy.Multiplier || limits[0].AutoHeadroom != tc.policy.AutoHeadroom {
				t.Fatalf("limit policy = %#v, want cleared strings preserved as %#v", limits[0], tc.policy)
			}
			updated, err := policies[0].ToResourceFromBase(ctx, &remote)
			if err != nil {
				t.Fatal(err)
			}
			limit := updated.Spec.LimitPolicies["cpu"]
			if limit.Multiplier != nil || limit.AutoHeadroom != nil {
				t.Fatalf("cleared actions must remain omitted after hydration: %#v", limit)
			}
		})
	}
}
