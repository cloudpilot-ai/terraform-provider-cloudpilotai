# Example: Basic Workload Rebalancing

This example demonstrates enabling CloudPilot AI's workload rebalance feature for automatic cost optimization and resource efficiency on your EKS cluster.

## Features

- CloudPilot AI agent installation
- Automatic workload rebalancing enabled
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

1. **Configure your API key** using one of these methods:
   - Set in the provider block: `api_key = "sk-your-api-key"`
   - Use a file: `api_key_profile = "/path/to/api-key-file"`

2. **Update configuration**:
   - `cluster_name`: Your EKS cluster name
   - `region`: AWS region where your cluster is located
   - `restore_node_number`: Current node count for restoration

3. **Apply the configuration**:

   ```bash
   terraform init
   terraform plan
   terraform apply
   ```

## Key Configuration

- `enable_rebalance = true`: Enables automatic workload rebalancing
- Default settings for optimal performance
- Outputs cluster information for monitoring

## What This Example Creates

- CloudPilot AI agent pods in your cluster
- Rebalance controller for workload optimization
- Automatic workload scheduling optimization
- Cost monitoring and recommendations

## Monitoring

After applying, you can:

- View optimization results in the CloudPilot AI dashboard
- Monitor cost savings and efficiency improvements
- Track workload movements and node utilization

See the `main.tf` file for the complete configuration.
