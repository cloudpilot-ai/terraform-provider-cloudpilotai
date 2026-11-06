package gke

import (
	"context"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestNodePoolMaximumBoundStateDistinguishesUnmanagedUnlimitedAndDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		state  types.Int64
		remote types.Int64
		want   types.Int64
	}{
		{name: "unmanaged absent", state: types.Int64Null(), remote: types.Int64Null(), want: types.Int64Null()},
		{name: "unmanaged remote bound", state: types.Int64Null(), remote: types.Int64Value(64), want: types.Int64Null()},
		{name: "explicit unlimited succeeded", state: types.Int64Value(0), remote: types.Int64Null(), want: types.Int64Value(0)},
		{name: "explicit unlimited remote drift", state: types.Int64Value(0), remote: types.Int64Value(64), want: types.Int64Value(64)},
		{name: "explicit unlimited unknown remote", state: types.Int64Value(0), remote: types.Int64Unknown(), want: types.Int64Unknown()},
		{name: "positive bound removed", state: types.Int64Value(64), remote: types.Int64Null(), want: types.Int64Null()},
		{name: "positive bound changed", state: types.Int64Value(64), remote: types.Int64Value(32), want: types.Int64Value(32)},
		{name: "unknown configuration absent remote", state: types.Int64Unknown(), remote: types.Int64Null(), want: types.Int64Null()},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := api.GCENodePoolModel{InstanceCPUMAX: test.state, InstanceMemoryMAX: test.state}
			remote := api.GCENodePoolModel{InstanceCPUMAX: test.remote, InstanceMemoryMAX: test.remote}
			got := preserveNodePoolStateRepresentation(context.Background(), remote, state)
			if !got.InstanceCPUMAX.Equal(test.want) {
				t.Errorf("instance_cpu_max = %s, want %s", got.InstanceCPUMAX, test.want)
			}
			if !got.InstanceMemoryMAX.Equal(test.want) {
				t.Errorf("instance_memory_max = %s, want %s", got.InstanceMemoryMAX, test.want)
			}
		})
	}
}

func TestNodePoolMissingMinimumStillExposesDriftFromExplicitZero(t *testing.T) {
	state := api.GCENodePoolModel{InstanceCPUMIN: types.Int64Value(0), InstanceMemoryMIN: types.Int64Value(0)}
	remote := api.GCENodePoolModel{InstanceCPUMIN: types.Int64Null(), InstanceMemoryMIN: types.Int64Null()}
	got := preserveNodePoolStateRepresentation(context.Background(), remote, state)
	if !got.InstanceCPUMIN.IsNull() || !got.InstanceMemoryMIN.IsNull() {
		t.Fatalf("missing minimum requirements were hidden: cpu=%s memory=%s", got.InstanceCPUMIN, got.InstanceMemoryMIN)
	}
}
