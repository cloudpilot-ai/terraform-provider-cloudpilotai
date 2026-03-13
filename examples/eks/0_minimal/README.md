# Example: Minimal Configuration

This example demonstrates a minimal setup that installs both the Node Autoscaler and Workload Autoscaler on an existing EKS cluster. It enables workload optimization for the `cloudpilot` namespace without enabling rebalance.

No nodeclass, nodepool, or workload template configuration is included — this is a quick-start example.

## What This Example Does

- Installs the CloudPilot AI Node Autoscaler (configurable via `only_install_agent`)
- Installs the Workload Autoscaler with a `balanced` recommendation policy
- Creates an autoscaling policy targeting all Deployments and StatefulSets in the `cloudpilot` namespace
- Workload rebalancing configurable via `enable_rebalance` (default: disabled)

## Prerequisites

- **[Terraform](https://developer.hashicorp.com/terraform/install)** - Version 1.0 or later
- **[AWS CLI](https://docs.aws.amazon.com/zh_cn/cli/latest/userguide/getting-started-install.html)** - Install and configure the AWS CLI with credentials that have EKS cluster management permissions. Required for EKS-related operations such as updating kubeconfig. If you haven't created an EKS cluster yet, see the example setup: [eks-ondemand](https://github.com/cloudpilot-ai/examples/tree/main/clusters/eks-ondemand)
- **[Kubectl](https://kubernetes.io/docs/tasks/tools)** - For cluster operations and component management
- **CloudPilot AI API key** - See [CloudPilot AI API Key Documentation](https://docs.cloudpilot.ai/guide/getting_started/get_apikeys) for setup instructions

## Usage

1. **Configure your variables**:
   ```bash
   cp terraform.tfvars.example terraform.tfvars
   ```
   Edit `terraform.tfvars` with your values:
   - `cloudpilot_api_key`: Your CloudPilot AI API key
   - `cluster_name`: Your EKS cluster name
   - `region`: AWS region where your cluster is located
   - `restore_node_number`: Node count for cluster restoration
   - `wa_storage_class`: StorageClass for VictoriaMetrics (required if no default StorageClass)

2. **Apply the configuration**:
   ```bash
   terraform init
   terraform plan
   terraform apply
   ```

   This installs the Node Autoscaler and Workload Autoscaler, and enables optimization for the `cloudpilot` namespace.

## Next Steps

- To enable workload rebalancing, see [`3_basic_rebalance`](../3_basic_rebalance/)
- To customize nodeclass and nodepool settings, see [`4_nodeclass_nodepool_rebalance`](../4_nodeclass_nodepool_rebalance/)
- For a full reference of all options, see [`1_details`](../1_details/)

See the `main.tf` file for the complete configuration.
