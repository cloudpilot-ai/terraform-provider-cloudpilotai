terraform {
  required_providers {
    cloudpilotai = {
      source = "cloudpilot-ai/cloudpilotai"
    }
  }
}

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

resource "cloudpilotai_workload_autoscaler" "example" {
  cluster_id = cloudpilotai_eks_cluster.example.cluster_id
  kubeconfig = cloudpilotai_eks_cluster.example.kubeconfig

  storage_class     = var.wa_storage_class
  enable_node_agent = var.wa_enable_node_agent

  recommendation_policies = []

  autoscaling_policies = []

  # --- Enable Proactive Optimization ---
  # Automatically enable proactive update for workloads matching the specified filters.
  # Each entry selects workloads by namespace and/or workload kind.
  enable_proactive = [
    {
      # Enable proactive optimization for all workloads in the "cloudpilot" namespace.
      namespaces = ["cloudpilot"]
    }
  ]
}

output "cluster_name" {
  description = "Name of the EKS cluster"
  value       = cloudpilotai_eks_cluster.example.cluster_name
}
