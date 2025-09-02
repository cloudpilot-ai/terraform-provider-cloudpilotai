// Package provider is the entry point for the provider.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/consts"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/nodeautoscale/eks"
)

type CloudpilotaiProvider struct {
	version string
}

func NewProvider(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CloudpilotaiProvider{
			version: version,
		}
	}
}

type CloudpilotaiProviderModel struct {
	APIKey        types.String `tfsdk:"api_key"`
	APIKeyProfile types.String `tfsdk:"api_key_profile"`
	APIEndpoint   types.String `tfsdk:"api_endpoint"`
}

func (p *CloudpilotaiProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data CloudpilotaiProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if data.APIEndpoint.IsNull() || data.APIEndpoint.IsUnknown() {
		data.APIEndpoint = types.StringValue(consts.DefaultAPIEndpoint)
	}

	var apikey string
	if !data.APIKey.IsNull() && !data.APIKey.IsUnknown() {
		apikey = data.APIKey.ValueString()
	} else if !data.APIKeyProfile.IsNull() && !data.APIKeyProfile.IsUnknown() {
		key, err := os.ReadFile(data.APIKeyProfile.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to read API key profile",
				"Could not read API key profile file: "+err.Error(),
			)
			return
		}
		apikey = string(key)
	}

	if apikey == "" {
		resp.Diagnostics.AddError(
			"API key not configured",
			"An API key must be provided either through the provider configuration (api_key or api_key_profile).",
		)
		return
	}

	client := client.NewCloudPilotClient(data.APIEndpoint.ValueString(), apikey)

	resp.ResourceData = client
}

func (p *CloudpilotaiProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func (p *CloudpilotaiProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = consts.ProviderName
	resp.Version = p.version
}

func (p *CloudpilotaiProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		eks.NewCluster,
	}
}

func (p *CloudpilotaiProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = ProviderSchema(ctx)
}

func ProviderSchema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Description: "API key for the Cloudpilotai API.",
				Optional:    true,
				Sensitive:   true,
			},

			"api_key_profile": schema.StringAttribute{
				Description: "API key profile for the Cloudpilotai API.",
				Optional:    true,
			},

			"api_endpoint": schema.StringAttribute{
				Description: "API Endpoint for the Cloudpilotai API.",
				Optional:    true,
			},
		},
	}
}
