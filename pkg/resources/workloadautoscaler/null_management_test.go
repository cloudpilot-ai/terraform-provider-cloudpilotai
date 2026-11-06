package workloadautoscaler

import (
	"context"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSyncPoliciesNullAndUnknownListsLeaveTrackedPoliciesRemote(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name        string
		data        WorkloadAutoscalerModel
		wantDeletes int
	}{
		{name: "null", data: WorkloadAutoscalerModel{
			RecommendationPolicies: customfield.NullObjectList[api.RecommendationPolicyModel](ctx),
			AutoscalingPolicies:    customfield.NullObjectList[api.AutoscalingPolicyModel](ctx),
		}},
		{name: "unknown", data: WorkloadAutoscalerModel{
			RecommendationPolicies: customfield.UnknownObjectList[api.RecommendationPolicyModel](ctx),
			AutoscalingPolicies:    customfield.UnknownObjectList[api.AutoscalingPolicyModel](ctx),
		}},
		{name: "empty", data: WorkloadAutoscalerModel{
			RecommendationPolicies: customfield.NewObjectListMust(ctx, []api.RecommendationPolicyModel{}),
			AutoscalingPolicies:    customfield.NewObjectListMust(ctx, []api.AutoscalingPolicyModel{}),
		}, wantDeletes: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeWorkloadAutoscalerClient{}
			w := WorkloadAutoscaler{client: client}
			if err := w.syncPolicies(ctx, &test.data, "cluster", map[string]struct{}{"rp": {}}, map[string]struct{}{"ap": {}}); err != nil {
				t.Fatal(err)
			}
			if len(client.deletedRPs) != test.wantDeletes || len(client.deletedAPs) != test.wantDeletes {
				t.Fatalf("deleted RPs=%v APs=%v, want %d each", client.deletedRPs, client.deletedAPs, test.wantDeletes)
			}
		})
	}
}

func TestTargetReferenceRefreshPreservesNullAndShowsEmptySelectorDrift(t *testing.T) {
	ctx := context.Background()
	for _, selector := range []customfield.NestedObject[api.LabelSelectorModel]{
		customfield.NullObject[api.LabelSelectorModel](ctx),
		customfield.NewObjectMust(ctx, &api.LabelSelectorModel{}),
	} {
		state := customfield.NewObjectListMust(ctx, []api.TargetRefModel{{
			APIVersion: types.StringValue("apps/v1"), Kind: types.StringValue("Deployment"), LabelSelector: selector,
		}})
		remote := customfield.NewObjectListMust(ctx, []api.TargetRefModel{{
			APIVersion: types.StringValue("apps/v1"), Kind: types.StringValue("Deployment"),
			Name: types.StringValue("api"), Namespace: types.StringValue("production"),
			LabelSelector: customfield.NewObjectMust(ctx, &api.LabelSelectorModel{
				MatchLabels: customfield.NewMapMust[types.String](ctx, map[string]types.String{"app": types.StringValue("api")}),
			}),
		}})
		got := preserveTargetRefsStateRepresentation(ctx, state, remote)
		items, diags := got.AsStructSliceT(ctx)
		if diags.HasError() || len(items) != 1 {
			t.Fatalf("refresh items=%#v diagnostics=%v", items, diags)
		}
		if !items[0].Name.IsNull() || !items[0].Namespace.IsNull() {
			t.Fatalf("unmanaged target identity imported: %#v", items[0])
		}
		if selector.IsNull() {
			if !items[0].LabelSelector.IsNull() {
				t.Fatal("unmanaged selector imported")
			}
		} else {
			value, diags := items[0].LabelSelector.Value(ctx)
			if diags.HasError() || value == nil || value.MatchLabels.IsNull() {
				t.Fatalf("remote drift from a cleared selector was hidden: %#v, %v", value, diags)
			}
		}
	}
}

func TestWAReleasedSchemaExcludesUnreleasedFiltersAndUID(t *testing.T) {
	ctx := context.Background()
	attrs := Schema(ctx).Attributes
	for _, name := range []string{"enable_proactive", "disable_proactive"} {
		filter := attrs[name].(schema.ListNestedAttribute).NestedObject.Attributes
		for _, field := range []string{"cpu_hpa_states", "memory_hpa_states"} {
			if _, ok := filter[field]; ok {
				t.Fatalf("%s.%s is unreleased in v1.36.4", name, field)
			}
		}
		if _, ok := filter["startup_boost_enabled"]; !ok {
			t.Fatalf("released startup filter missing from %s", name)
		}
	}
	policies := attrs["autoscaling_policies"].(schema.ListNestedAttribute).NestedObject.Attributes
	refs := policies["target_refs"].(schema.ListNestedAttribute).NestedObject.Attributes
	if _, ok := refs["uid"]; ok {
		t.Fatal("target_refs.uid is unreleased in v1.36.4")
	}
}
