package eks

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/utils/aws"
)

var _ datasource.DataSource = &ClusterDataSource{}

type ClusterDataSource struct {
	client cloudpilotaiclient.Interface
}

func NewClusterDataSource() datasource.DataSource {
	return &ClusterDataSource{}
}

func (d *ClusterDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_eks_cluster"
}

func (d *ClusterDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = Schema(ctx)
}

func (d *ClusterDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ClusterDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ClusterDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	accountID := data.AccountID.ValueString()
	if accountID == "" {
		detectedID, err := aws.GetAccountID()
		if err != nil {
			resp.Diagnostics.AddError("failed to detect AWS account ID", err.Error())
			return
		}
		accountID = detectedID
	}
	data.AccountID = types.StringValue(accountID)

	clusterUID := api.GenerateClusterUID(
		api.CloudProviderAWS,
		data.ClusterName.ValueString(),
		data.Region.ValueString(),
		accountID,
	)
	data.ClusterID = types.StringValue(clusterUID)

	summary, err := d.client.GetCluster(clusterUID)
	if err != nil {
		resp.Diagnostics.AddError("failed to read cluster", err.Error())
		return
	}

	data.CloudProvider = types.StringValue(summary.CloudProvider)
	data.Status = types.StringValue(string(summary.Status))
	data.AgentVersion = types.StringValue(summary.AgentVersion)
	data.RebalanceEnable = types.BoolValue(summary.RebalanceEnable)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
