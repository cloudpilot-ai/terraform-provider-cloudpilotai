package api

import (
	"context"
	"fmt"
	"sort"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
	StartupBoostEnabled       types.Bool      `tfsdk:"startup_boost_enabled"`
}

func (m *EnableProactiveModel) ToRequest() WAProactiveUpdateRequest {
	req := WAProactiveUpdateRequest{
		DisableProactiveUpdate: false,
	}
	populateListFilter(&req.ListFilter, m.WorkloadName, m.Namespaces, m.WorkloadKinds,
		m.AutoscalingPolicyNames, m.WorkloadState, m.OptimizationStates,
		m.DisableProactiveUpdate, m.RecommendationPolicyNames, m.RuntimeLanguages, m.Optimized)
	populateStartupBoostFilter(&req.ListFilter, m.StartupBoostEnabled)
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
	StartupBoostEnabled       types.Bool      `tfsdk:"startup_boost_enabled"`
}

func (m *DisableProactiveModel) ToRequest() WAProactiveUpdateRequest {
	req := WAProactiveUpdateRequest{
		DisableProactiveUpdate: true,
	}
	populateListFilter(&req.ListFilter, m.WorkloadName, m.Namespaces, m.WorkloadKinds,
		m.AutoscalingPolicyNames, m.WorkloadState, m.OptimizationStates,
		m.DisableProactiveUpdate, m.RecommendationPolicyNames, m.RuntimeLanguages, m.Optimized)
	populateStartupBoostFilter(&req.ListFilter, m.StartupBoostEnabled)
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

	StartupBoostEnabled                  types.Bool      `tfsdk:"startup_boost_enabled"`
	StartupBoostRecommendationPolicyName types.String    `tfsdk:"startup_boost_recommendation_policy_name"`
	StartupBoostUpdateResources          *[]types.String `tfsdk:"startup_boost_update_resources"`
	StartupBoostMinBoostDuration         types.String    `tfsdk:"startup_boost_min_boost_duration"`
	StartupBoostMinReadyDuration         types.String    `tfsdk:"startup_boost_min_ready_duration"`
	StartupBoostMultiplierCPU            types.String    `tfsdk:"startup_boost_multiplier_cpu"`
	StartupBoostMultiplierMemory         types.String    `tfsdk:"startup_boost_multiplier_memory"`

	InPlaceFallbackDefaultPolicy  types.String                  `tfsdk:"in_place_fallback_default_policy"`
	InPlaceFallbackReasonPolicies customfield.Map[types.String] `tfsdk:"in_place_fallback_reason_policies"`
}

func (m *AutoscalingPolicyModel) ToResource(ctx context.Context) (*AutoscalingPolicyResource, error) {
	return m.ToResourceFromBase(ctx, nil)
}

func (m *AutoscalingPolicyModel) ToResourceFromBase(ctx context.Context, base *AutoscalingPolicyResource) (*AutoscalingPolicyResource, error) {
	if m.StartupBoostUpdateResources != nil && len(*m.StartupBoostUpdateResources) == 0 {
		return nil, fmt.Errorf("startup_boost_update_resources must contain at least one resource")
	}
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
		applyStringValue(&ap.Spec.RecommendationPolicyName, m.RecommendationPolicyName)
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

	driftThresholds := mapsClone(ap.Spec.DriftThresholds)
	applyMapString(driftThresholds, "cpu", m.DriftThresholdCPU)
	applyMapString(driftThresholds, "memory", m.DriftThresholdMemory)
	ap.Spec.DriftThresholds = driftThresholds

	for _, apply := range []func() error{
		func() error { return m.applyTargetRefs(ctx, ap) },
		func() error { return m.applyUpdateSchedules(ctx, ap) },
		func() error { return m.applyLimitPolicies(ctx, ap) },
		func() error { return m.applyInPlaceFallback(ctx, ap) },
	} {
		if err := apply(); err != nil {
			return nil, err
		}
	}
	m.applyStartupBoost(ap)

	return ap, nil
}

func (m *AutoscalingPolicyModel) applyTargetRefs(ctx context.Context, ap *AutoscalingPolicyResource) error {
	if m.TargetRefs.IsNullOrUnknown() {
		return nil
	}
	targetRefs, diags := m.TargetRefs.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("failed to parse target_refs: %v", diags)
	}
	previous := ap.Spec.TargetRefs
	used := make([]bool, len(previous))
	ap.Spec.TargetRefs = nil
	for _, model := range targetRefs {
		target := matchingTargetReference(ctx, previous, used, model)
		applyStringValue(&target.APIVersion, model.APIVersion)
		applyStringValue(&target.Kind, model.Kind)
		applyStringValue(&target.Name, model.Name)
		applyStringValue(&target.Namespace, model.Namespace)
		selector, err := labelSelectorModelToAPIFromBase(ctx, model.LabelSelector, target.LabelSelector)
		if err != nil {
			return err
		}
		target.LabelSelector = selector
		ap.Spec.TargetRefs = append(ap.Spec.TargetRefs, target)
	}
	return nil
}

func (m *AutoscalingPolicyModel) applyUpdateSchedules(ctx context.Context, ap *AutoscalingPolicyResource) error {
	if m.UpdateSchedules.IsNullOrUnknown() {
		return nil
	}
	schedules, diags := m.UpdateSchedules.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("failed to parse update_schedules: %v", diags)
	}
	previous := ap.Spec.UpdateSchedule
	ap.Spec.UpdateSchedule = nil
	for _, model := range schedules {
		item := UpdateScheduleItem{}
		for _, existing := range previous {
			if existing.Name == model.Name.ValueString() {
				item = existing
				break
			}
		}
		applyStringValue(&item.Name, model.Name)
		applyStringValue(&item.Schedule, model.Schedule)
		applyStringValue(&item.Duration, model.Duration)
		applyStringValue(&item.Mode, model.Mode)
		ap.Spec.UpdateSchedule = append(ap.Spec.UpdateSchedule, item)
	}
	return nil
}

func (m *AutoscalingPolicyModel) applyLimitPolicies(ctx context.Context, ap *AutoscalingPolicyResource) error {
	if m.LimitPolicies.IsNullOrUnknown() {
		return nil
	}
	limitPolicies, diags := m.LimitPolicies.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("failed to parse limit_policies: %v", diags)
	}
	policies := make(map[string]ResourceLimitPolicy, len(limitPolicies))
	for _, model := range limitPolicies {
		resourceName := model.Resource.ValueString()
		policy, err := limitPolicyFromModel(ap.Spec.LimitPolicies[resourceName], model)
		if err != nil {
			return fmt.Errorf("limit_policies[%s]: %w", resourceName, err)
		}
		policies[resourceName] = policy
	}
	ap.Spec.LimitPolicies = policies
	return nil
}

func (m *AutoscalingPolicyModel) applyInPlaceFallback(ctx context.Context, ap *AutoscalingPolicyResource) error {
	defaultManaged := !m.InPlaceFallbackDefaultPolicy.IsNull() && !m.InPlaceFallbackDefaultPolicy.IsUnknown()
	reasonsManaged := !m.InPlaceFallbackReasonPolicies.IsNull() && !m.InPlaceFallbackReasonPolicies.IsUnknown()
	if !defaultManaged && !reasonsManaged {
		return nil
	}
	fallback := &InPlaceFallback{}
	if ap.Spec.InPlaceFallback != nil {
		*fallback = *ap.Spec.InPlaceFallback
	}
	applyStringValue(&fallback.DefaultPolicy, m.InPlaceFallbackDefaultPolicy)
	if reasonsManaged {
		values, diags := m.InPlaceFallbackReasonPolicies.Value(ctx)
		if diags.HasError() {
			return fmt.Errorf("in_place_fallback_reason_policies: %v", diags)
		}
		fallback.ReasonPolicies = applyStringMapValues(fallback.ReasonPolicies, values)
	}
	ap.Spec.InPlaceFallback = fallback
	return nil
}

func labelSelectorModelToAPI(ctx context.Context, model customfield.NestedObject[LabelSelectorModel]) (*LabelSelector, error) {
	return labelSelectorModelToAPIFromBase(ctx, model, nil)
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
	matchLabels := customfield.NullMap[types.String](ctx)
	if len(labels) > 0 {
		matchLabels = customfield.NewMapMust[types.String](ctx, labels)
	}
	expressions := make([]LabelSelectorRequirementModel, 0, len(in.MatchExpressions))
	for _, expr := range in.MatchExpressions {
		values := make([]types.String, 0, len(expr.Values))
		for _, v := range expr.Values {
			values = append(values, types.StringValue(v))
		}
		model := LabelSelectorRequirementModel{
			Key:      types.StringValue(expr.Key),
			Operator: types.StringValue(expr.Operator),
		}
		if len(values) > 0 {
			model.Values = &values
		}
		expressions = append(expressions, model)
	}
	matchExpressions := customfield.NullObjectList[LabelSelectorRequirementModel](ctx)
	if len(expressions) > 0 {
		matchExpressions = customfield.NewObjectListMust(ctx, expressions)
	}
	return customfield.NewObjectMust(ctx, &LabelSelectorModel{
		MatchLabels:      matchLabels,
		MatchExpressions: matchExpressions,
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

	m.UpdateResources = stringSliceModels(ap.Spec.UpdateResources)

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

	populateLimitPolicyModels(ctx, &m, ap.Spec.LimitPolicies)

	populateStartupBoostModel(&m, ap.Spec.ResourceStartupBoost)
	populateInPlaceFallbackModel(ctx, &m, ap.Spec.InPlaceFallback)

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

func populateLimitPolicyModels(ctx context.Context, m *AutoscalingPolicyModel, policies map[string]ResourceLimitPolicy) {
	// limit policies — use sorted keys for deterministic order
	limitPolicyModels := make([]LimitPolicyModel, 0, len(policies))
	sortedResources := make([]string, 0, len(policies))
	for resourceName := range policies {
		sortedResources = append(sortedResources, resourceName)
	}
	sort.Strings(sortedResources)
	for _, resourceName := range sortedResources {
		policy := policies[resourceName]
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
}

func populateInPlaceFallbackModel(ctx context.Context, m *AutoscalingPolicyModel, fallback *InPlaceFallback) {
	if fallback != nil {
		if fallback.DefaultPolicy != "" {
			m.InPlaceFallbackDefaultPolicy = types.StringValue(fallback.DefaultPolicy)
		}
		reasonPolicies := map[string]types.String{}
		for k, v := range fallback.ReasonPolicies {
			reasonPolicies[k] = types.StringValue(v)
		}
		m.InPlaceFallbackReasonPolicies = customfield.NewMapMust[types.String](ctx, reasonPolicies)
	}
}
