## Provider

variable "cloudpilot_api_key" {
  description = "CloudPilot AI API key. Obtain from https://console.cloudpilot.ai"
  type        = string
  sensitive   = true
}

variable "cloudpilot_api_endpoint" {
  description = "CloudPilot AI API endpoint URL."
  type        = string
  default     = "https://api.cloudpilot.ai"
}

## EKS Cluster

variable "cluster_name" {
  description = "Name of the EKS cluster to be managed."
  type        = string
}

variable "region" {
  description = "AWS region where the EKS cluster is located."
  type        = string
}

variable "restore_node_number" {
  description = "Node count for cluster restoration when uninstalling CloudPilot AI. Check current count with: kubectl get node --no-headers=true | wc -l"
  type        = number
  default     = 2
}

## Node Autoscaler Optimization
## Modify these in terraform.tfvars to enable optimization, then run `terraform apply`.

variable "only_install_agent" {
  description = "Only install the CloudPilot AI agent without additional optimization."
  type        = bool
  default     = true
}

variable "enable_rebalance" {
  description = "Enable automatic workload rebalancing across node pools."
  type        = bool
  default     = false
}

