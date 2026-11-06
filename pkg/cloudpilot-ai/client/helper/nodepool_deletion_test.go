package helper

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
)

type decommissioningClient struct {
	listCalls int
	removeAt  int
	err       error
	gcp       bool
}

func (c *decommissioningClient) ListNodePoolsContext(ctx context.Context, _ string) (api.RebalanceNodePoolList, error) {
	if err := ctx.Err(); err != nil {
		return api.RebalanceNodePoolList{}, err
	}
	c.listCalls++
	if c.err != nil {
		return api.RebalanceNodePoolList{}, c.err
	}
	if c.removeAt > 0 && c.listCalls >= c.removeAt {
		return api.RebalanceNodePoolList{}, nil
	}
	if c.gcp {
		return api.RebalanceNodePoolList{GCENodePools: []api.GCENodePool{{Name: "retiring"}}}, nil
	}
	return api.RebalanceNodePoolList{EC2NodePools: []api.EC2NodePool{{Name: "retiring"}}}, nil
}

func TestNodePoolRemovalWaitsForFinalization(t *testing.T) {
	for _, gcp := range []bool{false, true} {
		client := &decommissioningClient{removeAt: 3, gcp: gcp}
		err := pollForNodePoolRemoval(context.Background(), client, "example-cluster", "retiring", time.Millisecond, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if client.listCalls != 3 {
			t.Fatalf("returned before finalization: %d reads", client.listCalls)
		}
	}
}

func TestNodePoolRemovalHonorsCancellationAndErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := pollForNodePoolRemoval(ctx, &decommissioningClient{}, "example-cluster", "retiring", time.Millisecond, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	failure := errors.New("list unavailable")
	err = pollForNodePoolRemoval(context.Background(), &decommissioningClient{err: failure}, "example-cluster", "retiring", time.Millisecond, time.Second)
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v, want API error", err)
	}
}
