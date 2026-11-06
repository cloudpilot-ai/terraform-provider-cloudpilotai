package api

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestLabelSelectorNullPreservesAndEmptyObjectClears(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		selector customfield.NestedObject[LabelSelectorModel]
		clear    bool
	}{
		{name: "null", selector: customfield.NullObject[LabelSelectorModel](ctx)},
		{name: "unknown", selector: customfield.UnknownObject[LabelSelectorModel](ctx)},
		{name: "empty object", selector: customfield.NewObjectMust(ctx, &LabelSelectorModel{}), clear: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var base AutoscalingPolicyResource
			if err := json.Unmarshal([]byte(`{"name":"app","spec":{"recommendationPolicyName":"steady","targetRefs":[{"apiVersion":"apps/v1","kind":"Deployment","name":"api","labelSelector":{"matchLabels":{"app":"api"},"matchExpressions":[{"key":"tier","operator":"In","values":["backend"]}]}}]}}`), &base); err != nil {
				t.Fatal(err)
			}
			model := AutoscalingPolicyModel{Name: types.StringValue("app"), TargetRefs: customfield.NewObjectListMust(ctx, []TargetRefModel{{
				APIVersion: types.StringValue("apps/v1"), Kind: types.StringValue("Deployment"),
				Name: types.StringValue("api"), LabelSelector: tc.selector,
			}})}
			updated, err := model.ToResourceFromBase(ctx, &base)
			if err != nil {
				t.Fatal(err)
			}
			if tc.clear {
				if updated.Spec.TargetRefs[0].LabelSelector != nil {
					t.Fatal("an explicit empty object must clear the old selector")
				}
			} else if !reflect.DeepEqual(updated.Spec.TargetRefs[0].LabelSelector, base.Spec.TargetRefs[0].LabelSelector) {
				t.Fatal("an unmanaged selector changed")
			}
			payload, err := json.Marshal(updated)
			if err != nil {
				t.Fatal(err)
			}
			ref := decodePolicySpec(t, payload)["targetRefs"].([]any)[0].(map[string]any)
			_, present := ref["labelSelector"]
			if present == tc.clear {
				t.Fatalf("selector presence=%v clear=%v payload=%s", present, tc.clear, payload)
			}
			if base.Spec.TargetRefs[0].LabelSelector == nil {
				t.Fatal("conversion mutated the base selector")
			}
		})
	}
}

func TestAutoscalingPolicyResourceScopesKeepExplicitEmptyJSON(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		values  []string
		present bool
	}{
		{name: "omitted"},
		{name: "empty", values: []string{}, present: true},
		{name: "nonempty", values: []string{"memory"}, present: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := AutoscalingPolicyResource{Name: "app", Spec: AutoscalingPolicySpec{
				UpdateResources:      tc.values,
				ResourceStartupBoost: &WorkloadStartupResourceBoost{UpdateResources: tc.values},
			}}
			// An unrelated update must also preserve an externally stored empty scope.
			model := AutoscalingPolicyModel{Name: types.StringValue("app"), Priority: types.Int64Value(1)}
			updated, err := model.ToResourceFromBase(ctx, &base)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := json.Marshal(updated)
			if err != nil {
				t.Fatal(err)
			}
			spec := decodePolicySpec(t, payload)
			for name, object := range map[string]map[string]any{
				"policy": spec, "startup": spec["resourceStartupBoost"].(map[string]any),
			} {
				value, present := object["updateResources"]
				if present != tc.present {
					t.Fatalf("%s resource scope presence=%v want=%v payload=%s", name, present, tc.present, payload)
				}
				if present {
					resources, ok := value.([]any)
					if !ok || len(resources) != len(tc.values) {
						t.Fatalf("%s resources=%#v want=%#v", name, value, tc.values)
					}
				}
			}
			var remote AutoscalingPolicyResource
			if err := json.Unmarshal(payload, &remote); err != nil {
				t.Fatal(err)
			}
			read := AutoscalingPolicyModelFromResource(ctx, &remote)
			if (read.UpdateResources != nil) != tc.present || (read.StartupBoostUpdateResources != nil) != tc.present {
				t.Fatalf("scope distinction lost during read: %#v", read)
			}
		})
	}
}

func TestAutoscalingPolicyExplicitEmptyScopeReplacesOldResourceList(t *testing.T) {
	ctx := context.Background()
	var base AutoscalingPolicyResource
	if err := json.Unmarshal([]byte(`{"name":"app","spec":{"updateResources":["cpu","memory"],"futureOption":false}}`), &base); err != nil {
		t.Fatal(err)
	}
	model := AutoscalingPolicyModel{Name: types.StringValue("app"), UpdateResources: &[]types.String{}}
	updated, err := model.ToResourceFromBase(ctx, &base)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(updated)
	if err != nil {
		t.Fatal(err)
	}
	spec := decodePolicySpec(t, payload)
	if resources, ok := spec["updateResources"].([]any); !ok || len(resources) != 0 || spec["futureOption"] != false {
		t.Fatalf("empty scope or unknown sibling lost: %s", payload)
	}
}

func TestStartupBoostRejectsExplicitEmptyResourceScope(t *testing.T) {
	ctx := context.Background()
	model := AutoscalingPolicyModel{Name: types.StringValue("app"), StartupBoostUpdateResources: &[]types.String{}}
	for _, base := range []*AutoscalingPolicyResource{
		nil,
		{Name: "app", Spec: AutoscalingPolicySpec{ResourceStartupBoost: &WorkloadStartupResourceBoost{UpdateResources: []string{"memory"}}}},
	} {
		if _, err := model.ToResourceFromBase(ctx, base); err == nil {
			t.Fatal("an explicit empty startup resource list must be rejected before encoding")
		}
	}
}

func TestRecommendationPolicyPercentilesApplyIndependently(t *testing.T) {
	base := &RecommendationPolicyResource{Name: "app", Spec: RecommendationPolicySpec{
		StrategyPercentile: &StrategyPercentileConfiguration{CPU: 80, Memory: 90},
	}}
	for _, tc := range []struct {
		name  string
		model RecommendationPolicyModel
		want  StrategyPercentileConfiguration
	}{
		{name: "CPU with null memory", model: RecommendationPolicyModel{PercentileCPU: types.Int32Value(85)}, want: StrategyPercentileConfiguration{CPU: 85, Memory: 90}},
		{name: "memory with unknown CPU", model: RecommendationPolicyModel{PercentileCPU: types.Int32Unknown(), PercentileMemory: types.Int32Value(95)}, want: StrategyPercentileConfiguration{CPU: 80, Memory: 95}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.model.Name = types.StringValue("app")
			updated := tc.model.ToResourceFromBase(base)
			if *updated.Spec.StrategyPercentile != tc.want {
				t.Fatalf("percentiles=%#v want=%#v", updated.Spec.StrategyPercentile, tc.want)
			}
			if base.Spec.StrategyPercentile.CPU != 80 || base.Spec.StrategyPercentile.Memory != 90 {
				t.Fatal("partial update mutated the base")
			}
		})
	}
	created := (&RecommendationPolicyModel{PercentileCPU: types.Int32Value(85)}).ToResource()
	if created.Spec.StrategyPercentile.CPU != 85 || created.Spec.StrategyPercentile.Memory != 95 {
		t.Fatalf("create defaults=%#v", created.Spec.StrategyPercentile)
	}
}
