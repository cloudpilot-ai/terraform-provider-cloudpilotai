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

# Basic EKS cluster configuration with CloudPilot AI agent
resource "cloudpilotai_eks_cluster" "example" {
  cluster_name        = var.cluster_name
  region              = var.region
  restore_node_number = var.restore_node_number

  # --- Node Autoscaler Optimization ---
  # `only_install_agent` and `enable_rebalance` are controlled by variables in variables.tf.
  # To enable optimization, modify them in terraform.tfvars and re-apply.

  only_install_agent = var.only_install_agent
  enable_rebalance   = var.enable_rebalance

  # --- Optional configurations with default values shown ---

  # Disable automatic uploading of workload information to CloudPilot AI
  # Optional. Default is false.
  disable_workload_uploading = false

  # Enable upgrading the CloudPilot AI agent
  # Optional. Default is false.
  enable_upgrade_agent = false
  # Enable upgrading the CloudPilot AI rebalance component. Ignores `only_install_agent` if set to true.
  # Optional. Default is false.
  enable_upgrade_rebalance_component = false

  # Enable uploading of nodepool and nodeclass configuration to CloudPilot AI
  # Optional. Default is true.
  enable_upload_config = true
  # Enable diverse instance types for improved fault tolerance and cost optimization
  # Optional. Default is false.
  enable_diversity_instance_type = false

  # Define custom workload templates for different application types
  # Optional
  workload_templates = [
    {
      # Required
      template_name = "cloudpilotai-workload-template"

      # Rebalance able
      # Optional, default is true.
      rebalance_able = true
      # Spot friendly
      # Optional, default is true.
      spot_friendly = true
      # Min non spot replicas
      # Optional, default is 0.
      min_non_spot_replicas = 0
    }
  ]

  # Define custom workloads to be managed by CloudPilot AI
  # Optional
  workloads = [
    {
      # Required
      name = "cloudpilotai-workload"
      # Required
      type = "deployment"
      # Required
      namespace = "default"

      # Workload Template Name
      # Optional.
      template_name = "cloudpilotai-workload-template"

      # Rebalance able
      # Optional, default is true.
      rebalance_able = true
      # Spot friendly
      # Optional, default is true.
      spot_friendly = true
      # Min non spot replicas
      # Optional, default is 0.
      min_non_spot_replicas = 0
    }
  ]

  # Define custom nodeclass templates for reuse
  # Optional
  nodeclass_templates = [
    {
      # Required
      template_name = "default-nodeclass-template"

      # Each provisioned node will have the configured tags as key-value pairs.
      # Optional. Default is {"cloudpilot.ai/managed" = "true"}.
      instance_tags = { "cloudpilot.ai/managed" = "true" }
      # Each provisioned node's system storage size, default to be 20 GiB.
      # Optional. Default is 20.
      system_disk_size_gib = 20
      # Each provisioned node will have extra CPU allocation, used only for burstable pods.
      # Optional. Default is 0.
      extra_cpu_allocation_mcore = 0
      # Each provisioned node will have extra Memory allocation, used only for burstable pods.
      # Optional. Default is 0.
      extra_memory_allocation_mib = 0
    }
  ]

  # Define custom nodeclasses for different workload types.
  # The default name used by CloudPilot AI is "cloudpilot".
  # Optional
  nodeclasses = [
    {
      # Required. Must match the system default name "cloudpilot".
      name = "cloudpilot"

      # NodeClass Template Name
      # Optional.
      template_name = "default-nodeclass-template"

      # Each provisioned node will have the configured tags as key-value pairs.
      # Optional. Default is {"cloudpilot.ai/managed" = "true"}.
      instance_tags = { "cloudpilot.ai/managed" = "true" }
      # Each provisioned node's system storage size, default to be 20 GiB.
      # Optional. Default is 20.
      system_disk_size_gib = 20
      # Each provisioned node will have extra CPU allocation, used only for burstable pods.
      # Optional. Default is 0.
      extra_cpu_allocation_mcore = 0
      # Each provisioned node will have extra Memory allocation, used only for burstable pods.
      # Optional. Default is 0.
      extra_memory_allocation_mib = 0
    }
  ]

  # Define custom nodepool templates for reuse
  # Optional
  nodepool_templates = [
    {
      # Required.
      template_name = "default-nodepool-template"
      # Optional. Default is true.
      enable = true
      # Select the nodeclass to use for this nodepool. Must match a defined nodeclass name.
      # Required.
      nodeclass = "cloudpilot"
      # Enable GPU instances in this nodepool.
      # Optional, default is false.
      enable_gpu = false

      # The priority level of this nodepool. A larger number means a higher priority.
      # Optional, default is 2.
      provision_priority = 2
      # The target instance architecture.
      # Optional, default is amd64.
      instance_arch = ["amd64"]
      # The target instance family, like t3, m5 and so on.
      # Optional, default is all families.
      instance_family = []
      # The provisioned node's capacity type, on-demand or spot.
      # Optional, default is both on-demand and spot.
      capacity_type = ["spot", "on-demand"]
      # Each provisioned node will be located in the configured zone.
      # Optional, default is all zones in the region.
      zone = []
      # Minimum CPU cores per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.
      # Optional, default is 0.
      instance_cpu_min = 0
      # Maximum CPU cores per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.
      # Optional, default is 17.
      instance_cpu_max = 17
      # Minimum memory in MiB per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.
      # Optional, default is 0.
      instance_memory_min = 0
      # Maximum memory in MiB per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.
      # Optional, default is 32769.
      instance_memory_max = 32769
      # This specifies the maximum number of nodes that can be terminated at once, either as a fixed number (e.g., 2) or a percentage (e.g., 10%).
      # Optional, default is 2.
      node_disruption_limit = "2"
      # Specify the duration (e.g., 10s, 10m, or 10h) that the controller waits before terminating underutilized nodes.
      # Optional, default is 60m.
      node_disruption_delay = "60m"
    }
  ]

  # Define custom nodepools for workload distribution.
  # The default name used by CloudPilot AI is "cloudpilot-general".
  # Optional
  nodepools = [
    {
      # Required. Must match the system default name "cloudpilot-general".
      name = "cloudpilot-general"

      # Optional. Default is true.
      enable = true
      # Select the nodeclass to use for this nodepool. Must match a defined nodeclass name.
      # Required.
      nodeclass = "cloudpilot"

      # NodePool Template Name
      # Optional.
      template_name = "default-nodepool-template"

      # Enable GPU instances in this nodepool.
      # Optional, default is false.
      enable_gpu = false

      # The priority level of this nodepool. A larger number means a higher priority.
      # Optional, default is 2.
      provision_priority = 2
      # The target instance architecture.
      # Optional, default is amd64.
      instance_arch = ["amd64"]
      # The target instance family, like t3, m5 and so on.
      # Optional, default is all families.
      instance_family = []
      # The provisioned node's capacity type, on-demand or spot.
      # Optional, default is both on-demand and spot.
      capacity_type = ["spot", "on-demand"]
      # Each provisioned node will be located in the configured zone.
      # Optional, default is all zones in the region.
      zone = []
      # Minimum CPU cores per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.
      # Optional, default is 0.
      instance_cpu_min = 0
      # Maximum CPU cores per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.
      # Optional, default is 17.
      instance_cpu_max = 17
      # Minimum memory in MiB per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.
      # Optional, default is 0.
      instance_memory_min = 0
      # Maximum memory in MiB per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.
      # Optional, default is 32769.
      instance_memory_max = 32769
      # This specifies the maximum number of nodes that can be terminated at once, either as a fixed number (e.g., 2) or a percentage (e.g., 10%).
      # Optional, default is 2.
      node_disruption_limit = "2"
      # Specify the duration (e.g., 10s, 10m, or 10h) that the controller waits before terminating underutilized nodes.
      # Optional, default is 60m.
      node_disruption_delay = "60m"
    }
  ]
}

# ============================================================================
# Workload Autoscaler (standalone resource, cloud-provider independent)
# ============================================================================

# Deploy CloudPilot AI Workload Autoscaler on the same cluster.
# This is a separate resource from the EKS cluster and can work with any
# Kubernetes cluster managed by CloudPilot AI.
resource "cloudpilotai_workload_autoscaler" "example" {
  # The CloudPilot AI cluster ID.
  # Can reference the cluster_id output from a cloudpilotai_eks_cluster resource.
  # ⚠️ Required
  cluster_id = cloudpilotai_eks_cluster.example.cluster_id

  # Path to the kubeconfig file for kubectl/helm operations.
  # ⚠️ Required
  kubeconfig = cloudpilotai_eks_cluster.example.kubeconfig

  storage_class     = var.wa_storage_class
  enable_node_agent = var.wa_enable_node_agent

  # --- Recommendation Policies ---
  # These are the server-default policies. Include them to prevent Terraform from deleting them.
  # You can modify values or add additional policies as needed.
  recommendation_policies = [
    {
      name               = "balanced"
      strategy_type      = "percentile"
      percentile_cpu     = 95
      percentile_memory  = 99
      history_window_cpu    = "24h"
      history_window_memory = "48h"
      evaluation_period     = "1m"
      buffer_cpu         = "10%"
      buffer_memory      = "20%"
      request_min_cpu    = "25%"
      request_min_memory = "30%"
      request_max_cpu    = ""
      request_max_memory = ""
    },
    {
      name               = "cost-savings"
      strategy_type      = "percentile"
      percentile_cpu     = 90
      percentile_memory  = 95
      history_window_cpu    = "12h"
      history_window_memory = "24h"
      evaluation_period     = "1m"
      buffer_cpu         = ""
      buffer_memory      = ""
      request_min_cpu    = "30m"
      request_min_memory = "30Mi"
      request_max_cpu    = ""
      request_max_memory = ""
    },
    {
      name               = "burstable"
      strategy_type      = "percentile"
      percentile_cpu     = 90
      percentile_memory  = 98
      history_window_cpu    = "6h"
      history_window_memory = "12h"
      evaluation_period     = "20s"
      buffer_cpu         = "10%"
      buffer_memory      = "20%"
      request_min_cpu    = "25%"
      request_min_memory = "30%"
      request_max_cpu    = ""
      request_max_memory = ""
    }
  ]

  # --- Autoscaling Policies ---
  # These are the server-default policies. Include them to prevent Terraform from deleting them.
  # "cloudpilot" manages workloads in the cloudpilot namespace with auto-recreate.
  # "readonly" observes all workloads without making changes.
  autoscaling_policies = [
    {
      name                       = "cloudpilot"
      enable                     = true
      recommendation_policy_name = "balanced"
      priority                   = 10
      update_resources           = ["cpu", "memory"]
      drift_threshold_cpu        = "5%"
      drift_threshold_memory     = "5%"
      on_policy_removal          = "recreate"

      target_refs = [
        {
          api_version = "apps/v1"
          kind        = "Deployment"
          name        = ""
          namespace   = "cloudpilot"
        },
        {
          api_version = "apps/v1"
          kind        = "StatefulSet"
          name        = ""
          namespace   = "cloudpilot"
        }
      ]

      update_schedules = [
        {
          name     = "default"
          schedule = ""
          duration = ""
          mode     = "recreate"
        }
      ]

      limit_policies = [
        {
          resource      = "cpu"
          remove_limit  = true
          keep_limit    = false
          multiplier    = ""
          auto_headroom = ""
        },
        {
          resource      = "memory"
          remove_limit  = false
          keep_limit    = false
          multiplier    = ""
          auto_headroom = "2"
        }
      ]

      startup_boost_enabled            = false
      startup_boost_min_boost_duration  = ""
      startup_boost_min_ready_duration  = ""
      startup_boost_multiplier_cpu      = ""
      startup_boost_multiplier_memory   = ""
      in_place_fallback_default_policy  = ""
    },
    {
      name                       = "readonly"
      enable                     = true
      recommendation_policy_name = "cost-savings"
      priority                   = 0
      update_resources           = ["cpu", "memory"]
      drift_threshold_cpu        = "5%"
      drift_threshold_memory     = "5%"
      on_policy_removal          = "off"

      target_refs = [
        {
          api_version = "apps/v1"
          kind        = "Deployment"
          name        = ""
          namespace   = ""
        },
        {
          api_version = "apps/v1"
          kind        = "StatefulSet"
          name        = ""
          namespace   = ""
        }
      ]

      update_schedules = [
        {
          name     = "default"
          schedule = ""
          duration = ""
          mode     = "off"
        }
      ]

      limit_policies = [
        {
          resource      = "cpu"
          remove_limit  = true
          keep_limit    = false
          multiplier    = ""
          auto_headroom = ""
        },
        {
          resource      = "memory"
          remove_limit  = false
          keep_limit    = false
          multiplier    = ""
          auto_headroom = "2"
        }
      ]

      startup_boost_enabled            = false
      startup_boost_min_boost_duration  = ""
      startup_boost_min_ready_duration  = ""
      startup_boost_multiplier_cpu      = ""
      startup_boost_multiplier_memory   = ""
      in_place_fallback_default_policy  = ""
    }
  ]

  # --- Enable Proactive Optimization ---
  # Automatically enable proactive update for workloads matching the specified filters.
  # Each entry selects workloads by namespace and/or workload kind.
  enable_proactive = [
    {
      # Enable proactive optimization for all workloads in the "cloudpilot" namespace.
      namespaces = ["cloudpilot"]
    }
  ]

  # --- Disable Proactive Optimization ---
  # Automatically disable proactive update for workloads matching the specified filters.
  # Each entry selects workloads by namespace, workload kind, or other filter criteria.
  disable_proactive = [
    {
      # Disable proactive optimization for all workloads in the "kube-system" namespace.
      namespaces = ["kube-system"]
    }
  ]
}

# ============================================================================
# Data Sources (read-only queries, no changes made)
# ============================================================================

# Query cluster information: status, node counts, costs, and savings.
data "cloudpilotai_eks_cluster" "example" {
  cluster_name = var.cluster_name
  region       = var.region

  depends_on = [cloudpilotai_eks_cluster.example]
}

# Query Workload Autoscaler state: enabled/installed status and all policies.
data "cloudpilotai_workload_autoscaler" "example" {
  cluster_id = cloudpilotai_eks_cluster.example.cluster_id

  depends_on = [cloudpilotai_workload_autoscaler.example]
}

# ============================================================================
# Outputs — Resource
# ============================================================================

output "cluster_name" {
  description = "Name of the EKS cluster"
  value       = cloudpilotai_eks_cluster.example.cluster_name
}

output "enable_rebalance" {
  description = "Enable cloudpilot AI rebalance feature"
  value       = cloudpilotai_eks_cluster.example.enable_rebalance
}

# output "workload_autoscaler_cluster_id" {
#   description = "Cluster ID used by the Workload Autoscaler"
#   value       = cloudpilotai_workload_autoscaler.example.cluster_id
# }

# ============================================================================
# Outputs — Data Source: EKS Cluster
# ============================================================================

output "cluster_status" {
  description = "Current cluster status (online/offline/demo)"
  value       = data.cloudpilotai_eks_cluster.example.status
}

output "agent_version" {
  description = "CloudPilot AI agent version installed on the cluster"
  value       = data.cloudpilotai_eks_cluster.example.agent_version
}

# ============================================================================
# Outputs — Data Source: Workload Autoscaler
# ============================================================================

output "wa_enabled" {
  description = "Whether the Workload Autoscaler is enabled"
  value       = data.cloudpilotai_workload_autoscaler.example.enabled
}

output "wa_installed" {
  description = "Whether the Workload Autoscaler is installed"
  value       = data.cloudpilotai_workload_autoscaler.example.installed
}

# output "wa_recommendation_policies" {
#   description = "Current recommendation policies"
#   value       = data.cloudpilotai_workload_autoscaler.example.recommendation_policies
# }

# output "wa_autoscaling_policies" {
#   description = "Current autoscaling policies"
#   value       = data.cloudpilotai_workload_autoscaler.example.autoscaling_policies
# }
