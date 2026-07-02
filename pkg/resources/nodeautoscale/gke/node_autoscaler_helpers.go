package gke

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client/helper"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

var installCloudpilotAIRebalanceComponent = helper.InstallCloudpilotAIRebalanceComponent

func sanitizeRestoreNodePoolEnvSuffix(nodePool string) string {
	var out strings.Builder
	out.Grow(len(nodePool))
	for _, r := range nodePool {
		r = unicode.ToUpper(r)
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			out.WriteRune(r)
			continue
		}
		out.WriteByte('_')
	}
	return out.String()
}

func boolValue(value types.Bool) bool {
	return !value.IsNull() && !value.IsUnknown() && value.ValueBool()
}

func hydrateAllNodeClasses(ctx context.Context, client postWriteStateHydratorClient, clusterID string) (customfield.NestedObjectList[api.GCENodeClassModel], error) {
	remoteList, err := client.ListNodeClasses(clusterID)
	if err != nil {
		if errors.Is(err, cloudpilotaiclient.ErrNotFound) {
			return emptyGCENodeClassModelList(ctx)
		}
		return customfield.NullObjectList[api.GCENodeClassModel](ctx), err
	}

	models := make([]api.GCENodeClassModel, 0, len(remoteList.GCENodeClasses))
	for _, remote := range remoteList.GCENodeClasses {
		model, convErr := remote.ToGCENodeClassModel(ctx)
		if convErr != nil {
			return customfield.NullObjectList[api.GCENodeClassModel](ctx), convErr
		}
		if model != nil {
			models = append(models, *model)
		}
	}

	list, diags := customfield.NewObjectList(ctx, sortedValuesByName(sliceToNamedMap(models, func(item api.GCENodeClassModel) string {
		return item.Name.ValueString()
	}), func(item api.GCENodeClassModel) string {
		return item.Name.ValueString()
	}))
	if diags.HasError() {
		return customfield.NullObjectList[api.GCENodeClassModel](ctx), fmt.Errorf("failed to build imported nodeclasses state: %v", diags)
	}
	return list, nil
}

func hydrateAllNodePools(ctx context.Context, client postWriteStateHydratorClient, clusterID string) (customfield.NestedObjectList[api.GCENodePoolModel], error) {
	remoteList, err := client.ListNodePools(clusterID)
	if err != nil {
		if errors.Is(err, cloudpilotaiclient.ErrNotFound) {
			return emptyGCENodePoolModelList(ctx)
		}
		return customfield.NullObjectList[api.GCENodePoolModel](ctx), err
	}

	models := make([]api.GCENodePoolModel, 0, len(remoteList.GCENodePools))
	for _, remote := range remoteList.GCENodePools {
		model, convErr := remote.ToGCENodePoolModel(ctx)
		if convErr != nil {
			return customfield.NullObjectList[api.GCENodePoolModel](ctx), convErr
		}
		if model != nil {
			models = append(models, normalizeNodePoolComputedUnknowns(*model))
		}
	}

	list, diags := customfield.NewObjectList(ctx, sortedValuesByName(sliceToNamedMap(models, func(item api.GCENodePoolModel) string {
		return item.Name.ValueString()
	}), func(item api.GCENodePoolModel) string {
		return item.Name.ValueString()
	}))
	if diags.HasError() {
		return customfield.NullObjectList[api.GCENodePoolModel](ctx), fmt.Errorf("failed to build imported nodepools state: %v", diags)
	}
	return list, nil
}

func emptyGCENodeClassModelList(ctx context.Context) (customfield.NestedObjectList[api.GCENodeClassModel], error) {
	list, diags := customfield.NewObjectList(ctx, []api.GCENodeClassModel{})
	if diags.HasError() {
		return customfield.NullObjectList[api.GCENodeClassModel](ctx), fmt.Errorf("failed to build empty imported nodeclasses state: %v", diags)
	}
	return list, nil
}

func emptyGCENodePoolModelList(ctx context.Context) (customfield.NestedObjectList[api.GCENodePoolModel], error) {
	list, diags := customfield.NewObjectList(ctx, []api.GCENodePoolModel{})
	if diags.HasError() {
		return customfield.NullObjectList[api.GCENodePoolModel](ctx), fmt.Errorf("failed to build empty imported nodepools state: %v", diags)
	}
	return list, nil
}

func sliceToNamedMap[T any](items []T, getName func(T) string) map[string]T {
	result := make(map[string]T, len(items))
	for _, item := range items {
		result[getName(item)] = item
	}
	return result
}
