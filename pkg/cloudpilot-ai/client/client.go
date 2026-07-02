// Package client provides CloudPilot AI client functionality for making HTTP requests
// to the CloudPilot AI API with authentication capabilities.
package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/hashicorp/go-retryablehttp"
	"k8s.io/klog/v2"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
)

func validateProviderAwareScriptRequest(provider, clusterName string, requireClusterName bool) error {
	if provider == "" {
		return fmt.Errorf("provider is required for provider-aware script request")
	}
	if requireClusterName && clusterName == "" {
		return fmt.Errorf("cluster_name is required for provider-aware agent script request")
	}

	return nil
}

type Interface interface {
	// cluster
	GetCluster(clusterID string) (*api.ClusterCostsSummary, error)
	GetClusterSetting(clusterID string) (*api.ClusterSetting, error)
	UpdateClusterSetting(clusterID string, setting *api.ClusterSetting) error
	UpdateClusterMaintenanceStatus(clusterID string, status *api.ClusterMaintenanceStatus) error
	DeleteCluster(clusterID string) error

	// sh
	GetAgentSH(provider, clusterName string, disableWorkloadUploading bool) (string, error)
	GetClusterRestoreSH(clusterID, provider string) (string, error)
	GetClusterUpgradeSH(clusterID, provider string) (string, error)
	GetRebalanceSH(clusterID, provider string) (string, error)
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

	// workload autoscaler
	GetWorkloadAutoscalerSH() (string, error)
	GetWAConfiguration(clusterID string) (*api.WAConfiguration, error)
	UpdateWAConfiguration(clusterID string, config *api.WAConfiguration) error

	// recommendation policy
	ListRecommendationPolicies(clusterID string) ([]api.RecommendationPolicyResource, error)
	GetRecommendationPolicy(clusterID, name string) (*api.RecommendationPolicyResource, error)
	ApplyRecommendationPolicy(clusterID string, rp *api.RecommendationPolicyResource) error
	DeleteRecommendationPolicy(clusterID, name string) error

	// autoscaling policy
	ListAutoscalingPolicies(clusterID string) ([]api.AutoscalingPolicyResource, error)
	GetAutoscalingPolicy(clusterID, name string) (*api.AutoscalingPolicyResource, error)
	ApplyAutoscalingPolicy(clusterID string, ap *api.AutoscalingPolicyResource) error
	DeleteAutoscalingPolicy(clusterID, name string) error

	// workload proactive update
	UpdateWorkloadProactiveUpdate(clusterID string, req *api.WAProactiveUpdateRequest) error
}

type Client struct {
	Endpoint string
	APIKEY   string

	mu sync.Mutex
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

func (c *Client) GetClusterSetting(clusterID string) (*api.ClusterSetting, error) {
	url := fmt.Sprintf("%s/api/v1/clusters/%s/setting", c.Endpoint, clusterID)
	out, err := doJSON[api.ClusterSetting](c, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateClusterSetting(clusterID string, setting *api.ClusterSetting) error {
	url := fmt.Sprintf("%s/api/v1/clusters/%s/setting", c.Endpoint, clusterID)
	return doJSONNoData(c, http.MethodPost, url, setting)
}

func (c *Client) UpdateClusterMaintenanceStatus(clusterID string, status *api.ClusterMaintenanceStatus) error {
	url := fmt.Sprintf("%s/api/v1/clusters/%s/maintenance/status", c.Endpoint, clusterID)
	return doJSONNoData(c, http.MethodPost, url, status)
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

func (c *Client) GetAgentSH(provider, clusterName string, disableWorkloadUploading bool) (string, error) {
	if err := validateProviderAwareScriptRequest(provider, clusterName, true); err != nil {
		return "", err
	}

	values := url.Values{}
	values.Set("disable_workload_uploading", strconv.FormatBool(disableWorkloadUploading))
	values.Set("provider", provider)
	values.Set("cluster_name", clusterName)

	reqURL := fmt.Sprintf("%s/api/v1/agent/sh", c.Endpoint)
	if encoded := values.Encode(); encoded != "" {
		reqURL = fmt.Sprintf("%s?%s", reqURL, encoded)
	}

	sh, err := doJSON[string](c, http.MethodGet, reqURL, nil)
	if err != nil {
		klog.Errorf("GetAgentSH failed: %v", err)
		return "", err
	}

	return sh, nil
}

func (c *Client) GetClusterRestoreSH(clusterID, provider string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/clusters/%s/restore/sh?provider=%s", c.Endpoint, clusterID, url.QueryEscape(provider))
	out, err := doJSON[string](c, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	return out, nil
}

func (c *Client) GetClusterUpgradeSH(clusterID, provider string) (string, error) {
	if err := validateProviderAwareScriptRequest(provider, "", false); err != nil {
		return "", err
	}

	reqURL := fmt.Sprintf("%s/api/v1/clusters/%s/upgrade/sh", c.Endpoint, clusterID)
	reqURL = fmt.Sprintf("%s?%s", reqURL, url.Values{"provider": []string{provider}}.Encode())

	out, err := doJSON[string](c, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}

	return out, nil
}

func (c *Client) GetClusterUninstallSH(clusterID, clusterName, provider, region string) (string, error) {
	reqURL := fmt.Sprintf("%s/api/v1/clusters/%s/uninstall/sh?cluster_name=%s&provider=%s&region=%s",
		c.Endpoint, clusterID,
		url.QueryEscape(clusterName), url.QueryEscape(provider), url.QueryEscape(region))
	out, err := doJSON[string](c, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}

	return out, nil
}

func (c *Client) GetRebalanceSH(clusterID, provider string) (string, error) {
	if err := validateProviderAwareScriptRequest(provider, "", false); err != nil {
		return "", err
	}

	reqURL := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/sh", c.Endpoint, clusterID)
	reqURL = fmt.Sprintf("%s?%s", reqURL, url.Values{"provider": []string{provider}}.Encode())

	out, err := doJSON[string](c, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}

	return out, nil
}

func (c *Client) GetRebalanceConfiguration(clusterID string) (*api.RebalanceConfig, error) {
	url := fmt.Sprintf("%s/api/v1/rebalance/clusters/%s/configuration", c.Endpoint, clusterID)
	resp, err := c.requestWithHeaders(http.MethodGet, url, nil, map[string]string{
		"User-Agent": browserLikeUserAgent,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var stdResp api.ResponseBody
	if err := json.NewDecoder(resp.Body).Decode(&stdResp); err != nil {
		if resp.StatusCode != http.StatusOK {
			klog.Errorf("Server error (non-JSON), method(%s) url(%s): %s", http.MethodGet, url, resp.Status)
			return nil, fmt.Errorf("server error: %s", resp.Status)
		}
		klog.Errorf("Decode response body failed, method(%s) url(%s), err: %v", http.MethodGet, url, err)
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, ErrNotFound
		}
		msg := stdResp.Message
		if msg == "" {
			msg = resp.Status
		}
		klog.Errorf("Server error, method(%s) url(%s): %s", http.MethodGet, url, msg)
		return nil, fmt.Errorf("server error: %s", msg)
	}

	dataBytes, err := json.Marshal(stdResp.Data)
	if err != nil {
		klog.Errorf("Marshal stdResp.Data failed, method(%s) url(%s): %v", http.MethodGet, url, err)
		return nil, err
	}
	var out api.RebalanceConfig
	if len(dataBytes) > 0 && string(dataBytes) != "null" {
		if err := json.Unmarshal(dataBytes, &out); err != nil {
			klog.Errorf("Unmarshal to target type failed, method(%s) url(%s): %v", http.MethodGet, url, err)
			return nil, err
		}
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

// Workload Autoscaler

func (c *Client) GetWorkloadAutoscalerSH() (string, error) {
	url := fmt.Sprintf("%s/api/v1/workloadautoscaler/sh", c.Endpoint)
	sh, err := doJSON[string](c, http.MethodGet, url, nil)
	if err != nil {
		klog.Errorf("GetWorkloadAutoscalerSH failed: %v", err)
		return "", err
	}
	return sh, nil
}

func (c *Client) GetWAConfiguration(clusterID string) (*api.WAConfiguration, error) {
	url := fmt.Sprintf("%s/api/v1/workloadautoscaler/clusters/%s/configuration", c.Endpoint, clusterID)
	out, err := doJSON[api.WAConfiguration](c, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateWAConfiguration(clusterID string, config *api.WAConfiguration) error {
	url := fmt.Sprintf("%s/api/v1/workloadautoscaler/clusters/%s/configuration", c.Endpoint, clusterID)
	return doJSONNoData(c, http.MethodPost, url, config)
}

// Recommendation Policy

func (c *Client) ListRecommendationPolicies(clusterID string) ([]api.RecommendationPolicyResource, error) {
	url := fmt.Sprintf("%s/api/v1/workloadautoscaler/clusters/%s/recommendationpolicies", c.Endpoint, clusterID)
	out, err := doJSON[[]api.RecommendationPolicyResource](c, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetRecommendationPolicy(clusterID, name string) (*api.RecommendationPolicyResource, error) {
	url := fmt.Sprintf("%s/api/v1/workloadautoscaler/clusters/%s/recommendationpolicies/%s", c.Endpoint, clusterID, name)
	out, err := doJSON[api.RecommendationPolicyResource](c, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ApplyRecommendationPolicy(clusterID string, rp *api.RecommendationPolicyResource) error {
	url := fmt.Sprintf("%s/api/v1/workloadautoscaler/clusters/%s/recommendationpolicies/%s", c.Endpoint, clusterID, rp.Name)
	return doJSONNoData(c, http.MethodPost, url, rp)
}

func (c *Client) DeleteRecommendationPolicy(clusterID, name string) error {
	url := fmt.Sprintf("%s/api/v1/workloadautoscaler/clusters/%s/recommendationpolicies/%s", c.Endpoint, clusterID, name)
	return doJSONNoData(c, http.MethodDelete, url, nil)
}

// Autoscaling Policy

func (c *Client) ListAutoscalingPolicies(clusterID string) ([]api.AutoscalingPolicyResource, error) {
	url := fmt.Sprintf("%s/api/v1/workloadautoscaler/clusters/%s/autoscalingpolicies", c.Endpoint, clusterID)
	out, err := doJSON[[]api.AutoscalingPolicyResource](c, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetAutoscalingPolicy(clusterID, name string) (*api.AutoscalingPolicyResource, error) {
	url := fmt.Sprintf("%s/api/v1/workloadautoscaler/clusters/%s/autoscalingpolicies/%s", c.Endpoint, clusterID, name)
	out, err := doJSON[api.AutoscalingPolicyResource](c, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ApplyAutoscalingPolicy(clusterID string, ap *api.AutoscalingPolicyResource) error {
	url := fmt.Sprintf("%s/api/v1/workloadautoscaler/clusters/%s/autoscalingpolicies/%s", c.Endpoint, clusterID, ap.Name)
	return doJSONNoData(c, http.MethodPost, url, ap)
}

func (c *Client) DeleteAutoscalingPolicy(clusterID, name string) error {
	url := fmt.Sprintf("%s/api/v1/workloadautoscaler/clusters/%s/autoscalingpolicies/%s", c.Endpoint, clusterID, name)
	return doJSONNoData(c, http.MethodDelete, url, nil)
}

// Workload Proactive Update

func (c *Client) UpdateWorkloadProactiveUpdate(clusterID string, req *api.WAProactiveUpdateRequest) error {
	url := fmt.Sprintf("%s/api/v1/workloadautoscaler/clusters/%s/workloads/configurations/proactiveupdate", c.Endpoint, clusterID)
	return doJSONNoData(c, http.MethodPost, url, req)
}
