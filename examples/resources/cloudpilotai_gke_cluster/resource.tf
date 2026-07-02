# Minimal GKE cluster registration
resource "cloudpilotai_gke_cluster" "readonly" {
  cluster_name = "my-gke-cluster"
  region       = "us-central1"
  project_id   = "my-gcp-project"
  cluster_uid  = "kube-system-namespace-uid"
}

# Cluster registration plus cluster-level settings
resource "cloudpilotai_gke_cluster" "managed" {
  cluster_name = "my-gke-cluster"
  region       = "us-central1"
  project_id   = "my-gcp-project"
  cluster_uid  = "kube-system-namespace-uid"

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
      service_account = "nodes@my-gcp-project.iam.gserviceaccount.com"
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
