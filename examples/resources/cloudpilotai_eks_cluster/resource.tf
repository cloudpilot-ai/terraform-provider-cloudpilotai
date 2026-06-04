# Minimal — install the CloudPilot AI agent in read-only mode
resource "cloudpilotai_eks_cluster" "readonly" {
  cluster_name       = "my-eks-cluster"
  region             = "us-west-2"
  only_install_agent = true
}

# Basic rebalance — let CloudPilot AI optimize node placement
resource "cloudpilotai_eks_cluster" "rebalance" {
  cluster_name        = "my-eks-cluster"
  region              = "us-west-2"
  restore_node_number = 3
  enable_rebalance    = true

  cluster_setting = {
    enable_node_repair  = true
    enable_disk_monitor = true
    discount            = 0.15
    pre_run_command     = <<-EOT
      set -euo pipefail

      echo "pre run start"
      aws sts get-caller-identity
      kubectl get nodes
    EOT
    post_run_command    = <<-EOT
      set -euo pipefail

      echo "post run start"
      kubectl get pods -A
    EOT
  }
}

# Full configuration with node classes and node pools
resource "cloudpilotai_eks_cluster" "full" {
  cluster_name        = "my-eks-cluster"
  region              = "us-west-2"
  restore_node_number = 3
  enable_rebalance    = true

  nodeclasses = [
    {
      name                 = "cloudpilot"
      system_disk_size_gib = 30
      instance_tags        = { "cloudpilot.ai/managed" = "true" }
    }
  ]

  nodepools = [
    {
      name          = "cloudpilot-general"
      nodeclass     = "cloudpilot"
      enable        = true
      capacity_type = ["spot", "on-demand"]
      instance_arch = ["amd64"]
    }
  ]
}
