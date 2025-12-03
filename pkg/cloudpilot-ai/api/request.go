package api

import (
	"crypto/md5"
	"fmt"

	"github.com/google/uuid"
	"k8s.io/klog/v2"
)

type RegisterClusterRequest struct {
	Demo          bool           `json:"demo"`
	AgentVersion  string         `json:"agentVersion"`
	CloudProvider string         `json:"cloudProvider"`
	GPUInstances  []string       `json:"gpuInstances"`
	Arch          []string       `json:"arch"`
	EKS           *EKSParams     `json:"eks"`
	ClusterParams *ClusterParams `json:"clusterParams"`
}

type ClusterParams struct {
	ClusterName    string `json:"clusterName"`
	ClusterVersion string `json:"clusterVersion"`
	Region         string `json:"region"`
	AccountID      string `json:"accountId"`
}

type EKSParams struct {
	ClusterName    string `json:"clusterName"`
	ClusterVersion string `json:"clusterVersion"`
	Region         string `json:"region"`
	AccountID      string `json:"accountId"`
}

type ResponseBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type RegisterClusterResponse struct {
	ClusterID string `json:"clusterId"`
}

const (
	CloudProviderAWS          = "aws"
	CloudProviderAlibabaCloud = "alibabacloud"
)

func GenerateClusterUID(cloudProvider, clusterName, region, accountID string) string {
	var data string
	switch cloudProvider {
	case CloudProviderAWS:
		data = fmt.Sprintf("%s/%s/%s/%s",
			cloudProvider, clusterName, region, accountID)
	case CloudProviderAlibabaCloud:
		data = fmt.Sprintf("%s/%s/%s/%s",
			cloudProvider, clusterName, region, accountID)
	default:
		klog.Errorf("Unsupported cloud provider: %s", cloudProvider)
		return ""
	}

	hash := md5.New()
	hash.Write([]byte(data))
	hashed := hash.Sum(nil)
	return uuid.NewSHA1(uuid.Nil, hashed).String()
}
