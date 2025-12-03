// Package client provides CloudPilot AI client functionality for making HTTP requests
// to the CloudPilot AI API with authentication capabilities.
package client

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/hashicorp/go-retryablehttp"
	"k8s.io/klog/v2"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
)

type Interface interface {
	// cluster
	GetCluster(clusterID string) (*api.ClusterCostsSummary, error)
	DeleteCluster(clusterID string) error

	// sh
	GetAgentSH(disableWorkloadUploading bool) (string, error)
	GetClusterRestoreSH(clusterID string) (string, error)
	GetClusterUpgradeSH(clusterID string) (string, error)
	GetRebalanceSH(clusterID string) (string, error)
	GetClusterUninstallSH(clusterID, clusterName, provider, region string) (string, error)

	// rebalance configuration
	GetRebalanceConfiguration(clusterID string) (*api.RebalanceConfig, error)
	UpdateRebalanceConfiguration(clusterID string, config *api.RebalanceConfig) error

	// rebalance workload configuration
	GetWorkloadRebalanceConfiguration(clusterID string) (*api.ClusterWorkloadSpec, error)
	UpdateWorkloadRebalanceConfiguration(clusterID string, workload api.Workload) error

	// rebalance nodepool configuration
	GetNodePool(clusterID, nodePoolName string) (*api.RebalanceNodePool, error)
	ListNodePools(clusterID string) (api.RebalanceNodePoolList, error)
	ApplyNodePool(clusterID string, rebalanceNodePool api.RebalanceNodePool) error
	DeleteNodePool(clusterID, nodePoolName string) error

	// rebalance nodeclass configuration
	GetNodeClass(clusterID, nodeClassName string) (*api.RebalanceNodeClass, error)
	ListNodeClasses(clusterID string) (api.RebalanceNodeClassList, error)
	ApplyNodeClass(clusterID string, rebalanceNodeClass api.RebalanceNodeClass) error
	DeleteNodeClass(clusterID string, nodeClassName string) error
}

type Client struct {
	Endpoint string
	APIKEY   string

	rc *retryablehttp.Client
}

func NewCloudPilotClient(endpoint, apikey string) *Client {
	return &Client{
		Endpoint: endpoint,
		APIKEY:   apikey,
	}
}

func (c *Client) GetCluster(clusterID string) (*api.ClusterCostsSummary, error) {
	url := fmt.Sprintf("%s/api/v1/costs/clusters/%s/summary", c.Endpoint, clusterID)
	out, err := doJSON[api.ClusterCostsSummary](c, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return &out, err
}

func (c *Client) DeleteCluster(clusterID string) error {
	url := fmt.Sprintf("%s/api/v1/clusters/%s", c.Endpoint, clusterID)
	err := doJSONNoData(c, http.MethodDelete, url, nil)
	if err != nil {
		klog.Errorf("DeleteCluster failed: %v", err)
		return err
	}

	return nil
}

func (c *Client) GetAgentSH(disableWorkloadUploading bool) (string, error) {
	url := fmt.Sprintf("%s/api/v1/agent/sh?disable_workload_uploading=%s", c.Endpoint, strconv.FormatBool(disableWorkloadUploading))
	sh, err := doJSON[string](c, http.MethodGet, url, nil)
	if err != nil {
		klog.Errorf("GetAgentSH failed: %v", err)
		return "", err
	}

	return sh, nil
}

func (c *Client) GetClusterRestoreSH(clusterID string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/clusters/%s/restore/sh", c.Endpoint, clusterID)
	out, err := doJSON[string](c, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	return out, nil
}

func (c *Client) GetClusterUpgradeSH(clusterID string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/clusters/%s/upgrade/sh", c.Endpoint, clusterID)
	out, err := doJSON[string](c, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	return out, nil
}

func (c *Client) GetClusterUninstallSH(clusterID, clusterName, provider, region string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/clusters/%s/uninstall/sh?cluster_name=%s&provider=%s&region=%s", c.Endpoint, clusterID, clusterName, provider, region)
	out, err := doJSON[string](c, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	return out, nil
}

func (c *Client) GetRebalanceSH(clusterID string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/sh", c.Endpoint, clusterID)
	out, err := doJSON[string](c, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	return out, nil
}

func (c *Client) GetRebalanceConfiguration(clusterID string) (*api.RebalanceConfig, error) {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/configuration", c.Endpoint, clusterID)
	out, err := doJSON[api.RebalanceConfig](c, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *Client) UpdateRebalanceConfiguration(clusterID string, config *api.RebalanceConfig) error {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/configuration", c.Endpoint, clusterID)
	return doJSONNoData(c, http.MethodPost, url, config)
}

func (c *Client) GetWorkloadRebalanceConfiguration(clusterID string) (*api.ClusterWorkloadSpec, error) {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/workloads/configuration", c.Endpoint, clusterID)
	out, err := doJSON[api.ClusterWorkloadSpec](c, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *Client) UpdateWorkloadRebalanceConfiguration(clusterID string, workload api.Workload) error {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/workloads/configuration", c.Endpoint, clusterID)
	return doJSONNoData(c, http.MethodPost, url, workload)
}

func (c *Client) GetNodePool(clusterID, nodePoolName string) (*api.RebalanceNodePool, error) {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/nodepools/%s", c.Endpoint, clusterID, nodePoolName)
	out, err := doJSON[api.RebalanceNodePool](c, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *Client) ListNodePools(clusterID string) (api.RebalanceNodePoolList, error) {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/nodepools", c.Endpoint, clusterID)
	out, err := doJSON[api.RebalanceNodePoolList](c, http.MethodGet, url, nil)
	if err != nil {
		return api.RebalanceNodePoolList{}, err
	}

	return out, nil
}

func (c *Client) ApplyNodePool(clusterID string, rebalanceNodePool api.RebalanceNodePool) error {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/nodepools", c.Endpoint, clusterID)
	return doJSONNoData(c, http.MethodPost, url, rebalanceNodePool)
}

func (c *Client) DeleteNodePool(clusterID, nodePoolName string) error {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/nodepools/%s", c.Endpoint, clusterID, nodePoolName)
	return doJSONNoData(c, http.MethodDelete, url, nil)
}

func (c *Client) GetNodeClass(clusterID, nodeClassName string) (*api.RebalanceNodeClass, error) {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/nodeclasses/%s", c.Endpoint, clusterID, nodeClassName)
	out, err := doJSON[api.RebalanceNodeClass](c, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *Client) ListNodeClasses(clusterID string) (api.RebalanceNodeClassList, error) {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/nodeclasses", c.Endpoint, clusterID)
	out, err := doJSON[api.RebalanceNodeClassList](c, http.MethodGet, url, nil)
	if err != nil {
		return api.RebalanceNodeClassList{}, err
	}

	return out, nil
}

func (c *Client) ApplyNodeClass(clusterID string, rebalanceNodeClass api.RebalanceNodeClass) error {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/nodeclasses", c.Endpoint, clusterID)
	return doJSONNoData(c, http.MethodPost, url, rebalanceNodeClass)
}

func (c *Client) DeleteNodeClass(clusterID string, nodeClassName string) error {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/nodeclasses/%s", c.Endpoint, clusterID, nodeClassName)
	return doJSONNoData(c, http.MethodDelete, url, nil)
}
