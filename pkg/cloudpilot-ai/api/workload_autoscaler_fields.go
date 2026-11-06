package api

import "github.com/hashicorp/terraform-plugin-framework/types"

func populateStartupBoostFilter(filter *WAWorkloadListFilter, startup types.Bool) {
	if !startup.IsNull() && !startup.IsUnknown() {
		value := startup.ValueBool()
		filter.StartupBoostEnabled = &value
	}
}

func stringSliceValues(values *[]types.String) []string {
	if values == nil {
		return nil
	}
	result := make([]string, len(*values))
	for i, value := range *values {
		result[i] = value.ValueString()
	}
	return result
}

func stringSliceModels(values []string) *[]types.String {
	if values == nil {
		return nil
	}
	result := make([]types.String, len(values))
	for i, value := range values {
		result[i] = types.StringValue(value)
	}
	return &result
}

func applyOptionalString(target **string, value types.String) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	*target = nil
	if value.ValueString() != "" {
		text := value.ValueString()
		*target = &text
	}
}

func optionalStringValue(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func (m *AutoscalingPolicyModel) applyStartupBoost(ap *AutoscalingPolicyResource) {
	managed := !m.StartupBoostEnabled.IsNull() && !m.StartupBoostEnabled.IsUnknown()
	managed = managed || !m.StartupBoostRecommendationPolicyName.IsNull() && !m.StartupBoostRecommendationPolicyName.IsUnknown()
	managed = managed || m.StartupBoostUpdateResources != nil
	managed = managed || !m.StartupBoostMinBoostDuration.IsNull() && !m.StartupBoostMinBoostDuration.IsUnknown()
	managed = managed || !m.StartupBoostMinReadyDuration.IsNull() && !m.StartupBoostMinReadyDuration.IsUnknown()
	managed = managed || !m.StartupBoostMultiplierCPU.IsNull() && !m.StartupBoostMultiplierCPU.IsUnknown()
	managed = managed || !m.StartupBoostMultiplierMemory.IsNull() && !m.StartupBoostMultiplierMemory.IsUnknown()
	if !managed {
		return
	}
	boost := &WorkloadStartupResourceBoost{}
	if ap.Spec.ResourceStartupBoost != nil {
		*boost = *ap.Spec.ResourceStartupBoost
	}
	if !m.StartupBoostEnabled.IsNull() && !m.StartupBoostEnabled.IsUnknown() {
		boost.Enabled = m.StartupBoostEnabled.ValueBool()
	}
	if !m.StartupBoostRecommendationPolicyName.IsNull() && !m.StartupBoostRecommendationPolicyName.IsUnknown() {
		boost.RecommendationPolicyName = m.StartupBoostRecommendationPolicyName.ValueString()
	}
	if m.StartupBoostUpdateResources != nil {
		boost.UpdateResources = stringSliceValues(m.StartupBoostUpdateResources)
	}
	if !m.StartupBoostMinBoostDuration.IsNull() && !m.StartupBoostMinBoostDuration.IsUnknown() {
		boost.MinBoostDuration = m.StartupBoostMinBoostDuration.ValueString()
	}
	if !m.StartupBoostMinReadyDuration.IsNull() && !m.StartupBoostMinReadyDuration.IsUnknown() {
		boost.MinReadyDuration = m.StartupBoostMinReadyDuration.ValueString()
	}
	boost.ResourceMultipliers = mapsClone(boost.ResourceMultipliers)
	applyMapString(boost.ResourceMultipliers, "cpu", m.StartupBoostMultiplierCPU)
	applyMapString(boost.ResourceMultipliers, "memory", m.StartupBoostMultiplierMemory)
	ap.Spec.ResourceStartupBoost = boost
}

func applyMapString(target map[string]string, key string, value types.String) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	if value.ValueString() == "" {
		delete(target, key)
	} else {
		target[key] = value.ValueString()
	}
}

func populateStartupBoostModel(model *AutoscalingPolicyModel, boost *WorkloadStartupResourceBoost) {
	if boost == nil {
		return
	}
	model.StartupBoostEnabled = types.BoolValue(boost.Enabled)
	model.StartupBoostRecommendationPolicyName = stringOrNull(boost.RecommendationPolicyName)
	model.StartupBoostUpdateResources = stringSliceModels(boost.UpdateResources)
	model.StartupBoostMinBoostDuration = stringOrNull(boost.MinBoostDuration)
	model.StartupBoostMinReadyDuration = stringOrNull(boost.MinReadyDuration)
	if value, ok := boost.ResourceMultipliers["cpu"]; ok {
		model.StartupBoostMultiplierCPU = types.StringValue(value)
	}
	if value, ok := boost.ResourceMultipliers["memory"]; ok {
		model.StartupBoostMultiplierMemory = types.StringValue(value)
	}
}
