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

variable "aws_profile" {
  description = "AWS CLI named profile for STS and EKS kubeconfig operations. Empty uses the default profile or environment credentials."
  type        = string
  default     = ""
}

variable "custom_node_role" {
  description = "IAM role name for EC2 worker nodes; added to CloudPilot PassNodeIAMRole during rebalance install. Empty uses the CloudPilot default role."
  type        = string
  default     = ""
}

variable "skip_restore" {
  description = "If true, skip restoring original node groups before destroy. When false and restore_node_number > 0, restores nodes when CloudPilot-managed nodes exist."
  type        = bool
  default     = true
}

variable "restore_node_number" {
  description = "Number of nodes to restore from original node groups on destroy. Set to 0 to leave the cluster in its optimized state."
  type        = number
  default     = 0
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

