package clustersetting

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
)

type ClusterSettingModel struct {
	ClusterID          types.String  `tfsdk:"cluster_id"`
	EnableNodeRepair   types.Bool    `tfsdk:"enable_node_repair"`
	EnableDiskMonitor  types.Bool    `tfsdk:"enable_disk_monitor"`
	MaintenanceEnabled types.Bool    `tfsdk:"maintenance_enabled"`
	Discount           types.Float64 `tfsdk:"discount"`
	PreRunCommand      types.String  `tfsdk:"pre_run_command"`
	PostRunCommand     types.String  `tfsdk:"post_run_command"`
}

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

func (m ClusterSettingModel) ToMaintenanceStatus() *api.ClusterMaintenanceStatus {
	if m.MaintenanceEnabled.IsNull() || m.MaintenanceEnabled.IsUnknown() {
		return nil
	}
	v := m.MaintenanceEnabled.ValueBool()
	return &api.ClusterMaintenanceStatus{MaintenanceModeEnabled: &v}
}

func ClusterSettingModelFromAPI(clusterID string, in *api.ClusterSetting) ClusterSettingModel {
	m := ClusterSettingModel{ClusterID: types.StringValue(clusterID)}
	if in == nil {
		return m
	}
	if in.EnableNodeRepair != nil {
		m.EnableNodeRepair = types.BoolValue(*in.EnableNodeRepair)
	}
	if in.EnableDiskMonitor != nil {
		m.EnableDiskMonitor = types.BoolValue(*in.EnableDiskMonitor)
	}
	if in.MaintenanceEnabled != nil {
		m.MaintenanceEnabled = types.BoolValue(*in.MaintenanceEnabled)
	}
	if in.Discount != nil {
		m.Discount = types.Float64Value(*in.Discount)
	}
	if in.PreRunCommand != nil {
		m.PreRunCommand = types.StringValue(*in.PreRunCommand)
	}
	if in.PostRunCommand != nil {
		m.PostRunCommand = types.StringValue(*in.PostRunCommand)
	}
	return m
}
