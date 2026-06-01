package api

import "testing"

func TestGenerateClusterUIDMatchesServerAlgorithm(t *testing.T) {
	got := GenerateClusterUID("aws", "test-saving-20260601-144407", "us-east-2", "306107317780")
	want := "56b7849a-4ce1-5f91-8f91-731774644885"
	if got != want {
		t.Fatalf("got cluster ID %q, want %q", got, want)
	}
}
