# Minimal GKE Example

This example registers a GKE cluster with CloudPilot AI, manages the GKE node autoscaler configuration directly through `cloudpilotai_gke_cluster`, and then enables the shared Workload Autoscaler resource.

Required inputs:

- `cluster_name`
- `region`
- `project_id`
- `cluster_uid`
- `node_service_account`
- `cloudpilot_api_key`

Optional inputs:

- `cluster_location` for zonal clusters
- `cloudpilot_api_endpoint` if you are targeting a non-default API endpoint

Use the Kubernetes cluster UID from the `kube-system` namespace for `cluster_uid`:

```bash
kubectl get ns kube-system -o jsonpath='{.metadata.uid}'
```
