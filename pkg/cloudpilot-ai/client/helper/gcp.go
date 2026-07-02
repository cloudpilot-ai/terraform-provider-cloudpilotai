package helper

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type gcpNodeClassConfigurationClient interface {
	ListNodeClasses(clusterID string) (api.RebalanceNodeClassList, error)
	ApplyNodeClass(clusterID string, rebalanceNodeClass api.RebalanceNodeClass) error
	DeleteNodeClass(clusterID string, nodeClassName string) error
}

type gcpNodePoolConfigurationClient interface {
	ListNodePools(clusterID string) (api.RebalanceNodePoolList, error)
	ApplyNodePool(clusterID string, rebalanceNodePool api.RebalanceNodePool) error
	DeleteNodePool(clusterID, nodePoolName string) error
}

func SyncGCENodeClassConfiguration(ctx context.Context, client gcpNodeClassConfigurationClient, clusterUID string,
	nodeClassesNestedObjectList customfield.NestedObjectList[api.GCENodeClassModel],
	previousStateNames map[string]struct{},
) error {
	nodeClassNames, err := ApplyGCENodeClassConfiguration(ctx, client, clusterUID, nodeClassesNestedObjectList)
	if err != nil {
		return err
	}
	return DeleteStaleGCENodeClasses(ctx, client, clusterUID, nodeClassNames, previousStateNames)
}

func ApplyGCENodeClassConfiguration(ctx context.Context, client gcpNodeClassConfigurationClient, clusterUID string,
	nodeClassesNestedObjectList customfield.NestedObjectList[api.GCENodeClassModel],
) (map[string]struct{}, error) {
	if nodeClassesNestedObjectList.IsNullOrUnknown() {
		return nil, nil
	}

	nodeClasses, diagnostics := nodeClassesNestedObjectList.AsStructSliceT(ctx)
	if diagnostics.HasError() {
		return nil, fmt.Errorf("failed to parse gcp nodeclass configuration: %v", diagnostics)
	}

	existingNodeClasses, err := client.ListNodeClasses(clusterUID)
	if err != nil {
		return nil, err
	}

	nodeClassM := lo.SliceToMap(existingNodeClasses.GCENodeClasses, func(item api.GCENodeClass) (string, api.GCENodeClass) {
		return item.Name, item
	})

	nodeClassNames := make(map[string]struct{}, len(nodeClasses))
	for i := range nodeClasses {
		nodeClassNames[nodeClasses[i].Name.ValueString()] = struct{}{}

		current := api.GCENodeClass{Name: nodeClasses[i].Name.ValueString()}
		if existing, ok := nodeClassM[nodeClasses[i].Name.ValueString()]; ok {
			current = existing
		}

		nodeClass, err := nodeClasses[i].ToGCENodeClass(ctx, current)
		if err != nil {
			return nil, err
		}

		if err := client.ApplyNodeClass(clusterUID, api.RebalanceNodeClass{
			GCENodeClass: nodeClass,
		}); err != nil {
			return nil, err
		}
	}

	return nodeClassNames, nil
}

func DeleteStaleGCENodeClasses(ctx context.Context, client gcpNodeClassConfigurationClient, clusterUID string,
	desiredStateNames map[string]struct{},
	previousStateNames map[string]struct{},
) error {
	if desiredStateNames == nil {
		return nil
	}

	for name := range previousStateNames {
		if _, stillDesired := desiredStateNames[name]; !stillDesired {
			if err := client.DeleteNodeClass(clusterUID, name); err != nil {
				return err
			}
		}
	}

	return nil
}

func SyncGCENodePoolConfiguration(ctx context.Context, client gcpNodePoolConfigurationClient, clusterUID string,
	nodePoolsNestedObjectList customfield.NestedObjectList[api.GCENodePoolModel],
	previousStateNames map[string]struct{},
) error {
	if nodePoolsNestedObjectList.IsNullOrUnknown() {
		return nil
	}

	nodePools, diagnostics := nodePoolsNestedObjectList.AsStructSliceT(ctx)
	if diagnostics.HasError() {
		return fmt.Errorf("failed to parse gcp nodepool configuration: %v", diagnostics)
	}

	existingNodePools, err := client.ListNodePools(clusterUID)
	if err != nil {
		return err
	}

	nodePoolM := lo.SliceToMap(existingNodePools.GCENodePools, func(item api.GCENodePool) (string, api.GCENodePool) {
		return item.Name, item
	})

	nodePoolNames := make(map[string]struct{}, len(nodePools))
	for i := range nodePools {
		nodePoolNames[nodePools[i].Name.ValueString()] = struct{}{}

		current := api.GCENodePool{Name: nodePools[i].Name.ValueString()}
		if existing, ok := nodePoolM[nodePools[i].Name.ValueString()]; ok {
			current = existing
		}

		nodePool, err := nodePools[i].ToGCENodePool(ctx, current)
		if err != nil {
			return err
		}

		if nodePool.NodePoolSpec == nil ||
			nodePool.NodePoolSpec.Template.Spec.NodeClassRef == nil ||
			nodePool.NodePoolSpec.Template.Spec.NodeClassRef.Name == "" {
			return fmt.Errorf("nodepool %s must reference a valid nodeclass", nodePool.Name)
		}

		if err := client.ApplyNodePool(clusterUID, api.RebalanceNodePool{
			GCENodePool: nodePool,
		}); err != nil {
			return err
		}
	}

	for name := range previousStateNames {
		if _, stillDesired := nodePoolNames[name]; !stillDesired {
			if err := client.DeleteNodePool(clusterUID, name); err != nil {
				return err
			}
		}
	}

	return nil
}
