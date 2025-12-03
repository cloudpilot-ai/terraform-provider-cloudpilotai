

# CloudPilot AI Coding Agent Instructions

## Project Overview
Terraform provider for managing AWS EKS clusters and workloads using CloudPilot AI for automation and cost optimization. Modular architecture for maintainability and extensibility.

## Architecture & Key Files
- **Entrypoint:** `main.go` (plugin startup)
- **Provider registration:** `pkg/provider/provider.go` (provider config, resource registration)
- **Resource logic:** `pkg/resources/nodeautoscale/eks/cluster.go` (main EKS resource, agent install/sync)
- **API client:** `pkg/cloudpilot-ai/client/` (all CloudPilot API calls via `Client` struct; never call HTTP directly)
- **API models:** `pkg/cloudpilot-ai/api/` (cluster, workload, node pool, rebalance models)
- **Utilities:** `pkg/cloudpilot-ai/utils/` (cluster UID, AWS CLI integration)
- **Custom field helpers:** `third_party/cloudflare/customfield/` (nested object types for complex Terraform schemas)

## Developer Workflows
- **Build:** `make build` or `go build -v ./...`
- **Install provider locally:** `make install`
- **Lint:** `make lint` (uses `golangci-lint`)
- **Format:** `make fmt`
- **Unit tests:** `make test`
- **Acceptance tests:** `make testacc` (set `TF_ACC=1`, provide AWS credentials & CloudPilot AI API key)
- **Codegen:** `make generate` (if available)

## Project-Specific Patterns
- **Provider configuration:**
  - API key: set via provider block (`api_key`), file (`api_key_profile`)
  - API endpoint: use `api_endpoint` field (examples may show `api_url` but schema uses `api_endpoint`)
  - See `pkg/provider/provider.go` for configuration precedence and validation
- **Resource models:**
  - Use explicit `tfsdk:"field_name"` struct tags for Terraform schema mapping
  - Node pools/classes: support custom JSON via `origin_nodeclass_json`/`origin_nodepool_json` fields
  - Use `customfield.NewNestedObjectListType[T](ctx)` for complex nested objects (see `third_party/cloudflare/customfield/`)
- **Component install:**
  - Agent/rebalance components installed via shell scripts from API (`GetAgentSH()`, `GetRebalanceSH()`)
  - Scripts executed with `KUBECONFIG` environment variable set pointing to cluster config
  - Cluster UID: generated as UUID from MD5 hash of `provider/cluster/region/account` (see `api.GenerateClusterUID()`)
- **AWS integration:**
  - Use `pkg/cloudpilot-ai/utils/aws/cluster.go` `UpdateKubeconfig()` to update kubeconfig via `aws eks update-kubeconfig`
  - Requires AWS CLI configured with appropriate EKS permissions
- **Error handling & logging:**
  - Use `k8s.io/klog/v2` for structured logs; always log API errors before returning
  - Use `tflog.Trace()` for Terraform-specific logs
  - Client errors wrapped with context in `pkg/cloudpilot-ai/client/err.go`

## Integration Points
- **CloudPilot AI API:** All calls via `Client` in `pkg/cloudpilot-ai/client/` (never call HTTP directly)
- **AWS CLI:** For EKS kubeconfig management
- **kubectl:** For cluster operations/component install
- **Terraform Plugin Framework:** v1.15.1

## Configuration & Examples
- See `examples/nodeautoscale/eks/` for real-world configs:
  - `0_details/`: Full-featured EKS cluster with all options (workload templates, nodeclasses, complete configuration)
  - `1_read-only_access/`: Agent-only install (monitoring, no optimization)
  - `2_basic_rebalance/`: Basic rebalance enabled (simple cost optimization)
  - `3_nodeclass_nodepool_rebalance/`: Custom nodeclass/nodepool (advanced node management)
- Key resource options:
  - `only_install_agent`, `enable_upgrade_agent`, `disable_workload_uploading`, `enable_rebalance`, `restore_node_number`
  - Nodeclass/nodepool/workload templates support custom resource allocation and filtering
  - `restore_node_number`: Required for cluster restoration after uninstall - use `kubectl get node --no-headers=true | wc -l` to determine current count

## Naming & Patterns
- Resource models: `EC2NodeClassModel`, `EC2NodePoolModel`, `WorkloadModel`
- Rebalance types: `Rebalance*` prefix
- API methods: `GetX()`, `UpdateX()`, `ApplyX()`, `DeleteX()`
- Customization: Karpenter node class/pool JSON overrides, workload options (`rebalance_able`, `spot_friendly`, `min_non_spot_replicas`)

---

If any section is unclear or missing, please provide feedback for further refinement.
