data "cloudpilotai_gke_cluster" "production" {
  cluster_name = "production-gke"
  region       = "us-central1"
  cluster_uid  = "kube-system-namespace-uid"
}

output "cluster_status" {
  value = data.cloudpilotai_gke_cluster.production.status
}

output "agent_version" {
  value = data.cloudpilotai_gke_cluster.production.agent_version
}

output "onboard_manifest_version" {
  value = data.cloudpilotai_gke_cluster.production.onboard_manifest_version
}

output "need_upgrade" {
  value = data.cloudpilotai_gke_cluster.production.need_upgrade
}
