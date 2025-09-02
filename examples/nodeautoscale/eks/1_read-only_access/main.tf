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

resource "cloudpilotai_eks_cluster" "example" {
  # ⚠️ Required
  cluster_name = "my-eks-cluster"
  # ⚠️ Required
  region = "us-west-2"
  # Required node count when uninstalling CloudPilot AI after optimization is enabled.
  # Please configure this manually. A simple approach is to check current node count with:
  # kubectl get node --no-headers=true | wc -l
  # Then set this value to your desired node count for cluster restoration.
  # ⚠️ Required
  restore_node_number = 2

  only_install_agent = true
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
