# Example: Importing Existing Resources

This example demonstrates how to import existing CloudPilot AI-managed resources into Terraform state, allowing you to manage them with Terraform going forward.

## Overview

If you have already set up CloudPilot AI for your EKS cluster (e.g. through the console or API), you can import those resources into Terraform. After importing, Terraform will track and manage the existing configuration — no need to recreate anything.

Import only requires a valid CloudPilot AI API key. **No AWS credentials, profile, or kubeconfig are needed** — all configuration is fetched from the CloudPilot AI API.

## Prerequisites

- **[Terraform](https://developer.hashicorp.com/terraform/install)** >= 1.5 (required for `import` blocks and `terraform plan -generate-config-out`)
- **CloudPilot AI API key** — See [CloudPilot AI API Key Documentation](https://docs.cloudpilot.ai/guide/getting_started/get_apikeys)
- **Cluster ID** — Find this in the [CloudPilot AI Console](https://console.cloudpilot.ai) under your cluster settings

## Usage

### Step 1: Configure Variables

```bash
cp terraform.tfvars.example terraform.tfvars
```

Edit `terraform.tfvars` with your CloudPilot AI API key.

### Step 2: Set the Cluster ID

Edit `main.tf` and replace `<CLUSTER_ID>` with your actual cluster ID in both `import` blocks:

```hcl
import {
  to = cloudpilotai_eks_cluster.imported
  id = "your-cluster-id-here"
}

import {
  to = cloudpilotai_workload_autoscaler.imported
  id = "your-cluster-id-here"
}
```

You can find the cluster ID in the [CloudPilot AI Console](https://console.cloudpilot.ai) under your cluster settings.

### Step 3: Initialize Terraform

```bash
terraform init
```

### Step 4: Generate Configuration

Run `terraform plan` with `-generate-config-out` to auto-generate Terraform configuration from the remote state:

```bash
terraform plan -generate-config-out=generated.tf
```

This creates a `generated.tf` file containing the full resource configuration as it currently exists in CloudPilot AI. **Review this file carefully before proceeding.**

### Step 5: Apply (Import into State)

```bash
terraform apply
```

This imports the existing resources into your Terraform state without making any changes to the actual infrastructure.

### Step 6: Verify

```bash
terraform state show cloudpilotai_eks_cluster.imported
terraform state show cloudpilotai_workload_autoscaler.imported
terraform plan   # Should show no changes
```

## After Importing

Once the import is complete:

1. **Merge configuration** — Move the generated configuration from `generated.tf` into your main `.tf` files
2. **Remove import blocks** — Delete or comment out the `import` blocks in `main.tf` (they are only needed once)
3. **Add operational parameters** — If you plan to manage the cluster with Terraform (not just track it), add parameters like `aws_profile` or `kubeconfig` to the resource configuration for future CRUD operations
4. **Verify** — Run `terraform plan` to confirm there are no unintended changes

## Alternative: CLI Import

You can also import resources one at a time using the Terraform CLI:

```bash
terraform import cloudpilotai_eks_cluster.imported <CLUSTER_ID>
terraform import cloudpilotai_workload_autoscaler.imported <CLUSTER_ID>
```

> **Note:** The CLI approach requires you to write the resource configuration manually before importing. The `import` block approach with `-generate-config-out` is recommended as it auto-generates the configuration for you.
