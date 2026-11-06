package workloadautoscaler

import (
	"context"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestJVMHeapBoundsHydrationPreservesEquivalentQuantitySpelling(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name       string
		configured types.String
		remote     *string
		want       types.String
	}{
		{name: "canonical binary units", configured: types.StringValue("1024Mi"), remote: heapBoundString("1Gi"), want: types.StringValue("1024Mi")},
		{name: "canonical decimal units", configured: types.StringValue("1000M"), remote: heapBoundString("1G"), want: types.StringValue("1000M")},
		{name: "real drift", configured: types.StringValue("1000Mi"), remote: heapBoundString("1Gi"), want: types.StringValue("1Gi")},
		{name: "percentage", configured: types.StringValue("25.0%"), remote: heapBoundString("25.0%"), want: types.StringValue("25.0%")},
		{name: "unmanaged", configured: types.StringNull(), remote: heapBoundString("1Gi"), want: types.StringNull()},
		{name: "explicit clear", configured: types.StringValue(""), want: types.StringValue("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configured := api.RecommendationPolicyModel{
				Name:          types.StringValue("illustrative-policy"),
				JVMMinHeapXms: tc.configured, JVMMaxHeapXms: tc.configured,
				JVMMinHeapXmx: tc.configured, JVMMaxHeapXmx: tc.configured,
			}
			data := WorkloadAutoscalerModel{RecommendationPolicies: customfield.NewObjectListMust(ctx, []api.RecommendationPolicyModel{configured})}
			client := &fakePostWriteStateHydratorClient{recommendationPolicies: []api.RecommendationPolicyResource{{
				Name: "illustrative-policy", Spec: api.RecommendationPolicySpec{JVM: &api.JVMRecommendationConfiguration{
					MinHeapXms: tc.remote, MaxHeapXms: tc.remote,
					MinHeapXmx: tc.remote, MaxHeapXmx: tc.remote,
				}},
			}}}
			if err := hydratePostWriteState(ctx, client, "illustrative-cluster", &data); err != nil {
				t.Fatal(err)
			}
			policies, diags := data.RecommendationPolicies.AsStructSliceT(ctx)
			if diags.HasError() || len(policies) != 1 {
				t.Fatalf("recommendation policies = %#v, diagnostics = %v", policies, diags)
			}
			for name, got := range map[string]types.String{
				"min Xms": policies[0].JVMMinHeapXms, "max Xms": policies[0].JVMMaxHeapXms,
				"min Xmx": policies[0].JVMMinHeapXmx, "max Xmx": policies[0].JVMMaxHeapXmx,
			} {
				if !got.Equal(tc.want) {
					t.Errorf("%s = %v, want %v", name, got, tc.want)
				}
			}
		})
	}
}

func heapBoundString(value string) *string {
	return &value
}
