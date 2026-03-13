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

resource "cloudpilotai_eks_cluster" "example" {
  cluster_name        = var.cluster_name
  region              = var.region
  restore_node_number = var.restore_node_number

  only_install_agent = var.only_install_agent
  enable_rebalance   = var.enable_rebalance
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
