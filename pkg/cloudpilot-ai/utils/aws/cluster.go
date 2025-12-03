package aws

import (
	"fmt"
	"os/exec"
)

func UpdateKubeconfig(clusterName, region, kubeconfigPath string) error {
	cmd := exec.Command("aws", "eks", "update-kubeconfig", "--name", clusterName, "--region", region, "--kubeconfig", kubeconfigPath)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w, output: %s", err, string(output))
	}

	return nil
}
