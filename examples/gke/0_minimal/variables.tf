variable "cloudpilot_api_endpoint" {
  type = string
}

variable "cloudpilot_api_key" {
  type      = string
  sensitive = true
}

variable "cluster_name" {
  type = string
}

variable "region" {
  type = string
}

variable "project_id" {
  type = string
}

variable "cluster_uid" {
  description = "Kubernetes cluster UID. For GKE, use the kube-system namespace UID."
  type        = string
}

variable "cluster_location" {
  type    = string
  default = null
}

variable "node_service_account" {
  type = string
}
