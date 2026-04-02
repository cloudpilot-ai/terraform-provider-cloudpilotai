data "cloudpilotai_workload_autoscaler" "current" {
  cluster_id = cloudpilotai_eks_cluster.my_cluster.cluster_id
}

output "wa_enabled" {
  value = data.cloudpilotai_workload_autoscaler.current.enabled
}

output "wa_installed" {
  value = data.cloudpilotai_workload_autoscaler.current.installed
}
