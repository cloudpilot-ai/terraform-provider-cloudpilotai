
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

See the [`examples/`](examples/) directory for real-world configurations:

| Example | Description | Use Case |
|---------|-------------|----------|
| [`0_details`](examples/nodeautoscale/eks/0_details/) | Full-featured EKS cluster with all options | Production setup with workload templates, nodeclasses, and complete configuration |
| [`1_read-only_access`](examples/nodeautoscale/eks/1_read-only_access/) | Agent-only installation | Testing or monitoring without optimization changes |
| [`2_basic_rebalance`](examples/nodeautoscale/eks/2_basic_rebalance/) | Basic rebalance enabled | Simple cost optimization with workload rebalancing |
| [`3_nodeclass_nodepool_rebalance`](examples/nodeautoscale/eks/3_nodeclass_nodepool_rebalance/) | Custom nodeclass/nodepool | Advanced node management with custom configurations |

Each example folder contains a `main.tf` and a dedicated README with usage instructions.

---

## Support

- [CloudPilot AI Documentation](https://docs.cloudpilot.ai/)
- [Terraform Provider Registry](https://registry.terraform.io/providers/cloudpilot-ai/cloudpilotai/latest)
- Issues: [GitHub Issues](https://github.com/cloudpilot-ai/terraform-provider-cloudpilotai/issues)

---

**Note:** Never call CloudPilot API endpoints directly; always use the `Client` struct in `pkg/cloudpilot-ai/client/`.
