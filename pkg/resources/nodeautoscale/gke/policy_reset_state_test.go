package gke

import (
	"context"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestGCEResetPolicyStatePreservesSuccessfulResetAndShowsDrift(t *testing.T) {
	ctx := context.Background()
	empty := customfield.NewObjectMust(ctx, &api.VolumeSizePolicyModel{})
	absent := customfield.NullObject[api.VolumeSizePolicyModel](ctx)
	configured := customfield.NewObjectMust(ctx, &api.VolumeSizePolicyModel{MaxSizeGiB: types.Int64Value(512)})
	if got := preserveVolumeSizePolicy(ctx, absent, empty); !got.Equal(empty) {
		t.Fatalf("successful disk reset became %v", got)
	}
	if got := preserveVolumeSizePolicy(ctx, configured, empty); !got.Equal(configured) {
		t.Fatalf("disk reset drift was hidden: %v", got)
	}
	if got := preserveVolumeSizePolicy(ctx, configured, absent); !got.IsNull() {
		t.Fatalf("null disk policy became managed: %v", got)
	}
	jobEmpty := customfield.NewObjectMust(ctx, &api.JobNodePoolOptimizationModel{})
	jobAbsent := customfield.NullObject[api.JobNodePoolOptimizationModel](ctx)
	jobConfigured := customfield.NewObjectMust(ctx, &api.JobNodePoolOptimizationModel{MinActiveNodes: types.Int32Value(2)})
	if got := preserveJobOptimization(ctx, jobAbsent, jobEmpty); !got.Equal(jobEmpty) {
		t.Fatalf("successful Job reset became %v", got)
	}
	if got := preserveJobOptimization(ctx, jobConfigured, jobEmpty); !got.Equal(jobConfigured) {
		t.Fatalf("Job reset drift was hidden: %v", got)
	}
	if got := preserveJobOptimization(ctx, jobConfigured, jobAbsent); !got.IsNull() {
		t.Fatalf("null Job policy became managed: %v", got)
	}
}

func TestGCELocalSSDResetStatePreservesEmptyAndShowsDrift(t *testing.T) {
	ctx := context.Background()
	empty := customfield.NewObjectMust(ctx, &api.GCEEphemeralStorageLocalSSDModel{})
	for _, tc := range []struct {
		name    string
		remote  customfield.NestedObject[api.GCEEphemeralStorageLocalSSDModel]
		cleared bool
	}{
		{name: "absent", remote: customfield.NullObject[api.GCEEphemeralStorageLocalSSDModel](ctx), cleared: true},
		{name: "cleared", remote: customfield.NewObjectMust(ctx, &api.GCEEphemeralStorageLocalSSDModel{AutoCount: types.BoolValue(false)}), cleared: true},
		{name: "fixed count drift", remote: customfield.NewObjectMust(ctx, &api.GCEEphemeralStorageLocalSSDModel{Count: types.Int32Value(4), AutoCount: types.BoolValue(false)})},
		{name: "automatic count drift", remote: customfield.NewObjectMust(ctx, &api.GCEEphemeralStorageLocalSSDModel{AutoCount: types.BoolValue(true)})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, postWrite := range []bool{false, true} {
				got := preserveGCEEphemeralStorageLocalSSDStateRepresentation(ctx, tc.remote, empty, postWrite)
				want := tc.remote
				if tc.cleared {
					want = empty
				}
				if !got.Equal(want) {
					t.Fatalf("postWrite=%v: state=%v, want=%v", postWrite, got, want)
				}
			}
		})
	}
}

func TestGCEPolicyStringResetStatePreservesOnlySuccessfulClears(t *testing.T) {
	ctx := context.Background()
	empty := types.StringValue("")
	for _, remote := range []types.String{types.StringNull(), types.StringValue("5m")} {
		got := preserveNodePoolStateRepresentation(ctx, api.GCENodePoolModel{DisruptionCooldown: remote}, api.GCENodePoolModel{DisruptionCooldown: empty})
		want := remote
		if remote.IsNull() {
			want = empty
		}
		if !got.DisruptionCooldown.Equal(want) {
			t.Fatalf("cooldown=%v, want=%v", got.DisruptionCooldown, want)
		}
	}
	state := customfield.NewObjectMust(ctx, &api.JobNodePoolOptimizationModel{ProvisioningDelay: empty})
	if got := preserveJobOptimization(ctx, customfield.NullObject[api.JobNodePoolOptimizationModel](ctx), state); !got.Equal(state) {
		t.Fatalf("cleared final Job field lost its reset representation: %v", got)
	}
	for _, delay := range []types.String{types.StringNull(), types.StringValue("5m")} {
		remote := customfield.NewObjectMust(ctx, &api.JobNodePoolOptimizationModel{MinActiveNodes: types.Int32Value(2), ProvisioningDelay: delay})
		got := preserveJobOptimization(ctx, remote, state)
		value, diags := got.Value(ctx)
		if diags.HasError() || value == nil {
			t.Fatalf("Job state diagnostics=%v", diags)
		}
		want := delay
		if delay.IsNull() {
			want = empty
		}
		if !value.ProvisioningDelay.Equal(want) || !value.MinActiveNodes.IsNull() {
			t.Fatalf("Job state=%v, want delay=%v and unmanaged minimum", got, want)
		}
	}
}
