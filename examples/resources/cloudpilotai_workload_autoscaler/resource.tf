resource "cloudpilotai_workload_autoscaler" "example" {
  cluster_id = cloudpilotai_eks_cluster.my_cluster.cluster_id

  aws_profile     = cloudpilotai_eks_cluster.my_cluster.aws_profile
  aws_assume_role = cloudpilotai_eks_cluster.my_cluster.aws_assume_role

  recommendation_policies = [
    {
      name                  = "balanced"
      strategy_type         = "percentile"
      percentile_cpu        = 95
      percentile_memory     = 99
      history_window_cpu    = "24h"
      history_window_memory = "48h"
      evaluation_period     = "1m"
      buffer_cpu            = "10%"
      buffer_memory         = "20%"
    }
  ]

  autoscaling_policies = [
    {
      name                       = "default-ap"
      enable                     = true
      recommendation_policy_name = "balanced"

      target_refs = [
        {
          api_version = "apps/v1"
          kind        = "Deployment"
        }
      ]

      update_schedules = [
        {
          name = "default"
          mode = "inplace"
        }
      ]
    }
  ]

  enable_proactive = [
    {
      namespaces = ["my-namespace"]
    }
  ]

  disable_proactive = [
    {
      namespaces = ["kube-system"]
    }
  ]
}
