package aws

import (
	"context"
	"fmt"
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
