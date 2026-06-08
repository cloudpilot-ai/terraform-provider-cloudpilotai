// Package helper provides helper functions for the cloudpilot-ai client.
package helper

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/samber/lo"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type rebalanceConfigurationClient interface {
	GetRebalanceConfiguration(clusterID string) (*api.RebalanceConfig, error)
	UpdateRebalanceConfiguration(clusterID string, config *api.RebalanceConfig) error
}

type workloadConfigurationClient interface {
	GetWorkloadRebalanceConfiguration(clusterID string) (*api.ClusterWorkloadSpec, error)
	UpdateWorkloadRebalanceConfiguration(clusterID string, workload api.Workload) error
}

type clusterUpgradeClient interface {
	GetCluster(clusterID string) (*api.ClusterCostsSummary, error)
	GetClusterUpgradeSH(clusterID string) (string, error)
}

func InstallCloudpilotAIAgentComponent(ctx context.Context, client cloudpilotaiclient.Interface, kubeconfigPath string, disableWorkloadUploading bool, awsEnv map[string]string,
) error {
	agentSH, err := client.GetAgentSH(disableWorkloadUploading)
	if err != nil {
		return err
	}

	return ExecuteSH(ctx, agentSH, buildShellEnv(kubeconfigPath, nil, awsEnv))
}

func InstallCloudpilotAIRebalanceComponent(ctx context.Context, client cloudpilotaiclient.Interface,
	clusterUID, kubeconfigPath, customNodeRole string, awsEnv map[string]string,
) error {
	rebalanceSH, err := client.GetRebalanceSH(clusterUID)
	if err != nil {
		return err
	}

	return ExecuteSH(ctx, rebalanceSH, buildShellEnv(kubeconfigPath, map[string]string{"CUSTOM_NODE_ROLE": customNodeRole}, awsEnv))
}

func UpgradeCloudpilotAIComponentsIfNeeded(ctx context.Context, client clusterUpgradeClient,
	clusterUID, kubeconfigPath, customNodeRole string, awsEnv map[string]string,
) (bool, error) {
	return upgradeCloudpilotAIComponentsIfNeeded(ctx, client, clusterUID, kubeconfigPath, customNodeRole, awsEnv, ExecuteSH)
}

func upgradeCloudpilotAIComponentsIfNeeded(ctx context.Context, client clusterUpgradeClient,
	clusterUID, kubeconfigPath, customNodeRole string, awsEnv map[string]string,
	execute func(context.Context, string, map[string]string) error,
) (bool, error) {
	cluster, err := client.GetCluster(clusterUID)
	if err != nil {
		return false, err
	}
	if cluster == nil || !cluster.NeedUpgrade {
		return false, nil
	}

	upgradeSH, err := client.GetClusterUpgradeSH(clusterUID)
	if err != nil {
		return false, err
	}

	env := buildShellEnv(kubeconfigPath, map[string]string{"CUSTOM_NODE_ROLE": customNodeRole}, awsEnv)
	if err := execute(ctx, upgradeSH, env); err != nil {
		return false, err
	}

	return true, nil
}

const DeleteOptimizedNodesSH = `for node in $(kubectl get node -l node.cloudpilot.ai/managed=true 2>/dev/null | grep -v 'No resources found' | tail -n +2 | awk '{print $1}'); do
  kubectl drain $node --ignore-daemonsets --delete-emptydir-data --force
done
kubectl delete nodeclaim --all
while [[ $(kubectl get nodes -l node.cloudpilot.ai/managed=true -o json | jq -r '.items | length') -ne 0 ]]; do echo "Waiting for CloudPilot AI nodes to be removed..."; sleep 3; done`

func GetCloudpilotAIOptimizedNodeNumber(ctx context.Context, kubeconfigPath string, awsEnv map[string]string) (int64, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", `kubectl get nodes -l node.cloudpilot.ai/managed=true -o json | jq -r '.items | length'`)
	cmd.Env = append(cmd.Env, os.Environ()...)
	for key, value := range buildShellEnv(kubeconfigPath, nil, awsEnv) {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("failed to get optimized node number: err: %w, stdout: %s, stderr: %s", err, stdoutBuf.String(), stderrBuf.String())
	}

	output := strings.TrimSpace(stdoutBuf.String())
	if output == "" {
		return 0, fmt.Errorf("failed to get optimized node number: empty output")
	}

	num, err := strconv.ParseInt(output, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse optimized node number: %w", err)
	}

	return num, nil
}

//go:embed restore.sh
var restoreSH string

func RestoreCloudpilotAIComponent(ctx context.Context, client cloudpilotaiclient.Interface,
	clusterUID, clusterName, region, kubeconfigPath string, awsEnv map[string]string, restoreNodeNumber int64,
) error {
	env := buildShellEnv(kubeconfigPath, map[string]string{
		"CLUSTER_NAME":     clusterName,
		"CLUSTER_REGION":   region,
		"NEW_DESIRED_SIZE": fmt.Sprintf("%d", restoreNodeNumber),
	}, awsEnv)
	if err := ExecuteSH(ctx, restoreSH, env); err != nil {
		return err
	}

	deleteEnv := buildShellEnv(kubeconfigPath, nil, awsEnv)
	if err := ExecuteSH(ctx, DeleteOptimizedNodesSH, deleteEnv); err != nil {
		return err
	}

	return nil
}

func UninstallCloudpilotAIAgentComponent(ctx context.Context, client cloudpilotaiclient.Interface,
	clusterUID, clusterName, provider, region, kubeconfigPath string, awsEnv map[string]string,
) error {
	uninstallSH, err := client.GetClusterUninstallSH(clusterUID, clusterName, provider, region)
	if err != nil {
		return err
	}

	return ExecuteSH(ctx, uninstallSH, buildShellEnv(kubeconfigPath, nil, awsEnv))
}

const deleteCloudpilotNamespaceSH = `kubectl delete namespace cloudpilot --ignore-not-found`

func DeleteCloudpilotNamespace(ctx context.Context, kubeconfigPath string, awsEnv map[string]string) error {
	return ExecuteSH(ctx, deleteCloudpilotNamespaceSH, buildShellEnv(kubeconfigPath, nil, awsEnv))
}

func buildShellEnv(kubeconfigPath string, extra map[string]string, awsEnv map[string]string) map[string]string {
	env := map[string]string{
		"KUBECONFIG": kubeconfigPath,
	}
	for key, value := range extra {
		if value != "" {
			env[key] = value
		}
	}
	for key, value := range awsEnv {
		if value != "" {
			env[key] = value
		}
	}
	return env
}

func ExecuteSH(ctx context.Context, sh string, env map[string]string) error {
	cmd := exec.CommandContext(ctx, "bash", "-c", sh)
	cmd.Env = append(cmd.Env, os.Environ()...)
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Create buffers to capture stdout and stderr
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute SH: err: %w, stdout: %s, stderr: %s", err, stdoutBuf.String(), stderrBuf.String())
	}

	return nil
}

func SyncRebalanceConfiguration(ctx context.Context, client rebalanceConfigurationClient, clusterUID string, enableRebalance bool) error {
	config, err := client.GetRebalanceConfiguration(clusterUID)
	if err != nil {
		return err
	}

	if config.Enable != enableRebalance {
		rebalanceType := config.RebalanceType
		if enableRebalance && !config.Enable {
			rebalanceType = api.RebalanceTypeNode
		}
		if err := client.UpdateRebalanceConfiguration(clusterUID, &api.RebalanceConfig{
			Enable:                      enableRebalance,
			UploadConfig:                config.UploadConfig,
			EnableDiversityInstanceType: config.EnableDiversityInstanceType,
			RebalanceType:               rebalanceType,
		}); err != nil {
			return err
		}
	}

	return nil
}

func SyncWorkloadConfiguration(ctx context.Context, client workloadConfigurationClient, clusterUID string,
	workloadNestedObjectList customfield.NestedObjectList[api.WorkloadModel],
	workloadTemplateNestedObjectList customfield.NestedObjectList[api.WorkloadTemplateModel],
) error {
	if workloadNestedObjectList.IsNullOrUnknown() {
		return nil
	}

	templateM := make(map[string]api.WorkloadTemplateModel)
	if !workloadTemplateNestedObjectList.IsNullOrUnknown() {
		workloadTemplates, diagnostics := workloadTemplateNestedObjectList.AsStructSliceT(ctx)
		if diagnostics.HasError() {
			return fmt.Errorf("failed to parse workload template configuration: %v", diagnostics)
		}

		for wi := range workloadTemplates {
			templateM[workloadTemplates[wi].TemplateName.ValueString()] = workloadTemplates[wi]
		}
	}

	workloads, diagnostics := workloadNestedObjectList.AsStructSliceT(ctx)
	if diagnostics.HasError() {
		return fmt.Errorf("failed to parse workload configuration: %v", diagnostics)
	}

	workloadRebalanceConfiguration, err := client.GetWorkloadRebalanceConfiguration(clusterUID)
	if err != nil {
		return err
	}

	workloadM := lo.SliceToMap(workloadRebalanceConfiguration.Workloads, func(item api.Workload) (string, api.Workload) {
		return item.Namespace + item.Type + item.Name, item
	})

	for wi := range workloads {
		existing, ok := workloadM[workloads[wi].Namespace.ValueString()+workloads[wi].Type.ValueString()+workloads[wi].Name.ValueString()]
		if !ok {
			continue
		}

		var workloadTemplate *api.WorkloadTemplateModel
		if !workloads[wi].TemplateName.IsNull() && !workloads[wi].TemplateName.IsUnknown() {
			workloadsTemplateName := workloads[wi].TemplateName.ValueString()
			if workloadsTemplateName != "" {
				if t, ok := templateM[workloadsTemplateName]; ok {
					workloadTemplate = &t
				}
			}
		}

		if err := client.UpdateWorkloadRebalanceConfiguration(clusterUID, *workloads[wi].ToWorkload(&existing, workloadTemplate, existing.Replicas)); err != nil {
			return err
		}
	}

	return nil
}

func SyncEC2NodeClassConfiguration(ctx context.Context, client cloudpilotaiclient.Interface, clusterUID, clusterName string,
	nodeClassesNestedObjectList customfield.NestedObjectList[api.EC2NodeClassModel],
	nodeClassesTemplateNestedObjectList customfield.NestedObjectList[api.EC2NodeClassTemplateModel],
	previousStateNames map[string]struct{},
) error {
	if nodeClassesNestedObjectList.IsNullOrUnknown() {
		return nil
	}

	templateM := make(map[string]api.EC2NodeClassTemplateModel)
	if !nodeClassesTemplateNestedObjectList.IsNullOrUnknown() {
		nodeClassTemplates, diagnostics := nodeClassesTemplateNestedObjectList.AsStructSliceT(ctx)
		if diagnostics.HasError() {
			return fmt.Errorf("failed to parse nodeclass template configuration: %v", diagnostics)
		}

		for ni := range nodeClassTemplates {
			templateM[nodeClassTemplates[ni].TemplateName.ValueString()] = nodeClassTemplates[ni]
		}
	}

	nodeClasses, diagnostics := nodeClassesNestedObjectList.AsStructSliceT(ctx)
	if diagnostics.HasError() {
		return fmt.Errorf("failed to parse nodeclass configuration: %v", diagnostics)
	}

	existingNodeClasses, err := client.ListNodeClasses(clusterUID)
	if err != nil {
		return err
	}

	nodeClassM := lo.SliceToMap(existingNodeClasses.EC2NodeClasses, func(item api.EC2NodeClass) (string, api.EC2NodeClass) {
		return item.Name, item
	})

	nodeClassNames := make(map[string]struct{}, len(nodeClasses))

	for nci := range nodeClasses {
		nodeClassNames[nodeClasses[nci].Name.ValueString()] = struct{}{}

		ec2NodeClass := api.EC2NodeClass{Name: nodeClasses[nci].Name.ValueString()}
		if v, ok := nodeClassM[nodeClasses[nci].Name.ValueString()]; ok {
			ec2NodeClass = v
		}

		var nodeClassTemplate *api.EC2NodeClassTemplateModel
		if !nodeClasses[nci].TemplateName.IsNull() && !nodeClasses[nci].TemplateName.IsUnknown() {
			nodeClassesTemplateName := nodeClasses[nci].TemplateName.ValueString()
			if nodeClassesTemplateName != "" {
				if t, ok := templateM[nodeClassesTemplateName]; ok {
					nodeClassTemplate = &t
				}
			}
		}

		nodeClass, err := nodeClasses[nci].ToEC2NodeClass(ctx, clusterName, ec2NodeClass, nodeClassTemplate)
		if err != nil {
			return err
		}

		if err := client.ApplyNodeClass(clusterUID, api.RebalanceNodeClass{
			EC2NodeClass: nodeClass,
		}); err != nil {
			return err
		}
	}

	// Only delete nodeclasses that were previously tracked in Terraform state
	// but are no longer in the desired configuration. Server-side nodeclasses
	// not managed by Terraform are left untouched.
	for name := range previousStateNames {
		if _, stillDesired := nodeClassNames[name]; !stillDesired {
			if err := client.DeleteNodeClass(clusterUID, name); err != nil {
				return err
			}
		}
	}

	return nil
}

func SyncEC2NodePoolConfiguration(ctx context.Context, client cloudpilotaiclient.Interface, clusterUID string,
	nodePoolsNestedObjectList customfield.NestedObjectList[api.EC2NodePoolModel],
	nodePoolsTemplateNestedObjectList customfield.NestedObjectList[api.EC2NodePoolTemplateModel],
	previousStateNames map[string]struct{},
) error {
	if nodePoolsNestedObjectList.IsNullOrUnknown() {
		return nil
	}

	templateM := make(map[string]api.EC2NodePoolTemplateModel)
	if !nodePoolsTemplateNestedObjectList.IsNullOrUnknown() {
		nodePoolTemplates, diagnostics := nodePoolsTemplateNestedObjectList.AsStructSliceT(ctx)
		if diagnostics.HasError() {
			return fmt.Errorf("failed to parse nodepool template configuration: %v", diagnostics)
		}

		for ni := range nodePoolTemplates {
			templateM[nodePoolTemplates[ni].TemplateName.ValueString()] = nodePoolTemplates[ni]
		}
	}

	nodePools, diagnostics := nodePoolsNestedObjectList.AsStructSliceT(ctx)
	if diagnostics.HasError() {
		return fmt.Errorf("failed to parse nodepool configuration: %v", diagnostics)
	}

	existingNodePools, err := client.ListNodePools(clusterUID)
	if err != nil {
		return err
	}

	nodePoolM := lo.SliceToMap(existingNodePools.EC2NodePools, func(item api.EC2NodePool) (string, api.EC2NodePool) {
		return item.Name, item
	})

	nodePoolNames := make(map[string]struct{}, len(nodePools))

	for npi := range nodePools {
		nodePoolNames[nodePools[npi].Name.ValueString()] = struct{}{}

		ec2NodePool := api.EC2NodePool{Name: nodePools[npi].Name.ValueString()}
		if v, ok := nodePoolM[nodePools[npi].Name.ValueString()]; ok {
			ec2NodePool = v
		}

		var nodePoolTemplate *api.EC2NodePoolTemplateModel
		if !nodePools[npi].TemplateName.IsNull() && !nodePools[npi].TemplateName.IsUnknown() {
			nodePoolsTemplateName := nodePools[npi].TemplateName.ValueString()
			if nodePoolsTemplateName != "" {
				if t, ok := templateM[nodePoolsTemplateName]; ok {
					nodePoolTemplate = &t
				}
			}
		}

		nodePool, err := nodePools[npi].ToEC2NodePool(ctx, ec2NodePool, nodePoolTemplate)
		if err != nil {
			return err
		}

		if nodePool.NodePoolSpec == nil ||
			nodePool.NodePoolSpec.Template.Spec.NodeClassRef == nil ||
			nodePool.NodePoolSpec.Template.Spec.NodeClassRef.Name == "" {
			return fmt.Errorf("nodepool %s must reference a valid nodeclass", nodePool.Name)
		}

		if err := client.ApplyNodePool(clusterUID, api.RebalanceNodePool{
			EC2NodePool: nodePool,
		}); err != nil {
			return err
		}
	}

	// Only delete nodepools that were previously tracked in Terraform state
	// but are no longer in the desired configuration. Server-side nodepools
	// not managed by Terraform are left untouched.
	for name := range previousStateNames {
		if _, stillDesired := nodePoolNames[name]; !stillDesired {
			if err := client.DeleteNodePool(clusterUID, name); err != nil {
				return err
			}
		}
	}

	return nil
}
