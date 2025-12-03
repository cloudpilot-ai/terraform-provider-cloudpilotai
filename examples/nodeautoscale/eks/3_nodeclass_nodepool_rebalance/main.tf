
terraform {
  required_providers {
    cloudpilotai = {
      source = "cloudpilot-ai/cloudpilotai"
    }
  }
}

# variable "CLOUDPILOT_API_KEY" {
#   description = "CloudPilot AI API key"
#   type        = string
#   sensitive   = true
# }

provider "cloudpilotai" {
  # API key for CloudPilot AI - REQUIRED
  # Can be provided in multiple ways (in order of priority):
  # 1. Directly in provider block using 'api_key'
  # 2. Via file path using 'api_key_profile'
  # api_key_profile = ""       # Optional: Path to a file containing the API key
  # 3. Via environment variable 'TF_VAR_CLOUDPILOT_API_KEY'
  # api_key = var.CLOUDPILOT_API_KEY
  # If none of these methods provide an API key, an error will occur
  api_key = "sk-xxx" # Obtained via cloudpilot.ai console
}

# Basic EKS cluster configuration with CloudPilot AI agent
resource "cloudpilotai_eks_cluster" "example" {
  # Name of the EKS cluster to be managed
  # ⚠️ Required
  cluster_name = "my-eks-cluster"
  # AWS region where the EKS cluster is located
  # ⚠️ Required
  region = "us-west-2"
  # Required node count when uninstalling CloudPilot AI after optimization is enabled.
  # Please configure this manually. A simple approach is to check current node count with:
  # kubectl get node --no-headers=true | wc -l
  # Then set this value to your desired node count for cluster restoration.
  # ⚠️ Required
  restore_node_number = 2

  #   --- Optional configurations with default values shown ---

  # Disable automatic uploading of workload information to CloudPilot AI
  # Optional. Default is false.
  disable_workload_uploading = false

  # Only install the CloudPilot AI agent without additional configuration
  # Optional. Default is false.
  only_install_agent = false

  # Enable upgrading the CloudPilot AI agent
  # Optional. Default is false.
  enable_upgrade_agent = false
  # Enable upgrading the CloudPilot AI rebalance component. Ignores `only_install_agent` if set to true.
  # Optional. Default is false.
  enable_upgrade_rebalance_component = false

  # Enable automatic workload rebalancing across node pools. Ignores `only_install_agent` if set to true.
  # Optional. Default is true.
  enable_rebalance = true
  # Enable uploading of nodepool and nodeclass configuration to CloudPilot AI
  # Optional. Default is true.
  enable_upload_config = true
  # Enable diverse instance types for improved fault tolerance and cost optimization
  # Optional. Default is false.
  enable_diversity_instance_type = false



  # Define custom nodeclasses for different workload types
  # Optional
  nodeclasses = [
    {
      name = "cloudpilotai-nodeclass"

      # Each provisioned node will have the configured tags as key-value pairs. Defaults to `node.cloudpilot.ai/managed=true` if not specified.
      # Optional. Default is {"node.cloudpilot.ai/managed" = "true"}.
      instance_tags = { "node.cloudpilot.ai/managed" = "true" }
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

  # Define custom nodepools for workload distribution
  # Optional
  nodepools = [
    {
      name = "cloudpilotai-nodepool"

      enable = true
      # Select the nodeclass to use for this nodepool.
      nodeclass = "cloudpilotai-nodeclass"

      # Enable GPU instances in this nodepool.
      # Optional, default is false.
      enable_gpu = false

      # The priority level of this nodepool. A larger number means a higher priority.
      # Optional, default is 1.
      provision_priority = 1
      # "The target instance architecture, if the instance family is configured, this field will be ignored."
      # Optional, default is amd64 and arm64.
      instance_arch = ["amd64", "arm64"]
      # The target instance family, like t3, m5 and so on, split by comma.
      # Optional, default is all families.
      instance_family = []
      # The provisioned node's capacity type, on-demand or spot.
      # Optional, default is both on-demand and spot.
      capacity_type = ["on-demand", "spot"]
      # "Each provisioned node will located in the configured zone, formatted as us-west-1a,us-west-1b."
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
