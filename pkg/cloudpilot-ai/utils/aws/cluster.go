package aws

import (
	"fmt"
	"os/exec"
)

func UpdateKubeconfig(clusterName, region, kubeconfigPath, profile string) error {
	args := []string{"eks", "update-kubeconfig", "--name", clusterName, "--region", region, "--kubeconfig", kubeconfigPath}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	cmd := exec.Command("aws", args...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w, output: %s", err, string(output))
	}

	return nil
}
