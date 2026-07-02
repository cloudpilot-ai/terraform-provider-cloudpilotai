package gke

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
)

var _ datasource.DataSource = &ClusterDataSource{}

type ClusterDataSource struct {
	client cloudpilotaiclient.Interface
}

func applyClusterSummary(data *ClusterDataSourceModel, summary *api.ClusterCostsSummary) {
	if summary == nil {
		return
	}

	if summary.ClusterName != "" {
		data.ClusterName = types.StringValue(summary.ClusterName)
	}
	if summary.Region != "" {
		data.Region = types.StringValue(summary.Region)
	}
	data.CloudProvider = types.StringValue(summary.CloudProvider)
	data.Status = types.StringValue(string(summary.Status))
	data.AgentVersion = types.StringValue(summary.AgentVersion)
	data.OnboardManifestVersion = types.StringValue(summary.OnboardManifestVersion)
	data.NeedUpgrade = types.BoolValue(summary.NeedUpgrade)
	data.RebalanceEnable = types.BoolValue(summary.RebalanceEnable)
}

func NewClusterDataSource() datasource.DataSource {
	return &ClusterDataSource{}
}

func (d *ClusterDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gke_cluster"
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

	clusterID := stringValue(data.ClusterID)
	if clusterID == "" {
		if err := validateClusterIdentity(&data); err != nil {
			resp.Diagnostics.AddError("missing required gke identity fields", err.Error())
			return
		}
		clusterID = api.GenerateClusterUID(
			api.CloudProviderGCP,
			data.ClusterName.ValueString(),
			data.Region.ValueString(),
			data.ClusterUID.ValueString(),
		)
	}
	data.ClusterID = types.StringValue(clusterID)

	summary, err := d.client.GetCluster(clusterID)
	if err != nil {
		resp.Diagnostics.AddError("failed to read cluster", err.Error())
		return
	}

	applyClusterSummary(&data, summary)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func validateClusterIdentity(data *ClusterDataSourceModel) error {
	if stringValue(data.ClusterName) == "" || stringValue(data.Region) == "" || stringValue(data.ClusterUID) == "" {
		return fmt.Errorf("set cluster_name, region, and cluster_uid when cluster_id is unset")
	}
	return nil
}

func stringValue(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}
