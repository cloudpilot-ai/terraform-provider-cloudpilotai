package helper

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func InstallWorkloadAutoscaler(ctx context.Context, client cloudpilotaiclient.Interface,
	kubeconfigPath, storageClass string, enableNodeAgent bool,
) error {
	sh, err := client.GetWorkloadAutoscalerSH()
	if err != nil {
		return fmt.Errorf("failed to get workload autoscaler install script: %w", err)
	}

	env := map[string]string{
		"KUBECONFIG": kubeconfigPath,
	}
	if storageClass != "" {
		env["STORAGE_CLASS"] = storageClass
	}
	if enableNodeAgent {
		env["ENABLE_NODE_AGENT"] = "true"
	} else {
		env["ENABLE_NODE_AGENT"] = "false"
	}

	return ExecuteSH(ctx, sh, env)
}

const waitWorkloadAutoscalerResourcesCleanedSH = `while true; do
  ap=$( (kubectl get autoscalingpolicies -o json 2>/dev/null || echo '{"items":[]}') | jq -r '.items|length')
  apc=$( (kubectl get autoscalingpolicyconfigurations -A -o json 2>/dev/null || echo '{"items":[]}') | jq -r '.items|length')
  rp=$( (kubectl get recommendationpolicies -o json 2>/dev/null || echo '{"items":[]}') | jq -r '.items|length')
  echo "Waiting Workload Autoscaler related resource to be cleaned..."
  echo "AutoscalingPolicy=$ap, AutoscalingPolicyConfiguration=$apc, RecommendationPolicy=$rp"
  [[ "$ap" -eq 0 && "$apc" -eq 0 && "$rp" -eq 0 ]] && break
  sleep 3
done
echo "All Workload Autoscaler related resources are cleaned."`

const uninstallWorkloadAutoscalerSH = `helm uninstall workload-autoscaler -n cloudpilot --ignore-not-found 2>/dev/null || true`

func UninstallWorkloadAutoscaler(ctx context.Context, client cloudpilotaiclient.Interface, clusterID, kubeconfigPath string) error {
	enableFalse := false
	tflog.Info(ctx, "disabling Workload Autoscaler via API before uninstall")
	if err := client.UpdateWAConfiguration(clusterID, &api.WAConfiguration{
		EnableWorkloadAutoscaler: &enableFalse,
	}); err != nil {
		return fmt.Errorf("failed to disable Workload Autoscaler: %w", err)
	}

	env := map[string]string{
		"KUBECONFIG": kubeconfigPath,
	}

	tflog.Info(ctx, "waiting for Workload Autoscaler related resources to be cleaned")
	if err := ExecuteSH(ctx, waitWorkloadAutoscalerResourcesCleanedSH, env); err != nil {
		return fmt.Errorf("failed waiting for Workload Autoscaler resources cleanup: %w", err)
	}

	tflog.Info(ctx, "uninstalling Workload Autoscaler helm release")
	return ExecuteSH(ctx, uninstallWorkloadAutoscalerSH, env)
}

func ApplyRecommendationPolicies(ctx context.Context, client cloudpilotaiclient.Interface, clusterID string,
	rpList customfield.NestedObjectList[api.RecommendationPolicyModel],
) (desiredNames map[string]struct{}, err error) {
	if rpList.IsNullOrUnknown() {
		return nil, nil
	}

	rpModels, diags := rpList.AsStructSliceT(ctx)
	if diags.HasError() {
		return nil, fmt.Errorf("failed to parse recommendation policy configuration: %v", diags)
	}

	desiredNames = make(map[string]struct{}, len(rpModels))
	for i := range rpModels {
		desiredNames[rpModels[i].Name.ValueString()] = struct{}{}
		rp := rpModels[i].ToResource()
		tflog.Info(ctx, fmt.Sprintf("applying recommendation policy: %s", rp.Name))
		if err := client.ApplyRecommendationPolicy(clusterID, rp); err != nil {
			return nil, fmt.Errorf("failed to apply recommendation policy %s: %w", rp.Name, err)
		}
	}

	return desiredNames, nil
}

// DeleteStaleRecommendationPolicies deletes policies that were previously
// tracked in Terraform state but are no longer in the desired configuration.
// Only policies listed in previousStateNames are candidates for deletion;
// server-side policies not tracked by Terraform are left untouched.
func DeleteStaleRecommendationPolicies(ctx context.Context, client cloudpilotaiclient.Interface, clusterID string,
	desiredNames, previousStateNames map[string]struct{},
) error {
	if previousStateNames == nil {
		return nil
	}

	for name := range previousStateNames {
		if _, stillDesired := desiredNames[name]; !stillDesired {
			tflog.Info(ctx, fmt.Sprintf("deleting recommendation policy removed from config: %s", name))
			if err := client.DeleteRecommendationPolicy(clusterID, name); err != nil {
				if strings.Contains(err.Error(), "not found") {
					tflog.Info(ctx, fmt.Sprintf("recommendation policy %s already removed, skipping", name))
					continue
				}
				if strings.Contains(err.Error(), "in use") {
					tflog.Warn(ctx, fmt.Sprintf("skipping deletion of recommendation policy %s: %v", name, err))
					continue
				}
				return fmt.Errorf("failed to delete recommendation policy %s: %w", name, err)
			}
		}
	}

	return nil
}

func ApplyAutoscalingPolicies(ctx context.Context, client cloudpilotaiclient.Interface, clusterID string,
	apList customfield.NestedObjectList[api.AutoscalingPolicyModel],
) (desiredNames map[string]struct{}, err error) {
	if apList.IsNullOrUnknown() {
		return nil, nil
	}

	apModels, diags := apList.AsStructSliceT(ctx)
	if diags.HasError() {
		return nil, fmt.Errorf("failed to parse autoscaling policy configuration: %v", diags)
	}

	desiredNames = make(map[string]struct{}, len(apModels))
	for i := range apModels {
		desiredNames[apModels[i].Name.ValueString()] = struct{}{}
		ap, convErr := apModels[i].ToResource(ctx)
		if convErr != nil {
			return nil, fmt.Errorf("failed to convert autoscaling policy %s: %w", apModels[i].Name.ValueString(), convErr)
		}
		tflog.Info(ctx, fmt.Sprintf("applying autoscaling policy: %s", ap.Name))
		if err := client.ApplyAutoscalingPolicy(clusterID, ap); err != nil {
			return nil, fmt.Errorf("failed to apply autoscaling policy %s: %w", ap.Name, err)
		}
	}

	return desiredNames, nil
}

func ApplyProactiveUpdates(ctx context.Context, client cloudpilotaiclient.Interface, clusterID string,
	proactiveList customfield.NestedObjectList[api.EnableProactiveModel],
) error {
	if proactiveList.IsNullOrUnknown() {
		return nil
	}

	models, diags := proactiveList.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("failed to parse enable_proactive configuration: %v", diags)
	}

	for i := range models {
		req := models[i].ToRequest()
		tflog.Info(ctx, fmt.Sprintf("enabling proactive update for namespaces=%v, workloadKinds=%v",
			req.ListFilter.Namespaces, req.ListFilter.WorkloadKinds))
		if err := client.UpdateWorkloadProactiveUpdate(clusterID, &req); err != nil {
			return fmt.Errorf("failed to enable proactive update: %w", err)
		}
	}

	return nil
}

func ApplyDisableProactiveUpdates(ctx context.Context, client cloudpilotaiclient.Interface, clusterID string,
	proactiveList customfield.NestedObjectList[api.DisableProactiveModel],
) error {
	if proactiveList.IsNullOrUnknown() {
		return nil
	}

	models, diags := proactiveList.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("failed to parse disable_proactive configuration: %v", diags)
	}

	for i := range models {
		req := models[i].ToRequest()
		tflog.Info(ctx, fmt.Sprintf("disabling proactive update for namespaces=%v, workloadKinds=%v",
			req.ListFilter.Namespaces, req.ListFilter.WorkloadKinds))
		if err := client.UpdateWorkloadProactiveUpdate(clusterID, &req); err != nil {
			return fmt.Errorf("failed to disable proactive update: %w", err)
		}
	}

	return nil
}

// DeleteStaleAutoscalingPolicies deletes policies that were previously tracked
// in Terraform state but are no longer in the desired configuration.
// Only policies listed in previousStateNames are candidates for deletion;
// server-side policies not tracked by Terraform are left untouched.
func DeleteStaleAutoscalingPolicies(ctx context.Context, client cloudpilotaiclient.Interface, clusterID string,
	desiredNames, previousStateNames map[string]struct{},
) error {
	if previousStateNames == nil {
		return nil
	}

	for name := range previousStateNames {
		if _, stillDesired := desiredNames[name]; !stillDesired {
			tflog.Info(ctx, fmt.Sprintf("deleting autoscaling policy removed from config: %s", name))
			if err := client.DeleteAutoscalingPolicy(clusterID, name); err != nil {
				if strings.Contains(err.Error(), "not found") {
					tflog.Info(ctx, fmt.Sprintf("autoscaling policy %s already removed, skipping", name))
					continue
				}
				return fmt.Errorf("failed to delete autoscaling policy %s: %w", name, err)
			}
		}
	}

	return nil
}
