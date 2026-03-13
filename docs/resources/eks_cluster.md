---
page_title: "cloudpilotai_eks_cluster Resource - cloudpilotai"
subcategory: "Node Autoscale"
description: |-
  Manages an EKS cluster with CloudPilot AI agent, rebalance components, and node configuration.
---

# cloudpilotai_eks_cluster (Resource)

Manages an EKS cluster registered with CloudPilot AI. This resource handles the full lifecycle: installing the CloudPilot AI agent, configuring rebalance settings, and managing node pools and node classes.

## Example Usage

### Read-Only Agent Installation

```terraform
resource "cloudpilotai_eks_cluster" "readonly" {
  cluster_name       = "my-eks-cluster"
  region             = "us-west-2"
  restore_node_number = 0
  only_install_agent = true
}
```

### Basic Rebalance

```terraform
resource "cloudpilotai_eks_cluster" "rebalance" {
  cluster_name       = "my-eks-cluster"
  region             = "us-west-2"
  restore_node_number = 3
  enable_rebalance   = true
}
```

### With Node Classes and Node Pools

```terraform
resource "cloudpilotai_eks_cluster" "full" {
  cluster_name       = "my-eks-cluster"
  region             = "us-west-2"
  restore_node_number = 3
  enable_rebalance   = true

  nodeclasses {
    name              = "default"
    system_disk_size_gib = 30
  }

  nodepools {
    name      = "default"
    nodeclass = "default"
    enable    = true
    capacity_type  = ["spot", "on-demand"]
    instance_arch  = ["amd64"]
  }
}
```

## Schema

### Required

- `cluster_name` (String) — Name of the EKS cluster to be managed.
- `region` (String) — AWS region where the EKS cluster is located.
- `restore_node_number` (Number) — Number of nodes to restore when deleting the cluster resource. Set to 0 if no nodes need restoring.

### Optional

- `kubeconfig` (String) — Path to the kubeconfig file. If not provided, the provider generates one using AWS CLI.
- `account_id` (String) — AWS account ID. Auto-detected from AWS CLI if not set.
- `disable_workload_uploading` (Boolean) — Disable uploading workload information. Default: `false`.
- `only_install_agent` (Boolean) — Only install the agent without rebalance. Default: `false`.
- `enable_upgrade_agent` (Boolean) — Upgrade the agent on next apply. Default: `false`.
- `enable_upgrade_rebalance_component` (Boolean) — Upgrade the rebalance component. Default: `false`.
- `enable_rebalance` (Boolean) — Enable automatic workload rebalancing. Default: `false`.
- `enable_upload_config` (Boolean) — Upload nodepool/nodeclass config to CloudPilot AI. Default: `true`.
- `enable_diversity_instance_type` (Boolean) — Enable diverse instance types. Default: `false`.
- `workload_templates` (List of Object) — Workload template configurations.
- `workloads` (List of Object) — Workload rebalance configurations.
- `nodeclass_templates` (List of Object) — NodeClass template configurations.
- `nodeclasses` (List of Object) — NodeClass configurations.
- `nodepool_templates` (List of Object) — NodePool template configurations.
- `nodepools` (List of Object) — NodePool configurations.

### Read-Only

- `cluster_id` (String) — Unique identifier of the cluster (computed).

## Import

This resource does not support import.
