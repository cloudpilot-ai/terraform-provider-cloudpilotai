package gke

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
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
	if in.Discount != nil {
		model.Discount = types.Float64Value(*in.Discount)
		hasValue = true
	}
	if in.PreRunCommand != nil {
		model.PreRunCommand = types.StringValue(*in.PreRunCommand)
		hasValue = true
	}
	if in.PostRunCommand != nil {
		model.PostRunCommand = types.StringValue(*in.PostRunCommand)
		hasValue = true
	}

	if !hasValue {
		return customfield.NullObject[ClusterSettingModel](ctx)
	}
	return customfield.NewObjectMust(ctx, &model)
}
