// Package utils provides utilities for the cloudpilot-ai.
package utils

import "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"

const (
	CloudProviderAWS          = api.CloudProviderAWS
	CloudProviderAlibabaCloud = api.CloudProviderAlibabaCloud
	CloudProviderGCP          = api.CloudProviderGCP
)

func GenerateClusterUID(para api.RegisterClusterRequest) string {
	switch para.CloudProvider {
	case CloudProviderAWS:
		return api.GenerateClusterUID(para.CloudProvider, para.EKS.ClusterName, para.EKS.Region, para.EKS.AccountID)
	case CloudProviderAlibabaCloud:
		return api.GenerateClusterUID(para.CloudProvider, para.ClusterParams.ClusterName, para.ClusterParams.Region, para.ClusterParams.AccountID)
	case CloudProviderGCP:
		return api.GenerateClusterUID(para.CloudProvider, para.ClusterParams.ClusterName, para.ClusterParams.Region, para.ClusterParams.AccountID)
	default:
		return ""
	}
}
