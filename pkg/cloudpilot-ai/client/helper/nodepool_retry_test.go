package helper

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
)

type retryNodePoolClient struct {
	present       bool
	gcp           bool
	deleteCalls   int
	deleteErr     error
	listErr       error
	retainOnError bool
}

func (c *retryNodePoolClient) ListNodePools(string) (api.RebalanceNodePoolList, error) {
	return c.ListNodePoolsContext(context.Background(), "")
}

func (c *retryNodePoolClient) ListNodePoolsContext(ctx context.Context, _ string) (api.RebalanceNodePoolList, error) {
	if err := ctx.Err(); err != nil {
		return api.RebalanceNodePoolList{}, err
	}
	if c.listErr != nil || !c.present {
		return api.RebalanceNodePoolList{}, c.listErr
	}
	if c.gcp {
		return api.RebalanceNodePoolList{GCENodePools: []api.GCENodePool{{Name: "retiring"}}}, nil
	}
	return api.RebalanceNodePoolList{EC2NodePools: []api.EC2NodePool{{Name: "retiring"}}}, nil
}

func (c *retryNodePoolClient) DeleteNodePoolContext(context.Context, string, string) error {
	c.deleteCalls++
	if c.deleteErr == nil || !c.retainOnError {
		c.present = false
	}
	return c.deleteErr
}

func (c *retryNodePoolClient) ApplyNodePool(string, api.RebalanceNodePool) error { return nil }

func TestNodePoolDeletionRetriesAreIdempotent(t *testing.T) {
	failure := errors.New("delete response unavailable")
	for _, gcp := range []bool{false, true} {
		for _, tc := range []struct {
			name          string
			present       bool
			deleteErr     error
			listErr       error
			retainOnError bool
			wantErr       error
			wantDeletes   int
		}{
			{name: "already absent"},
			{name: "normal deletion", present: true, wantDeletes: 1},
			{name: "lost response", present: true, deleteErr: failure, wantDeletes: 1},
			{name: "failed deletion", present: true, deleteErr: failure, retainOnError: true, wantErr: failure, wantDeletes: 1},
			{name: "list failure", present: true, listErr: failure, wantErr: failure},
		} {
			t.Run(tc.name+map[bool]string{false: "/EKS", true: "/GKE"}[gcp], func(t *testing.T) {
				client := &retryNodePoolClient{present: tc.present, gcp: gcp, deleteErr: tc.deleteErr,
					listErr: tc.listErr, retainOnError: tc.retainOnError}
				previous := map[string]struct{}{"retiring": {}}
				var err error
				if gcp {
					err = ApplyGCENodePoolConfiguration(client, &GCENodePoolPlan{clusterUID: "example-cluster", previousNames: previous})
				} else {
					err = ApplyEC2NodePoolConfiguration(client, &EC2NodePoolPlan{clusterUID: "example-cluster", previousNames: previous})
				}
				if !errors.Is(err, tc.wantErr) || client.deleteCalls != tc.wantDeletes {
					t.Fatalf("error = %v, deletes = %d; want %v, %d", err, client.deleteCalls, tc.wantErr, tc.wantDeletes)
				}
			})
		}
	}
}
