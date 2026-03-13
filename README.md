
# CloudPilot AI Terraform Provider

Easily manage Amazon EKS clusters and workloads with CloudPilot AI's automation and cost optimization. This provider enables seamless integration of CloudPilot AI's agent, workload rebalancing, and node pool management into your Terraform workflow.

---

## Features

- **Provision and manage EKS clusters** with CloudPilot AI integration
- **Automated agent and rebalance component installation**
- **Node pool and node class management** (including custom Karpenter JSON)
- **Workload cost optimization** (rebalance, spot-friendly, min non-spot replicas)
- **Seamless AWS CLI and kubeconfig integration**

---

## Getting Started

### Prerequisites

- **[Terraform](https://developer.hashicorp.com/terraform/install)** - Version 1.0 or later
- **[AWS CLI](https://docs.aws.amazon.com/zh_cn/cli/latest/userguide/getting-started-install.html)** - Install and configure the AWS CLI with credentials that have EKS cluster management permissions. Required for EKS-related operations such as updating kubeconfig. If you haven't created an EKS cluster yet, see the example setup: [eks-ondemand](https://github.com/cloudpilot-ai/examples/tree/main/clusters/eks-ondemand)
  - **Important**: Even if you provide a `kubeconfig` path, AWS CLI permissions are still required. The kubeconfig parameter only prevents the provider from generating a new kubeconfig file; all kubectl operations still require proper AWS credentials.
  - **Multi-account setup**: If your AWS CLI is configured with multiple accounts/profiles, ensure you switch to the correct account that matches your EKS cluster before running Terraform commands.
- **[Kubectl](https://kubernetes.io/docs/tasks/tools)** - For cluster operations and component management
- **CloudPilot AI API key** - See [CloudPilot AI API Key Documentation](https://docs.cloudpilot.ai/guide/getting_started/get_apikeys) for setup instructions

---

## Example Usage

See the [`examples/eks/`](examples/eks/) directory for EKS configurations, ordered from minimal to full-featured.

| Example | Description | Use Case |
|---------|-------------|----------|
| [`0_minimal`](examples/eks/0_minimal/) | Minimal setup with Node Autoscaler and Workload Autoscaler | Quick start: installs both autoscalers, enables optimization for `cloudpilot` namespace, no rebalance |
| [`1_details`](examples/eks/1_details/) | Full-featured reference with all options | Production setup with workload templates, nodeclasses, nodepools, Workload Autoscaler, and data sources |
| [`2_read-only_access`](examples/eks/2_read-only_access/) | Agent-only installation | Testing or monitoring without any optimization changes |
| [`3_basic_rebalance`](examples/eks/3_basic_rebalance/) | Basic rebalance configuration | Simple cost optimization with rebalancing |
| [`4_nodeclass_nodepool_rebalance`](examples/eks/4_nodeclass_nodepool_rebalance/) | Custom nodeclass and nodepool | Advanced node management with instance filtering and disruption controls |

Each example folder contains:
- `main.tf` — resource definitions
- `variables.tf` — variable declarations
- `terraform.tfvars.example` — sample variable values (copy to `terraform.tfvars` to use)
- `README.md` — usage instructions

---

## Support

- [CloudPilot AI Documentation](https://docs.cloudpilot.ai/)
- [Terraform Provider Registry](https://registry.terraform.io/providers/cloudpilot-ai/cloudpilotai/latest)
- Issues: [GitHub Issues](https://github.com/cloudpilot-ai/terraform-provider-cloudpilotai/issues)

---

**Note:** Never call CloudPilot API endpoints directly; always use the `Client` struct in `pkg/cloudpilot-ai/client/`.
