# Example: Basic Workload Rebalancing

This example demonstrates enabling CloudPilot AI's workload rebalance feature for automatic cost optimization and resource efficiency on your EKS cluster.

**By default, optimization is not enabled.** You can enable rebalance by modifying the variables in `terraform.tfvars` and re-applying.

## Features

- CloudPilot AI agent installation
- Automatic workload rebalancing (when enabled)
- Cost optimization through intelligent scheduling
- Minimal configuration for quick setup

## Benefits

- **Cost Reduction**: Automatically moves workloads to more cost-effective nodes
- **Resource Efficiency**: Optimizes resource utilization across the cluster
- **Spot Instance Support**: Intelligently uses spot instances when appropriate
- **Zero Downtime**: Performs rebalancing without service interruption

### Prerequisites

- **[Terraform](https://developer.hashicorp.com/terraform/install)** - Version 1.0 or later
- **[AWS CLI](https://docs.aws.amazon.com/zh_cn/cli/latest/userguide/getting-started-install.html)** - Install and configure the AWS CLI with credentials that have EKS cluster management permissions. Required for EKS-related operations such as updating kubeconfig. If you haven't created an EKS cluster yet, see the example setup: [eks-ondemand](https://github.com/cloudpilot-ai/examples/tree/main/clusters/eks-ondemand)
- **[Kubectl](https://kubernetes.io/docs/tasks/tools)** - For cluster operations and component management
- **CloudPilot AI API key** - See [CloudPilot AI API Key Documentation](https://docs.cloudpilot.ai/guide/getting_started/get_apikeys) for setup instructions

## Usage

### Step 1: Basic Installation

1. **Configure your variables**:
   ```bash
   cp terraform.tfvars.example terraform.tfvars
   ```
   Edit `terraform.tfvars` with your values:
   - `cloudpilot_api_key`: Your CloudPilot AI API key
   - `cluster_name`: Your EKS cluster name
   - `region`: AWS region where your cluster is located
   - `restore_node_number`: Current node count for restoration

2. **Apply the configuration**:
   ```bash
   terraform init
   terraform plan
   terraform apply
   ```

   This installs the CloudPilot AI agent without enabling optimization.

### Step 2: Enable Rebalance (When Ready)

When you're ready to enable workload rebalancing, edit `terraform.tfvars`:

```hcl
enable_rebalance = true
```

Then re-apply:

```bash
terraform plan
terraform apply
```

## Monitoring

After enabling rebalance, you can:

- View optimization results in the CloudPilot AI dashboard
- Monitor cost savings and efficiency improvements
- Track workload movements and node utilization

See the `main.tf` file for the complete configuration.
