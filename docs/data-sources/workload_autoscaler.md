---
page_title: "cloudpilotai_workload_autoscaler Data Source - cloudpilotai"
subcategory: "Workload Autoscaler"
description: |-
  Retrieves the Workload Autoscaler configuration for a given cluster.
---

# cloudpilotai_workload_autoscaler (Data Source)

Retrieves read-only information about the Workload Autoscaler configuration on a cluster registered with CloudPilot AI. Use this data source to check whether the autoscaler is enabled and installed without making any changes.

## Example Usage

```terraform
data "cloudpilotai_workload_autoscaler" "current" {
  cluster_id = cloudpilotai_eks_cluster.my_cluster.cluster_id
}

output "wa_enabled" {
  value = data.cloudpilotai_workload_autoscaler.current.enabled
}

output "wa_installed" {
  value = data.cloudpilotai_workload_autoscaler.current.installed
}
```

## Schema

### Required

- `cluster_id` (String) — The CloudPilot AI cluster ID.

### Read-Only

- `enabled` (Boolean) — Whether the Workload Autoscaler is enabled on this cluster.
- `installed` (Boolean) — Whether the Workload Autoscaler is installed on this cluster.
