package clustersetting

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
)

var (
	_ resource.Resource                = &ClusterSetting{}
	_ resource.ResourceWithImportState = &ClusterSetting{}
)

type ClusterSetting struct {
	client cloudpilotaiclient.Interface
}

func NewClusterSetting() resource.Resource {
	return &ClusterSetting{}
}

func (r *ClusterSetting) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_setting"
}

func (r *ClusterSetting) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = Schema(ctx)
}

func (r *ClusterSetting) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(cloudpilotaiclient.Interface)
	if !ok {
		resp.Diagnostics.AddError(
			"unexpected resource configure type",
			fmt.Sprintf("Expected cloudpilotaiclient.Interface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.client = client
}

func (r *ClusterSetting) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("cluster_id"), req, resp)
}

func (r *ClusterSetting) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterSettingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.applyClusterSetting(data); err != nil {
		resp.Diagnostics.AddError("failed to update cluster setting", err.Error())
		return
	}
	setting, err := r.client.GetClusterSetting(data.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("failed to read cluster setting", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, ClusterSettingModelFromAPI(data.ClusterID.ValueString(), setting))...)
}

func (r *ClusterSetting) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClusterSettingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	setting, err := r.client.GetClusterSetting(data.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("failed to read cluster setting", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, ClusterSettingModelFromAPI(data.ClusterID.ValueString(), setting))...)
}

func (r *ClusterSetting) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ClusterSettingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.applyClusterSetting(data); err != nil {
		resp.Diagnostics.AddError("failed to update cluster setting", err.Error())
		return
	}
	setting, err := r.client.GetClusterSetting(data.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("failed to read cluster setting", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, ClusterSettingModelFromAPI(data.ClusterID.ValueString(), setting))...)
}

func (r *ClusterSetting) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.State.RemoveResource(ctx)
}

func (r *ClusterSetting) applyClusterSetting(data ClusterSettingModel) error {
	clusterID := data.ClusterID.ValueString()
	if err := r.client.UpdateClusterSetting(clusterID, data.ToAPI()); err != nil {
		return err
	}
	if status := data.ToMaintenanceStatus(); status != nil {
		if err := r.client.UpdateClusterMaintenanceStatus(clusterID, status); err != nil {
			return err
		}
	}
	return nil
}
