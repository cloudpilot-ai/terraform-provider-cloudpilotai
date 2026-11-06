// Package api provides data models for CloudPilot AI API interactions including
// cluster information, cost summaries, and resource metrics.
package api

import (
	"fmt"
	"math"
	"regexp"
	"time"
)

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

type ClusterSetting struct {
	EnableNodeRepair           *bool    `json:"enableNodeRepair,omitempty"`
	EnableDiskMonitor          *bool    `json:"enableDiskMonitor,omitempty"`
	EnableNodePoolDecommission *bool    `json:"enableNodePoolDecommission,omitempty"`
	EnableWorkloadMinNonSpot   *bool    `json:"enableWorkloadMinNonSpot,omitempty"`
	MaintenanceEnabled         *bool    `json:"maintenanceEnabled,omitempty"`
	Discount                   *float64 `json:"discount,omitempty"`
	PreRunCommand              *string  `json:"preRunCommand,omitempty"`
	PostRunCommand             *string  `json:"postRunCommand,omitempty"`
	DisruptionCooldown         *string  `json:"disruptionCooldown,omitempty"`
	AWSSpotAllocationStrategy  *string  `json:"awsSpotAllocationStrategy,omitempty"`
}

type ClusterMaintenanceStatus struct {
	MaintenanceModeEnabled *bool `json:"maintenanceModeEnabled,omitempty"`
}

var clusterDisruptionCooldownPattern = regexp.MustCompile(`^([0-9]+(h|m|s))+$`)

// Validate rejects cluster settings that the server cannot apply.
func (s ClusterSetting) Validate() error {
	if s.DisruptionCooldown != nil {
		duration, err := time.ParseDuration(*s.DisruptionCooldown)
		if err != nil || duration < 0 || !clusterDisruptionCooldownPattern.MatchString(*s.DisruptionCooldown) {
			return fmt.Errorf("disruption_cooldown must be a non-negative duration using h, m, or s")
		}
	}
	if s.AWSSpotAllocationStrategy != nil && *s.AWSSpotAllocationStrategy != "lowest-price" && *s.AWSSpotAllocationStrategy != "price-capacity-optimized" {
		return fmt.Errorf("aws_spot_allocation_strategy must be lowest-price or price-capacity-optimized")
	}
	if s.Discount != nil && (*s.Discount <= 0 || *s.Discount > 1 || math.IsNaN(*s.Discount) || math.IsInf(*s.Discount, 0)) {
		return fmt.Errorf("discount must be in range (0, 1]")
	}
	return nil
}
