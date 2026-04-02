terraform {
  required_version = ">= 1.0"

  required_providers {
    cloudpilotai = {
      source  = "cloudpilot-ai/cloudpilotai"
      version = ">= 0.2"
    }
  }
}

provider "cloudpilotai" {
  api_key = var.cloudpilotai_api_key
}
