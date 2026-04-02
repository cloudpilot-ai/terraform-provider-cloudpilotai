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
