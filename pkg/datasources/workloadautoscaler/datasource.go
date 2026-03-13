package workloadautoscaler

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
)

var _ datasource.DataSource = &WorkloadAutoscalerDataSource{}

type WorkloadAutoscalerDataSource struct {
	client cloudpilotaiclient.Interface
}

func NewWorkloadAutoscalerDataSource() datasource.DataSource {
	return &WorkloadAutoscalerDataSource{}
}

func (d *WorkloadAutoscalerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workload_autoscaler"
}

func (d *WorkloadAutoscalerDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = Schema(ctx)
}

func (d *WorkloadAutoscalerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(cloudpilotaiclient.Interface)
	if !ok {
		resp.Diagnostics.AddError(
			"unexpected data source configure type",
			fmt.Sprintf("Expected cloudpilotaiclient.Interface, got: %T.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *WorkloadAutoscalerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data WorkloadAutoscalerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := data.ClusterID.ValueString()

	conf, err := d.client.GetWAConfiguration(clusterID)
	if err != nil {
		resp.Diagnostics.AddError("failed to read Workload Autoscaler configuration", err.Error())
		return
	}

	data.Enabled = boolPtrToValue(conf.EnableWorkloadAutoscaler)
	data.Installed = boolPtrToValue(conf.WorkloadAutoscalerInstalled)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func boolPtrToValue(b *bool) types.Bool {
	if b == nil {
		return types.BoolValue(false)
	}
	return types.BoolValue(*b)
}
