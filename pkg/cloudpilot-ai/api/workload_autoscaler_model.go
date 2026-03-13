package api

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

// EnableProactiveModel is the Terraform model for enabling proactive update on filtered workloads.
type EnableProactiveModel struct {
	WorkloadName              types.String    `tfsdk:"workload_name"`
	Namespaces                *[]types.String `tfsdk:"namespaces"`
	WorkloadKinds             *[]types.String `tfsdk:"workload_kinds"`
	AutoscalingPolicyNames    *[]types.String `tfsdk:"autoscaling_policy_names"`
	WorkloadState             types.String    `tfsdk:"workload_state"`
	OptimizationStates        *[]types.String `tfsdk:"optimization_states"`
	DisableProactiveUpdate    types.Bool      `tfsdk:"disable_proactive_update"`
	RecommendationPolicyNames *[]types.String `tfsdk:"recommendation_policy_names"`
	RuntimeLanguages          *[]types.String `tfsdk:"runtime_languages"`
	Optimized                 types.Bool      `tfsdk:"optimized"`
}

func (m *EnableProactiveModel) ToRequest() WAProactiveUpdateRequest {
	req := WAProactiveUpdateRequest{
		DisableProactiveUpdate: false,
	}
	populateListFilter(&req.ListFilter, m.WorkloadName, m.Namespaces, m.WorkloadKinds,
		m.AutoscalingPolicyNames, m.WorkloadState, m.OptimizationStates,
		m.DisableProactiveUpdate, m.RecommendationPolicyNames, m.RuntimeLanguages, m.Optimized)
	return req
}

// DisableProactiveModel is the Terraform model for disabling proactive update on filtered workloads.
type DisableProactiveModel struct {
	WorkloadName              types.String    `tfsdk:"workload_name"`
	Namespaces                *[]types.String `tfsdk:"namespaces"`
	WorkloadKinds             *[]types.String `tfsdk:"workload_kinds"`
	AutoscalingPolicyNames    *[]types.String `tfsdk:"autoscaling_policy_names"`
	WorkloadState             types.String    `tfsdk:"workload_state"`
	OptimizationStates        *[]types.String `tfsdk:"optimization_states"`
	DisableProactiveUpdate    types.Bool      `tfsdk:"disable_proactive_update"`
	RecommendationPolicyNames *[]types.String `tfsdk:"recommendation_policy_names"`
	RuntimeLanguages          *[]types.String `tfsdk:"runtime_languages"`
	Optimized                 types.Bool      `tfsdk:"optimized"`
}

func (m *DisableProactiveModel) ToRequest() WAProactiveUpdateRequest {
	req := WAProactiveUpdateRequest{
		DisableProactiveUpdate: true,
	}
	populateListFilter(&req.ListFilter, m.WorkloadName, m.Namespaces, m.WorkloadKinds,
		m.AutoscalingPolicyNames, m.WorkloadState, m.OptimizationStates,
		m.DisableProactiveUpdate, m.RecommendationPolicyNames, m.RuntimeLanguages, m.Optimized)
	return req
}

func populateListFilter(f *WAWorkloadListFilter,
	workloadName types.String,
	namespaces *[]types.String,
	workloadKinds *[]types.String,
	autoscalingPolicyNames *[]types.String,
	workloadState types.String,
	optimizationStates *[]types.String,
	disableProactiveUpdate types.Bool,
	recommendationPolicyNames *[]types.String,
	runtimeLanguages *[]types.String,
	optimized types.Bool,
) {
	if !workloadName.IsNull() && !workloadName.IsUnknown() && workloadName.ValueString() != "" {
		f.WorkloadName = workloadName.ValueString()
	}
	if namespaces != nil {
		for _, ns := range *namespaces {
			f.Namespaces = append(f.Namespaces, ns.ValueString())
		}
	}
	if workloadKinds != nil {
		for _, wk := range *workloadKinds {
			f.WorkloadKinds = append(f.WorkloadKinds, wk.ValueString())
		}
	}
	if autoscalingPolicyNames != nil {
		for _, ap := range *autoscalingPolicyNames {
			f.AutoscalingPolicyNames = append(f.AutoscalingPolicyNames, ap.ValueString())
		}
	}
	if !workloadState.IsNull() && !workloadState.IsUnknown() && workloadState.ValueString() != "" {
		f.WorkloadState = workloadState.ValueString()
	}
	if optimizationStates != nil {
		for _, os := range *optimizationStates {
			f.OptimizationStates = append(f.OptimizationStates, os.ValueString())
		}
	}
	if !disableProactiveUpdate.IsNull() && !disableProactiveUpdate.IsUnknown() {
		v := disableProactiveUpdate.ValueBool()
		f.DisableProactiveUpdate = &v
	}
	if recommendationPolicyNames != nil {
		for _, rp := range *recommendationPolicyNames {
			f.RecommendationPolicyNames = append(f.RecommendationPolicyNames, rp.ValueString())
		}
	}
	if runtimeLanguages != nil {
		for _, rl := range *runtimeLanguages {
			f.RuntimeLanguages = append(f.RuntimeLanguages, rl.ValueString())
		}
	}
	if !optimized.IsNull() && !optimized.IsUnknown() {
		v := optimized.ValueBool()
		f.Optimized = &v
	}
}

// RecommendationPolicyModel is the Terraform model for a RecommendationPolicy.
type RecommendationPolicyModel struct {
	Name types.String `tfsdk:"name"`

	StrategyType     types.String `tfsdk:"strategy_type"`
	PercentileCPU    types.Int64  `tfsdk:"percentile_cpu"`
	PercentileMemory types.Int64  `tfsdk:"percentile_memory"`

	HistoryWindowCPU    types.String `tfsdk:"history_window_cpu"`
	HistoryWindowMemory types.String `tfsdk:"history_window_memory"`
	EvaluationPeriod    types.String `tfsdk:"evaluation_period"`

	BufferCPU    types.String `tfsdk:"buffer_cpu"`
	BufferMemory types.String `tfsdk:"buffer_memory"`

	RequestMinCPU    types.String `tfsdk:"request_min_cpu"`
	RequestMinMemory types.String `tfsdk:"request_min_memory"`
	RequestMaxCPU    types.String `tfsdk:"request_max_cpu"`
	RequestMaxMemory types.String `tfsdk:"request_max_memory"`
}

func (m *RecommendationPolicyModel) ToResource() *RecommendationPolicyResource {
	rp := &RecommendationPolicyResource{
		Name: m.Name.ValueString(),
		Spec: RecommendationPolicySpec{
			StrategyType:     m.StrategyType.ValueString(),
			EvaluationPeriod: m.EvaluationPeriod.ValueString(),
			HistoryWindowDuration: WindowDuration{
				CPU:    m.HistoryWindowCPU.ValueString(),
				Memory: m.HistoryWindowMemory.ValueString(),
			},
		},
	}

	if !m.PercentileCPU.IsNull() && !m.PercentileMemory.IsNull() {
		rp.Spec.StrategyPercentile = &StrategyPercentileConfiguration{
			CPU:    int32(m.PercentileCPU.ValueInt64()),
			Memory: int32(m.PercentileMemory.ValueInt64()),
		}
	}

	buffer := make(map[string]string)
	if !m.BufferCPU.IsNull() && !m.BufferCPU.IsUnknown() && m.BufferCPU.ValueString() != "" {
		buffer["cpu"] = m.BufferCPU.ValueString()
	}
	if !m.BufferMemory.IsNull() && !m.BufferMemory.IsUnknown() && m.BufferMemory.ValueString() != "" {
		buffer["memory"] = m.BufferMemory.ValueString()
	}
	if len(buffer) > 0 {
		rp.Spec.Buffer = buffer
	}

	limits := RecommendationLimits{}
	hasLimits := false
	requestMin := make(map[string]string)
	if !m.RequestMinCPU.IsNull() && !m.RequestMinCPU.IsUnknown() && m.RequestMinCPU.ValueString() != "" {
		requestMin["cpu"] = m.RequestMinCPU.ValueString()
	}
	if !m.RequestMinMemory.IsNull() && !m.RequestMinMemory.IsUnknown() && m.RequestMinMemory.ValueString() != "" {
		requestMin["memory"] = m.RequestMinMemory.ValueString()
	}
	if len(requestMin) > 0 {
		limits.RequestMin = requestMin
		hasLimits = true
	}

	requestMax := make(map[string]string)
	if !m.RequestMaxCPU.IsNull() && !m.RequestMaxCPU.IsUnknown() && m.RequestMaxCPU.ValueString() != "" {
		requestMax["cpu"] = m.RequestMaxCPU.ValueString()
	}
	if !m.RequestMaxMemory.IsNull() && !m.RequestMaxMemory.IsUnknown() && m.RequestMaxMemory.ValueString() != "" {
		requestMax["memory"] = m.RequestMaxMemory.ValueString()
	}
	if len(requestMax) > 0 {
		limits.RequestMax = requestMax
		hasLimits = true
	}
	if hasLimits {
		rp.Spec.Limits = limits
	}

	return rp
}

func RecommendationPolicyModelFromResource(rp *RecommendationPolicyResource) RecommendationPolicyModel {
	m := RecommendationPolicyModel{
		Name:                types.StringValue(rp.Name),
		StrategyType:        types.StringValue(rp.Spec.StrategyType),
		EvaluationPeriod:    types.StringValue(NormalizeDuration(rp.Spec.EvaluationPeriod)),
		HistoryWindowCPU:    types.StringValue(NormalizeDuration(rp.Spec.HistoryWindowDuration.CPU)),
		HistoryWindowMemory: types.StringValue(NormalizeDuration(rp.Spec.HistoryWindowDuration.Memory)),
	}

	if rp.Spec.StrategyPercentile != nil {
		m.PercentileCPU = types.Int64Value(int64(rp.Spec.StrategyPercentile.CPU))
		m.PercentileMemory = types.Int64Value(int64(rp.Spec.StrategyPercentile.Memory))
	} else {
		m.PercentileCPU = types.Int64Value(95)
		m.PercentileMemory = types.Int64Value(95)
	}

	if v, ok := rp.Spec.Buffer["cpu"]; ok {
		m.BufferCPU = types.StringValue(v)
	} else {
		m.BufferCPU = types.StringValue("")
	}
	if v, ok := rp.Spec.Buffer["memory"]; ok {
		m.BufferMemory = types.StringValue(v)
	} else {
		m.BufferMemory = types.StringValue("")
	}

	if v, ok := rp.Spec.Limits.RequestMin["cpu"]; ok {
		m.RequestMinCPU = types.StringValue(v)
	} else {
		m.RequestMinCPU = types.StringValue("")
	}
	if v, ok := rp.Spec.Limits.RequestMin["memory"]; ok {
		m.RequestMinMemory = types.StringValue(v)
	} else {
		m.RequestMinMemory = types.StringValue("")
	}
	if v, ok := rp.Spec.Limits.RequestMax["cpu"]; ok {
		m.RequestMaxCPU = types.StringValue(v)
	} else {
		m.RequestMaxCPU = types.StringValue("")
	}
	if v, ok := rp.Spec.Limits.RequestMax["memory"]; ok {
		m.RequestMaxMemory = types.StringValue(v)
	} else {
		m.RequestMaxMemory = types.StringValue("")
	}

	return m
}

// TargetRefModel is the Terraform model for a target workload reference.
type TargetRefModel struct {
	APIVersion types.String `tfsdk:"api_version"`
	Kind       types.String `tfsdk:"kind"`
	Name       types.String `tfsdk:"name"`
	Namespace  types.String `tfsdk:"namespace"`
}

// UpdateScheduleModel is the Terraform model for an update schedule item.
type UpdateScheduleModel struct {
	Name     types.String `tfsdk:"name"`
	Schedule types.String `tfsdk:"schedule"`
	Duration types.String `tfsdk:"duration"`
	Mode     types.String `tfsdk:"mode"`
}

// LimitPolicyModel is the Terraform model for a per-resource limit policy.
type LimitPolicyModel struct {
	Resource     types.String `tfsdk:"resource"`
	RemoveLimit  types.Bool   `tfsdk:"remove_limit"`
	KeepLimit    types.Bool   `tfsdk:"keep_limit"`
	Multiplier   types.String `tfsdk:"multiplier"`
	AutoHeadroom types.String `tfsdk:"auto_headroom"`
}

// AutoscalingPolicyModel is the Terraform model for an AutoscalingPolicy.
type AutoscalingPolicyModel struct {
	Name   types.String `tfsdk:"name"`
	Enable types.Bool   `tfsdk:"enable"`

	RecommendationPolicyName types.String `tfsdk:"recommendation_policy_name"`
	Priority                 types.Int64  `tfsdk:"priority"`

	UpdateResources      *[]types.String `tfsdk:"update_resources"`
	DriftThresholdCPU    types.String    `tfsdk:"drift_threshold_cpu"`
	DriftThresholdMemory types.String    `tfsdk:"drift_threshold_memory"`
	OnPolicyRemoval      types.String    `tfsdk:"on_policy_removal"`

	TargetRefs      customfield.NestedObjectList[TargetRefModel]      `tfsdk:"target_refs"`
	UpdateSchedules customfield.NestedObjectList[UpdateScheduleModel] `tfsdk:"update_schedules"`
	LimitPolicies   customfield.NestedObjectList[LimitPolicyModel]    `tfsdk:"limit_policies"`

	StartupBoostEnabled          types.Bool   `tfsdk:"startup_boost_enabled"`
	StartupBoostMinBoostDuration types.String `tfsdk:"startup_boost_min_boost_duration"`
	StartupBoostMinReadyDuration types.String `tfsdk:"startup_boost_min_ready_duration"`
	StartupBoostMultiplierCPU    types.String `tfsdk:"startup_boost_multiplier_cpu"`
	StartupBoostMultiplierMemory types.String `tfsdk:"startup_boost_multiplier_memory"`

	InPlaceFallbackDefaultPolicy types.String `tfsdk:"in_place_fallback_default_policy"`
}

func (m *AutoscalingPolicyModel) ToResource(ctx context.Context) (*AutoscalingPolicyResource, error) {
	ap := &AutoscalingPolicyResource{
		Name:   m.Name.ValueString(),
		Enable: m.Enable.ValueBool(),
		Spec: AutoscalingPolicySpec{
			Priority:                 int32(m.Priority.ValueInt64()),
			RecommendationPolicyName: m.RecommendationPolicyName.ValueString(),
			OnPolicyRemoval:          m.OnPolicyRemoval.ValueString(),
		},
	}

	if m.UpdateResources != nil {
		resources := make([]string, len(*m.UpdateResources))
		for i, r := range *m.UpdateResources {
			resources[i] = r.ValueString()
		}
		ap.Spec.UpdateResources = resources
	}

	driftThresholds := make(map[string]string)
	if !m.DriftThresholdCPU.IsNull() && !m.DriftThresholdCPU.IsUnknown() && m.DriftThresholdCPU.ValueString() != "" {
		driftThresholds["cpu"] = m.DriftThresholdCPU.ValueString()
	}
	if !m.DriftThresholdMemory.IsNull() && !m.DriftThresholdMemory.IsUnknown() && m.DriftThresholdMemory.ValueString() != "" {
		driftThresholds["memory"] = m.DriftThresholdMemory.ValueString()
	}
	if len(driftThresholds) > 0 {
		ap.Spec.DriftThresholds = driftThresholds
	}

	if !m.TargetRefs.IsNullOrUnknown() {
		targetRefs, diags := m.TargetRefs.AsStructSliceT(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("failed to parse target_refs: %v", diags)
		}
		for _, tr := range targetRefs {
			ap.Spec.TargetRefs = append(ap.Spec.TargetRefs, TypedObjectReference{
				APIVersion: tr.APIVersion.ValueString(),
				Kind:       tr.Kind.ValueString(),
				Name:       tr.Name.ValueString(),
				Namespace:  tr.Namespace.ValueString(),
			})
		}
	}

	if !m.UpdateSchedules.IsNullOrUnknown() {
		schedules, diags := m.UpdateSchedules.AsStructSliceT(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("failed to parse update_schedules: %v", diags)
		}
		for _, s := range schedules {
			ap.Spec.UpdateSchedule = append(ap.Spec.UpdateSchedule, UpdateScheduleItem{
				Name:     s.Name.ValueString(),
				Schedule: s.Schedule.ValueString(),
				Duration: s.Duration.ValueString(),
				Mode:     s.Mode.ValueString(),
			})
		}
	}

	if !m.LimitPolicies.IsNullOrUnknown() {
		limitPolicies, diags := m.LimitPolicies.AsStructSliceT(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("failed to parse limit_policies: %v", diags)
		}
		policies := make(map[string]ResourceLimitPolicy)
		for _, lp := range limitPolicies {
			resourceName := lp.Resource.ValueString()
			policy := ResourceLimitPolicy{}
			if !lp.RemoveLimit.IsNull() && !lp.RemoveLimit.IsUnknown() && lp.RemoveLimit.ValueBool() {
				v := true
				policy.RemoveLimit = &v
			}
			if !lp.KeepLimit.IsNull() && !lp.KeepLimit.IsUnknown() && lp.KeepLimit.ValueBool() {
				v := true
				policy.KeepLimit = &v
			}
			if !lp.Multiplier.IsNull() && !lp.Multiplier.IsUnknown() && lp.Multiplier.ValueString() != "" {
				v := lp.Multiplier.ValueString()
				policy.Multiplier = &v
			}
			if !lp.AutoHeadroom.IsNull() && !lp.AutoHeadroom.IsUnknown() && lp.AutoHeadroom.ValueString() != "" {
				v := lp.AutoHeadroom.ValueString()
				policy.AutoHeadroom = &v
			}
			policies[resourceName] = policy
		}
		if len(policies) > 0 {
			ap.Spec.LimitPolicies = policies
		}
	}

	if m.StartupBoostEnabled.ValueBool() {
		boost := &WorkloadStartupResourceBoost{
			Enabled:          true,
			MinBoostDuration: m.StartupBoostMinBoostDuration.ValueString(),
			MinReadyDuration: m.StartupBoostMinReadyDuration.ValueString(),
		}
		multipliers := make(map[string]string)
		if !m.StartupBoostMultiplierCPU.IsNull() && !m.StartupBoostMultiplierCPU.IsUnknown() && m.StartupBoostMultiplierCPU.ValueString() != "" {
			multipliers["cpu"] = m.StartupBoostMultiplierCPU.ValueString()
		}
		if !m.StartupBoostMultiplierMemory.IsNull() && !m.StartupBoostMultiplierMemory.IsUnknown() && m.StartupBoostMultiplierMemory.ValueString() != "" {
			multipliers["memory"] = m.StartupBoostMultiplierMemory.ValueString()
		}
		if len(multipliers) > 0 {
			boost.ResourceMultipliers = multipliers
		}
		ap.Spec.ResourceStartupBoost = boost
	}

	if !m.InPlaceFallbackDefaultPolicy.IsNull() && !m.InPlaceFallbackDefaultPolicy.IsUnknown() && m.InPlaceFallbackDefaultPolicy.ValueString() != "" {
		ap.Spec.InPlaceFallback = &InPlaceFallback{
			DefaultPolicy: m.InPlaceFallbackDefaultPolicy.ValueString(),
		}
	}

	return ap, nil
}

func AutoscalingPolicyModelFromResource(ctx context.Context, ap *AutoscalingPolicyResource) AutoscalingPolicyModel {
	m := AutoscalingPolicyModel{
		Name:                     types.StringValue(ap.Name),
		Enable:                   types.BoolValue(ap.Enable),
		RecommendationPolicyName: types.StringValue(ap.Spec.RecommendationPolicyName),
		Priority:                 types.Int64Value(int64(ap.Spec.Priority)),
		OnPolicyRemoval:          types.StringValue(ap.Spec.OnPolicyRemoval),
	}

	if len(ap.Spec.UpdateResources) > 0 {
		resources := make([]types.String, len(ap.Spec.UpdateResources))
		for i, r := range ap.Spec.UpdateResources {
			resources[i] = types.StringValue(r)
		}
		m.UpdateResources = &resources
	}

	if v, ok := ap.Spec.DriftThresholds["cpu"]; ok {
		m.DriftThresholdCPU = types.StringValue(v)
	} else {
		m.DriftThresholdCPU = types.StringValue("")
	}
	if v, ok := ap.Spec.DriftThresholds["memory"]; ok {
		m.DriftThresholdMemory = types.StringValue(v)
	} else {
		m.DriftThresholdMemory = types.StringValue("")
	}

	// target refs
	targetRefModels := make([]TargetRefModel, 0, len(ap.Spec.TargetRefs))
	for _, tr := range ap.Spec.TargetRefs {
		targetRefModels = append(targetRefModels, TargetRefModel{
			APIVersion: types.StringValue(tr.APIVersion),
			Kind:       types.StringValue(tr.Kind),
			Name:       types.StringValue(tr.Name),
			Namespace:  types.StringValue(tr.Namespace),
		})
	}
	m.TargetRefs = customfield.NewObjectListMust(ctx, targetRefModels)

	// update schedules
	scheduleModels := make([]UpdateScheduleModel, 0, len(ap.Spec.UpdateSchedule))
	for _, s := range ap.Spec.UpdateSchedule {
		scheduleModels = append(scheduleModels, UpdateScheduleModel{
			Name:     types.StringValue(s.Name),
			Schedule: types.StringValue(s.Schedule),
			Duration: types.StringValue(s.Duration),
			Mode:     types.StringValue(s.Mode),
		})
	}
	m.UpdateSchedules = customfield.NewObjectListMust(ctx, scheduleModels)

	// limit policies — use sorted keys for deterministic order
	limitPolicyModels := make([]LimitPolicyModel, 0, len(ap.Spec.LimitPolicies))
	sortedResources := make([]string, 0, len(ap.Spec.LimitPolicies))
	for resourceName := range ap.Spec.LimitPolicies {
		sortedResources = append(sortedResources, resourceName)
	}
	sort.Strings(sortedResources)
	for _, resourceName := range sortedResources {
		policy := ap.Spec.LimitPolicies[resourceName]
		lp := LimitPolicyModel{
			Resource: types.StringValue(resourceName),
		}
		if policy.RemoveLimit != nil {
			lp.RemoveLimit = types.BoolValue(*policy.RemoveLimit)
		} else {
			lp.RemoveLimit = types.BoolValue(false)
		}
		if policy.KeepLimit != nil {
			lp.KeepLimit = types.BoolValue(*policy.KeepLimit)
		} else {
			lp.KeepLimit = types.BoolValue(false)
		}
		if policy.Multiplier != nil {
			lp.Multiplier = types.StringValue(*policy.Multiplier)
		} else {
			lp.Multiplier = types.StringValue("")
		}
		if policy.AutoHeadroom != nil {
			lp.AutoHeadroom = types.StringValue(*policy.AutoHeadroom)
		} else {
			lp.AutoHeadroom = types.StringValue("")
		}
		limitPolicyModels = append(limitPolicyModels, lp)
	}
	m.LimitPolicies = customfield.NewObjectListMust(ctx, limitPolicyModels)

	// startup boost
	if ap.Spec.ResourceStartupBoost != nil && ap.Spec.ResourceStartupBoost.Enabled {
		m.StartupBoostEnabled = types.BoolValue(true)
		m.StartupBoostMinBoostDuration = types.StringValue(ap.Spec.ResourceStartupBoost.MinBoostDuration)
		m.StartupBoostMinReadyDuration = types.StringValue(ap.Spec.ResourceStartupBoost.MinReadyDuration)
		if v, ok := ap.Spec.ResourceStartupBoost.ResourceMultipliers["cpu"]; ok {
			m.StartupBoostMultiplierCPU = types.StringValue(v)
		} else {
			m.StartupBoostMultiplierCPU = types.StringValue("")
		}
		if v, ok := ap.Spec.ResourceStartupBoost.ResourceMultipliers["memory"]; ok {
			m.StartupBoostMultiplierMemory = types.StringValue(v)
		} else {
			m.StartupBoostMultiplierMemory = types.StringValue("")
		}
	} else {
		m.StartupBoostEnabled = types.BoolValue(false)
		m.StartupBoostMinBoostDuration = types.StringValue("")
		m.StartupBoostMinReadyDuration = types.StringValue("")
		m.StartupBoostMultiplierCPU = types.StringValue("")
		m.StartupBoostMultiplierMemory = types.StringValue("")
	}

	if ap.Spec.InPlaceFallback != nil {
		m.InPlaceFallbackDefaultPolicy = types.StringValue(ap.Spec.InPlaceFallback.DefaultPolicy)
	} else {
		m.InPlaceFallbackDefaultPolicy = types.StringValue("")
	}

	return m
}
