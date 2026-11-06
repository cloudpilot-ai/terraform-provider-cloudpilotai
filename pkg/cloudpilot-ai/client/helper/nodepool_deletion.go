package helper

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"k8s.io/apimachinery/pkg/util/wait"
)

type nodePoolListClient interface {
	ListNodePoolsContext(ctx context.Context, clusterID string) (api.RebalanceNodePoolList, error)
}

type nodePoolDeletionClient interface {
	nodePoolListClient
	DeleteNodePoolContext(ctx context.Context, clusterID, name string) error
}

type nodeClassDeletionClient interface {
	DeleteNodeClass(clusterID, name string) error
}

// A successful DELETE can start decommissioning rather than remove the pool.
// Wait for the agent to finish before removing its NodeClass or Terraform state.
func waitForNodePoolRemoval(ctx context.Context, client nodePoolListClient, clusterID, name string) error {
	return pollForNodePoolRemoval(ctx, client, clusterID, name, 5*time.Second, 30*time.Minute)
}

func pollForNodePoolRemoval(ctx context.Context, client nodePoolListClient, clusterID, name string,
	interval, timeout time.Duration,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(pollCtx context.Context) (bool, error) {
		present, err := nodePoolExists(pollCtx, client, clusterID, name)
		return !present, err
	})
	if err != nil {
		return fmt.Errorf("waiting for nodepool %q decommission to finish: %w", name, err)
	}
	return nil
}

func nodePoolExists(ctx context.Context, client nodePoolListClient, clusterID, name string) (bool, error) {
	pools, err := client.ListNodePoolsContext(ctx, clusterID)
	if err != nil {
		return false, err
	}
	for _, pool := range pools.EC2NodePools {
		if pool.Name == name {
			return true, nil
		}
	}
	for _, pool := range pools.GCENodePools {
		if pool.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func deleteNodePoolAndWait(ctx context.Context, client nodePoolDeletionClient, clusterID, name string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	present, err := nodePoolExists(ctx, client, clusterID, name)
	if err != nil || !present {
		return err
	}
	if err := client.DeleteNodePoolContext(ctx, clusterID, name); err != nil {
		// The request may have succeeded before its response was lost, or a
		// concurrently decommissioning pool may have disappeared before DELETE.
		present, listErr := nodePoolExists(ctx, client, clusterID, name)
		if listErr == nil && !present {
			return nil
		}
		return err
	}
	return waitForNodePoolRemoval(ctx, client, clusterID, name)
}

// Delete stale classes only after the pools referencing them have been removed.
func DeleteStaleEC2NodeClasses(ctx context.Context, client nodeClassDeletionClient, clusterID string,
	desired customfield.NestedObjectList[api.EC2NodeClassModel], previousNames map[string]struct{},
) error {
	if desired.IsNullOrUnknown() {
		return nil
	}
	classes, diags := desired.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("failed to parse nodeclass configuration: %v", diags)
	}
	names := make(map[string]struct{}, len(classes))
	for _, class := range classes {
		names[class.Name.ValueString()] = struct{}{}
	}
	for name := range previousNames {
		if _, keep := names[name]; keep {
			continue
		}
		if err := client.DeleteNodeClass(clusterID, name); err != nil {
			return err
		}
	}
	return nil
}
