# Example: Agent-Only Installation

This example demonstrates installing only the CloudPilot AI agent on an existing EKS cluster without enabling optimization features. This is ideal for monitoring and evaluation purposes.

## Features

- Agent-only installation (no workload optimization)
- Minimal configuration requirements
- Read-only monitoring of cluster resources
- Safe evaluation without cluster changes

## Use Cases

- **Evaluation**: Test CloudPilot AI without making changes to your cluster
- **Monitoring**: Observe cluster metrics and recommendations
- **Staging**: Install agent in non-production environments first

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
   - `restore_node_number`: Current node count in your cluster

3. **Apply the configuration**:

   ```bash
   terraform init
   terraform plan
   terraform apply
   ```

## Key Configuration

- `only_install_agent = true`: Installs only the monitoring agent
- No rebalance or optimization features enabled
- Minimal resource footprint

## What This Example Creates

- CloudPilot AI agent pods in your cluster
- Monitoring and metrics collection
- Connection to CloudPilot AI dashboard for insights
- No workload modifications or optimizations

See the `main.tf` file for the complete configuration.
