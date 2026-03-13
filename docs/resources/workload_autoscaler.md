---
page_title: "cloudpilotai_workload_autoscaler Resource - cloudpilotai"
subcategory: "Workload Autoscaler"
description: |-
  Manages the CloudPilot AI Workload Autoscaler with recommendation and autoscaling policies.
---

# cloudpilotai_workload_autoscaler (Resource)

Manages the CloudPilot AI Workload Autoscaler on a Kubernetes cluster. This resource installs the autoscaler components, configures recommendation policies, and sets up autoscaling policies for workload right-sizing.

## Example Usage

```terraform
resource "cloudpilotai_workload_autoscaler" "example" {
  cluster_id = cloudpilotai_eks_cluster.my_cluster.cluster_id
  kubeconfig = "/path/to/kubeconfig"

  recommendation_policies {
    name                 = "default-rp"
    strategy_type        = "percentile"
    percentile_cpu       = 95
    percentile_memory    = 95
    history_window_cpu   = "168h"
    history_window_memory = "168h"
    evaluation_period    = "1h"
  }

  autoscaling_policies {
    name                       = "default-ap"
    enable                     = true
    recommendation_policy_name = "default-rp"

    target_refs {
      api_version = "apps/v1"
      kind        = "Deployment"
    }

    update_schedules {
      name = "default"
      mode = "inplace"
    }
  }

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
```

## Schema

### Required

- `cluster_id` (String) — The CloudPilot AI cluster ID to deploy Workload Autoscaler on.
- `kubeconfig` (String) — Path to the kubeconfig file for the target Kubernetes cluster.

### Optional

- `storage_class` (String) — StorageClass name for VictoriaMetrics persistent volume. Default: cluster default.
- `enable_node_agent` (Boolean) — Enable the Node Agent DaemonSet for per-node metrics. Default: `true`.
- `recommendation_policies` (List of Object) — List of recommendation policies. See [Recommendation Policy](#recommendation-policy) below.
- `autoscaling_policies` (List of Object) — List of autoscaling policies. See [Autoscaling Policy](#autoscaling-policy) below.
- `enable_proactive` (List of Object) — Workload filters to enable proactive optimization. See [Proactive Filter](#proactive-filter) below.
- `disable_proactive` (List of Object) — Workload filters to disable proactive optimization. See [Proactive Filter](#proactive-filter) below.

### Recommendation Policy

Each recommendation policy supports:

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | String | Yes | Policy name |
| `strategy_type` | String | No | Strategy type (`percentile`). Default: `percentile` |
| `percentile_cpu` | Number | No | CPU percentile (50-100). Default: `95` |
| `percentile_memory` | Number | No | Memory percentile (50-100). Default: `95` |
| `history_window_cpu` | String | Yes | CPU history window duration (e.g. `168h`) |
| `history_window_memory` | String | Yes | Memory history window duration |
| `evaluation_period` | String | Yes | Evaluation period duration (e.g. `1h`) |
| `buffer_cpu` | String | No | CPU buffer (e.g. `10%` or `100m`) |
| `buffer_memory` | String | No | Memory buffer (e.g. `10%` or `128Mi`) |
| `request_min_cpu` | String | No | Minimum CPU request recommendation |
| `request_min_memory` | String | No | Minimum Memory request recommendation |
| `request_max_cpu` | String | No | Maximum CPU request recommendation |
| `request_max_memory` | String | No | Maximum Memory request recommendation |

### Autoscaling Policy

Each autoscaling policy supports:

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | String | Yes | Policy name |
| `enable` | Boolean | No | Whether enabled. Default: `true` |
| `recommendation_policy_name` | String | Yes | Associated recommendation policy |
| `priority` | Number | No | Priority (higher wins). Default: `0` |
| `update_resources` | List(String) | No | Resources to optimize (e.g. `["cpu", "memory"]`) |
| `drift_threshold_cpu` | String | No | CPU drift threshold |
| `drift_threshold_memory` | String | No | Memory drift threshold |
| `on_policy_removal` | String | No | Behavior on removal: `off`, `recreate`, `inplace`. Default: `off` |
| `target_refs` | List(Object) | No | Target workload references |
| `update_schedules` | List(Object) | No | Update schedule items |
| `limit_policies` | List(Object) | No | Per-resource limit policies |
| `startup_boost_enabled` | Boolean | No | Enable startup resource boost. Default: `false` |
| `in_place_fallback_default_policy` | String | No | Fallback policy: `recreate` or `hold` |

### Proactive Filter

Each `enable_proactive` and `disable_proactive` entry supports the same set of filter attributes:

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `workload_name` | String | No | Filter by workload name (substring match) |
| `namespaces` | List(String) | No | Namespaces to filter workloads |
| `workload_kinds` | List(String) | No | Workload kinds (e.g. `Deployment`, `StatefulSet`) |
| `autoscaling_policy_names` | List(String) | No | Filter by autoscaling policy names |
| `workload_state` | String | No | Filter by workload state |
| `optimization_states` | List(String) | No | Filter by optimization states |
| `disable_proactive_update` | Boolean | No | Filter by whether proactive update is disabled |
| `recommendation_policy_names` | List(String) | No | Filter by recommendation policy names |
| `runtime_languages` | List(String) | No | Filter by container runtime languages |
| `optimized` | Boolean | No | Filter by whether the workload is optimized |

## Import

This resource does not support import.
