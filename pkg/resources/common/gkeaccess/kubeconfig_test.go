package gkeaccess

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
)

type fakeKubeconfigClient struct {
	listNodeClassesErr error
}

func (f fakeKubeconfigClient) GetCluster(string) (*api.ClusterCostsSummary, error) {
	return &api.ClusterCostsSummary{}, nil
}

func (f fakeKubeconfigClient) ListNodeClasses(string) (api.RebalanceNodeClassList, error) {
	if f.listNodeClassesErr != nil {
		return api.RebalanceNodeClassList{}, f.listNodeClassesErr
	}
	return api.RebalanceNodeClassList{}, nil
}

func TestProjectIDCandidatesFromRemoteNodeClassesParsesFullSelfLinks(t *testing.T) {
	got := ProjectIDCandidatesFromRemoteNodeClasses(api.RebalanceNodeClassList{
		GCENodeClasses: []api.GCENodeClass{{
			Name: "cloudpilot",
			NodeClassSpec: &api.GCENodeClassSpec{
				NetworkConfig: &api.GCENetworkConfig{
					Subnetwork: "https://www.googleapis.com/compute/v1/projects/prod/regions/us-central1/subnetworks/default",
				},
			},
		}},
	})

	if len(got) != 1 || got[0] != "prod" {
		t.Fatalf("ProjectIDCandidatesFromRemoteNodeClasses() = %#v, want [prod]", got)
	}
}

func TestEnsureKubeconfigAvailableFallsBackToCurrentProjectWhenNodeClassesUnavailable(t *testing.T) {
	ctx := context.Background()
	t.Chdir(t.TempDir())

	originalCurrentProject := RunGcloudCurrentProject
	originalUpdateKubeconfig := RunGcloudUpdateKubeconfig
	defer func() {
		RunGcloudCurrentProject = originalCurrentProject
		RunGcloudUpdateKubeconfig = originalUpdateKubeconfig
	}()

	currentProjectCalls := 0
	RunGcloudCurrentProject = func(context.Context) (string, error) {
		currentProjectCalls++
		return "fallback-project", nil
	}

	var gotProjectID string
	RunGcloudUpdateKubeconfig = func(_ context.Context, _, _, projectID, kubeconfigPath string) error {
		gotProjectID = projectID
		return os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600)
	}

	info := &AccessInfo{
		ClusterID:   "cluster-1",
		ClusterName: "demo-gke",
		Region:      "us-central1",
	}
	err := EnsureKubeconfigAvailable(ctx, fakeKubeconfigClient{
		listNodeClassesErr: fmt.Errorf("nodeclasses endpoint unavailable"),
	}, info, nil)
	if err != nil {
		t.Fatalf("EnsureKubeconfigAvailable() error = %v", err)
	}
	if currentProjectCalls != 1 {
		t.Fatalf("current project calls = %d, want 1", currentProjectCalls)
	}
	if gotProjectID != "fallback-project" {
		t.Fatalf("update kubeconfig projectID = %q, want fallback-project", gotProjectID)
	}
	if info.ProjectID != "fallback-project" {
		t.Fatalf("info.ProjectID = %q, want fallback-project", info.ProjectID)
	}
}
