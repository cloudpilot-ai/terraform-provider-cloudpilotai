terraform {
  required_version = ">= 1.0"

  required_providers {
    cloudpilotai = {
      source  = "cloudpilot-ai/cloudpilotai"
      version = ">= 0.2"
    }
  }
}

provider "cloudpilotai" {
  api_endpoint = var.cloudpilot_api_endpoint
  api_key      = var.cloudpilot_api_key
}

resource "cloudpilotai_gke_cluster" "example" {
  cluster_name     = var.cluster_name
  region           = var.region
  project_id       = var.project_id
  cluster_uid      = var.cluster_uid
  cluster_location = var.cluster_location

  only_install_agent = false
  enable_rebalance   = true
  enable_upgrade     = true

  cluster_setting = {
    enable_node_repair  = true
    enable_disk_monitor = true
    discount            = 0.15
  }

  nodeclasses = [
    {
      name            = "cloudpilot"
      service_account = var.node_service_account
      image_selector_terms = [
        {
          family  = "ContainerOptimizedOS"
          channel = "cluster"
        }
      ]
      disks = [
        {
          boot     = true
          category = "pd-balanced"
          size_gib = 80
        }
      ]
    }
  ]

  nodepools = [
    {
      name            = "cloudpilot-general"
      enable          = true
      nodeclass       = "cloudpilot"
      capacity_type   = ["spot", "on-demand"]
      instance_arch   = ["amd64"]
      instance_family = ["n4", "n2", "e2"]
    }
  ]
}

resource "cloudpilotai_workload_autoscaler" "example" {
  cluster_id = cloudpilotai_gke_cluster.example.cluster_id

  enable_node_agent = true
}
