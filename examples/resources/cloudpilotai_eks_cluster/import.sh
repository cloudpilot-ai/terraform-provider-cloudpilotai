# Import using cluster ID (only requires a valid CloudPilot AI API key; no AWS credentials needed)
terraform import cloudpilotai_eks_cluster.example <cluster-id>

# Or use an import block in your .tf file to auto-generate configuration:
#
#   import {
#     to = cloudpilotai_eks_cluster.example
#     id = "<cluster-id>"
#   }
#
#   terraform plan -generate-config-out=generated.tf
