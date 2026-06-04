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
	PercentileCPU    types.Int32  `tfsdk:"percentile_cpu"`
	PercentileMemory types.Int32  `tfsdk:"percentile_memory"`

	HistoryWindowCPU    types.String `tfsdk:"history_window_cpu"`
	HistoryWindowMemory types.String `tfsdk:"history_window_memory"`
	EvaluationPeriod    types.String `tfsdk:"evaluation_period"`

	BufferCPU    types.String `tfsdk:"buffer_cpu"`
	BufferMemory types.String `tfsdk:"buffer_memory"`

	RequestMinCPU    types.String `tfsdk:"request_min_cpu"`
	RequestMinMemory types.String `tfsdk:"request_min_memory"`
	RequestMaxCPU    types.String `tfsdk:"request_max_cpu"`
	RequestMaxMemory types.String `tfsdk:"request_max_memory"`

	JVMHeapBuffer              types.String `tfsdk:"jvm_heap_buffer"`
	JVMMinHeapXmsRatioOfMemory types.String `tfsdk:"jvm_min_heap_xms_ratio_of_memory"`
	JVMRecentNonHeapWindow     types.String `tfsdk:"jvm_recent_non_heap_window"`
	JVMHeapUsedPercentile      types.Int32  `tfsdk:"jvm_heap_used_percentile"`
}

func (m *RecommendationPolicyModel) ToResource() *RecommendationPolicyResource {
	return m.ToResourceFromBase(nil)
}

func (m *RecommendationPolicyModel) ToResourceFromBase(base *RecommendationPolicyResource) *RecommendationPolicyResource {
	rp := &RecommendationPolicyResource{
		Name: m.Name.ValueString(),
		Spec: RecommendationPolicySpec{
			StrategyType:     "percentile",
			EvaluationPeriod: m.EvaluationPeriod.ValueString(),
			HistoryWindowDuration: WindowDuration{
				CPU:    m.HistoryWindowCPU.ValueString(),
				Memory: m.HistoryWindowMemory.ValueString(),
			},
		},
	}
	if base != nil {
		*rp = *base
		rp.Name = m.Name.ValueString()
		rp.Spec.EvaluationPeriod = m.EvaluationPeriod.ValueString()
		rp.Spec.HistoryWindowDuration = WindowDuration{
			CPU:    m.HistoryWindowCPU.ValueString(),
			Memory: m.HistoryWindowMemory.ValueString(),
		}
	} else {
		rp.Spec.StrategyPercentile = &StrategyPercentileConfiguration{
			CPU:    95,
			Memory: 95,
		}
	}
	if !m.StrategyType.IsNull() && !m.StrategyType.IsUnknown() && m.StrategyType.ValueString() != "" {
		rp.Spec.StrategyType = m.StrategyType.ValueString()
	}

	if !m.PercentileCPU.IsNull() && !m.PercentileCPU.IsUnknown() && !m.PercentileMemory.IsNull() && !m.PercentileMemory.IsUnknown() {
		rp.Spec.StrategyPercentile = &StrategyPercentileConfiguration{
			CPU:    m.PercentileCPU.ValueInt32(),
			Memory: m.PercentileMemory.ValueInt32(),
		}
	}

	buffer := mapsClone(rp.Spec.Buffer)
	if !m.BufferCPU.IsNull() && !m.BufferCPU.IsUnknown() {
		if m.BufferCPU.ValueString() == "" {
			delete(buffer, "cpu")
		} else {
			buffer["cpu"] = m.BufferCPU.ValueString()
		}
	}
	if !m.BufferMemory.IsNull() && !m.BufferMemory.IsUnknown() {
		if m.BufferMemory.ValueString() == "" {
			delete(buffer, "memory")
		} else {
			buffer["memory"] = m.BufferMemory.ValueString()
		}
	}
	if len(buffer) > 0 {
		rp.Spec.Buffer = buffer
	} else {
		rp.Spec.Buffer = nil
	}

	limits := rp.Spec.Limits
	requestMin := mapsClone(limits.RequestMin)
	if !m.RequestMinCPU.IsNull() && !m.RequestMinCPU.IsUnknown() {
		if m.RequestMinCPU.ValueString() == "" {
			delete(requestMin, "cpu")
		} else {
			requestMin["cpu"] = m.RequestMinCPU.ValueString()
		}
	}
	if !m.RequestMinMemory.IsNull() && !m.RequestMinMemory.IsUnknown() {
		if m.RequestMinMemory.ValueString() == "" {
			delete(requestMin, "memory")
		} else {
			requestMin["memory"] = m.RequestMinMemory.ValueString()
		}
	}
	if len(requestMin) > 0 {
		limits.RequestMin = requestMin
	} else {
		limits.RequestMin = nil
	}

	requestMax := mapsClone(limits.RequestMax)
	if !m.RequestMaxCPU.IsNull() && !m.RequestMaxCPU.IsUnknown() {
		if m.RequestMaxCPU.ValueString() == "" {
			delete(requestMax, "cpu")
		} else {
			requestMax["cpu"] = m.RequestMaxCPU.ValueString()
		}
	}
	if !m.RequestMaxMemory.IsNull() && !m.RequestMaxMemory.IsUnknown() {
		if m.RequestMaxMemory.ValueString() == "" {
			delete(requestMax, "memory")
		} else {
			requestMax["memory"] = m.RequestMaxMemory.ValueString()
		}
	}
	if len(requestMax) > 0 {
		limits.RequestMax = requestMax
	} else {
		limits.RequestMax = nil
	}
	if limits.RequestMin != nil || limits.RequestMax != nil {
		rp.Spec.Limits = limits
	} else {
		rp.Spec.Limits = RecommendationLimits{}
	}

	jvm := rp.Spec.JVM
	if jvm == nil {
		jvm = &JVMRecommendationConfiguration{}
	}
	if !m.JVMHeapBuffer.IsNull() && !m.JVMHeapBuffer.IsUnknown() {
		if m.JVMHeapBuffer.ValueString() == "" {
			jvm.HeapBuffer = nil
		} else {
			v := m.JVMHeapBuffer.ValueString()
			jvm.HeapBuffer = &v
		}
	}
	if !m.JVMMinHeapXmsRatioOfMemory.IsNull() && !m.JVMMinHeapXmsRatioOfMemory.IsUnknown() {
		if m.JVMMinHeapXmsRatioOfMemory.ValueString() == "" {
			jvm.MinHeapXmsRatioOfMemory = nil
		} else {
			v := m.JVMMinHeapXmsRatioOfMemory.ValueString()
			jvm.MinHeapXmsRatioOfMemory = &v
		}
	}
	if !m.JVMRecentNonHeapWindow.IsNull() && !m.JVMRecentNonHeapWindow.IsUnknown() {
		if m.JVMRecentNonHeapWindow.ValueString() == "" {
			jvm.RecentNonHeapWindow = nil
		} else {
			v := m.JVMRecentNonHeapWindow.ValueString()
			jvm.RecentNonHeapWindow = &v
		}
	}
	if !m.JVMHeapUsedPercentile.IsNull() && !m.JVMHeapUsedPercentile.IsUnknown() {
		v := m.JVMHeapUsedPercentile.ValueInt32()
		jvm.HeapUsedPercentile = &v
	}
	if jvm.HeapBuffer != nil || jvm.MinHeapXmsRatioOfMemory != nil || jvm.RecentNonHeapWindow != nil || jvm.HeapUsedPercentile != nil {
		rp.Spec.JVM = jvm
	} else {
		rp.Spec.JVM = nil
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
		m.PercentileCPU = types.Int32Value(rp.Spec.StrategyPercentile.CPU)
		m.PercentileMemory = types.Int32Value(rp.Spec.StrategyPercentile.Memory)
	}

	if v, ok := rp.Spec.Buffer["cpu"]; ok {
		m.BufferCPU = types.StringValue(v)
	}
	if v, ok := rp.Spec.Buffer["memory"]; ok {
		m.BufferMemory = types.StringValue(v)
	}

	if v, ok := rp.Spec.Limits.RequestMin["cpu"]; ok {
		m.RequestMinCPU = types.StringValue(v)
	}
	if v, ok := rp.Spec.Limits.RequestMin["memory"]; ok {
		m.RequestMinMemory = types.StringValue(v)
	}
	if v, ok := rp.Spec.Limits.RequestMax["cpu"]; ok {
		m.RequestMaxCPU = types.StringValue(v)
	}
	if v, ok := rp.Spec.Limits.RequestMax["memory"]; ok {
		m.RequestMaxMemory = types.StringValue(v)
	}

	if rp.Spec.JVM != nil {
		if rp.Spec.JVM.HeapBuffer != nil {
			m.JVMHeapBuffer = types.StringValue(*rp.Spec.JVM.HeapBuffer)
		}
		if rp.Spec.JVM.MinHeapXmsRatioOfMemory != nil {
			m.JVMMinHeapXmsRatioOfMemory = types.StringValue(*rp.Spec.JVM.MinHeapXmsRatioOfMemory)
		}
		if rp.Spec.JVM.RecentNonHeapWindow != nil {
			m.JVMRecentNonHeapWindow = types.StringValue(NormalizeDuration(*rp.Spec.JVM.RecentNonHeapWindow))
		}
		if rp.Spec.JVM.HeapUsedPercentile != nil {
			m.JVMHeapUsedPercentile = types.Int32Value(*rp.Spec.JVM.HeapUsedPercentile)
		}
	}

	return m
}

// TargetRefModel is the Terraform model for a target workload reference.
type TargetRefModel struct {
	APIVersion    types.String                                 `tfsdk:"api_version"`
	Kind          types.String                                 `tfsdk:"kind"`
	Name          types.String                                 `tfsdk:"name"`
	Namespace     types.String                                 `tfsdk:"namespace"`
	LabelSelector customfield.NestedObject[LabelSelectorModel] `tfsdk:"label_selector"`
}

type LabelSelectorRequirementModel struct {
	Key      types.String    `tfsdk:"key"`
	Operator types.String    `tfsdk:"operator"`
	Values   *[]types.String `tfsdk:"values"`
}

type LabelSelectorModel struct {
	MatchLabels      customfield.Map[types.String]                               `tfsdk:"match_labels"`
	MatchExpressions customfield.NestedObjectList[LabelSelectorRequirementModel] `tfsdk:"match_expressions"`
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

	RecommendationPolicyName   types.String `tfsdk:"recommendation_policy_name"`
	Priority                   types.Int64  `tfsdk:"priority"`
	DisableRuntimeOptimization types.Bool   `tfsdk:"disable_runtime_optimization"`

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

	InPlaceFallbackDefaultPolicy  types.String                  `tfsdk:"in_place_fallback_default_policy"`
	InPlaceFallbackReasonPolicies customfield.Map[types.String] `tfsdk:"in_place_fallback_reason_policies"`
}

func (m *AutoscalingPolicyModel) ToResource(ctx context.Context) (*AutoscalingPolicyResource, error) {
	return m.ToResourceFromBase(ctx, nil)
}

func (m *AutoscalingPolicyModel) ToResourceFromBase(ctx context.Context, base *AutoscalingPolicyResource) (*AutoscalingPolicyResource, error) {
	ap := &AutoscalingPolicyResource{
		Name:   m.Name.ValueString(),
		Enable: true,
		Spec: AutoscalingPolicySpec{
			Priority:                 0,
			RecommendationPolicyName: m.RecommendationPolicyName.ValueString(),
			OnPolicyRemoval:          "off",
		},
	}
	if base != nil {
		*ap = *base
		ap.Name = m.Name.ValueString()
		ap.Spec.RecommendationPolicyName = m.RecommendationPolicyName.ValueString()
	}
	if !m.Enable.IsNull() && !m.Enable.IsUnknown() {
		ap.Enable = m.Enable.ValueBool()
	}
	if !m.Priority.IsNull() && !m.Priority.IsUnknown() {
		ap.Spec.Priority = int32(m.Priority.ValueInt64())
	}
	if !m.OnPolicyRemoval.IsNull() && !m.OnPolicyRemoval.IsUnknown() && m.OnPolicyRemoval.ValueString() != "" {
		ap.Spec.OnPolicyRemoval = m.OnPolicyRemoval.ValueString()
	}

	if !m.DisableRuntimeOptimization.IsNull() && !m.DisableRuntimeOptimization.IsUnknown() {
		ap.Spec.DisableRuntimeOptimization = m.DisableRuntimeOptimization.ValueBool()
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
		ap.Spec.TargetRefs = nil
		for _, tr := range targetRefs {
			selector, err := labelSelectorModelToAPI(ctx, tr.LabelSelector)
			if err != nil {
				return nil, err
			}
			ap.Spec.TargetRefs = append(ap.Spec.TargetRefs, TypedObjectReference{
				APIVersion:    tr.APIVersion.ValueString(),
				Kind:          tr.Kind.ValueString(),
				Name:          tr.Name.ValueString(),
				Namespace:     tr.Namespace.ValueString(),
				LabelSelector: selector,
			})
		}
	}

	if !m.UpdateSchedules.IsNullOrUnknown() {
		schedules, diags := m.UpdateSchedules.AsStructSliceT(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("failed to parse update_schedules: %v", diags)
		}
		ap.Spec.UpdateSchedule = nil
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

	fallback := &InPlaceFallback{}
	hasFallback := false
	if !m.InPlaceFallbackDefaultPolicy.IsNull() && !m.InPlaceFallbackDefaultPolicy.IsUnknown() && m.InPlaceFallbackDefaultPolicy.ValueString() != "" {
		fallback.DefaultPolicy = m.InPlaceFallbackDefaultPolicy.ValueString()
		hasFallback = true
	}
	if !m.InPlaceFallbackReasonPolicies.IsNull() && !m.InPlaceFallbackReasonPolicies.IsUnknown() {
		values, diags := m.InPlaceFallbackReasonPolicies.Value(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("in_place_fallback_reason_policies: %v", diags)
		}
		fallback.ReasonPolicies = map[string]string{}
		for k, v := range values {
			fallback.ReasonPolicies[k] = v.ValueString()
		}
		if len(fallback.ReasonPolicies) > 0 {
			hasFallback = true
		}
	}
	if hasFallback {
		ap.Spec.InPlaceFallback = fallback
	}

	return ap, nil
}

func labelSelectorModelToAPI(ctx context.Context, model customfield.NestedObject[LabelSelectorModel]) (*LabelSelector, error) {
	if model.IsNull() || model.IsUnknown() {
		return nil, nil
	}
	value, diags := model.Value(ctx)
	if diags.HasError() {
		return nil, fmt.Errorf("label_selector: %v", diags)
	}
	if value == nil {
		return nil, nil
	}
	out := &LabelSelector{}
	if !value.MatchLabels.IsNull() && !value.MatchLabels.IsUnknown() {
		labels, labelDiags := value.MatchLabels.Value(ctx)
		if labelDiags.HasError() {
			return nil, fmt.Errorf("label_selector.match_labels: %v", labelDiags)
		}
		out.MatchLabels = map[string]string{}
		for k, v := range labels {
			out.MatchLabels[k] = v.ValueString()
		}
	}
	if !value.MatchExpressions.IsNullOrUnknown() {
		expressions, exprDiags := value.MatchExpressions.AsStructSliceT(ctx)
		if exprDiags.HasError() {
			return nil, fmt.Errorf("label_selector.match_expressions: %v", exprDiags)
		}
		for _, expr := range expressions {
			item := LabelSelectorRequirement{
				Key:      expr.Key.ValueString(),
				Operator: expr.Operator.ValueString(),
			}
			if expr.Values != nil {
				item.Values = make([]string, 0, len(*expr.Values))
				for _, v := range *expr.Values {
					item.Values = append(item.Values, v.ValueString())
				}
			}
			out.MatchExpressions = append(out.MatchExpressions, item)
		}
	}
	if len(out.MatchLabels) == 0 && len(out.MatchExpressions) == 0 {
		return nil, nil
	}
	return out, nil
}

func labelSelectorModelFromAPI(ctx context.Context, in *LabelSelector) customfield.NestedObject[LabelSelectorModel] {
	if in == nil {
		return customfield.NullObject[LabelSelectorModel](ctx)
	}
	if len(in.MatchLabels) == 0 && len(in.MatchExpressions) == 0 {
		return customfield.NullObject[LabelSelectorModel](ctx)
	}
	labels := map[string]types.String{}
	for k, v := range in.MatchLabels {
		labels[k] = types.StringValue(v)
	}
	expressions := make([]LabelSelectorRequirementModel, 0, len(in.MatchExpressions))
	for _, expr := range in.MatchExpressions {
		values := make([]types.String, 0, len(expr.Values))
		for _, v := range expr.Values {
			values = append(values, types.StringValue(v))
		}
		expressions = append(expressions, LabelSelectorRequirementModel{
			Key:      types.StringValue(expr.Key),
			Operator: types.StringValue(expr.Operator),
			Values:   &values,
		})
	}
	return customfield.NewObjectMust(ctx, &LabelSelectorModel{
		MatchLabels:      customfield.NewMapMust[types.String](ctx, labels),
		MatchExpressions: customfield.NewObjectListMust(ctx, expressions),
	})
}

func AutoscalingPolicyModelFromResource(ctx context.Context, ap *AutoscalingPolicyResource) AutoscalingPolicyModel {
	m := AutoscalingPolicyModel{
		Name:                       types.StringValue(ap.Name),
		Enable:                     types.BoolValue(ap.Enable),
		RecommendationPolicyName:   types.StringValue(ap.Spec.RecommendationPolicyName),
		Priority:                   types.Int64Value(int64(ap.Spec.Priority)),
		DisableRuntimeOptimization: types.BoolValue(ap.Spec.DisableRuntimeOptimization),
		OnPolicyRemoval:            types.StringValue(ap.Spec.OnPolicyRemoval),
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
	}
	if v, ok := ap.Spec.DriftThresholds["memory"]; ok {
		m.DriftThresholdMemory = types.StringValue(v)
	}

	// target refs
	targetRefModels := make([]TargetRefModel, 0, len(ap.Spec.TargetRefs))
	for _, tr := range ap.Spec.TargetRefs {
		targetRefModels = append(targetRefModels, TargetRefModel{
			APIVersion:    types.StringValue(tr.APIVersion),
			Kind:          types.StringValue(tr.Kind),
			Name:          stringOrNull(tr.Name),
			Namespace:     stringOrNull(tr.Namespace),
			LabelSelector: labelSelectorModelFromAPI(ctx, tr.LabelSelector),
		})
	}
	if len(targetRefModels) > 0 {
		m.TargetRefs = customfield.NewObjectListMust(ctx, targetRefModels)
	} else {
		m.TargetRefs = customfield.NullObjectList[TargetRefModel](ctx)
	}

	// update schedules
	scheduleModels := make([]UpdateScheduleModel, 0, len(ap.Spec.UpdateSchedule))
	for _, s := range ap.Spec.UpdateSchedule {
		scheduleModels = append(scheduleModels, UpdateScheduleModel{
			Name:     types.StringValue(s.Name),
			Schedule: stringOrNull(s.Schedule),
			Duration: stringOrNull(s.Duration),
			Mode:     types.StringValue(s.Mode),
		})
	}
	if len(scheduleModels) > 0 {
		m.UpdateSchedules = customfield.NewObjectListMust(ctx, scheduleModels)
	} else {
		m.UpdateSchedules = customfield.NullObjectList[UpdateScheduleModel](ctx)
	}

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
		}
		if policy.KeepLimit != nil {
			lp.KeepLimit = types.BoolValue(*policy.KeepLimit)
		}
		if policy.Multiplier != nil {
			lp.Multiplier = types.StringValue(*policy.Multiplier)
		}
		if policy.AutoHeadroom != nil {
			lp.AutoHeadroom = types.StringValue(*policy.AutoHeadroom)
		}
		limitPolicyModels = append(limitPolicyModels, lp)
	}
	if len(limitPolicyModels) > 0 {
		m.LimitPolicies = customfield.NewObjectListMust(ctx, limitPolicyModels)
	} else {
		m.LimitPolicies = customfield.NullObjectList[LimitPolicyModel](ctx)
	}

	// startup boost
	if ap.Spec.ResourceStartupBoost != nil && ap.Spec.ResourceStartupBoost.Enabled {
		m.StartupBoostEnabled = types.BoolValue(true)
		m.StartupBoostMinBoostDuration = types.StringValue(ap.Spec.ResourceStartupBoost.MinBoostDuration)
		m.StartupBoostMinReadyDuration = types.StringValue(ap.Spec.ResourceStartupBoost.MinReadyDuration)
		if v, ok := ap.Spec.ResourceStartupBoost.ResourceMultipliers["cpu"]; ok {
			m.StartupBoostMultiplierCPU = types.StringValue(v)
		}
		if v, ok := ap.Spec.ResourceStartupBoost.ResourceMultipliers["memory"]; ok {
			m.StartupBoostMultiplierMemory = types.StringValue(v)
		}
	}

	if ap.Spec.InPlaceFallback != nil {
		if ap.Spec.InPlaceFallback.DefaultPolicy != "" {
			m.InPlaceFallbackDefaultPolicy = types.StringValue(ap.Spec.InPlaceFallback.DefaultPolicy)
		}
		reasonPolicies := map[string]types.String{}
		for k, v := range ap.Spec.InPlaceFallback.ReasonPolicies {
			reasonPolicies[k] = types.StringValue(v)
		}
		m.InPlaceFallbackReasonPolicies = customfield.NewMapMust[types.String](ctx, reasonPolicies)
	}

	return m
}

func mapsClone(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringOrNull(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}
