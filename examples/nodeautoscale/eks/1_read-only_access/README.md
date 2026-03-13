# Example: Agent-Only Installation (Read-Only Access)

This example demonstrates installing only the CloudPilot AI agent on an existing EKS cluster without enabling optimization features. This is ideal for monitoring and evaluation purposes.

**By default, only the agent is installed (read-only).** You can progressively enable node autoscaler and rebalancing by modifying the variables in `terraform.tfvars` and re-applying.

## Features

- Agent-only installation (no workload optimization)
- Minimal configuration requirements
- Read-only monitoring of cluster resources
- Safe evaluation without cluster changes
- Progressive enablement of optimization features

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

### Step 1: Basic Installation (Agent Only)

1. **Configure your variables**:
   ```bash
   cp terraform.tfvars.example terraform.tfvars
   ```
   Edit `terraform.tfvars` with your values:
   - `cloudpilot_api_key`: Your CloudPilot AI API key
   - `cluster_name`: Your EKS cluster name
   - `region`: AWS region where your cluster is located
   - `restore_node_number`: Current node count in your cluster

2. **Apply the configuration**:
   ```bash
   terraform init
   terraform plan
   terraform apply
   ```

   This installs only the CloudPilot AI agent for monitoring — no optimization changes are made to your cluster.

### Step 2: Install Node Autoscaler (When Ready)

When you're ready to install the node autoscaler, edit `terraform.tfvars`:

```hcl
only_install_agent = false
```

Then re-apply:

```bash
terraform plan
terraform apply
```

### Step 3: Enable Rebalance

After the node autoscaler is installed, you can enable workload rebalancing. Edit `terraform.tfvars`:

```hcl
enable_rebalance = true
```

Then re-apply:

```bash
terraform plan
terraform apply
```

## What This Example Creates

- CloudPilot AI agent pods in your cluster
- Monitoring and metrics collection
- Connection to CloudPilot AI dashboard for insights
- (Optional) Node autoscaler for workload optimization
- (Optional) Workload rebalancing across node pools

See the `main.tf` file for the complete configuration.
