package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func stubUpdateKubeconfigCommand(t *testing.T) *string {
	t.Helper()
	original := runAWSCommand
	t.Cleanup(func() { runAWSCommand = original })

	var writtenPath string
	runAWSCommand = func(_ context.Context, _ map[string]string, args ...string) ([]byte, error) {
		for i := range args {
			if args[i] == "--kubeconfig" && i+1 < len(args) {
				writtenPath = args[i+1]
				if err := os.WriteFile(writtenPath, []byte("apiVersion: v1\n"), 0o600); err != nil {
					return nil, err
				}
				break
			}
		}
		return nil, nil
	}
	return &writtenPath
}

func TestEnsureKubeconfigAvailablePreservesConfiguredPathOnlyInRuntimeResult(t *testing.T) {
	t.Chdir(t.TempDir())
	writtenPath := stubUpdateKubeconfigCommand(t)

	got, err := EnsureKubeconfigAvailable(context.Background(), "demo", "us-east-2", "./configured-kubeconfig", ExecutionAuthConfig{})
	if err != nil {
		t.Fatalf("EnsureKubeconfigAvailable() error = %v", err)
	}
	if !filepath.IsAbs(got) || filepath.Base(got) != "configured-kubeconfig" {
		t.Fatalf("runtime path = %q, want absolute configured path", got)
	}
	if *writtenPath != got {
		t.Fatalf("aws wrote %q, want %q", *writtenPath, got)
	}
}

func TestEnsureKubeconfigAvailableFromStateReplacesMissingLegacyPath(t *testing.T) {
	t.Chdir(t.TempDir())
	writtenPath := stubUpdateKubeconfigCommand(t)
	legacyPath := "/Users/another-user/project/.terragrunt-cache/old/us-east-2_demo_kubeconfig"

	got, err := EnsureKubeconfigAvailableFromState(context.Background(), "demo", "us-east-2", legacyPath, ExecutionAuthConfig{})
	if err != nil {
		t.Fatalf("EnsureKubeconfigAvailableFromState() error = %v", err)
	}
	if got == legacyPath {
		t.Fatalf("runtime path reused stale legacy state path %q", got)
	}
	if filepath.Base(got) != "us-east-2_demo_kubeconfig" {
		t.Fatalf("runtime path = %q, want current execution-local kubeconfig", got)
	}
	if *writtenPath != got {
		t.Fatalf("aws wrote %q, want %q", *writtenPath, got)
	}
}

func TestEnsureKubeconfigAvailableFromStateReusesExistingPath(t *testing.T) {
	existingPath := filepath.Join(t.TempDir(), "existing-kubeconfig")
	if err := os.WriteFile(existingPath, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	writtenPath := stubUpdateKubeconfigCommand(t)

	got, err := EnsureKubeconfigAvailableFromState(context.Background(), "demo", "us-east-2", existingPath, ExecutionAuthConfig{})
	if err != nil {
		t.Fatalf("EnsureKubeconfigAvailableFromState() error = %v", err)
	}
	if got != existingPath {
		t.Fatalf("runtime path = %q, want existing state path %q", got, existingPath)
	}
	if *writtenPath != "" {
		t.Fatalf("aws unexpectedly regenerated kubeconfig at %q", *writtenPath)
	}
}

func TestEnsureKubeconfigAvailableRefreshesExistingImplicitPath(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("us-east-2_demo_kubeconfig", []byte("stale\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	writtenPath := stubUpdateKubeconfigCommand(t)

	got, err := EnsureKubeconfigAvailable(context.Background(), "demo", "us-east-2", "", ExecutionAuthConfig{AssumeRoleARN: "arn:aws:iam::123456789012:role/new-role"})
	if err != nil {
		t.Fatalf("EnsureKubeconfigAvailable() error = %v", err)
	}
	if *writtenPath != got {
		t.Fatalf("aws wrote %q, want refreshed implicit path %q", *writtenPath, got)
	}
}
