
# CloudPilot AI Terraform Provider

Easily manage Amazon EKS and Google Kubernetes Engine clusters and workloads with CloudPilot AI's automation and cost optimization. This provider enables seamless integration of CloudPilot AI's agent, workload rebalancing, and node pool management into your Terraform workflow.

---

## Features

- **Provision and manage EKS and GKE clusters** with CloudPilot AI integration
- **Automated agent and rebalance component installation**
- **Node pool and node class management** (including custom Karpenter JSON)
- **Workload cost optimization** (rebalance, spot-friendly, min non-spot replicas)
- **Seamless AWS CLI and kubeconfig integration**

For reusable node/workload template composition on EKS, prefer the [`cloudpilot-ai/eks/cloudpilotai`](https://registry.terraform.io/modules/cloudpilot-ai/eks/cloudpilotai/latest) module. The provider's own `*_templates` and `template_name` fields remain available for compatibility today, but they are deprecated and planned for removal in a future major provider release.

---

## Getting Started

### Prerequisites

- **[Terraform](https://developer.hashicorp.com/terraform/install)** - Version 1.0 or later
- **[AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html)** — Install and configure the AWS CLI with credentials that have EKS cluster management permissions. Required for EKS-related operations such as generating kubeconfig, running install scripts, and executing `kubectl`/`helm` through the generated kubeconfig.
  - **Terraform-only CI/CD:** If your Terraform run already has AWS base credentials or OIDC, set `aws_assume_role` on `cloudpilotai_eks_cluster` to make CloudPilot assume the same target role for its AWS CLI and kubeconfig operations.
  - **Local development:** `aws_profile` remains available as the source-credential fallback. Generated kubeconfigs carry that profile for the current operation, but their environment-local paths are not stored in Terraform state.
- **[Kubectl](https://kubernetes.io/docs/tasks/tools)** - For cluster operations and component management
- **[gcloud CLI](https://cloud.google.com/sdk/docs/install)** - Required for GKE-related operations such as generating kubeconfig and running CloudPilot install scripts against GKE clusters
- **CloudPilot AI API key** - See [CloudPilot AI API Key Documentation](https://docs.cloudpilot.ai/guide/getting_started/get_apikeys) for setup instructions

---

## Example Usage

See the [`examples/eks/`](examples/eks/) directory for EKS configurations, ordered from minimal to full-featured.

See the [`examples/gke/`](examples/gke/) directory for GKE configurations.

| Example | Description | Use Case |
|---------|-------------|----------|
| [`0_minimal`](examples/eks/0_minimal/) | Minimal setup with Node Autoscaler and Workload Autoscaler | Quick start: installs both autoscalers, enables optimization for `cloudpilot` namespace, no rebalance |
| [`1_details`](examples/eks/1_details/) | Full-featured reference with all options | Production setup covering the full provider surface, including deprecated provider-side template inputs, Workload Autoscaler, and data sources |
| [`2_read-only_access`](examples/eks/2_read-only_access/) | Agent-only installation | Testing or monitoring without any optimization changes |
| [`3_basic_rebalance`](examples/eks/3_basic_rebalance/) | Basic rebalance configuration | Simple cost optimization with rebalancing |
| [`4_nodeclass_nodepool_rebalance`](examples/eks/4_nodeclass_nodepool_rebalance/) | Custom nodeclass and nodepool | Advanced node management with instance filtering and disruption controls |
| [`5_import`](examples/eks/5_import_test/) | Import existing resources | Bring existing CloudPilot AI-managed clusters into Terraform state without recreating them |

Each example folder contains:
- `main.tf` — resource definitions
- `variables.tf` — variable declarations
- `terraform.tfvars.example` — sample variable values (copy to `terraform.tfvars` to use)
- `README.md` — usage instructions

For GKE, use `cloudpilotai_gke_cluster` for cluster registration, agent install, GCP node autoscaler management, and typed nodeclass/nodepool configuration, then attach the platform-neutral `cloudpilotai_workload_autoscaler` resource to the resulting `cluster_id`. The `cluster_uid` input for GKE should come from `kubectl get ns kube-system -o jsonpath='{.metadata.uid}'`.

---

## Support

- [CloudPilot AI Documentation](https://docs.cloudpilot.ai/)
- [Terraform Provider Registry](https://registry.terraform.io/providers/cloudpilot-ai/cloudpilotai/latest)
- Issues: [GitHub Issues](https://github.com/cloudpilot-ai/terraform-provider-cloudpilotai/issues)

---

**Note:** Never call CloudPilot API endpoints directly; always use the `Client` struct in `pkg/cloudpilot-ai/client/`.
