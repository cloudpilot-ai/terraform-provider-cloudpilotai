terraform {
  required_version = ">= 1.5"

  required_providers {
    cloudpilotai = {
      source  = "cloudpilot-ai/cloudpilotai"
      version = ">= 0.2"
    }
  }
}

provider "cloudpilotai" {
  api_endpoint = var.cloudpilot_api_endpoint
  api_key      = var.cloudpilot_api_key
}

# ──────────────────────────────────────────────────────────────────
# Import blocks
# ──────────────────────────────────────────────────────────────────
# These blocks tell Terraform to import existing remote resources
# into your local state. Replace <CLUSTER_ID> below with your
# actual cluster ID (find it in the CloudPilot AI console).
#
# After importing, run:
#   terraform plan -generate-config-out=generated.tf
# to auto-generate the full resource configuration.
# ──────────────────────────────────────────────────────────────────

import {
  to = cloudpilotai_eks_cluster.imported
  id = "<CLUSTER_ID>"
}

import {
  to = cloudpilotai_workload_autoscaler.imported
  id = "<CLUSTER_ID>"
}
