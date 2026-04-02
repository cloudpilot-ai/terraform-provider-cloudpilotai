#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

echo "==> Building provider binary..."
go build -o "${REPO_ROOT}/terraform-provider-cloudpilotai"

echo "==> Generating documentation with tfplugindocs..."
go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate \
  -provider-name cloudpilotai \
  -rendered-provider-name "CloudPilot AI" \
  -provider-dir "${REPO_ROOT}"

echo "==> Documentation generated successfully in docs/"

rm -f "${REPO_ROOT}/terraform-provider-cloudpilotai"
