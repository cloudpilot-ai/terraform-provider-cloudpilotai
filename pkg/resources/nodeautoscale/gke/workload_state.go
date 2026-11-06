package gke

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type workloadStateReader interface {
	GetWorkloadRebalanceConfiguration(string) (*api.ClusterWorkloadSpec, error)
}

func hydrateWorkloadsState(ctx context.Context, client workloadStateReader, clusterID string, current customfield.NestedObjectList[api.WorkloadModel], isImport bool) (customfield.NestedObjectList[api.WorkloadModel], error) {
	if current.IsNullOrUnknown() && !isImport {
		return current, nil
	}
	config, err := client.GetWorkloadRebalanceConfiguration(clusterID)
	if err != nil && !errors.Is(err, cloudpilotaiclient.ErrNotFound) {
		return current, fmt.Errorf("failed to get workload configuration: %w", err)
	}
	if config == nil {
		config = &api.ClusterWorkloadSpec{}
	}
	if current.IsNullOrUnknown() {
		if len(config.Workloads) == 0 {
			return current, nil
		}
		models := make([]api.WorkloadModel, 0, len(config.Workloads))
		for _, workload := range config.Workloads {
			models = append(models, *workload.ToWorkloadModel())
		}
		list, diagnostics := customfield.NewObjectList(ctx, models)
		if diagnostics.HasError() {
			return current, fmt.Errorf("failed to build imported workload state: %v", diagnostics)
		}
		return list, nil
	}
	models, diags := current.AsStructSliceT(ctx)
	if diags.HasError() {
		return current, fmt.Errorf("failed to parse workload state: %v", diags)
	}
	byID := make(map[string]api.Workload, len(config.Workloads))
	for _, workload := range config.Workloads {
		byID[workloadStateID(workload.Type, workload.Namespace, workload.Name)] = workload
	}
	for i, state := range models {
		if remote, ok := byID[workloadStateID(state.Type.ValueString(), state.Namespace.ValueString(), state.Name.ValueString())]; ok {
			model := *remote.ToWorkloadModel()
			model.TemplateName = state.TemplateName
			model.RebalanceAble = preserveManagedBool(state.RebalanceAble, model.RebalanceAble)
			model.SpotFriendly = preserveManagedBool(state.SpotFriendly, model.SpotFriendly)
			model.MinNonSpotReplicas = preserveManagedInt64(state.MinNonSpotReplicas, model.MinNonSpotReplicas)
			models[i] = model
		}
	}
	list, diagnostics := customfield.NewObjectList(ctx, models)
	if diagnostics.HasError() {
		return current, fmt.Errorf("failed to build workload state: %v", diagnostics)
	}
	return list, nil
}

func workloadStateID(kind, namespace, name string) string { return kind + "/" + namespace + "/" + name }
