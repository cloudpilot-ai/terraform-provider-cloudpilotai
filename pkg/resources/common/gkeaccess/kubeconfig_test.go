package gkeaccess

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func TestEnsureKubeconfigAvailableCreatesMissingConfiguredPathExactly(t *testing.T) {
	ctx := context.Background()
	t.Chdir(t.TempDir())
	originalUpdateKubeconfig := RunGcloudUpdateKubeconfig
	defer func() { RunGcloudUpdateKubeconfig = originalUpdateKubeconfig }()

	configuredPath := filepath.Join("nested", "configured-kubeconfig")
	if err := os.MkdirAll(filepath.Dir(configuredPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	var writtenPath string
	RunGcloudUpdateKubeconfig = func(_ context.Context, _, _, _, kubeconfigPath string) error {
		writtenPath = kubeconfigPath
		return os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600)
	}

	info := &AccessInfo{
		ClusterName:     "demo-gke",
		Region:          "us-central1-a",
		ClusterLocation: "us-central1-a",
		ProjectID:       "target-project",
		Kubeconfig:      configuredPath,
	}
	if err := EnsureKubeconfigAvailable(ctx, fakeKubeconfigClient{}, info, nil); err != nil {
		t.Fatalf("EnsureKubeconfigAvailable() error = %v", err)
	}
	wantPath, err := filepath.Abs(configuredPath)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	if writtenPath != wantPath || info.Kubeconfig != wantPath {
		t.Fatalf("written path = %q, runtime path = %q, want %q", writtenPath, info.Kubeconfig, wantPath)
	}
}

func TestEnsureKubeconfigAvailableFromStateReplacesMissingLegacyPath(t *testing.T) {
	ctx := context.Background()
	t.Chdir(t.TempDir())
	originalUpdateKubeconfig := RunGcloudUpdateKubeconfig
	defer func() { RunGcloudUpdateKubeconfig = originalUpdateKubeconfig }()

	var writtenPath string
	RunGcloudUpdateKubeconfig = func(_ context.Context, _, _, _, kubeconfigPath string) error {
		writtenPath = kubeconfigPath
		return os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600)
	}
	legacyPath := "/Users/another-user/project/.terragrunt-cache/old/target-project_us-central1_demo-gke_kubeconfig"
	info := &AccessInfo{
		ClusterName: "demo-gke",
		Region:      "us-central1",
		ProjectID:   "target-project",
		Kubeconfig:  legacyPath,
	}
	if err := EnsureKubeconfigAvailableFromState(ctx, fakeKubeconfigClient{}, info, nil); err != nil {
		t.Fatalf("EnsureKubeconfigAvailableFromState() error = %v", err)
	}
	if info.Kubeconfig == legacyPath || filepath.Base(writtenPath) != "target-project_us-central1_demo-gke_kubeconfig" {
		t.Fatalf("runtime path = %q, written path = %q, want current execution-local path", info.Kubeconfig, writtenPath)
	}
}
