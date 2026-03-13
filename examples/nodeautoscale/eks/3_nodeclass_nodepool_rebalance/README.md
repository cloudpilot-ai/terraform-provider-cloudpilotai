# Example: Advanced Node Management with Custom Classes and Pools

This example demonstrates advanced EKS cluster management using custom nodeclasses and nodepools with CloudPilot AI's intelligent rebalancing and optimization features.

**By default, optimization is not enabled.** You can enable optimization features by modifying the variables in `terraform.tfvars` and re-applying.

## Features

- Custom nodeclass definitions with specific resource allocations
- Advanced nodepool configuration with instance type filtering
- Comprehensive rebalance and optimization settings
- GPU support and specialized instance configurations
- Zone-specific deployments and disruption controls

## Advanced Capabilities

- **Custom Node Classes**: Define node specifications for different workload types
- **Instance Filtering**: Control CPU, memory, and instance family selection
- **Capacity Management**: Mix on-demand and spot instances intelligently
- **Disruption Controls**: Configure node replacement timing and limits
- **Architecture Support**: Target specific architectures (amd64, arm64)

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
   - `restore_node_number`: Node count for cluster restoration

2. **Review and customize node configuration** in `main.tf`:
   - `nodeclasses`: Customize node specifications
   - `nodepools`: Adjust instance filtering and priorities

3. **Apply the configuration**:
   ```bash
   terraform init
   terraform plan
   terraform apply
   ```

   This installs the CloudPilot AI agent and configures nodeclasses/nodepools without enabling optimization.

### Step 2: Enable Optimization (When Ready)

When you're ready to enable optimization features, edit `terraform.tfvars`:

```hcl
enable_rebalance = true
```

Then re-apply:

```bash
terraform plan
terraform apply
```

## Key Configuration Options

### Nodeclass Configuration

- `instance_tags`: Custom tags for provisioned nodes
- `system_disk_size_gib`: Storage configuration
- `extra_cpu_allocation_mcore`: Additional CPU for burstable workloads
- `extra_memory_allocation_mib`: Additional memory allocation

### Nodepool Configuration

- `provision_priority`: Priority levels for different pools
- `instance_arch`: Target architectures (amd64, arm64)
- `instance_family`: Specific EC2 instance families
- `capacity_type`: Mix of on-demand and spot instances
- `instance_cpu_min/instance_cpu_max`: CPU core filtering
- `instance_memory_min/instance_memory_max`: Memory filtering
- `node_disruption_limit`: Maximum concurrent node replacements
- `node_disruption_delay`: Wait time before replacing underutilized nodes

## Use Cases

- **Multi-workload Clusters**: Different node types for different applications
- **Cost Optimization**: Intelligent spot instance usage
- **Performance Tuning**: Specific instance types for specific workloads
- **High Availability**: Zone distribution and disruption controls

See the `main.tf` file for detailed configuration with comprehensive inline comments.
