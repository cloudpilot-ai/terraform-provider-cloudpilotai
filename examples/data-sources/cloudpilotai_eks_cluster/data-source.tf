data "cloudpilotai_eks_cluster" "production" {
  cluster_name = "production-cluster"
  region       = "us-west-2"
}

output "cluster_status" {
  value = data.cloudpilotai_eks_cluster.production.status
}

output "agent_version" {
  value = data.cloudpilotai_eks_cluster.production.agent_version
}

output "onboard_manifest_version" {
  value = data.cloudpilotai_eks_cluster.production.onboard_manifest_version
}

output "need_upgrade" {
  value = data.cloudpilotai_eks_cluster.production.need_upgrade
}
