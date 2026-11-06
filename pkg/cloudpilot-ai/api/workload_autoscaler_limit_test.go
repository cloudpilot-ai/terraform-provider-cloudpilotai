package api

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestLimitPolicyModeSwitchReplacesCompetingActions(t *testing.T) {
	ctx := context.Background()
	actions := []struct {
		name  string
		value any
		model LimitPolicyModel
	}{
		{name: "removeLimit", value: true, model: LimitPolicyModel{RemoveLimit: types.BoolValue(true)}},
		{name: "keepLimit", value: true, model: LimitPolicyModel{KeepLimit: types.BoolValue(true)}},
		{name: "multiplier", value: "2", model: LimitPolicyModel{Multiplier: types.StringValue("2")}},
		{name: "autoHeadroom", value: "1.5", model: LimitPolicyModel{AutoHeadroom: types.StringValue("1.5")}},
	}
	for _, old := range actions {
		for _, next := range actions {
			t.Run(old.name+" to "+next.name, func(t *testing.T) {
				encoded, err := json.Marshal(map[string]any{
					"name": "app", "spec": map[string]any{"limitPolicies": map[string]any{
						"cpu": map[string]any{old.name: old.value, "futureActionOption": false},
					}},
				})
				if err != nil {
					t.Fatal(err)
				}
				var base AutoscalingPolicyResource
				if err := json.Unmarshal(encoded, &base); err != nil {
					t.Fatal(err)
				}
				before, err := json.Marshal(base)
				if err != nil {
					t.Fatal(err)
				}
				item := next.model
				item.Resource = types.StringValue("cpu")
				model := AutoscalingPolicyModel{
					Name:          types.StringValue("app"),
					LimitPolicies: customfield.NewObjectListMust(ctx, []LimitPolicyModel{item}),
				}
				updated, err := model.ToResourceFromBase(ctx, &base)
				if err != nil {
					t.Fatal(err)
				}
				payload, err := json.Marshal(updated)
				if err != nil {
					t.Fatal(err)
				}
				policy := decodePolicySpec(t, payload)["limitPolicies"].(map[string]any)["cpu"]
				want := map[string]any{next.name: next.value, "futureActionOption": false}
				if !reflect.DeepEqual(policy, want) {
					t.Fatalf("limit policy = %#v, want %#v", policy, want)
				}
				after, err := json.Marshal(base)
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) {
					t.Fatal("mode switch mutated the base policy")
				}
			})
		}
	}
}

func TestLimitPolicyWithoutSelectedActionPreservesRemoteMode(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		model LimitPolicyModel
	}{
		{name: "null actions"},
		{name: "unknown actions", model: LimitPolicyModel{
			RemoveLimit: types.BoolUnknown(), KeepLimit: types.BoolUnknown(),
			Multiplier: types.StringUnknown(), AutoHeadroom: types.StringUnknown(),
		}},
		{name: "inactive actions", model: LimitPolicyModel{
			RemoveLimit: types.BoolValue(false), KeepLimit: types.BoolValue(false),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			multiplier := "2"
			base := &AutoscalingPolicyResource{Name: "app", Spec: AutoscalingPolicySpec{
				LimitPolicies: map[string]ResourceLimitPolicy{"cpu": {Multiplier: &multiplier}},
			}}
			tc.model.Resource = types.StringValue("cpu")
			model := AutoscalingPolicyModel{
				Name:          types.StringValue("app"),
				LimitPolicies: customfield.NewObjectListMust(ctx, []LimitPolicyModel{tc.model}),
			}
			got, err := model.ToResourceFromBase(ctx, base)
			if err != nil {
				t.Fatal(err)
			}
			policy := got.Spec.LimitPolicies["cpu"]
			if policy.Multiplier == nil || *policy.Multiplier != "2" {
				t.Fatalf("unmanaged multiplier lost: %#v", policy)
			}
			if policy.RemoveLimit != nil || policy.KeepLimit != nil {
				t.Fatalf("inactive actions must be omitted: %#v", policy)
			}
		})
	}
}

func TestLimitPolicyExplicitDisabledActionClearsOnlyItself(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		raw   string
		model LimitPolicyModel
		want  map[string]any
	}{
		{name: "remove", raw: `{"removeLimit":true}`, model: LimitPolicyModel{RemoveLimit: types.BoolValue(false)}, want: map[string]any{}},
		{name: "keep", raw: `{"keepLimit":true}`, model: LimitPolicyModel{KeepLimit: types.BoolValue(false)}, want: map[string]any{}},
		{name: "multiplier", raw: `{"multiplier":"2"}`, model: LimitPolicyModel{Multiplier: types.StringValue("")}, want: map[string]any{}},
		{name: "headroom", raw: `{"autoHeadroom":"1.5"}`, model: LimitPolicyModel{AutoHeadroom: types.StringValue("")}, want: map[string]any{}},
		{name: "disabled peer", raw: `{"keepLimit":true}`, model: LimitPolicyModel{RemoveLimit: types.BoolValue(false), Multiplier: types.StringValue("")}, want: map[string]any{"keepLimit": true}},
		{name: "selected with disabled peers", raw: `{"keepLimit":true}`, model: LimitPolicyModel{RemoveLimit: types.BoolValue(false), KeepLimit: types.BoolValue(false), Multiplier: types.StringValue("2"), AutoHeadroom: types.StringValue("")}, want: map[string]any{"multiplier": "2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var base AutoscalingPolicyResource
			if err := json.Unmarshal([]byte(`{"name":"app","spec":{"limitPolicies":{"cpu":`+tc.raw+`}}}`), &base); err != nil {
				t.Fatal(err)
			}
			tc.model.Resource = types.StringValue("cpu")
			model := AutoscalingPolicyModel{Name: types.StringValue("app"), LimitPolicies: customfield.NewObjectListMust(ctx, []LimitPolicyModel{tc.model})}
			got, err := model.ToResourceFromBase(ctx, &base)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			policy := decodePolicySpec(t, payload)["limitPolicies"].(map[string]any)["cpu"]
			if !reflect.DeepEqual(policy, tc.want) {
				t.Fatalf("limit policy = %#v, want %#v", policy, tc.want)
			}
		})
	}
}

func TestLimitPolicyRejectsMultipleSelectedActions(t *testing.T) {
	ctx := context.Background()
	for _, item := range []LimitPolicyModel{
		{RemoveLimit: types.BoolValue(true), KeepLimit: types.BoolValue(true)},
		{KeepLimit: types.BoolValue(true), Multiplier: types.StringValue("2")},
		{Multiplier: types.StringValue("2"), AutoHeadroom: types.StringValue("1.5")},
	} {
		item.Resource = types.StringValue("cpu")
		model := AutoscalingPolicyModel{Name: types.StringValue("app"), LimitPolicies: customfield.NewObjectListMust(ctx, []LimitPolicyModel{item})}
		if _, err := model.ToResource(ctx); err == nil || !strings.Contains(err.Error(), "at most one") {
			t.Fatalf("multiple actions error = %v, want mutually exclusive action error", err)
		}
	}
}
