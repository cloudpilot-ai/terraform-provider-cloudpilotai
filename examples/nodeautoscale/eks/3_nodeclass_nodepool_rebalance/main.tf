terraform {
  required_providers {
    cloudpilotai = {
      source = "cloudpilot-ai/cloudpilotai"
    }
  }
}

# Configure the CloudPilot AI Provider
provider "cloudpilotai" {
  api_endpoint = var.cloudpilot_api_endpoint
  api_key      = var.cloudpilot_api_key
}

# Basic EKS cluster configuration with CloudPilot AI agent
resource "cloudpilotai_eks_cluster" "example" {
  cluster_name        = var.cluster_name
  region              = var.region
  restore_node_number = var.restore_node_number

  # --- Node Autoscaler Optimization ---
  # `only_install_agent` and `enable_rebalance` are controlled by variables in variables.tf.
  # To enable optimization, modify them in terraform.tfvars and re-apply.

  only_install_agent = var.only_install_agent
  enable_rebalance   = var.enable_rebalance

  # --- Optional configurations with default values shown ---

  # Disable automatic uploading of workload information to CloudPilot AI
  # Optional. Default is false.
  disable_workload_uploading = false

  # Enable upgrading the CloudPilot AI agent
  # Optional. Default is false.
  enable_upgrade_agent = false
  # Enable upgrading the CloudPilot AI rebalance component. Ignores `only_install_agent` if set to true.
  # Optional. Default is false.
  enable_upgrade_rebalance_component = false

  # Enable uploading of nodepool and nodeclass configuration to CloudPilot AI
  # Optional. Default is true.
  enable_upload_config = true
  # Enable diverse instance types for improved fault tolerance and cost optimization
  # Optional. Default is false.
  enable_diversity_instance_type = false

  # Define custom nodeclasses for different workload types.
  # The default name used by CloudPilot AI is "cloudpilot".
  # Optional
  nodeclasses = [
    {
      # Required. Must match the system default name "cloudpilot".
      name = "cloudpilot"

      # Each provisioned node will have the configured tags as key-value pairs.
      # Optional. Default is {"cloudpilot.ai/managed" = "true"}.
      instance_tags = { "cloudpilot.ai/managed" = "true" }
      # Each provisioned node's system storage size, default to be 20 GiB.
      # Optional. Default is 20.
      system_disk_size_gib = 20
      # Each provisioned node will have extra CPU allocation, used only for burstable pods.
      # Optional. Default is 0.
      extra_cpu_allocation_mcore = 0
      # Each provisioned node will have extra Memory allocation, used only for burstable pods.
      # Optional. Default is 0.
      extra_memory_allocation_mib = 0
    }
  ]

  # Define custom nodepools for workload distribution.
  # The default name used by CloudPilot AI is "cloudpilot-general".
  # Optional
  nodepools = [
    {
      # Required. Must match the system default name "cloudpilot-general".
      name = "cloudpilot-general"

      # Optional. Default is true.
      enable = true
      # Select the nodeclass to use for this nodepool. Must match a defined nodeclass name.
      # Required.
      nodeclass = "cloudpilot"

      # Enable GPU instances in this nodepool.
      # Optional, default is false.
      enable_gpu = false

      # The priority level of this nodepool. A larger number means a higher priority.
      # Optional, default is 2.
      provision_priority = 2
      # The target instance architecture.
      # Optional, default is amd64.
      instance_arch = ["amd64"]
      # The target instance family, like t3, m5 and so on.
      # Optional, default is all families.
      instance_family = []
      # The provisioned node's capacity type, on-demand or spot.
      # Optional, default is both on-demand and spot.
      capacity_type = ["spot", "on-demand"]
      # Each provisioned node will be located in the configured zone.
      # Optional, default is all zones in the region.
      zone = []
      # Minimum CPU cores per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.
      # Optional, default is 0.
      instance_cpu_min = 0
      # Maximum CPU cores per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.
      # Optional, default is 17.
      instance_cpu_max = 17
      # Minimum memory in MiB per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.
      # Optional, default is 0.
      instance_memory_min = 0
      # Maximum memory in MiB per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.
      # Optional, default is 32769.
      instance_memory_max = 32769
      # This specifies the maximum number of nodes that can be terminated at once, either as a fixed number (e.g., 2) or a percentage (e.g., 10%).
      # Optional, default is 2.
      node_disruption_limit = "2"
      # Specify the duration (e.g., 10s, 10m, or 10h) that the controller waits before terminating underutilized nodes.
      # Optional, default is 60m.
      node_disruption_delay = "60m"
    }
  ]
}

# Output cluster information
output "cluster_name" {
  description = "Name of the EKS cluster"
  value       = cloudpilotai_eks_cluster.example.cluster_name
}

output "enable_rebalance" {
  description = "Enable cloudpilot AI rebalance feature"
  value       = cloudpilotai_eks_cluster.example.enable_rebalance
}
