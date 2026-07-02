package api

import "testing"

func TestGenerateClusterUIDMatchesServerAlgorithm(t *testing.T) {
	got := GenerateClusterUID("aws", "test-saving-20260601-144407", "us-east-2", "306107317780")
	want := "56b7849a-4ce1-5f91-8f91-731774644885"
	if got != want {
		t.Fatalf("got cluster ID %q, want %q", got, want)
	}
}

func TestGenerateClusterUIDSupportsGCP(t *testing.T) {
	got := GenerateClusterUID("gcp", "test-gke", "us-central1", "gke-cluster-uid-123")
	want := "0afb8050-e986-517e-8310-e83e668a659e"
	if got != want {
		t.Fatalf("got cluster ID %q, want %q", got, want)
	}
}
