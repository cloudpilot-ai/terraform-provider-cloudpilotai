package gke

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
)

type fakeReadClusterClient struct {
	cloudpilotaiclient.Interface
	summary         *api.ClusterCostsSummary
	getClusterCalls int
	gotClusterID    string
}

func (f *fakeReadClusterClient) GetCluster(clusterID string) (*api.ClusterCostsSummary, error) {
	f.getClusterCalls++
	f.gotClusterID = clusterID
	return f.summary, nil
}

func TestSchemaAllowsClusterIDOnlyLookup(t *testing.T) {
	s := Schema(context.Background())

	for _, name := range []string{"cluster_name", "region", "cluster_uid"} {
		attr, ok := s.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s attribute has unexpected type %T", name, s.Attributes[name])
		}
		if !attr.IsOptional() || !attr.IsComputed() {
			t.Fatalf("%s should be optional+computed for cluster_id-only lookup", name)
		}
	}
}

func TestReadUsesConfiguredClusterIDWithoutIdentityFields(t *testing.T) {
	ctx := context.Background()
	client := &fakeReadClusterClient{
		summary: &api.ClusterCostsSummary{
			ClusterInfo: api.ClusterInfo{
				ClusterName:     "demo-gke",
				Region:          "us-central1",
				CloudProvider:   api.CloudProviderGCP,
				Status:          api.ClusterStatusOnline,
				RebalanceEnable: true,
			},
			AgentVersion:           "v1.20.0",
			OnboardManifestVersion: "v1.20.1",
			NeedUpgrade:            true,
		},
	}

	req := datasource.ReadRequest{
		Config: tfsdk.Config{
			Schema: Schema(ctx),
			Raw: tftypes.NewValue(
				tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"cluster_name":             tftypes.String,
						"region":                   tftypes.String,
						"cluster_uid":              tftypes.String,
						"cluster_location":         tftypes.String,
						"cluster_id":               tftypes.String,
						"cloud_provider":           tftypes.String,
						"status":                   tftypes.String,
						"agent_version":            tftypes.String,
						"onboard_manifest_version": tftypes.String,
						"need_upgrade":             tftypes.Bool,
						"rebalance_enable":         tftypes.Bool,
					},
				},
				map[string]tftypes.Value{
					"cluster_name":             tftypes.NewValue(tftypes.String, nil),
					"region":                   tftypes.NewValue(tftypes.String, nil),
					"cluster_uid":              tftypes.NewValue(tftypes.String, nil),
					"cluster_location":         tftypes.NewValue(tftypes.String, nil),
					"cluster_id":               tftypes.NewValue(tftypes.String, "cluster-1"),
					"cloud_provider":           tftypes.NewValue(tftypes.String, nil),
					"status":                   tftypes.NewValue(tftypes.String, nil),
					"agent_version":            tftypes.NewValue(tftypes.String, nil),
					"onboard_manifest_version": tftypes.NewValue(tftypes.String, nil),
					"need_upgrade":             tftypes.NewValue(tftypes.Bool, nil),
					"rebalance_enable":         tftypes.NewValue(tftypes.Bool, nil),
				},
			),
		},
	}
	resp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: Schema(ctx)},
	}

	(&ClusterDataSource{client: client}).Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics = %v", resp.Diagnostics)
	}
	if client.getClusterCalls != 1 {
		t.Fatalf("GetCluster calls = %d, want 1", client.getClusterCalls)
	}
	if client.gotClusterID != "cluster-1" {
		t.Fatalf("GetCluster clusterID = %q, want cluster-1", client.gotClusterID)
	}

	var got ClusterDataSourceModel
	getDiags := resp.State.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.State.Get() diagnostics = %v", getDiags)
	}
	if got.ClusterName != types.StringValue("demo-gke") {
		t.Fatalf("ClusterName = %#v, want demo-gke", got.ClusterName)
	}
	if got.Region != types.StringValue("us-central1") {
		t.Fatalf("Region = %#v, want us-central1", got.Region)
	}
	if got.CloudProvider != types.StringValue(api.CloudProviderGCP) {
		t.Fatalf("CloudProvider = %#v, want gcp", got.CloudProvider)
	}
}
