package gkeaccess

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

var (
	projectIDFromResourcePathRe   = regexp.MustCompile(`(?:^|/)projects/([^/]+)/`)
	projectIDFromServiceAccountRe = regexp.MustCompile(`^[^@]+@([^.]+)\.iam\.gserviceaccount\.com$`)
	kubeconfigPathUnsafePartRe    = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)
	RunGcloudUpdateKubeconfig     = updateKubeconfig
	RunGcloudCurrentProject       = currentProject
	RunKubectlGetClusterUID       = clusterUIDFromKubeconfig
)

type Client interface {
	GetCluster(clusterID string) (*api.ClusterCostsSummary, error)
	ListNodeClasses(clusterID string) (api.RebalanceNodeClassList, error)
}

type AccessInfo struct {
	ClusterID       string
	ClusterName     string
	Region          string
	ClusterLocation string
	ProjectID       string
	Kubeconfig      string
}

func EnsureKubeconfigAvailable(ctx context.Context, client Client, info *AccessInfo, projectHints []string) error {
	kubeconfigPath, err := normalizeKubeconfigPath(info.Kubeconfig)
	if err != nil {
		return err
	}
	if kubeconfigPath != "" {
		info.Kubeconfig = kubeconfigPath
		return nil
	}

	if err := fillClusterIdentity(client, info); err != nil {
		return err
	}

	info.ProjectID = firstNonEmpty(append([]string{info.ProjectID}, projectHints...)...)
	if info.ProjectID == "" && info.ClusterID != "" {
		nodeClasses, err := client.ListNodeClasses(info.ClusterID)
		if err == nil {
			info.ProjectID = firstNonEmpty(ProjectIDCandidatesFromRemoteNodeClasses(nodeClasses)...)
		}
	}
	if info.ProjectID == "" {
		currentProject, err := RunGcloudCurrentProject(ctx)
		if err != nil {
			return err
		}
		info.ProjectID = currentProject
	}

	if info.ProjectID == "" {
		return fmt.Errorf("project_id could not be inferred when kubeconfig is unset; set project_id or kubeconfig explicitly")
	}

	location := firstNonEmpty(info.ClusterLocation, info.Region)
	if info.ClusterName == "" || location == "" {
		return fmt.Errorf("cluster_name and cluster location could not be inferred when kubeconfig is unset")
	}

	kubeconfigPath = fmt.Sprintf(
		"%s_%s_%s_kubeconfig",
		kubeconfigPathPart(info.ProjectID),
		kubeconfigPathPart(location),
		kubeconfigPathPart(info.ClusterName),
	)
	if err := RunGcloudUpdateKubeconfig(ctx, info.ClusterName, location, info.ProjectID, kubeconfigPath); err != nil {
		return err
	}

	kubeconfigPath, err = normalizeKubeconfigPath(kubeconfigPath)
	if err != nil {
		return err
	}
	if kubeconfigPath == "" {
		return fmt.Errorf("generated kubeconfig path %q was not created", kubeconfigPath)
	}

	info.Kubeconfig = kubeconfigPath
	return nil
}

func kubeconfigPathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return kubeconfigPathUnsafePartRe.ReplaceAllString(value, "_")
}

func ProjectIDCandidatesFromNodeClassModels(
	ctx context.Context,
	nodeClasses customfield.NestedObjectList[api.GCENodeClassModel],
) ([]string, error) {
	if nodeClasses.IsNull() || nodeClasses.IsUnknown() {
		return nil, nil
	}

	models, diags := nodeClasses.AsStructSliceT(ctx)
	if diags.HasError() {
		return nil, fmt.Errorf("failed to decode nodeclasses: %v", diags)
	}

	seen := map[string]struct{}{}
	candidates := make([]string, 0)
	for _, model := range models {
		appendUniqueProjectID(ctx, &candidates, seen, projectIDFromString(terraformString(model.ServiceAccount)))

		if model.NetworkConfig.IsNull() || model.NetworkConfig.IsUnknown() {
			continue
		}

		networkConfig, diags := model.NetworkConfig.Value(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("failed to decode nodeclass network_config: %v", diags)
		}
		if networkConfig == nil {
			continue
		}

		appendUniqueProjectID(ctx, &candidates, seen, projectIDFromString(terraformString(networkConfig.Subnetwork)))

		if networkConfig.AdditionalNetworkInterfaces.IsNull() || networkConfig.AdditionalNetworkInterfaces.IsUnknown() {
			continue
		}
		nics, nicDiags := networkConfig.AdditionalNetworkInterfaces.AsStructSliceT(ctx)
		if nicDiags.HasError() {
			return nil, fmt.Errorf("failed to decode nodeclass additional_network_interfaces: %v", nicDiags)
		}
		for _, nic := range nics {
			appendUniqueProjectID(ctx, &candidates, seen, projectIDFromString(terraformString(nic.Subnetwork)))
		}
	}

	return candidates, nil
}

func ProjectIDCandidatesFromRemoteNodeClasses(nodeClasses api.RebalanceNodeClassList) []string {
	seen := map[string]struct{}{}
	candidates := make([]string, 0)

	for _, nodeClass := range nodeClasses.GCENodeClasses {
		if nodeClass.NodeClassSpec == nil {
			continue
		}

		appendUniqueProjectID(context.Background(), &candidates, seen, projectIDFromString(nodeClass.NodeClassSpec.ServiceAccount))

		if nodeClass.NodeClassSpec.NetworkConfig == nil {
			continue
		}

		appendUniqueProjectID(context.Background(), &candidates, seen, projectIDFromString(nodeClass.NodeClassSpec.NetworkConfig.Subnetwork))
		for _, nic := range nodeClass.NodeClassSpec.NetworkConfig.AdditionalNetworkInterfaces {
			appendUniqueProjectID(context.Background(), &candidates, seen, projectIDFromString(nic.Subnetwork))
		}
	}

	return candidates
}

func fillClusterIdentity(client Client, info *AccessInfo) error {
	if info.ClusterName != "" && info.Region != "" {
		return nil
	}
	if info.ClusterID == "" {
		return fmt.Errorf("cluster_id is required when kubeconfig is unset and cluster identity is incomplete")
	}

	summary, err := client.GetCluster(info.ClusterID)
	if err != nil {
		return fmt.Errorf("failed to infer cluster identity from cluster_id %q: %w", info.ClusterID, err)
	}
	if info.ClusterName == "" {
		info.ClusterName = summary.ClusterName
	}
	if info.Region == "" {
		info.Region = summary.Region
	}
	return nil
}

func normalizeKubeconfigPath(kubeconfigPath string) (string, error) {
	if strings.TrimSpace(kubeconfigPath) == "" {
		return "", nil
	}

	_, err := os.Stat(kubeconfigPath)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if os.IsNotExist(err) {
		return "", nil
	}

	fp, err := filepath.Abs(kubeconfigPath)
	if err != nil {
		return "", err
	}
	return fp, nil
}

func updateKubeconfig(ctx context.Context, clusterName, region, projectID, kubeconfigPath string) error {
	cmd := exec.CommandContext(
		ctx,
		"gcloud",
		"container",
		"clusters",
		"get-credentials",
		clusterName,
		"--location", region,
		"--project", projectID,
	)
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update gke kubeconfig: %w: %s", err, string(output))
	}
	return nil
}

func currentProject(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "gcloud", "config", "get-value", "project")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get current gcloud project: %w: %s", err, string(output))
	}
	projectID := strings.TrimSpace(string(output))
	if projectID == "" || projectID == "(unset)" {
		return "", nil
	}
	return projectID, nil
}

func clusterUIDFromKubeconfig(ctx context.Context, kubeconfigPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "get", "ns", "kube-system", "-o", "jsonpath={.metadata.uid}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get kube-system namespace uid from kubeconfig: %w: %s", err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func appendUniqueProjectID(_ context.Context, out *[]string, seen map[string]struct{}, candidate string) {
	if candidate == "" {
		return
	}
	if _, ok := seen[candidate]; ok {
		return
	}
	seen[candidate] = struct{}{}
	*out = append(*out, candidate)
}

func projectIDFromString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if matches := projectIDFromResourcePathRe.FindStringSubmatch(value); len(matches) == 2 {
		return matches[1]
	}
	if matches := projectIDFromServiceAccountRe.FindStringSubmatch(value); len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func terraformString(value interface {
	IsNull() bool
	IsUnknown() bool
	ValueString() string
}) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}
