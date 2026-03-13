---
page_title: "cloudpilotai_eks_cluster Data Source - cloudpilotai"
subcategory: "Node Autoscale"
description: |-
  Retrieves information about an existing EKS cluster registered with CloudPilot AI.
---

# cloudpilotai_eks_cluster (Data Source)

Retrieves read-only information about an EKS cluster that is already registered with CloudPilot AI. Use this data source to query cluster status and agent information without making any changes.

## Example Usage

```terraform
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
```

## Schema

### Required

- `cluster_name` (String) — Name of the EKS cluster.
- `region` (String) — AWS region where the EKS cluster is located.

### Optional

- `account_id` (String) — AWS account ID. If not provided, it is auto-detected from the current AWS CLI credentials.

### Read-Only

- `cluster_id` (String) — CloudPilot AI cluster identifier.
- `cloud_provider` (String) — Cloud provider (e.g. `aws`).
- `status` (String) — Current cluster status: `online`, `offline`, or `demo`.
- `agent_version` (String) — Version of the CloudPilot AI agent installed.
- `rebalance_enable` (Boolean) — Whether rebalancing is enabled.
