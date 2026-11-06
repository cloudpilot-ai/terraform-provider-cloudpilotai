package eks

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client/helper"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
)

func hydratePostWriteState(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, data *ClusterModel) error {
	var err error
	data.ClusterSetting, err = hydrateClusterSettingPostWrite(ctx, client, clusterUID, data.ClusterSetting)
	if err != nil {
		return err
	}
	data.NodeClasses, err = hydrateNodeClassesPostWrite(ctx, client, clusterUID, data.NodeClasses)
	if err != nil {
		return err
	}
	data.NodeClassTemplates, err = normalizeNodeClassTemplatesPostWrite(ctx, data.NodeClassTemplates)
	if err != nil {
		return err
	}
	data.NodePools, err = hydrateNodePoolsPostWrite(ctx, client, clusterUID, data.NodePools)
	if err != nil {
		return err
	}
	data.NodePoolTemplates, err = normalizeNodePoolTemplatesPostWrite(ctx, data.NodePoolTemplates)
	if err != nil {
		return err
	}
	data.ScheduledRebalances, err = helper.HydrateScheduledRebalancesPostWrite(ctx, client, clusterUID, data.ScheduledRebalances)
	if err != nil {
		return fmt.Errorf("failed to hydrate scheduled rebalances: %w", err)
	}
	return nil
}

func hydrateNodeClassesPostWrite(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObjectList[api.EC2NodeClassModel]) (customfield.NestedObjectList[api.EC2NodeClassModel], error) {
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

	remoteByName := make(map[string]api.EC2NodeClassModel, len(remoteList.EC2NodeClasses))
	for i := range remoteList.EC2NodeClasses {
		model, err := remoteList.EC2NodeClasses[i].ToEC2NodeClassModel(ctx)
		if err != nil {
			return current, fmt.Errorf("failed to convert nodeclass %q: %w", remoteList.EC2NodeClasses[i].Name, err)
		}
		if model != nil {
			remoteByName[remoteList.EC2NodeClasses[i].Name] = *model
		}
	}

	hydrated := make([]api.EC2NodeClassModel, 0, len(stateNodeClasses))
	for _, stateNodeClass := range stateNodeClasses {
		if remote, ok := remoteByName[stateNodeClass.Name.ValueString()]; ok {
			preserved, err := preserveNodeClassStateRepresentation(ctx, remote, stateNodeClass)
			if err != nil {
				return current, fmt.Errorf("failed to preserve nodeclass state for %q: %w", stateNodeClass.Name.ValueString(), err)
			}
			hydrated = append(hydrated, preserved)
			continue
		}

		hydrated = append(hydrated, normalizeNodeClassComputedUnknowns(stateNodeClass))
	}

	list, diags := customfield.NewObjectList(ctx, hydrated)
	if diags.HasError() {
		return current, fmt.Errorf("failed to build nodeclasses state: %v", diags)
	}
	return list, nil
}

func normalizeNodeClassComputedUnknowns(model api.EC2NodeClassModel) api.EC2NodeClassModel {
	if model.AmiAlias.IsUnknown() {
		model.AmiAlias = types.StringNull()
	}
	if model.UserData.IsUnknown() {
		model.UserData = types.StringNull()
	}
	return model
}

func normalizeNodeClassTemplatesPostWrite(ctx context.Context, current customfield.NestedObjectList[api.EC2NodeClassTemplateModel]) (customfield.NestedObjectList[api.EC2NodeClassTemplateModel], error) {
	if current.IsNullOrUnknown() {
		return current, nil
	}

	templates, diags := current.AsStructSliceT(ctx)
	if diags.HasError() {
		return current, fmt.Errorf("failed to parse nodeclass_templates: %v", diags)
	}
	for i := range templates {
		if templates[i].AmiAlias.IsUnknown() {
			templates[i].AmiAlias = types.StringNull()
		}
		if templates[i].UserData.IsUnknown() {
			templates[i].UserData = types.StringNull()
		}
	}

	list, diags := customfield.NewObjectList(ctx, templates)
	if diags.HasError() {
		return current, fmt.Errorf("failed to build nodeclass_templates state: %v", diags)
	}
	return list, nil
}

func hydrateNodePoolsPostWrite(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObjectList[api.EC2NodePoolModel]) (customfield.NestedObjectList[api.EC2NodePoolModel], error) {
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

	remoteByName := make(map[string]api.EC2NodePoolModel, len(remoteList.EC2NodePools))
	for i := range remoteList.EC2NodePools {
		model, err := remoteList.EC2NodePools[i].ToEC2NodePoolModel()
		if err != nil {
			return current, fmt.Errorf("failed to convert nodepool %q: %w", remoteList.EC2NodePools[i].Name, err)
		}
		if model != nil {
			remoteByName[remoteList.EC2NodePools[i].Name] = normalizeNodePoolComputedUnknowns(*model)
		}
	}

	hydrated := make([]api.EC2NodePoolModel, 0, len(stateNodePools))
	for _, stateNodePool := range stateNodePools {
		if remote, ok := remoteByName[stateNodePool.Name.ValueString()]; ok {
			hydrated = append(hydrated, normalizeNodePoolComputedUnknowns(preserveNodePoolStateRepresentation(ctx, remote, stateNodePool)))
			continue
		}

		hydrated = append(hydrated, normalizeNodePoolComputedUnknowns(stateNodePool))
	}

	list, diags := customfield.NewObjectList(ctx, hydrated)
	if diags.HasError() {
		return current, fmt.Errorf("failed to build nodepools state: %v", diags)
	}
	return list, nil
}

func normalizeNodePoolComputedUnknowns(model api.EC2NodePoolModel) api.EC2NodePoolModel {
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

func normalizeNodePoolTemplatesPostWrite(ctx context.Context, current customfield.NestedObjectList[api.EC2NodePoolTemplateModel]) (customfield.NestedObjectList[api.EC2NodePoolTemplateModel], error) {
	if current.IsNullOrUnknown() {
		return current, nil
	}

	templates, diags := current.AsStructSliceT(ctx)
	if diags.HasError() {
		return current, fmt.Errorf("failed to parse nodepool_templates: %v", diags)
	}
	for i := range templates {
		if templates[i].NodeClass.IsUnknown() {
			templates[i].NodeClass = types.StringNull()
		}
		if templates[i].InstanceCPUMIN.IsUnknown() {
			templates[i].InstanceCPUMIN = types.Int64Null()
		}
		if templates[i].InstanceMemoryMIN.IsUnknown() {
			templates[i].InstanceMemoryMIN = types.Int64Null()
		}
	}

	list, diags := customfield.NewObjectList(ctx, templates)
	if diags.HasError() {
		return current, fmt.Errorf("failed to build nodepool_templates state: %v", diags)
	}
	return list, nil
}

func validateNodeClassDiskFields(ctx context.Context, data *ClusterModel) error {
	if err := validateNodeClassDiskFieldsForClasses(ctx, data.NodeClasses); err != nil {
		return err
	}
	return validateNodeClassDiskFieldsForTemplates(ctx, data.NodeClassTemplates)
}

func validateNodeClassDiskFieldsForClasses(ctx context.Context, list customfield.NestedObjectList[api.EC2NodeClassModel]) error {
	if list.IsNullOrUnknown() {
		return nil
	}
	items, diags := list.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("nodeclasses: %v", diags)
	}
	for _, item := range items {
		if hasSystemDiskSize(item.SystemDiskSizeGib) && !item.BlockDeviceMappings.IsNullOrUnknown() {
			return fmt.Errorf("nodeclass %q configures both system_disk_size_gib and block_device_mappings; choose one disk representation", item.Name.ValueString())
		}
	}
	return nil
}

func validateNodeClassDiskFieldsForTemplates(ctx context.Context, list customfield.NestedObjectList[api.EC2NodeClassTemplateModel]) error {
	if list.IsNullOrUnknown() {
		return nil
	}
	items, diags := list.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("nodeclass_templates: %v", diags)
	}
	for _, item := range items {
		if hasSystemDiskSize(item.SystemDiskSizeGib) && !item.BlockDeviceMappings.IsNullOrUnknown() {
			return fmt.Errorf("nodeclass template %q configures both system_disk_size_gib and block_device_mappings; choose one disk representation", item.TemplateName.ValueString())
		}
	}
	return nil
}

func hasSystemDiskSize(value types.Int64) bool {
	return !value.IsNull() && !value.IsUnknown()
}

func preserveNodeClassStateRepresentation(ctx context.Context, remote, state api.EC2NodeClassModel) (api.EC2NodeClassModel, error) {
	remote.TemplateName = state.TemplateName
	remote.Role = preserveManagedString(state.Role, remote.Role)
	remote.EnableImageAccelerator = preserveManagedBool(state.EnableImageAccelerator, remote.EnableImageAccelerator)
	remote.EnableLocalSSDEphemeralStorage = preserveManagedBool(state.EnableLocalSSDEphemeralStorage, remote.EnableLocalSSDEphemeralStorage)
	remote.AmiAlias = preserveManagedString(state.AmiAlias, remote.AmiAlias)
	remote.UserData = preserveMemoryExtensionUserData(state.UserData, preserveManagedString(state.UserData, remote.UserData), memoryExtensionEnabled(ctx, remote.MemoryExtension))
	remote.MemoryExtension = preserveMemoryExtension(ctx, remote.MemoryExtension, state.MemoryExtension)
	remote.SubnetSelectorTerms = preserveManagedObjectList(ctx, state.SubnetSelectorTerms, remote.SubnetSelectorTerms)
	remote.SecurityGroupSelectorTerms = preserveManagedObjectList(ctx, state.SecurityGroupSelectorTerms, remote.SecurityGroupSelectorTerms)
	remote.InstanceTags = preserveManagedMap(ctx, state.InstanceTags, remote.InstanceTags)
	remote.ExtraCPUAllocationMCore = preserveManagedInt64(state.ExtraCPUAllocationMCore, remote.ExtraCPUAllocationMCore)
	remote.ExtraMemoryAllocationMib = preserveManagedInt64(state.ExtraMemoryAllocationMib, remote.ExtraMemoryAllocationMib)
	remote.OriginNodeClassJSON = preserveManagedString(state.OriginNodeClassJSON, remote.OriginNodeClassJSON)

	if !hasSystemDiskSize(state.SystemDiskSizeGib) {
		remote.SystemDiskSizeGib = preserveManagedInt64(state.SystemDiskSizeGib, remote.SystemDiskSizeGib)
		remote.BlockDeviceMappings = preserveBlockDeviceMappings(ctx, remote.BlockDeviceMappings, state.BlockDeviceMappings)
		return remote, nil
	}

	size, ok, err := systemDiskSizeFromBlockDeviceMappings(ctx, remote.BlockDeviceMappings)
	if err != nil {
		return remote, err
	}
	if ok {
		remote.SystemDiskSizeGib = size
	} else {
		remote.SystemDiskSizeGib = types.Int64Null()
	}
	remote.BlockDeviceMappings = customfield.NullObjectList[api.BlockDeviceMappingModel](ctx)
	return remote, nil
}

func preserveNodePoolStateRepresentation(ctx context.Context, remote, state api.EC2NodePoolModel) api.EC2NodePoolModel {
	remote.TemplateName = state.TemplateName
	remote.Enable = preserveManagedBool(state.Enable, remote.Enable)
	remote.NodeClass = preserveManagedString(state.NodeClass, remote.NodeClass)
	remote.EnableImageAccelerator = preserveManagedBool(state.EnableImageAccelerator, remote.EnableImageAccelerator)
	remote.EnableGPU = preserveManagedBool(state.EnableGPU, remote.EnableGPU)
	remote.ProvisionPriority = preserveManagedInt32(state.ProvisionPriority, remote.ProvisionPriority)
	remote.InstanceFamily = preserveManagedList(state.InstanceFamily, remote.InstanceFamily)
	remote.InstanceArch = preserveManagedList(state.InstanceArch, remote.InstanceArch)
	remote.CapacityType = preserveManagedList(state.CapacityType, remote.CapacityType)
	remote.Zone = preserveManagedList(state.Zone, remote.Zone)
	remote.InstanceCPUMAX = preserveManagedInt64(state.InstanceCPUMAX, remote.InstanceCPUMAX)
	remote.InstanceCPUMIN = preserveManagedInt64(state.InstanceCPUMIN, remote.InstanceCPUMIN)
	remote.InstanceMemoryMAX = preserveManagedInt64(state.InstanceMemoryMAX, remote.InstanceMemoryMAX)
	remote.InstanceMemoryMIN = preserveManagedInt64(state.InstanceMemoryMIN, remote.InstanceMemoryMIN)
	remote.NodeDisruptionLimit = preserveManagedString(state.NodeDisruptionLimit, remote.NodeDisruptionLimit)
	remote.NodeDisruptionBudgets = preserveDisruptionBudgetsStateRepresentation(ctx, remote.NodeDisruptionBudgets, state.NodeDisruptionBudgets)
	remote.NodeDisruptionDelay = preserveManagedDuration(state.NodeDisruptionDelay, remote.NodeDisruptionDelay)
	remote.DisruptionCooldown = preserveClearedDuration(state.DisruptionCooldown, remote.DisruptionCooldown)
	remote.JobNodePoolOptimization = preserveJobOptimization(ctx, remote.JobNodePoolOptimization, state.JobNodePoolOptimization)
	remote.NRISwapPolicy = preserveNRISwapPolicy(ctx, remote.NRISwapPolicy, state.NRISwapPolicy)
	remote.Labels = preserveManagedMap(ctx, state.Labels, remote.Labels)
	remote.Taints = preserveManagedObjectList(ctx, state.Taints, remote.Taints)
	remote.OriginNodePoolJSON = preserveManagedString(state.OriginNodePoolJSON, remote.OriginNodePoolJSON)
	return remote
}

func preserveWorkloadStateRepresentation(remote, state api.WorkloadModel) api.WorkloadModel {
	remote.TemplateName = state.TemplateName
	remote.RebalanceAble = preserveManagedBool(state.RebalanceAble, remote.RebalanceAble)
	remote.SpotFriendly = preserveManagedBool(state.SpotFriendly, remote.SpotFriendly)
	remote.MinNonSpotReplicas = preserveManagedInt64(state.MinNonSpotReplicas, remote.MinNonSpotReplicas)
	return remote
}

func systemDiskSizeFromBlockDeviceMappings(ctx context.Context, mappings customfield.NestedObjectList[api.BlockDeviceMappingModel]) (types.Int64, bool, error) {
	if mappings.IsNullOrUnknown() {
		return types.Int64Null(), false, nil
	}
	items, diags := mappings.AsStructSliceT(ctx)
	if diags.HasError() {
		return types.Int64Null(), false, fmt.Errorf("block_device_mappings: %v", diags)
	}
	if len(items) == 0 || items[0].EBS.IsNull() || items[0].EBS.IsUnknown() {
		return types.Int64Null(), false, nil
	}
	ebs, ebsDiags := items[0].EBS.Value(ctx)
	if ebsDiags.HasError() {
		return types.Int64Null(), false, fmt.Errorf("block_device_mappings.ebs: %v", ebsDiags)
	}
	if ebs == nil || ebs.VolumeSize.IsNull() || ebs.VolumeSize.IsUnknown() || ebs.VolumeSize.ValueString() == "" {
		return types.Int64Null(), false, nil
	}
	quantity, err := k8sresource.ParseQuantity(ebs.VolumeSize.ValueString())
	if err != nil {
		return types.Int64Null(), false, fmt.Errorf("block_device_mappings.ebs.volume_size: %w", err)
	}
	return types.Int64Value(quantity.Value() / int64(api.BytesToGiB)), true, nil
}

// orderByStateName returns server items that are tracked in state, preserving
// state order. Items on the server but NOT in state are ignored — Terraform
// only manages resources it declared.
func orderByStateName[T any](stateItems []T, serverByName map[string]T, getName func(T) string) []T {
	result := make([]T, 0, len(stateItems))
	for _, stateItem := range stateItems {
		name := getName(stateItem)
		if serverItem, ok := serverByName[name]; ok {
			result = append(result, serverItem)
		}
	}
	return result
}

// extractResourceNames extracts the set of names from a NestedObjectList in
// Terraform state. Returns nil if the list is null/unknown. This is used to
// determine which resources were previously managed by Terraform so that only
// those can be considered for deletion during sync.
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

func resolveClusterUID(preferred, fallback, clusterName, region, accountID types.String) string {
	if !preferred.IsNull() && !preferred.IsUnknown() && preferred.ValueString() != "" {
		return preferred.ValueString()
	}
	if !fallback.IsNull() && !fallback.IsUnknown() && fallback.ValueString() != "" {
		return fallback.ValueString()
	}
	return api.GenerateClusterUID(api.CloudProviderAWS, clusterName.ValueString(), region.ValueString(), accountID.ValueString())
}

func resolveDeleteClusterUID(clusterID, clusterName, region, accountID types.String) string {
	return resolveClusterUID(clusterID, clusterID, clusterName, region, accountID)
}

// preserveSemanticDuration keeps the state value when the state and server
// values represent the same duration (e.g. "60m" vs "1h"), avoiding false
// Terraform diffs caused by different textual representations.
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

func isManagedBool(value types.Bool) bool {
	return !value.IsNull()
}

func boolValueOrDefault(value types.Bool, fallback bool) bool {
	if !isManagedBool(value) {
		return fallback
	}
	return value.ValueBool()
}

func int64ValueOrDefault(value types.Int64, fallback int64) int64 {
	if value.IsNull() || value.IsUnknown() {
		return fallback
	}
	return value.ValueInt64()
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

func preserveDisruptionBudgetsStateRepresentation(ctx context.Context, remote, state customfield.NestedObjectList[api.DisruptionBudgetModel]) customfield.NestedObjectList[api.DisruptionBudgetModel] {
	base := preserveManagedObjectList(ctx, state, remote)
	if state.IsNull() || base.IsNullOrUnknown() {
		return base
	}

	stateBudgets, stateDiags := state.AsStructSliceT(ctx)
	remoteBudgets, remoteDiags := base.AsStructSliceT(ctx)
	if stateDiags.HasError() || remoteDiags.HasError() {
		return base
	}
	for index := range remoteBudgets {
		if index >= len(stateBudgets) {
			break
		}
		remoteBudgets[index].Duration = preserveSemanticDuration(stateBudgets[index].Duration, remoteBudgets[index].Duration)
	}

	result, diags := customfield.NewObjectList(ctx, remoteBudgets)
	if diags.HasError() {
		return base
	}
	return result
}

func (c *Cluster) readClusterSetting(ctx context.Context, data *ClusterModel, clusterUID string, isImport bool, resp *resource.ReadResponse) bool {
	clusterSetting, err := c.client.GetClusterSetting(clusterUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to get cluster setting",
			err.Error(),
		)
		return false
	}

	rebalanceConfiguration, err := c.client.GetRebalanceConfiguration(clusterUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to get rebalance configuration",
			err.Error(),
		)
		return false
	}

	if isImport || !data.EnableRebalance.IsNull() {
		data.EnableRebalance = types.BoolValue(rebalanceConfiguration.Enable)
	}

	if !data.ClusterSetting.IsNull() && !data.ClusterSetting.IsUnknown() {
		data.ClusterSetting, err = clusterSettingObjectPreservingState(ctx, data.ClusterSetting, clusterSetting)
		if err != nil {
			resp.Diagnostics.AddError("failed to preserve cluster setting state", err.Error())
			return false
		}
	} else if isImport {
		data.ClusterSetting = clusterSettingObjectFromAPI(ctx, clusterSetting)
	}

	return true
}
func (c *Cluster) readWorkloads(ctx context.Context, data *ClusterModel, clusterUID string, isImport bool, resp *resource.ReadResponse) bool {
	// Always fetch workload configuration from server (supports both normal read and import)
	workloadConfiguration, err := c.client.GetWorkloadRebalanceConfiguration(clusterUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to get workload rebalance configuration",
			err.Error(),
		)
		return false
	}
	if workloadConfiguration == nil {
		workloadConfiguration = &api.ClusterWorkloadSpec{}
	}

	workloadM := lo.SliceToMap(workloadConfiguration.Workloads, func(item api.Workload) (string, api.Workload) {
		return item.Namespace + "/" + item.Type + "/" + item.Name, item
	})

	if !data.Workloads.IsNullOrUnknown() {
		workloadModels, diagnostics := data.Workloads.AsStructSliceT(ctx)
		if diagnostics.HasError() {
			resp.Diagnostics.Append(diagnostics...)
			return false
		}

		for wi := range workloadModels {
			k := workloadModels[wi].Namespace.ValueString() + "/" + workloadModels[wi].Type.ValueString() + "/" + workloadModels[wi].Name.ValueString()
			if v, ok := workloadM[k]; ok {
				stateModel := workloadModels[wi]
				workloadModels[wi] = preserveWorkloadStateRepresentation(*v.ToWorkloadModel(), stateModel)
			}
		}
		data.Workloads = customfield.NewObjectListMust(ctx, workloadModels)
	} else if isImport && len(workloadConfiguration.Workloads) > 0 {
		allWorkloads := make([]api.WorkloadModel, 0, len(workloadConfiguration.Workloads))
		for i := range workloadConfiguration.Workloads {
			if m := workloadConfiguration.Workloads[i].ToWorkloadModel(); m != nil {
				allWorkloads = append(allWorkloads, *m)
			}
		}
		data.Workloads = customfield.NewObjectListMust(ctx, allWorkloads)
	}

	return true
}
func (c *Cluster) readNodeClasses(ctx context.Context, data *ClusterModel, clusterUID string, isImport bool, resp *resource.ReadResponse) bool {
	// Always fetch nodeclasses from server (supports both normal read and import)
	nodeClassList, err := c.client.ListNodeClasses(clusterUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to list nodeclasses",
			err.Error(),
		)
		return false
	}

	ncByName := make(map[string]api.EC2NodeClassModel, len(nodeClassList.EC2NodeClasses))
	for ni := range nodeClassList.EC2NodeClasses {
		ncModel, err := nodeClassList.EC2NodeClasses[ni].ToEC2NodeClassModel(ctx)
		if err != nil {
			resp.Diagnostics.AddError("failed to convert nodeclass", err.Error())
			return false
		}
		if ncModel != nil {
			ncByName[nodeClassList.EC2NodeClasses[ni].Name] = *ncModel
		}
	}

	if !data.NodeClasses.IsNullOrUnknown() {
		stateNCs, diags := data.NodeClasses.AsStructSliceT(ctx)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return false
		}

		stateNCByName := make(map[string]api.EC2NodeClassModel, len(stateNCs))
		for _, s := range stateNCs {
			stateNCByName[s.Name.ValueString()] = s
		}
		for name, nc := range ncByName {
			if stateNC, ok := stateNCByName[name]; ok {
				var err error
				nc, err = preserveNodeClassStateRepresentation(ctx, nc, stateNC)
				if err != nil {
					resp.Diagnostics.AddError("failed to preserve nodeclass state", err.Error())
					return false
				}
				ncByName[name] = nc
			}
		}

		orderedNCs := orderByStateName(stateNCs, ncByName, func(m api.EC2NodeClassModel) string {
			return m.Name.ValueString()
		})
		nodeClasses, diag := customfield.NewObjectList(ctx, orderedNCs)
		if diag.HasError() {
			resp.Diagnostics.Append(diag...)
			return false
		}
		data.NodeClasses = nodeClasses
	} else if isImport && len(ncByName) > 0 {
		allNCs := sortedValuesByName(ncByName, func(m api.EC2NodeClassModel) string {
			return m.Name.ValueString()
		})
		nodeClasses, diag := customfield.NewObjectList(ctx, allNCs)
		if diag.HasError() {
			resp.Diagnostics.Append(diag...)
			return false
		}
		data.NodeClasses = nodeClasses
	}

	return true
}
func (c *Cluster) readNodePools(ctx context.Context, data *ClusterModel, clusterUID string, isImport bool, resp *resource.ReadResponse) bool {
	// Always fetch nodepools from server (supports both normal read and import)
	nodePoolList, err := c.client.ListNodePools(clusterUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to list nodepools",
			err.Error(),
		)
		return false
	}

	npByName := make(map[string]api.EC2NodePoolModel, len(nodePoolList.EC2NodePools))
	for ni := range nodePoolList.EC2NodePools {
		npModel, err := nodePoolList.EC2NodePools[ni].ToEC2NodePoolModel()
		if err != nil {
			resp.Diagnostics.AddError("failed to convert nodepool", err.Error())
			return false
		}
		if npModel != nil {
			npByName[nodePoolList.EC2NodePools[ni].Name] = *npModel
		}
	}

	if !data.NodePools.IsNullOrUnknown() {
		stateNPs, diags := data.NodePools.AsStructSliceT(ctx)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return false
		}

		stateNPByName := make(map[string]api.EC2NodePoolModel, len(stateNPs))
		for _, s := range stateNPs {
			stateNPByName[s.Name.ValueString()] = s
		}
		for name, np := range npByName {
			if stateNP, ok := stateNPByName[name]; ok {
				np = preserveNodePoolStateRepresentation(ctx, np, stateNP)
				npByName[name] = np
			}
		}

		orderedNPs := orderByStateName(stateNPs, npByName, func(m api.EC2NodePoolModel) string {
			return m.Name.ValueString()
		})
		nodePools, diag := customfield.NewObjectList(ctx, orderedNPs)
		if diag.HasError() {
			resp.Diagnostics.Append(diag...)
			return false
		}
		data.NodePools = nodePools
	} else if isImport && len(npByName) > 0 {
		allNPs := sortedValuesByName(npByName, func(m api.EC2NodePoolModel) string {
			return m.Name.ValueString()
		})
		for i := range allNPs {
			// Import the complete budgets without also owning their deprecated alias.
			if !allNPs[i].NodeDisruptionBudgets.IsNullOrUnknown() {
				allNPs[i].NodeDisruptionLimit = types.StringNull()
			}
		}
		nodePools, diag := customfield.NewObjectList(ctx, allNPs)
		if diag.HasError() {
			resp.Diagnostics.Append(diag...)
			return false
		}
		data.NodePools = nodePools
	}

	return true
}

func preservePolicyObject[T any](ctx context.Context, remote, state customfield.NestedObject[T], preserve func(remote, state T) T) customfield.NestedObject[T] {
	if state.IsNull() {
		return customfield.NullObject[T](ctx)
	}
	if state.IsUnknown() || remote.IsNull() || remote.IsUnknown() || api.IsKnownEmptyObject(state) {
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
	if api.IsKnownEmptyObject(state) && remote.IsNull() {
		return state
	}
	return preservePolicyObject(ctx, remote, state, func(remote, state api.VolumeSizePolicyModel) api.VolumeSizePolicyModel {
		remote.PerVCPUGiB = preserveManagedInt64(state.PerVCPUGiB, remote.PerVCPUGiB)
		remote.MaxSizeGiB = preserveManagedInt64(state.MaxSizeGiB, remote.MaxSizeGiB)
		return remote
	})
}

func preserveJobOptimization(ctx context.Context, remote, state customfield.NestedObject[api.JobNodePoolOptimizationModel]) customfield.NestedObject[api.JobNodePoolOptimizationModel] {
	if remote.IsNull() && (api.IsKnownEmptyObject(state) || jobOptimizationClearsLastField(ctx, state)) {
		return state
	}
	return preservePolicyObject(ctx, remote, state, func(remote, state api.JobNodePoolOptimizationModel) api.JobNodePoolOptimizationModel {
		remote.MinActiveNodes = preserveManagedInt32(state.MinActiveNodes, remote.MinActiveNodes)
		remote.ProvisioningDelay = preserveClearedDuration(state.ProvisioningDelay, remote.ProvisioningDelay)
		return remote
	})
}

func preserveMemoryExtension(ctx context.Context, remote, state customfield.NestedObject[api.MemoryExtensionModel]) customfield.NestedObject[api.MemoryExtensionModel] {
	if api.IsKnownEmptyObject(state) {
		if remote.IsNull() || memoryExtensionDisabled(ctx, remote) {
			return state
		}
		return remote
	}
	return preservePolicyObject(ctx, remote, state, func(remote, state api.MemoryExtensionModel) api.MemoryExtensionModel {
		remote.Enabled = preserveManagedBool(state.Enabled, remote.Enabled)
		remote.ReclaimPolicy = preserveResetString(state.ReclaimPolicy, remote.ReclaimPolicy, "Conservative")
		remote.ColdMemory = preserveColdMemory(ctx, remote.ColdMemory, state.ColdMemory)
		remote.CachedMemory = preserveCachedMemory(ctx, remote.CachedMemory, state.CachedMemory)
		return remote
	})
}

func preserveBlockDeviceMappings(ctx context.Context, remote, state customfield.NestedObjectList[api.BlockDeviceMappingModel]) customfield.NestedObjectList[api.BlockDeviceMappingModel] {
	if state.IsNull() {
		return customfield.NullObjectList[api.BlockDeviceMappingModel](ctx)
	}
	if !state.IsUnknown() && len(state.Elements()) == 0 && remote.IsNull() {
		return state
	}
	if state.IsUnknown() || remote.IsNullOrUnknown() {
		return remote
	}
	previous, previousDiags := state.AsStructSliceT(ctx)
	current, currentDiags := remote.AsStructSliceT(ctx)
	if previousDiags.HasError() || currentDiags.HasError() {
		return remote
	}
	for i := range current {
		if i >= len(previous) {
			break
		}
		current[i].DeviceName = preserveManagedString(previous[i].DeviceName, current[i].DeviceName)
		current[i].RootVolume = preserveManagedBool(previous[i].RootVolume, current[i].RootVolume)
		current[i].EBS = preserveEBS(ctx, current[i].EBS, previous[i].EBS)
	}
	result, diags := customfield.NewObjectList(ctx, current)
	if diags.HasError() {
		return remote
	}
	return result
}
