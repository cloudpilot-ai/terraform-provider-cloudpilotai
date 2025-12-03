# Example: Full EKS Cluster with All Options

This example demonstrates a comprehensive configuration of a CloudPilot AI-managed EKS cluster, showcasing all major features and configuration options available in the provider.

## Features

- Full EKS cluster management with CloudPilot AI integration
- Agent and rebalance component installation and upgrades
- Custom workload templates and workload-specific settings
- Custom nodeclass and nodepool templates for reusability
- Advanced node provisioning with instance filtering
- Node restoration configuration for safe uninstall

### Prerequisites

- **[Terraform](https://developer.hashicorp.com/terraform/install)** - Version 1.0 or later
- **[AWS CLI](https://docs.aws.amazon.com/zh_cn/cli/latest/userguide/getting-started-install.html)** - Install and configure the AWS CLI with credentials that have EKS cluster management permissions. Required for EKS-related operations such as updating kubeconfig. If you haven't created an EKS cluster yet, see the example setup: [eks-ondemand](https://github.com/cloudpilot-ai/examples/tree/main/clusters/eks-ondemand)
- **[Kubectl](https://kubernetes.io/docs/tasks/tools)** - For cluster operations and component management
- **CloudPilot AI API key** - See [CloudPilot AI API Key Documentation](https://docs.cloudpilot.ai/guide/getting_started/get_apikeys) for setup instructions

## Usage

1. **Configure your API key** using one of these methods:
   - Set in the provider block: `api_key = "sk-your-api-key"`
   - Use a file: `api_key_profile = "/path/to/api-key-file"`

2. **Review and adjust configuration**:
   - `cluster_name`: Your EKS cluster name
   - `region`: AWS region where your cluster is located
   - `restore_node_number`: Node count for cluster restoration

3. **Apply the configuration**:

   ```bash
   terraform init
   terraform plan
   terraform apply
   ```

## Key Configuration Options

- **Agent Options**: `only_install_agent`, `enable_upgrade_agent`, `disable_workload_uploading`
- **Rebalance Features**: `enable_rebalance`, `enable_upload_config`, `enable_diversity_instance_type`
- **Templates**: `workload_templates`, `nodeclass_templates`, `nodepool_templates`
- **Custom Resources**: `workloads`, `nodeclasses`, `nodepools`
- **Instance Filtering**: CPU/memory limits, instance families, availability zones

## What This Example Creates

- CloudPilot AI agent installation on your EKS cluster
- Rebalance component for workload optimization
- Custom workload template for application optimization
- Custom nodeclass with specific resource allocations
- Custom nodepool with instance type filtering and disruption controls

See the `main.tf` file for detailed configuration with inline comments.
