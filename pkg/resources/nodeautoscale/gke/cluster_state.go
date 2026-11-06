package gke

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client/helper"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func hydrateClusterPostWriteState(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, data *ClusterModel) error {
	var err error
	data.ClusterSetting, err = hydrateClusterSettingPostWrite(ctx, client, clusterUID, data.ClusterSetting)
	if err != nil {
		return err
	}
	data.NodeClasses, err = hydrateNodeClassesPostWrite(ctx, client, clusterUID, data.NodeClasses)
	if err != nil {
		return err
	}
	data.NodePools, err = hydrateNodePoolsPostWrite(ctx, client, clusterUID, data.NodePools)
	if err != nil {
		return err
	}
	data.Workloads, err = hydrateWorkloadsState(ctx, client, clusterUID, data.Workloads, false)
	if err != nil {
		return err
	}
	data.ScheduledRebalances, err = helper.HydrateScheduledRebalancesPostWrite(ctx, client, clusterUID, data.ScheduledRebalances)
	if err != nil {
		return fmt.Errorf("failed to hydrate scheduled rebalances: %w", err)
	}

	if !data.EnableRebalance.IsNull() && !data.EnableRebalance.IsUnknown() {
		rebalanceConfig, err := client.GetRebalanceConfiguration(clusterUID)
		if err != nil && !errors.Is(err, cloudpilotaiclient.ErrNotFound) {
			return err
		}
		if rebalanceConfig != nil {
			data.EnableRebalance = types.BoolValue(rebalanceConfig.Enable)
		}
	}

	return nil
}

func mergeClusterIdentityFromSummary(data *ClusterModel, summary *api.ClusterCostsSummary) {
	if summary == nil {
		return
	}
	if summary.ClusterName != "" {
		data.ClusterName = types.StringValue(summary.ClusterName)
	}
	if summary.Region != "" {
		data.Region = types.StringValue(summary.Region)
	}
}

func (c *Cluster) readClusterManagementState(ctx context.Context, data *ClusterModel, clusterID string, isImport bool) error {
	var err error
	data.Workloads, err = hydrateWorkloadsState(ctx, c.client, clusterID, data.Workloads, isImport)
	if err != nil {
		return err
	}
	clusterSetting, err := c.client.GetClusterSetting(clusterID)
	if err != nil {
		return fmt.Errorf("failed to get cluster setting: %w", err)
	}
	if !data.ClusterSetting.IsNull() && !data.ClusterSetting.IsUnknown() {
		data.ClusterSetting, err = clusterSettingObjectPreservingState(ctx, data.ClusterSetting, clusterSetting)
		if err != nil {
			return err
		}
	} else if isImport {
		data.ClusterSetting = clusterSettingObjectFromAPI(ctx, clusterSetting)
	}

	rebalanceConfig, err := c.client.GetRebalanceConfiguration(clusterID)
	if err != nil && !errors.Is(err, cloudpilotaiclient.ErrNotFound) {
		return fmt.Errorf("failed to get rebalance configuration: %w", err)
	}
	if rebalanceConfig != nil && (isImport || !data.EnableRebalance.IsNull()) {
		data.EnableRebalance = types.BoolValue(rebalanceConfig.Enable)
	} else if isImport && rebalanceConfig == nil {
		data.EnableRebalance = types.BoolValue(false)
	}

	if !data.NodeClasses.IsNullOrUnknown() {
		data.NodeClasses, err = refreshNodeClassesState(ctx, c.client, clusterID, data.NodeClasses)
		if err != nil {
			return err
		}
	} else if isImport {
		importedNodeClasses, err := hydrateAllNodeClasses(ctx, c.client, clusterID)
		if err != nil {
			return err
		}
		importedNodeClassesSlice, diags := importedNodeClasses.AsStructSliceT(ctx)
		if diags.HasError() {
			return fmt.Errorf("failed to parse imported nodeclasses: %v", diags)
		}
		if len(importedNodeClassesSlice) > 0 {
			data.NodeClasses = importedNodeClasses
		}
	}

	if !data.NodePools.IsNullOrUnknown() {
		data.NodePools, err = refreshNodePoolsState(ctx, c.client, clusterID, data.NodePools)
		if err != nil {
			return err
		}
	} else if isImport {
		importedNodePools, err := hydrateAllNodePools(ctx, c.client, clusterID)
		if err != nil {
			return err
		}
		importedNodePoolsSlice, diags := importedNodePools.AsStructSliceT(ctx)
		if diags.HasError() {
			return fmt.Errorf("failed to parse imported nodepools: %v", diags)
		}
		if len(importedNodePoolsSlice) > 0 {
			data.NodePools = importedNodePools
		}
	}
	if !data.ScheduledRebalances.IsNullOrUnknown() {
		data.ScheduledRebalances, err = helper.RefreshScheduledRebalances(ctx, c.client, clusterID, data.ScheduledRebalances)
		if err != nil {
			return fmt.Errorf("failed to hydrate scheduled rebalances: %w", err)
		}
	} else if isImport {
		data.ScheduledRebalances, err = helper.ImportScheduledRebalances(ctx, c.client, clusterID)
		if err != nil {
			return fmt.Errorf("failed to import scheduled rebalances: %w", err)
		}
	}

	return nil
}

func hydrateNodeClassesPostWrite(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObjectList[api.GCENodeClassModel]) (customfield.NestedObjectList[api.GCENodeClassModel], error) {
	return hydrateNodeClassesState(ctx, client, clusterUID, current, true)
}

func refreshNodeClassesState(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObjectList[api.GCENodeClassModel]) (customfield.NestedObjectList[api.GCENodeClassModel], error) {
	return hydrateNodeClassesState(ctx, client, clusterUID, current, false)
}

func hydrateNodeClassesState(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObjectList[api.GCENodeClassModel], preserveMissing bool) (customfield.NestedObjectList[api.GCENodeClassModel], error) {
	if current.IsNullOrUnknown() {
		return current, nil
	}

	stateNodeClasses, diags := current.AsStructSliceT(ctx)
	if diags.HasError() {
		return current, fmt.Errorf("failed to parse nodeclasses: %v", diags)
	}

	remoteList, err := client.ListNodeClasses(clusterUID)
	if err != nil {
		return current, fmt.Errorf("failed to list nodeclasses: %w", err)
	}

	remoteByName := make(map[string]api.GCENodeClassModel, len(remoteList.GCENodeClasses))
	for i := range remoteList.GCENodeClasses {
		model, err := remoteList.GCENodeClasses[i].ToGCENodeClassModel(ctx)
		if err != nil {
			return current, fmt.Errorf("failed to convert nodeclass %q: %w", remoteList.GCENodeClasses[i].Name, err)
		}
		if model != nil {
			remoteByName[remoteList.GCENodeClasses[i].Name] = *model
		}
	}

	hydrated := make([]api.GCENodeClassModel, 0, len(stateNodeClasses))
	for _, stateNodeClass := range stateNodeClasses {
		if remote, ok := remoteByName[stateNodeClass.Name.ValueString()]; ok {
			hydrated = append(hydrated, preserveNodeClassStateRepresentationForRefresh(ctx, remote, stateNodeClass, preserveMissing))
			continue
		}
		if preserveMissing {
			hydrated = append(hydrated, stateNodeClass)
		}
	}

	list, diags := customfield.NewObjectList(ctx, hydrated)
	if diags.HasError() {
		return current, fmt.Errorf("failed to build nodeclasses state: %v", diags)
	}
	return list, nil
}

func hydrateNodePoolsPostWrite(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObjectList[api.GCENodePoolModel]) (customfield.NestedObjectList[api.GCENodePoolModel], error) {
	return hydrateNodePoolsState(ctx, client, clusterUID, current, true)
}

func refreshNodePoolsState(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObjectList[api.GCENodePoolModel]) (customfield.NestedObjectList[api.GCENodePoolModel], error) {
	return hydrateNodePoolsState(ctx, client, clusterUID, current, false)
}

func hydrateNodePoolsState(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObjectList[api.GCENodePoolModel], preserveMissing bool) (customfield.NestedObjectList[api.GCENodePoolModel], error) {
	if current.IsNullOrUnknown() {
		return current, nil
	}

	stateNodePools, diags := current.AsStructSliceT(ctx)
	if diags.HasError() {
		return current, fmt.Errorf("failed to parse nodepools: %v", diags)
	}

	remoteList, err := client.ListNodePools(clusterUID)
	if err != nil {
		return current, fmt.Errorf("failed to list nodepools: %w", err)
	}

	remoteByName := make(map[string]api.GCENodePoolModel, len(remoteList.GCENodePools))
	for i := range remoteList.GCENodePools {
		model, err := remoteList.GCENodePools[i].ToGCENodePoolModel(ctx)
		if err != nil {
			return current, fmt.Errorf("failed to convert nodepool %q: %w", remoteList.GCENodePools[i].Name, err)
		}
		if model != nil {
			remoteByName[remoteList.GCENodePools[i].Name] = normalizeNodePoolComputedUnknowns(*model)
		}
	}

	hydrated := make([]api.GCENodePoolModel, 0, len(stateNodePools))
	for _, stateNodePool := range stateNodePools {
		if remote, ok := remoteByName[stateNodePool.Name.ValueString()]; ok {
			hydrated = append(hydrated, normalizeNodePoolComputedUnknowns(preserveNodePoolStateRepresentation(ctx, remote, stateNodePool)))
			continue
		}
		if preserveMissing {
			hydrated = append(hydrated, normalizeNodePoolComputedUnknowns(stateNodePool))
		}
	}

	list, diags := customfield.NewObjectList(ctx, hydrated)
	if diags.HasError() {
		return current, fmt.Errorf("failed to build nodepools state: %v", diags)
	}
	return list, nil
}

func preserveNodeClassStateRepresentation(ctx context.Context, remote, state api.GCENodeClassModel) api.GCENodeClassModel {
	return preserveNodeClassStateRepresentationForRefresh(ctx, remote, state, true)
}

func preserveNodeClassStateRepresentationForRefresh(ctx context.Context, remote, state api.GCENodeClassModel, preserveMissing bool) api.GCENodeClassModel {
	remote.EnableImageAccelerator = preserveManagedBool(state.EnableImageAccelerator, remote.EnableImageAccelerator)
	remote.EnableLocalSSDEphemeralStorage = preserveManagedBool(state.EnableLocalSSDEphemeralStorage, remote.EnableLocalSSDEphemeralStorage)
	remote.EphemeralStorageLocalSSD = preserveGCEEphemeralStorageLocalSSDStateRepresentation(ctx, remote.EphemeralStorageLocalSSD, state.EphemeralStorageLocalSSD, preserveMissing)
	remote.ServiceAccount = preserveManagedString(state.ServiceAccount, remote.ServiceAccount)
	remote.Disks = preserveGCEDiskStateRepresentation(ctx, remote.Disks, state.Disks)
	remote.ImageSelectorTerms = preserveGCEImageSelectorTermStateRepresentation(ctx, remote.ImageSelectorTerms, state.ImageSelectorTerms)
	remote.SubnetRangeName = preserveManagedString(state.SubnetRangeName, remote.SubnetRangeName)
	remote.KubeletConfiguration = preserveGCEKubeletConfigurationStateRepresentation(ctx, remote.KubeletConfiguration, state.KubeletConfiguration)
	remote.Labels = preserveManagedMap(ctx, state.Labels, remote.Labels)
	remote.Metadata = preserveManagedMap(ctx, state.Metadata, remote.Metadata)
	remote.NetworkTags = preserveManagedStringSlice(state.NetworkTags, remote.NetworkTags)
	remote.ConfidentialInstanceType = preserveManagedString(state.ConfidentialInstanceType, remote.ConfidentialInstanceType)
	remote.NetworkConfig = preserveGCENetworkConfigStateRepresentation(ctx, remote.NetworkConfig, state.NetworkConfig)
	remote.AutoGPUTaint = preserveManagedBool(state.AutoGPUTaint, remote.AutoGPUTaint)
	remote.GPUDriverVersion = preserveManagedString(state.GPUDriverVersion, remote.GPUDriverVersion)
	remote.OriginNodeClassJSON = preserveStateStringWhenRemoteNull(state.OriginNodeClassJSON, remote.OriginNodeClassJSON)
	return remote
}

func preserveGCEEphemeralStorageLocalSSDStateRepresentation(ctx context.Context, remote, state customfield.NestedObject[api.GCEEphemeralStorageLocalSSDModel], preserveMissing bool) customfield.NestedObject[api.GCEEphemeralStorageLocalSSDModel] {
	if state.IsNull() || state.IsUnknown() {
		return state
	}
	if api.IsKnownEmptyObject(state) {
		return preserveEmptyLocalSSDReset(ctx, remote, state)
	}
	if remote.IsNull() {
		if preserveMissing {
			return state
		}
		return remote
	}
	if remote.IsUnknown() {
		return state
	}
	stateValue, stateDiags := state.Value(ctx)
	remoteValue, remoteDiags := remote.Value(ctx)
	if stateDiags.HasError() || remoteDiags.HasError() || stateValue == nil || remoteValue == nil {
		return state
	}
	remoteValue.Count = preserveManagedInt32(stateValue.Count, remoteValue.Count)
	remoteValue.AutoCount = preserveManagedBool(stateValue.AutoCount, remoteValue.AutoCount)
	return customfield.NewObjectMust(ctx, remoteValue)
}

func preserveNodePoolStateRepresentation(ctx context.Context, remote, state api.GCENodePoolModel) api.GCENodePoolModel {
	remote.Enable = preserveManagedBool(state.Enable, remote.Enable)
	remote.EnableImageAccelerator = preserveManagedBool(state.EnableImageAccelerator, remote.EnableImageAccelerator)
	remote.NodeClass = preserveManagedString(state.NodeClass, remote.NodeClass)
	remote.EnableGPU = preserveManagedBool(state.EnableGPU, remote.EnableGPU)
	remote.ProvisionPriority = preserveManagedInt32(state.ProvisionPriority, remote.ProvisionPriority)
	remote.InstanceFamily = preserveManagedList(state.InstanceFamily, remote.InstanceFamily)
	remote.InstanceArch = preserveManagedList(state.InstanceArch, remote.InstanceArch)
	remote.CapacityType = preserveManagedList(state.CapacityType, remote.CapacityType)
	remote.Zone = preserveManagedList(state.Zone, remote.Zone)
	remote.InstanceCPUMAX = preserveUnlimitedMaxBound(state.InstanceCPUMAX, remote.InstanceCPUMAX)
	remote.InstanceCPUMIN = preserveManagedInt64(state.InstanceCPUMIN, remote.InstanceCPUMIN)
	remote.InstanceMemoryMAX = preserveUnlimitedMaxBound(state.InstanceMemoryMAX, remote.InstanceMemoryMAX)
	remote.InstanceMemoryMIN = preserveManagedInt64(state.InstanceMemoryMIN, remote.InstanceMemoryMIN)
	remote.Labels = preserveManagedMap(ctx, state.Labels, remote.Labels)
	remote.Taints = preserveTaintStateRepresentation(ctx, remote.Taints, state.Taints)
	remote.NodeDisruptionLimit = preserveManagedString(state.NodeDisruptionLimit, remote.NodeDisruptionLimit)
	remote.NodeDisruptionBudgets = preserveManagedObjectListByIndex(ctx, state.NodeDisruptionBudgets, remote.NodeDisruptionBudgets, func(remoteBudget, stateBudget api.DisruptionBudgetModel) api.DisruptionBudgetModel {
		remoteBudget.Duration = preserveSemanticDuration(stateBudget.Duration, remoteBudget.Duration)
		return remoteBudget
	})
	remote.NodeDisruptionDelay = preserveManagedDuration(state.NodeDisruptionDelay, remote.NodeDisruptionDelay)
	remote.DisruptionCooldown = preserveResetDuration(state.DisruptionCooldown, remote.DisruptionCooldown)
	remote.JobNodePoolOptimization = preserveJobOptimization(ctx, remote.JobNodePoolOptimization, state.JobNodePoolOptimization)
	remote.OriginNodePoolJSON = preserveStateStringWhenRemoteNull(state.OriginNodePoolJSON, remote.OriginNodePoolJSON)
	return remote
}

func normalizeNodePoolComputedUnknowns(model api.GCENodePoolModel) api.GCENodePoolModel {
	if model.NodeClass.IsUnknown() {
		model.NodeClass = types.StringNull()
	}
	if model.InstanceCPUMIN.IsUnknown() {
		model.InstanceCPUMIN = types.Int64Null()
	}
	if model.InstanceMemoryMIN.IsUnknown() {
		model.InstanceMemoryMIN = types.Int64Null()
	}
	return model
}

func extractResourceNames[T any](ctx context.Context, list customfield.NestedObjectList[T], getName func(T) string) map[string]struct{} {
	if list.IsNullOrUnknown() {
		return nil
	}
	items, diags := list.AsStructSliceT(ctx)
	if diags.HasError() {
		return nil
	}
	names := make(map[string]struct{}, len(items))
	for _, item := range items {
		names[getName(item)] = struct{}{}
	}
	return names
}

func sortedValuesByName[T any](values map[string]T, getName func(T) string) []T {
	result := make([]T, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return getName(result[i]) < getName(result[j])
	})

	return result
}

func resolveDeleteClusterUID(clusterID, clusterName, region, clusterUID types.String) string {
	return resolveClusterUID(clusterID, clusterID, clusterName, region, clusterUID)
}

func applyClusterSummaryStatus(data *ClusterModel, summary *api.ClusterCostsSummary) {
	if summary == nil {
		return
	}

	data.AgentVersion = types.StringValue(summary.AgentVersion)
	data.OnboardManifestVersion = types.StringValue(summary.OnboardManifestVersion)
	data.NeedUpgrade = types.BoolValue(summary.NeedUpgrade)
}

func preserveSemanticDuration(stateVal, serverVal types.String) types.String {
	if stateVal.IsNull() || stateVal.IsUnknown() || serverVal.IsNull() || serverVal.IsUnknown() {
		return serverVal
	}
	stateD, err1 := time.ParseDuration(stateVal.ValueString())
	serverD, err2 := time.ParseDuration(serverVal.ValueString())
	if err1 == nil && err2 == nil && stateD == serverD {
		return stateVal
	}
	return serverVal
}

func preserveManagedBool(stateVal, remoteVal types.Bool) types.Bool {
	if stateVal.IsNull() {
		return types.BoolNull()
	}
	return remoteVal
}

func preserveManagedInt64(stateVal, remoteVal types.Int64) types.Int64 {
	if stateVal.IsNull() {
		return types.Int64Null()
	}
	return remoteVal
}

// Zero explicitly removes a maximum requirement. Retain that configured value
// only while the server still has no bound, so later limits remain visible.
func preserveUnlimitedMaxBound(stateVal, remoteVal types.Int64) types.Int64 {
	if !stateVal.IsNull() && !stateVal.IsUnknown() && stateVal.ValueInt64() == 0 && remoteVal.IsNull() {
		return stateVal
	}
	return preserveManagedInt64(stateVal, remoteVal)
}

func preserveManagedInt32(stateVal, remoteVal types.Int32) types.Int32 {
	if stateVal.IsNull() {
		return types.Int32Null()
	}
	return remoteVal
}

func preserveManagedString(stateVal, remoteVal types.String) types.String {
	if stateVal.IsNull() {
		return types.StringNull()
	}
	return remoteVal
}

func preserveGCEDiskStateRepresentation(ctx context.Context, remote, state customfield.NestedObjectList[api.GCEDiskModel]) customfield.NestedObjectList[api.GCEDiskModel] {
	return preserveManagedObjectListByIndex(ctx, state, remote, func(remoteDisk, stateDisk api.GCEDiskModel) api.GCEDiskModel {
		remoteDisk.SizeGiB = preserveManagedInt64(stateDisk.SizeGiB, remoteDisk.SizeGiB)
		remoteDisk.SizePolicy = preserveVolumeSizePolicy(ctx, remoteDisk.SizePolicy, stateDisk.SizePolicy)
		remoteDisk.Category = preserveManagedString(stateDisk.Category, remoteDisk.Category)
		remoteDisk.Boot = preserveManagedBool(stateDisk.Boot, remoteDisk.Boot)
		return remoteDisk
	})
}

func preserveGCEImageSelectorTermStateRepresentation(ctx context.Context, remote, state customfield.NestedObjectList[api.GCEImageSelectorTermModel]) customfield.NestedObjectList[api.GCEImageSelectorTermModel] {
	return preserveManagedObjectListByIndex(ctx, state, remote, func(remoteTerm, stateTerm api.GCEImageSelectorTermModel) api.GCEImageSelectorTermModel {
		remoteTerm.ID = preserveManagedString(stateTerm.ID, remoteTerm.ID)
		remoteTerm.Family = preserveManagedString(stateTerm.Family, remoteTerm.Family)
		remoteTerm.Channel = preserveManagedString(stateTerm.Channel, remoteTerm.Channel)
		remoteTerm.Version = preserveManagedString(stateTerm.Version, remoteTerm.Version)
		return remoteTerm
	})
}

func preserveGCEKubeletConfigurationStateRepresentation(ctx context.Context, remote, state customfield.NestedObject[api.GCEKubeletConfigurationModel]) customfield.NestedObject[api.GCEKubeletConfigurationModel] {
	if state.IsNull() {
		return customfield.NullObject[api.GCEKubeletConfigurationModel](ctx)
	}
	if remote.IsNull() || remote.IsUnknown() {
		return remote
	}

	stateValue, diags := state.Value(ctx)
	if diags.HasError() || stateValue == nil {
		return remote
	}
	remoteValue, diags := remote.Value(ctx)
	if diags.HasError() || remoteValue == nil {
		return remote
	}

	remoteValue.KubeReserved = preserveManagedMap(ctx, stateValue.KubeReserved, remoteValue.KubeReserved)
	remoteValue.SystemReserved = preserveManagedMap(ctx, stateValue.SystemReserved, remoteValue.SystemReserved)
	remoteValue.EvictionHard = preserveManagedMap(ctx, stateValue.EvictionHard, remoteValue.EvictionHard)
	remoteValue.EvictionSoft = preserveManagedMap(ctx, stateValue.EvictionSoft, remoteValue.EvictionSoft)

	object, diags := customfield.NewObject(ctx, remoteValue)
	if diags.HasError() {
		return remote
	}
	return object
}

func preserveGCENetworkConfigStateRepresentation(ctx context.Context, remote, state customfield.NestedObject[api.GCENetworkConfigModel]) customfield.NestedObject[api.GCENetworkConfigModel] {
	if state.IsNull() {
		return customfield.NullObject[api.GCENetworkConfigModel](ctx)
	}
	if remote.IsNull() || remote.IsUnknown() {
		return remote
	}

	stateValue, diags := state.Value(ctx)
	if diags.HasError() || stateValue == nil {
		return remote
	}
	remoteValue, diags := remote.Value(ctx)
	if diags.HasError() || remoteValue == nil {
		return remote
	}

	remoteValue.EnablePrivateNodes = preserveManagedBool(stateValue.EnablePrivateNodes, remoteValue.EnablePrivateNodes)
	remoteValue.Subnetwork = preserveManagedString(stateValue.Subnetwork, remoteValue.Subnetwork)
	remoteValue.AdditionalNetworkInterfaces = preserveGCEAdditionalNetworkInterfaceStateRepresentation(ctx, remoteValue.AdditionalNetworkInterfaces, stateValue.AdditionalNetworkInterfaces)

	object, diags := customfield.NewObject(ctx, remoteValue)
	if diags.HasError() {
		return remote
	}
	return object
}

func preserveGCEAdditionalNetworkInterfaceStateRepresentation(ctx context.Context, remote, state customfield.NestedObjectList[api.GCEAdditionalNetworkInterfaceModel]) customfield.NestedObjectList[api.GCEAdditionalNetworkInterfaceModel] {
	return preserveManagedObjectListByIndex(ctx, state, remote, func(remoteNIC, stateNIC api.GCEAdditionalNetworkInterfaceModel) api.GCEAdditionalNetworkInterfaceModel {
		remoteNIC.Network = preserveManagedString(stateNIC.Network, remoteNIC.Network)
		remoteNIC.Subnetwork = preserveManagedString(stateNIC.Subnetwork, remoteNIC.Subnetwork)
		return remoteNIC
	})
}

func preserveTaintStateRepresentation(ctx context.Context, remote, state customfield.NestedObjectList[api.TaintModel]) customfield.NestedObjectList[api.TaintModel] {
	return preserveManagedObjectListByIndex(ctx, state, remote, func(remoteTaint, stateTaint api.TaintModel) api.TaintModel {
		remoteTaint.Key = preserveManagedString(stateTaint.Key, remoteTaint.Key)
		remoteTaint.Value = preserveManagedString(stateTaint.Value, remoteTaint.Value)
		remoteTaint.Effect = preserveManagedString(stateTaint.Effect, remoteTaint.Effect)
		return remoteTaint
	})
}

func preserveStateStringWhenRemoteNull(stateVal, remoteVal types.String) types.String {
	if stateVal.IsNull() {
		return types.StringNull()
	}
	if remoteVal.IsNull() || remoteVal.IsUnknown() {
		return stateVal
	}
	return remoteVal
}

func preserveManagedDuration(stateVal, remoteVal types.String) types.String {
	if stateVal.IsNull() || stateVal.IsUnknown() {
		return types.StringNull()
	}
	return preserveSemanticDuration(stateVal, remoteVal)
}

func preserveManagedList(stateVal, remoteVal *[]types.String) *[]types.String {
	if stateVal == nil {
		return nil
	}
	if remoteVal == nil && len(*stateVal) == 0 {
		return stateVal
	}
	return remoteVal
}

func preserveManagedMap(ctx context.Context, stateVal, remoteVal customfield.Map[types.String]) customfield.Map[types.String] {
	if stateVal.IsNull() {
		return customfield.NullMap[types.String](ctx)
	}

	stateValues, diags := stateVal.Value(ctx)
	if diags.HasError() {
		return remoteVal
	}
	if remoteVal.IsNull() || remoteVal.IsUnknown() {
		if len(stateValues) == 0 {
			return stateVal
		}
		return remoteVal
	}

	remoteValues, diags := remoteVal.Value(ctx)
	if diags.HasError() {
		return remoteVal
	}

	filtered := make(map[string]types.String, len(stateValues))
	for key, stateValue := range stateValues {
		if remoteValue, ok := remoteValues[key]; ok {
			filtered[key] = remoteValue
			continue
		}
		filtered[key] = stateValue
	}

	return customfield.NewMapMust[types.String](ctx, filtered)
}

func preserveManagedObject[T any](ctx context.Context, stateVal, remoteVal customfield.NestedObject[T]) customfield.NestedObject[T] {
	if stateVal.IsNull() {
		return customfield.NullObject[T](ctx)
	}
	return remoteVal
}

func preserveManagedObjectList[T any](ctx context.Context, stateVal, remoteVal customfield.NestedObjectList[T]) customfield.NestedObjectList[T] {
	if stateVal.IsNull() {
		return customfield.NullObjectList[T](ctx)
	}
	if remoteVal.IsNullOrUnknown() {
		values, diags := stateVal.AsStructSliceT(ctx)
		if !diags.HasError() && len(values) == 0 {
			return stateVal
		}
	}
	return remoteVal
}

func preserveManagedObjectListByIndex[T any](ctx context.Context, stateVal, remoteVal customfield.NestedObjectList[T], preserve func(remote, state T) T) customfield.NestedObjectList[T] {
	base := preserveManagedObjectList(ctx, stateVal, remoteVal)
	if stateVal.IsNull() || base.IsNullOrUnknown() {
		return base
	}

	stateItems, diags := stateVal.AsStructSliceT(ctx)
	if diags.HasError() {
		return base
	}
	remoteItems, diags := base.AsStructSliceT(ctx)
	if diags.HasError() {
		return base
	}

	for i := range remoteItems {
		if i >= len(stateItems) {
			break
		}
		remoteItems[i] = preserve(remoteItems[i], stateItems[i])
	}

	list, diags := customfield.NewObjectList(ctx, remoteItems)
	if diags.HasError() {
		return base
	}
	return list
}

func preserveManagedStringSlice(stateVal, remoteVal []types.String) []types.String {
	if stateVal == nil {
		return nil
	}
	if remoteVal == nil && len(stateVal) == 0 {
		return stateVal
	}
	return remoteVal
}

func preservePolicyObject[T any](ctx context.Context, remote, state customfield.NestedObject[T], preserve func(remote, state T) T) customfield.NestedObject[T] {
	if state.IsNull() {
		return customfield.NullObject[T](ctx)
	}
	if state.IsUnknown() || remote.IsNull() || remote.IsUnknown() {
		return remote
	}
	stateValue, stateDiags := state.Value(ctx)
	remoteValue, remoteDiags := remote.Value(ctx)
	if stateDiags.HasError() || remoteDiags.HasError() || stateValue == nil || remoteValue == nil {
		return remote
	}
	value := preserve(*remoteValue, *stateValue)
	return customfield.NewObjectMust(ctx, &value)
}

func preserveVolumeSizePolicy(ctx context.Context, remote, state customfield.NestedObject[api.VolumeSizePolicyModel]) customfield.NestedObject[api.VolumeSizePolicyModel] {
	if value, reset := preserveEmptyPolicyReset(ctx, remote, state); reset {
		return value
	}
	return preservePolicyObject(ctx, remote, state, func(remote, state api.VolumeSizePolicyModel) api.VolumeSizePolicyModel {
		remote.PerVCPUGiB = preserveManagedInt64(state.PerVCPUGiB, remote.PerVCPUGiB)
		remote.MaxSizeGiB = preserveManagedInt64(state.MaxSizeGiB, remote.MaxSizeGiB)
		return remote
	})
}

func preserveJobOptimization(ctx context.Context, remote, state customfield.NestedObject[api.JobNodePoolOptimizationModel]) customfield.NestedObject[api.JobNodePoolOptimizationModel] {
	if value, reset := preserveEmptyPolicyReset(ctx, remote, state); reset {
		return value
	}
	if remote.IsNull() && isJobDelayReset(ctx, state) {
		return state
	}
	return preservePolicyObject(ctx, remote, state, func(remote, state api.JobNodePoolOptimizationModel) api.JobNodePoolOptimizationModel {
		remote.MinActiveNodes = preserveManagedInt32(state.MinActiveNodes, remote.MinActiveNodes)
		remote.ProvisioningDelay = preserveResetDuration(state.ProvisioningDelay, remote.ProvisioningDelay)
		return remote
	})
}
