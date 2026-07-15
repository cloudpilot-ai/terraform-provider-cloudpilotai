package aws

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func UpdateKubeconfig(ctx context.Context, clusterName, region, kubeconfigPath string, auth ExecutionAuthConfig) error {
	if _, err := runAWSCommand(ctx, nil, BuildUpdateKubeconfigArgs(clusterName, region, kubeconfigPath, auth)...); err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w", err)
	}

	if auth.Profile != "" {
		if err := PatchKubeconfigExecEnv(kubeconfigPath, map[string]string{"AWS_PROFILE": auth.Profile}); err != nil {
			return fmt.Errorf("failed to patch kubeconfig exec env: %w", err)
		}
	}

	return nil
}

// EnsureKubeconfigAvailable returns an absolute kubeconfig path that is usable
// for the current provider operation. A configured path is honored exactly;
// when it does not exist yet, aws eks update-kubeconfig creates it there. When
// no path is configured, the provider creates its usual per-cluster file in the
// current working directory. Callers must treat an unconfigured path as
// execution-local data and must not persist the generated path in Terraform
// state.
func EnsureKubeconfigAvailable(ctx context.Context, clusterName, region, configuredPath string, auth ExecutionAuthConfig) (string, error) {
	kubeconfigPath := strings.TrimSpace(configuredPath)
	isConfigured := kubeconfigPath != ""
	if !isConfigured {
		kubeconfigPath = strings.Join([]string{region, clusterName, "kubeconfig"}, "_")
	}

	absolutePath, err := filepath.Abs(kubeconfigPath)
	if err != nil {
		return "", err
	}

	if isConfigured {
		_, err = os.Stat(absolutePath)
		if err == nil {
			return absolutePath, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
	}

	if err := UpdateKubeconfig(ctx, clusterName, region, absolutePath, auth); err != nil {
		return "", err
	}
	return absolutePath, nil
}

// EnsureKubeconfigAvailableFromState accepts a path read from prior Terraform
// state. If that path no longer exists (for example, it points into another
// runner's Terragrunt cache), it is treated as legacy generated state and a
// fresh execution-local kubeconfig is created instead. This keeps destroy and
// refresh operations working during migration from providers that persisted
// generated kubeconfig paths.
func EnsureKubeconfigAvailableFromState(ctx context.Context, clusterName, region, statePath string, auth ExecutionAuthConfig) (string, error) {
	statePath = strings.TrimSpace(statePath)
	if statePath != "" {
		absolutePath, err := filepath.Abs(statePath)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(absolutePath); err == nil {
			return absolutePath, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}

	return EnsureKubeconfigAvailable(ctx, clusterName, region, "", auth)
}
