package utils

import (
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
)

func TestGenerateClusterUIDMatchesAPIForAWS(t *testing.T) {
	req := api.RegisterClusterRequest{
		CloudProvider: CloudProviderAWS,
		EKS: &api.EKSParams{
			ClusterName: "test-saving-20260601-144407",
			Region:      "us-east-2",
			AccountID:   "306107317780",
		},
	}

	got := GenerateClusterUID(req)
	want := api.GenerateClusterUID(api.CloudProviderAWS, "test-saving-20260601-144407", "us-east-2", "306107317780")
	if got != want {
		t.Fatalf("got cluster ID %q, want %q", got, want)
	}
}

func TestGenerateClusterUIDMatchesAPIForGCP(t *testing.T) {
	req := api.RegisterClusterRequest{
		CloudProvider: CloudProviderGCP,
		ClusterParams: &api.ClusterParams{
			ClusterName: "test-gke",
			Region:      "us-central1",
			AccountID:   "gke-cluster-uid-123",
		},
	}

	got := GenerateClusterUID(req)
	want := api.GenerateClusterUID(api.CloudProviderGCP, "test-gke", "us-central1", "gke-cluster-uid-123")
	if got != want {
		t.Fatalf("got cluster ID %q, want %q", got, want)
	}
}
