---
page_title: "CloudPilot AI Provider"
subcategory: ""
description: |-
  The CloudPilot AI provider enables Terraform to manage EKS clusters and workload autoscaling with CloudPilot AI's cost optimization platform.
---

# CloudPilot AI Provider

The CloudPilot AI provider enables you to manage Amazon EKS clusters and workloads through [CloudPilot AI](https://cloudpilot.ai/)'s automation and cost optimization platform.

## Features

- Provision and manage EKS clusters with CloudPilot AI integration
- Automated agent and rebalance component installation
- Node pool and node class management (including custom Karpenter JSON)
- Workload cost optimization (rebalance, spot-friendly, min non-spot replicas)
- Workload Autoscaler with recommendation and autoscaling policies
- Read-only data sources for querying existing cluster and autoscaler state

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.0
- [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) configured with EKS permissions
- [Kubectl](https://kubernetes.io/docs/tasks/tools/) for cluster operations
- A CloudPilot AI API key — see [Getting API Keys](https://docs.cloudpilot.ai/guide/getting_started/get_apikeys)

## Example Usage

```terraform
provider "cloudpilotai" {
  api_key = var.cloudpilotai_api_key
}
```

## Authentication

The provider requires a CloudPilot AI API key. You can supply it in two ways:

- **`api_key`** — Pass the key directly (use a Terraform variable to avoid hardcoding).
- **`api_key_profile`** — Path to a file containing the API key.

## Schema

### Optional

- `api_key` (String, Sensitive) — API key for the CloudPilot AI API.
- `api_key_profile` (String) — Path to a file containing the API key.
- `api_endpoint` (String) — Custom API endpoint. Defaults to `https://api.cloudpilot.ai`.
