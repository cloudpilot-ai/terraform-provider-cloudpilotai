package gke

import (
	"context"
	"fmt"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func (m ClusterSettingModel) ToAPI() *api.ClusterSetting {
	out := &api.ClusterSetting{}
	if !m.EnableNodeRepair.IsNull() && !m.EnableNodeRepair.IsUnknown() {
		v := m.EnableNodeRepair.ValueBool()
		out.EnableNodeRepair = &v
	}
	if !m.EnableDiskMonitor.IsNull() && !m.EnableDiskMonitor.IsUnknown() {
		v := m.EnableDiskMonitor.ValueBool()
		out.EnableDiskMonitor = &v
	}
	if !m.EnableNodePoolDecommission.IsNull() && !m.EnableNodePoolDecommission.IsUnknown() {
		v := m.EnableNodePoolDecommission.ValueBool()
		out.EnableNodePoolDecommission = &v
	}
	if !m.EnableWorkloadMinNonSpot.IsNull() && !m.EnableWorkloadMinNonSpot.IsUnknown() {
		v := m.EnableWorkloadMinNonSpot.ValueBool()
		out.EnableWorkloadMinNonSpot = &v
	}
	if !m.DisruptionCooldown.IsNull() && !m.DisruptionCooldown.IsUnknown() {
		v := m.DisruptionCooldown.ValueString()
		out.DisruptionCooldown = &v
	}
	if !m.Discount.IsNull() && !m.Discount.IsUnknown() {
		v := m.Discount.ValueFloat64()
		out.Discount = &v
	}
	if !m.PreRunCommand.IsNull() && !m.PreRunCommand.IsUnknown() {
		v := m.PreRunCommand.ValueString()
		out.PreRunCommand = &v
	}
	if !m.PostRunCommand.IsNull() && !m.PostRunCommand.IsUnknown() {
		v := m.PostRunCommand.ValueString()
		out.PostRunCommand = &v
	}
	return out
}

func clusterSettingObjectFromAPI(ctx context.Context, in *api.ClusterSetting) customfield.NestedObject[ClusterSettingModel] {
	if in == nil {
		return customfield.NullObject[ClusterSettingModel](ctx)
	}
	model := ClusterSettingModel{}
	hasValue := false
	if in.EnableNodeRepair != nil {
		model.EnableNodeRepair = types.BoolValue(*in.EnableNodeRepair)
		hasValue = true
	}
	if in.EnableDiskMonitor != nil {
		model.EnableDiskMonitor = types.BoolValue(*in.EnableDiskMonitor)
		hasValue = true
	}
	if in.EnableNodePoolDecommission != nil {
		model.EnableNodePoolDecommission = types.BoolValue(*in.EnableNodePoolDecommission)
		hasValue = true
	}
	if in.EnableWorkloadMinNonSpot != nil {
		model.EnableWorkloadMinNonSpot = types.BoolValue(*in.EnableWorkloadMinNonSpot)
		hasValue = true
	}
	if in.DisruptionCooldown != nil {
		model.DisruptionCooldown = types.StringValue(*in.DisruptionCooldown)
		hasValue = true
	}
	if in.Discount != nil {
		model.Discount = types.Float64Value(*in.Discount)
		hasValue = true
	}
	if in.PreRunCommand != nil && *in.PreRunCommand != "" {
		model.PreRunCommand = types.StringValue(*in.PreRunCommand)
		hasValue = true
	}
	if in.PostRunCommand != nil && *in.PostRunCommand != "" {
		model.PostRunCommand = types.StringValue(*in.PostRunCommand)
		hasValue = true
	}
	if !hasValue {
		return customfield.NullObject[ClusterSettingModel](ctx)
	}
	return customfield.NewObjectMust(ctx, &model)
}

func hydrateClusterSettingPostWrite(ctx context.Context, client postWriteStateHydratorClient, clusterUID string, current customfield.NestedObject[ClusterSettingModel]) (customfield.NestedObject[ClusterSettingModel], error) {
	if current.IsNull() || current.IsUnknown() {
		return current, nil
	}
	remote, err := client.GetClusterSetting(clusterUID)
	if err != nil {
		return current, fmt.Errorf("failed to get cluster setting: %w", err)
	}
	return clusterSettingObjectPreservingState(ctx, current, remote)
}

func clusterSettingObjectPreservingState(ctx context.Context, current customfield.NestedObject[ClusterSettingModel], remote *api.ClusterSetting) (customfield.NestedObject[ClusterSettingModel], error) {
	if current.IsNull() || current.IsUnknown() {
		return current, nil
	}

	value, diags := current.Value(ctx)
	if diags.HasError() {
		return current, fmt.Errorf("failed to parse cluster_setting: %v", diags)
	}

	hydrated := ClusterSettingModel{}
	if value != nil {
		hydrated = *value
	}
	mergeClusterSettingFromAPI(&hydrated, remote)
	normalizeClusterSettingUnknowns(&hydrated)

	object, diags := customfield.NewObject(ctx, &hydrated)
	if diags.HasError() {
		return current, fmt.Errorf("failed to build cluster_setting state: %v", diags)
	}
	return object, nil
}

func normalizeClusterSettingUnknowns(setting *ClusterSettingModel) {
	if setting.EnableNodeRepair.IsUnknown() {
		setting.EnableNodeRepair = types.BoolNull()
	}
	if setting.EnableDiskMonitor.IsUnknown() {
		setting.EnableDiskMonitor = types.BoolNull()
	}
	if setting.EnableNodePoolDecommission.IsUnknown() {
		setting.EnableNodePoolDecommission = types.BoolNull()
	}
	if setting.EnableWorkloadMinNonSpot.IsUnknown() {
		setting.EnableWorkloadMinNonSpot = types.BoolNull()
	}
	if setting.DisruptionCooldown.IsUnknown() {
		setting.DisruptionCooldown = types.StringNull()
	}
	if setting.Discount.IsUnknown() {
		setting.Discount = types.Float64Null()
	}
	if setting.PreRunCommand.IsUnknown() {
		setting.PreRunCommand = types.StringNull()
	}
	if setting.PostRunCommand.IsUnknown() {
		setting.PostRunCommand = types.StringNull()
	}
}

func mergeClusterSettingFromAPI(setting *ClusterSettingModel, remote *api.ClusterSetting) {
	if remote == nil {
		remote = &api.ClusterSetting{}
	}
	if !setting.EnableNodeRepair.IsNull() {
		setting.EnableNodeRepair = boolPointerToTerraform(remote.EnableNodeRepair)
	}
	if !setting.EnableDiskMonitor.IsNull() {
		setting.EnableDiskMonitor = boolPointerToTerraform(remote.EnableDiskMonitor)
	}
	if !setting.EnableNodePoolDecommission.IsNull() {
		setting.EnableNodePoolDecommission = boolPointerToTerraform(remote.EnableNodePoolDecommission)
	}
	if !setting.EnableWorkloadMinNonSpot.IsNull() {
		setting.EnableWorkloadMinNonSpot = boolPointerToTerraform(remote.EnableWorkloadMinNonSpot)
	}
	if !setting.DisruptionCooldown.IsNull() {
		setting.DisruptionCooldown = preserveSemanticDuration(setting.DisruptionCooldown, stringPointerToTerraform(remote.DisruptionCooldown))
	}
	if !setting.Discount.IsNull() {
		setting.Discount = float64PointerToTerraform(remote.Discount)
	}
	if !setting.PreRunCommand.IsNull() {
		setting.PreRunCommand = stringPointerToTerraform(remote.PreRunCommand)
	}
	if !setting.PostRunCommand.IsNull() {
		setting.PostRunCommand = stringPointerToTerraform(remote.PostRunCommand)
	}
}

func boolPointerToTerraform(value *bool) types.Bool {
	if value == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*value)
}

func float64PointerToTerraform(value *float64) types.Float64 {
	if value == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*value)
}

func stringPointerToTerraform(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func (m ClusterSettingModel) Validate() error {
	return m.ToAPI().Validate()
}

func validateClusterSetting(ctx context.Context, value customfield.NestedObject[ClusterSettingModel]) error {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	setting, diags := value.Value(ctx)
	if diags.HasError() {
		return fmt.Errorf("failed to parse cluster_setting: %v", diags)
	}
	if setting == nil {
		return nil
	}
	return setting.Validate()
}
