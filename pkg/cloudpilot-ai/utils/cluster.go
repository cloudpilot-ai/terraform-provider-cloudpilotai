// Package utils provides utilities for the cloudpilot-ai.
package utils

import (
	"crypto/md5"
	"fmt"

	"github.com/google/uuid"
	"k8s.io/klog/v2"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
)

const (
	CloudProviderAWS          = "aws"
	CloudProviderAlibabaCloud = "alibabacloud"
)

func GenerateClusterUID(para api.RegisterClusterRequest) string {
	var data string
	switch para.CloudProvider {
	case CloudProviderAWS:
		data = fmt.Sprintf("%s/%s/%s/%s",
			para.CloudProvider, para.EKS.ClusterName, para.EKS.Region, para.EKS.AccountID)
	case CloudProviderAlibabaCloud:
		data = fmt.Sprintf("%s/%s/%s/%s",
			para.CloudProvider, para.ClusterParams.ClusterName, para.ClusterParams.Region, para.ClusterParams.AccountID)
	default:
		klog.Errorf("Unsupported cloud provider: %s", para.CloudProvider)
		return ""
	}

	hash := md5.New()
	hash.Write([]byte(data))
	hashed := hash.Sum(nil)
	return uuid.NewSHA1(uuid.Nil, hashed).String()
}
