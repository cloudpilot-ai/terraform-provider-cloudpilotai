// Package api provides data models for CloudPilot AI API interactions including
// cluster information, cost summaries, and resource metrics.
package api

import "time"

type ClusterStatus string

const (
	ClusterStatusOnline  ClusterStatus = "online"
	ClusterStatusOffline ClusterStatus = "offline"
	StatusDemoCluster    ClusterStatus = "demo"
)

type Currency string

type CurrencySymbol string

type ClusterInfo struct {
	ClusterName   string `json:"clusterName"`
	CloudProvider string `json:"cloudProvider"`
	ClusterID     string `json:"clusterID"`

	RebalanceEnable bool           `json:"rebalanceEnable"`
	Version         string         `json:"version"`
	Region          string         `json:"region"`
	Status          ClusterStatus  `json:"status"`
	Currency        Currency       `json:"currency"`
	Symbol          CurrencySymbol `json:"symbol"`
}

type ClusterCostsSummary struct {
	ClusterInfo

	NeedUpgrade            bool   `json:"needUpgrade"`
	AgentVersion           string `json:"agentVersion"`
	OnboardManifestVersion string `json:"onboardManifestVersion"`

	NodesNumber         int `json:"nodesNumber"`
	OnDemandNodesNumber int `json:"onDemandNodesNumber"`
	SpotNodesNumber     int `json:"spotNodesNumber"`

	InitialMonthlyCost          float64 `json:"initialMonthlyCost"`
	InitialOnDemandMonthlyCost  float64 `json:"initialOnDemandMonthlyCost"`
	InitialSpotMonthlyCost      float64 `json:"initialSpotMonthlyCost"`
	InitialOptimizedMonthlyCost float64 `json:"initialOptimizedMonthlyCost"`

	MonthlyCost         float64 `json:"monthlyCost"`
	OnDemandMonthlyCost float64 `json:"onDemandMonthlyCost"`
	SpotMonthlyCost     float64 `json:"spotMonthlyCost"`

	EstimateMonthlySaving float64 `json:"estimateMonthlySaving"`

	ClusterUsedResource        map[string]float64 `json:"clusterUsedResource"`
	ClusterRequestResource     map[string]float64 `json:"clusterRequestResource"`
	ClusterAllocatableResource map[string]float64 `json:"clusterAllocatableResource"`
	ClusterProvisionedResource map[string]float64 `json:"clusterProvisionedResource"`

	ZoneNodes               map[string]int `json:"zoneNodes"`
	InstanceTypeFamilyNodes map[string]int `json:"instanceTypeFamilyNodes"`
	PodStatus               map[string]int `json:"podStatus"`

	JoinTime time.Time `json:"joinTime"`
}
