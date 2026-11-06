package workloadautoscaler

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/gkeaccess"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type importAccessClient struct {
	*fakeWorkloadAutoscalerClient
	summaryErr     error
	nodeClassesErr error
	summaryCalls   int
	nodeClassCalls int
}

func (f *importAccessClient) GetCluster(clusterID string) (*api.ClusterCostsSummary, error) {
	f.summaryCalls++
	if f.summaryErr != nil {
		return nil, f.summaryErr
	}
	return f.fakeWorkloadAutoscalerClient.GetCluster(clusterID)
}

func (f *importAccessClient) ListNodeClasses(clusterID string) (api.RebalanceNodeClassList, error) {
	f.nodeClassCalls++
	return f.nodeClasses, f.nodeClassesErr
}

func (f *importAccessClient) GetWAConfiguration(string) (*api.WAConfiguration, error) {
	return &api.WAConfiguration{}, nil
}

func (f *importAccessClient) ListRecommendationPolicies(string) ([]api.RecommendationPolicyResource, error) {
	return nil, nil
}

func (f *importAccessClient) ListAutoscalingPolicies(string) ([]api.AutoscalingPolicyResource, error) {
	return nil, nil
}

func TestHydrateImportedGCPProject(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name              string
		isImport          bool
		cloudProvider     string
		configuredProject types.String
		kubeconfig        types.String
		location          types.String
		remoteProject     string
		currentProject    string
		summaryErr        error
		nodeClassesErr    error
		currentProjectErr error
		wantProject       types.String
		wantSummaryCalls  int
		wantNodeCalls     int
		wantCurrentCalls  int
		wantError         bool
	}{
		{name: "regional GKE import discovers project", isImport: true, cloudProvider: api.CloudProviderGCP, remoteProject: "remote-project", currentProject: "unrelated-project", wantProject: types.StringValue("remote-project"), wantSummaryCalls: 1, wantNodeCalls: 1},
		{name: "GKE import falls back to current project", isImport: true, cloudProvider: api.CloudProviderGCP, currentProject: "current-project", wantProject: types.StringValue("current-project"), wantSummaryCalls: 1, wantNodeCalls: 1, wantCurrentCalls: 1},
		{name: "nodeclass lookup failure still allows fallback", isImport: true, cloudProvider: api.CloudProviderGCP, nodeClassesErr: errors.New("unavailable"), currentProject: "current-project", wantProject: types.StringValue("current-project"), wantSummaryCalls: 1, wantNodeCalls: 1, wantCurrentCalls: 1},
		{name: "configured project is preserved", isImport: true, cloudProvider: api.CloudProviderGCP, configuredProject: types.StringValue("explicit-project"), remoteProject: "remote-project", wantProject: types.StringValue("explicit-project")},
		{name: "configured kubeconfig and location are preserved", isImport: true, cloudProvider: api.CloudProviderGCP, kubeconfig: types.StringValue("/configured/kubeconfig"), location: types.StringValue("us-central1-a"), remoteProject: "remote-project", wantProject: types.StringValue("remote-project"), wantSummaryCalls: 1, wantNodeCalls: 1},
		{name: "explicit empty project is preserved", isImport: true, cloudProvider: api.CloudProviderGCP, configuredProject: types.StringValue(""), remoteProject: "remote-project", wantProject: types.StringValue("")},
		{name: "unknown project is preserved", isImport: true, cloudProvider: api.CloudProviderGCP, configuredProject: types.StringUnknown(), wantProject: types.StringUnknown()},
		{name: "ordinary GKE refresh leaves null unmanaged", cloudProvider: api.CloudProviderGCP, remoteProject: "remote-project", wantProject: types.StringNull()},
		{name: "AWS import does not discover GCP access", isImport: true, cloudProvider: api.CloudProviderAWS, currentProject: "unrelated-project", wantProject: types.StringNull(), wantSummaryCalls: 1},
		{name: "unset current project stays null", isImport: true, cloudProvider: api.CloudProviderGCP, wantProject: types.StringNull(), wantSummaryCalls: 1, wantNodeCalls: 1, wantCurrentCalls: 1},
		{name: "summary error leaves state unchanged", isImport: true, summaryErr: errors.New("unavailable"), wantProject: types.StringNull(), wantSummaryCalls: 1, wantError: true},
		{name: "current project failure leaves state unchanged", isImport: true, cloudProvider: api.CloudProviderGCP, currentProjectErr: errors.New("gcloud unavailable"), wantProject: types.StringNull(), wantSummaryCalls: 1, wantNodeCalls: 1, wantCurrentCalls: 1, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &importAccessClient{
				fakeWorkloadAutoscalerClient: &fakeWorkloadAutoscalerClient{
					summary: &api.ClusterCostsSummary{ClusterInfo: api.ClusterInfo{CloudProvider: test.cloudProvider}},
				},
				summaryErr: test.summaryErr, nodeClassesErr: test.nodeClassesErr,
			}
			if test.remoteProject != "" {
				client.nodeClasses.GCENodeClasses = []api.GCENodeClass{{NodeClassSpec: &api.GCENodeClassSpec{
					NetworkConfig: &api.GCENetworkConfig{Subnetwork: "projects/" + test.remoteProject + "/regions/us-central1/subnetworks/default"},
				}}}
			}
			originalCurrent := gkeaccess.RunGcloudCurrentProject
			originalUpdate := gkeaccess.RunGcloudUpdateKubeconfig
			t.Cleanup(func() {
				gkeaccess.RunGcloudCurrentProject = originalCurrent
				gkeaccess.RunGcloudUpdateKubeconfig = originalUpdate
			})
			currentCalls := 0
			gkeaccess.RunGcloudCurrentProject = func(context.Context) (string, error) {
				currentCalls++
				return test.currentProject, test.currentProjectErr
			}
			gkeaccess.RunGcloudUpdateKubeconfig = func(context.Context, string, string, string, string) error {
				t.Fatal("imported access metadata discovery must not generate kubeconfig")
				return nil
			}
			data := WorkloadAutoscalerModel{
				ClusterID:    types.StringValue("cluster-1"),
				GCPProjectID: test.configuredProject,
				Kubeconfig:   test.kubeconfig,
				GCPLocation:  test.location,
			}
			want := data
			want.GCPProjectID = test.wantProject
			err := (&WorkloadAutoscaler{client: client}).hydrateImportedGCPProject(ctx, &data, test.isImport)
			if (err != nil) != test.wantError {
				t.Fatalf("hydrateImportedGCPProject() error = %v, want error %t", err, test.wantError)
			}
			if !reflect.DeepEqual(data, want) {
				t.Fatalf("import hydration changed unexpected state: got %#v, want %#v", data, want)
			}
			if client.summaryCalls != test.wantSummaryCalls || client.nodeClassCalls != test.wantNodeCalls || currentCalls != test.wantCurrentCalls {
				t.Fatalf("discovery calls = summary %d, nodeclasses %d, current project %d; want %d/%d/%d", client.summaryCalls, client.nodeClassCalls, currentCalls, test.wantSummaryCalls, test.wantNodeCalls, test.wantCurrentCalls)
			}
		})
	}
}

func TestReadDoesNotHydrateUnmanagedGCPAccess(t *testing.T) {
	ctx := context.Background()
	client := &importAccessClient{fakeWorkloadAutoscalerClient: &fakeWorkloadAutoscalerClient{
		summary: &api.ClusterCostsSummary{ClusterInfo: api.ClusterInfo{CloudProvider: api.CloudProviderGCP}},
	}}
	state := tfsdk.State{Schema: Schema(ctx)}
	if diags := state.Set(ctx, &WorkloadAutoscalerModel{ClusterID: types.StringValue("cluster-1")}); diags.HasError() {
		t.Fatalf("state setup diagnostics = %v", diags)
	}
	response := resource.ReadResponse{State: state}
	(&WorkloadAutoscaler{client: client}).Read(ctx, resource.ReadRequest{State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics = %v", response.Diagnostics)
	}
	if !state.Raw.Equal(response.State.Raw) {
		t.Fatalf("ordinary Read changed unmanaged state: got %s, want %s", response.State.Raw, state.Raw)
	}
	if client.summaryCalls != 0 || client.nodeClassCalls != 0 {
		t.Fatalf("ordinary Read performed import-only access discovery: summary %d, nodeclasses %d", client.summaryCalls, client.nodeClassCalls)
	}
}
