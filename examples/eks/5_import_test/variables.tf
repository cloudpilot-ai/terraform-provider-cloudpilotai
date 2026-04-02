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
