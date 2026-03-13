# Example: Full EKS Cluster with All Options

This example demonstrates a comprehensive configuration of a CloudPilot AI-managed EKS cluster, showcasing all major features and configuration options available in the provider, including the standalone Workload Autoscaler resource.

**By default, only the CloudPilot AI agent is installed without enabling optimization.** You can enable optimization features by modifying the variables in `terraform.tfvars` and re-applying.

## Features

- Full EKS cluster management with CloudPilot AI integration
- Agent and rebalance component installation and upgrades
- Custom workload templates and workload-specific settings
- Custom nodeclass and nodepool templates for reusability
- Advanced node provisioning with instance filtering
- Node restoration configuration for safe uninstall
- **Workload Autoscaler** deployment with RecommendationPolicy and AutoscalingPolicy configuration
- **Data Sources** for querying cluster status, costs, node counts, and autoscaler policies (read-only)

### Prerequisites

- **[Terraform](https://developer.hashicorp.com/terraform/install)** - Version 1.0 or later
- **[AWS CLI](https://docs.aws.amazon.com/zh_cn/cli/latest/userguide/getting-started-install.html)** - Install and configure the AWS CLI with credentials that have EKS cluster management permissions. Required for EKS-related operations such as updating kubeconfig. If you haven't created an EKS cluster yet, see the example setup: [eks-ondemand](https://github.com/cloudpilot-ai/examples/tree/main/clusters/eks-ondemand)
- **[Kubectl](https://kubernetes.io/docs/tasks/tools)** - For cluster operations and component management
- **[Helm](https://helm.sh/docs/intro/install/)** - Required for Workload Autoscaler installation
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
   - `restore_node_number`: Node count for cluster restoration

2. **Apply the configuration**:
   ```bash
   terraform init
   terraform plan
   terraform apply
   ```

   This installs only the CloudPilot AI agent for monitoring — no optimization changes are made to your cluster.

### Step 2: Enable Optimization (When Ready)

When you're ready to enable optimization features, edit `terraform.tfvars`:

```hcl
# Disable agent-only mode
only_install_agent = false

# Enable the features you need
enable_rebalance       = true
```

Then re-apply:

```bash
terraform plan
terraform apply
```

See `terraform.tfvars.example` for all available optimization variables.

## Key Configuration Options

### Node Autoscaler (`cloudpilotai_eks_cluster`)

- **Agent Options**: `only_install_agent`, `enable_upgrade_agent`, `disable_workload_uploading`
- **Rebalance Features**: `enable_rebalance`, `enable_upload_config`, `enable_diversity_instance_type`
- **Templates**: `workload_templates`, `nodeclass_templates`, `nodepool_templates`
- **Custom Resources**: `workloads`, `nodeclasses`, `nodepools`
- **Instance Filtering**: CPU/memory limits, instance families, availability zones

### Workload Autoscaler (`cloudpilotai_workload_autoscaler`)

- **Installation**: `cluster_id`, `kubeconfig`, `storage_class`, `enable_node_agent`
- **RecommendationPolicy**: `strategy_type`, `percentile_cpu/memory`, `history_window_cpu/memory`, `evaluation_period`, `buffer_cpu/memory`, `request_min/max_cpu/memory`
- **AutoscalingPolicy**: `recommendation_policy_name`, `priority`, `target_refs`, `update_resources`, `drift_threshold_cpu/memory`, `on_policy_removal`, `update_schedules`, `limit_policies`, startup boost, in-place fallback

## Data Sources

This example also demonstrates the **read-only data sources**:

- `data.cloudpilotai_eks_cluster` — Queries cluster status, agent version, and rebalance state.
- `data.cloudpilotai_workload_autoscaler` — Queries whether the Workload Autoscaler is enabled and installed.

After `terraform apply`, view the queried information with:

```bash
terraform output cluster_status
terraform output agent_version
terraform output wa_enabled
terraform output wa_installed
```

See the `main.tf` file for detailed configuration with inline comments.
