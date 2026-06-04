package api

import (
	"fmt"
	"time"
)

// NormalizeDuration converts Go duration strings like "1m0s", "24h0m0s" to
// their shortest form "1m", "24h" so Terraform won't show false diffs.
func NormalizeDuration(s string) string {
	d, err := time.ParseDuration(s)
	if err != nil {
		return s
	}

	if d == 0 {
		return "0s"
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	result := ""
	if hours > 0 {
		result += fmt.Sprintf("%dh", hours)
	}
	if minutes > 0 {
		result += fmt.Sprintf("%dm", minutes)
	}
	if seconds > 0 {
		result += fmt.Sprintf("%ds", seconds)
	}
	if result == "" {
		return "0s"
	}
	return result
}

// WAConfiguration represents the cluster-level Workload Autoscaler configuration.
type WAConfiguration struct {
	AllowWorkloadAutoscaler     *bool   `json:"allowWorkloadAutoscaler,omitempty"`
	EnableWorkloadAutoscaler    *bool   `json:"enableWorkloadAutoscaler,omitempty"`
	WorkloadAutoscalerInstalled *bool   `json:"workloadAutoscalerInstalled,omitempty"`
	WorkloadAutoscalerVersion   *string `json:"workloadAutoscalerVersion,omitempty"`

	EnableNewWorkloadsProactiveUpdate        *bool   `json:"enableNewWorkloadsProactiveUpdate,omitempty"`
	LimiterQuotaPerWindow                    *int    `json:"limiterQuotaPerWindow,omitempty"`
	LimiterBurst                             *int    `json:"limiterBurst,omitempty"`
	LimiterWindowSeconds                     *int    `json:"limiterWindowSeconds,omitempty"`
	EnablePreemptedPodGC                     *bool   `json:"enablePreemptedPodGC,omitempty"`
	PreemptedPodGCTTL                        *string `json:"preemptedPodGCTTL,omitempty"`
	EnableInitialOptimizationDataWindowCheck *bool   `json:"enableInitialOptimizationDataWindowCheck,omitempty"`
}

// RecommendationPolicyResource represents a RecommendationPolicy stored in the backend.
type RecommendationPolicyResource struct {
	Name string                   `json:"name"`
	Spec RecommendationPolicySpec `json:"spec"`
}

type RecommendationPolicySpec struct {
	StrategyType          string                           `json:"strategyType"`
	StrategyPercentile    *StrategyPercentileConfiguration `json:"strategyPercentile,omitempty"`
	HistoryWindowDuration WindowDuration                   `json:"historyWindowDuration"`
	EvaluationPeriod      string                           `json:"evaluationPeriod"`
	Buffer                map[string]string                `json:"buffer,omitempty"`
	Limits                RecommendationLimits             `json:"limits,omitempty"`
	JVM                   *JVMRecommendationConfiguration  `json:"jvm,omitempty"`
}

type StrategyPercentileConfiguration struct {
	CPU    int32 `json:"cpu"`
	Memory int32 `json:"memory"`
}

type WindowDuration struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type RecommendationLimits struct {
	RequestMin map[string]string `json:"requestMin,omitempty"`
	RequestMax map[string]string `json:"requestMax,omitempty"`
}

type JVMRecommendationConfiguration struct {
	RecentNonHeapWindow     *string `json:"recentNonHeapWindow,omitempty"`
	MinHeapXmsRatioOfMemory *string `json:"minHeapXmsRatioOfMemory,omitempty"`
	HeapBuffer              *string `json:"heapBuffer,omitempty"`
	HeapUsedPercentile      *int32  `json:"heapUsedPercentile,omitempty"`
}

// AutoscalingPolicyResource represents an AutoscalingPolicy stored in the backend.
type AutoscalingPolicyResource struct {
	Name   string                `json:"name"`
	Spec   AutoscalingPolicySpec `json:"spec"`
	Enable bool                  `json:"enable"`
}

type AutoscalingPolicySpec struct {
	Priority                   int32                          `json:"priority,omitempty"`
	RecommendationPolicyName   string                         `json:"recommendationPolicyName"`
	DisableRuntimeOptimization bool                           `json:"disableRuntimeOptimization,omitempty"`
	TargetRefs                 []TypedObjectReference         `json:"targetRefs,omitempty"`
	UpdateSchedule             []UpdateScheduleItem           `json:"updateSchedule,omitempty"`
	UpdateResources            []string                       `json:"updateResources,omitempty"`
	DriftThresholds            map[string]string              `json:"driftThresholds,omitempty"`
	OnPolicyRemoval            string                         `json:"onPolicyRemoval,omitempty"`
	LimitPolicies              map[string]ResourceLimitPolicy `json:"limitPolicies,omitempty"`
	ResourceStartupBoost       *WorkloadStartupResourceBoost  `json:"resourceStartupBoost,omitempty"`
	InPlaceFallback            *InPlaceFallback               `json:"inPlaceFallback,omitempty"`
}

type TypedObjectReference struct {
	APIVersion    string         `json:"apiVersion"`
	Kind          string         `json:"kind"`
	Name          string         `json:"name,omitempty"`
	Namespace     string         `json:"namespace,omitempty"`
	LabelSelector *LabelSelector `json:"labelSelector,omitempty"`
}

type LabelSelector struct {
	MatchLabels      map[string]string          `json:"matchLabels,omitempty"`
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

type LabelSelectorRequirement struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

type UpdateScheduleItem struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule,omitempty"`
	Duration string `json:"duration,omitempty"`
	Mode     string `json:"mode"`
}

type ResourceLimitPolicy struct {
	RemoveLimit  *bool   `json:"removeLimit,omitempty"`
	KeepLimit    *bool   `json:"keepLimit,omitempty"`
	Multiplier   *string `json:"multiplier,omitempty"`
	AutoHeadroom *string `json:"autoHeadroom,omitempty"`
}

type WorkloadStartupResourceBoost struct {
	Enabled             bool              `json:"enabled,omitempty"`
	MinBoostDuration    string            `json:"minBoostDuration,omitempty"`
	MinReadyDuration    string            `json:"minReadyDuration,omitempty"`
	ResourceMultipliers map[string]string `json:"resourceMultipliers,omitempty"`
}

type InPlaceFallback struct {
	DefaultPolicy  string            `json:"defaultPolicy,omitempty"`
	ReasonPolicies map[string]string `json:"reasonPolicies,omitempty"`
}

// WAWorkloadListFilter is the filter used to select workloads for batch operations
// such as enabling/disabling proactive update. Mirrors the backend WAWorkloadListFilters.
type WAWorkloadListFilter struct {
	WorkloadName              string   `json:"workloadName,omitempty"`
	Namespaces                []string `json:"namespaces,omitempty"`
	WorkloadKinds             []string `json:"workloadKinds,omitempty"`
	AutoscalingPolicyNames    []string `json:"autoscalingPolicyNames,omitempty"`
	WorkloadState             string   `json:"workloadState,omitempty"`
	OptimizationStates        []string `json:"optimizationStates,omitempty"`
	DisableProactiveUpdate    *bool    `json:"disableProactiveUpdate,omitempty"`
	RecommendationPolicyNames []string `json:"recommendationPolicyNames,omitempty"`
	RuntimeLanguages          []string `json:"runtimeLanguages,omitempty"`
	Optimized                 *bool    `json:"optimized,omitempty"`
}

// WAProactiveUpdateRequest is the request body for the proactive update API.
type WAProactiveUpdateRequest struct {
	ListFilter             WAWorkloadListFilter `json:"listFilter"`
	DisableProactiveUpdate bool                 `json:"disableProactiveUpdate"`
}
