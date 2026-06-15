package eks

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

func TestSchemaExposesUpgradeStatusFields(t *testing.T) {
	s := Schema(context.Background())

	agentVersionAttr, ok := s.Attributes["agent_version"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("agent_version attribute has unexpected type %T", s.Attributes["agent_version"])
	}
	if !agentVersionAttr.IsComputed() {
		t.Fatalf("agent_version should be computed")
	}

	onboardAttr, ok := s.Attributes["onboard_manifest_version"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("onboard_manifest_version attribute has unexpected type %T", s.Attributes["onboard_manifest_version"])
	}
	if !onboardAttr.IsComputed() {
		t.Fatalf("onboard_manifest_version should be computed")
	}

	needUpgradeAttr, ok := s.Attributes["need_upgrade"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("need_upgrade attribute has unexpected type %T", s.Attributes["need_upgrade"])
	}
	if !needUpgradeAttr.IsComputed() {
		t.Fatalf("need_upgrade should be computed")
	}
}

func TestApplyClusterSummarySetsVersionAndUpgradeFields(t *testing.T) {
	data := &ClusterDataSourceModel{}

	applyClusterSummary(data, &api.ClusterCostsSummary{
		ClusterInfo: api.ClusterInfo{
			CloudProvider:   "aws",
			Status:          api.ClusterStatusOnline,
			RebalanceEnable: true,
		},
		AgentVersion:           "v1.18.6",
		OnboardManifestVersion: "v1.18.7",
		NeedUpgrade:            true,
	})

	if data.CloudProvider != types.StringValue("aws") {
		t.Fatalf("CloudProvider = %#v, want aws", data.CloudProvider)
	}
	if data.Status != types.StringValue(string(api.ClusterStatusOnline)) {
		t.Fatalf("Status = %#v, want online", data.Status)
	}
	if data.AgentVersion != types.StringValue("v1.18.6") {
		t.Fatalf("AgentVersion = %#v, want v1.18.6", data.AgentVersion)
	}
	if data.OnboardManifestVersion != types.StringValue("v1.18.7") {
		t.Fatalf("OnboardManifestVersion = %#v, want v1.18.7", data.OnboardManifestVersion)
	}
	if data.NeedUpgrade != types.BoolValue(true) {
		t.Fatalf("NeedUpgrade = %#v, want true", data.NeedUpgrade)
	}
	if data.RebalanceEnable != types.BoolValue(true) {
		t.Fatalf("RebalanceEnable = %#v, want true", data.RebalanceEnable)
	}
}

func TestApplyClusterSummarySkipsNilSummary(t *testing.T) {
	data := &ClusterDataSourceModel{
		AgentVersion:           types.StringValue("old-agent"),
		OnboardManifestVersion: types.StringValue("old-manifest"),
		NeedUpgrade:            types.BoolValue(false),
		RebalanceEnable:        types.BoolValue(true),
	}

	applyClusterSummary(data, nil)

	if data.AgentVersion != types.StringValue("old-agent") {
		t.Fatalf("AgentVersion = %#v, want old-agent", data.AgentVersion)
	}
	if data.OnboardManifestVersion != types.StringValue("old-manifest") {
		t.Fatalf("OnboardManifestVersion = %#v, want old-manifest", data.OnboardManifestVersion)
	}
	if data.NeedUpgrade != types.BoolValue(false) {
		t.Fatalf("NeedUpgrade = %#v, want false", data.NeedUpgrade)
	}
	if data.RebalanceEnable != types.BoolValue(true) {
		t.Fatalf("RebalanceEnable = %#v, want true", data.RebalanceEnable)
	}
}

func TestReadWritesUpgradeFieldsToState(t *testing.T) {
	ctx := context.Background()
	client := &fakeReadClusterClient{
		summary: &api.ClusterCostsSummary{
			ClusterInfo: api.ClusterInfo{
				CloudProvider:   "aws",
				Status:          api.ClusterStatusOnline,
				RebalanceEnable: true,
			},
			AgentVersion:           "v1.18.6",
			OnboardManifestVersion: "v1.18.7",
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
						"account_id":               tftypes.String,
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
					"cluster_name":             tftypes.NewValue(tftypes.String, "demo"),
					"region":                   tftypes.NewValue(tftypes.String, "us-east-2"),
					"account_id":               tftypes.NewValue(tftypes.String, "123456789012"),
					"cluster_id":               tftypes.NewValue(tftypes.String, nil),
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
	wantClusterID := api.GenerateClusterUID(api.CloudProviderAWS, "demo", "us-east-2", "123456789012")
	if client.gotClusterID != wantClusterID {
		t.Fatalf("GetCluster clusterID = %q, want %q", client.gotClusterID, wantClusterID)
	}

	var got ClusterDataSourceModel
	getDiags := resp.State.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.State.Get() diagnostics = %v", getDiags)
	}
	if got.AgentVersion != types.StringValue("v1.18.6") {
		t.Fatalf("AgentVersion = %#v, want v1.18.6", got.AgentVersion)
	}
	if got.OnboardManifestVersion != types.StringValue("v1.18.7") {
		t.Fatalf("OnboardManifestVersion = %#v, want v1.18.7", got.OnboardManifestVersion)
	}
	if got.NeedUpgrade != types.BoolValue(true) {
		t.Fatalf("NeedUpgrade = %#v, want true", got.NeedUpgrade)
	}
	if got.RebalanceEnable != types.BoolValue(true) {
		t.Fatalf("RebalanceEnable = %#v, want true", got.RebalanceEnable)
	}
}

func TestReadUsesConfiguredClusterIDOverride(t *testing.T) {
	ctx := context.Background()
	client := &fakeReadClusterClient{
		summary: &api.ClusterCostsSummary{
			ClusterInfo: api.ClusterInfo{
				CloudProvider: "aws",
				Status:        api.ClusterStatusOnline,
			},
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
						"account_id":               tftypes.String,
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
					"cluster_name":             tftypes.NewValue(tftypes.String, "demo"),
					"region":                   tftypes.NewValue(tftypes.String, "us-east-2"),
					"account_id":               tftypes.NewValue(tftypes.String, "123456789012"),
					"cluster_id":               tftypes.NewValue(tftypes.String, "custom-cluster-id"),
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
	if client.gotClusterID != "custom-cluster-id" {
		t.Fatalf("GetCluster clusterID = %q, want %q", client.gotClusterID, "custom-cluster-id")
	}

	var got ClusterDataSourceModel
	getDiags := resp.State.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.State.Get() diagnostics = %v", getDiags)
	}
	if got.ClusterID != types.StringValue("custom-cluster-id") {
		t.Fatalf("ClusterID = %#v, want custom-cluster-id", got.ClusterID)
	}
}

func TestReadUsesConfiguredClusterIDOverrideWithoutDetectingAccountID(t *testing.T) {
	ctx := context.Background()
	client := &fakeReadClusterClient{
		summary: &api.ClusterCostsSummary{
			ClusterInfo: api.ClusterInfo{
				CloudProvider: "aws",
				Status:        api.ClusterStatusOnline,
			},
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
						"account_id":               tftypes.String,
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
					"cluster_name":             tftypes.NewValue(tftypes.String, "demo"),
					"region":                   tftypes.NewValue(tftypes.String, "us-east-2"),
					"account_id":               tftypes.NewValue(tftypes.String, nil),
					"cluster_id":               tftypes.NewValue(tftypes.String, "custom-cluster-id"),
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
	if client.gotClusterID != "custom-cluster-id" {
		t.Fatalf("GetCluster clusterID = %q, want %q", client.gotClusterID, "custom-cluster-id")
	}

	var got ClusterDataSourceModel
	getDiags := resp.State.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.State.Get() diagnostics = %v", getDiags)
	}
	if got.ClusterID != types.StringValue("custom-cluster-id") {
		t.Fatalf("ClusterID = %#v, want custom-cluster-id", got.ClusterID)
	}
	if !got.AccountID.IsNull() {
		t.Fatalf("AccountID = %#v, want null", got.AccountID)
	}
}
