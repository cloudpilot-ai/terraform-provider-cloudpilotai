package api

import "github.com/hashicorp/terraform-plugin-framework/types"

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
	JVMMinHeapXms              types.String `tfsdk:"jvm_min_heap_xms"`
	JVMMaxHeapXms              types.String `tfsdk:"jvm_max_heap_xms"`
	JVMMinHeapXmx              types.String `tfsdk:"jvm_min_heap_xmx"`
	JVMMaxHeapXmx              types.String `tfsdk:"jvm_max_heap_xmx"`
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
		applyStringValue(&rp.Spec.EvaluationPeriod, m.EvaluationPeriod)
		applyStringValue(&rp.Spec.HistoryWindowDuration.CPU, m.HistoryWindowCPU)
		applyStringValue(&rp.Spec.HistoryWindowDuration.Memory, m.HistoryWindowMemory)
	} else {
		rp.Spec.StrategyPercentile = &StrategyPercentileConfiguration{
			CPU:    95,
			Memory: 95,
		}
	}
	if !m.StrategyType.IsNull() && !m.StrategyType.IsUnknown() && m.StrategyType.ValueString() != "" {
		rp.Spec.StrategyType = m.StrategyType.ValueString()
	}

	if !m.PercentileCPU.IsNull() && !m.PercentileCPU.IsUnknown() || !m.PercentileMemory.IsNull() && !m.PercentileMemory.IsUnknown() {
		percentile := StrategyPercentileConfiguration{CPU: 95, Memory: 95}
		if rp.Spec.StrategyPercentile != nil {
			percentile = *rp.Spec.StrategyPercentile
		}
		if !m.PercentileCPU.IsNull() && !m.PercentileCPU.IsUnknown() {
			percentile.CPU = m.PercentileCPU.ValueInt32()
		}
		if !m.PercentileMemory.IsNull() && !m.PercentileMemory.IsUnknown() {
			percentile.Memory = m.PercentileMemory.ValueInt32()
		}
		rp.Spec.StrategyPercentile = &percentile
	}

	m.applyRecommendationBuffer(rp)
	m.applyRecommendationLimits(rp)
	m.applyJVMConfiguration(rp)

	return rp
}

func (m *RecommendationPolicyModel) applyRecommendationBuffer(rp *RecommendationPolicyResource) {
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
}

func (m *RecommendationPolicyModel) applyRecommendationLimits(rp *RecommendationPolicyResource) {
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
}

func (m *RecommendationPolicyModel) applyJVMConfiguration(rp *RecommendationPolicyResource) {
	jvm := &JVMRecommendationConfiguration{}
	if rp.Spec.JVM != nil {
		*jvm = *rp.Spec.JVM
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
	if !m.JVMMinHeapXms.IsNull() && !m.JVMMinHeapXms.IsUnknown() {
		if m.JVMMinHeapXms.ValueString() == "" {
			jvm.MinHeapXms = nil
		} else {
			v := m.JVMMinHeapXms.ValueString()
			jvm.MinHeapXms = &v
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
	applyOptionalString(&jvm.MaxHeapXms, m.JVMMaxHeapXms)
	applyOptionalString(&jvm.MinHeapXmx, m.JVMMinHeapXmx)
	applyOptionalString(&jvm.MaxHeapXmx, m.JVMMaxHeapXmx)

	if jvm.HeapBuffer != nil || jvm.MinHeapXms != nil || jvm.MinHeapXmsRatioOfMemory != nil || jvm.RecentNonHeapWindow != nil || jvm.HeapUsedPercentile != nil || jvm.MaxHeapXms != nil || jvm.MinHeapXmx != nil || jvm.MaxHeapXmx != nil || rp.Spec.JVM != nil {
		rp.Spec.JVM = jvm
	} else {
		rp.Spec.JVM = nil
	}
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
		m.JVMMaxHeapXms = optionalStringValue(rp.Spec.JVM.MaxHeapXms)
		m.JVMMinHeapXmx = optionalStringValue(rp.Spec.JVM.MinHeapXmx)
		m.JVMMaxHeapXmx = optionalStringValue(rp.Spec.JVM.MaxHeapXmx)
		if rp.Spec.JVM.HeapBuffer != nil {
			m.JVMHeapBuffer = types.StringValue(*rp.Spec.JVM.HeapBuffer)
		}
		if rp.Spec.JVM.MinHeapXmsRatioOfMemory != nil {
			m.JVMMinHeapXmsRatioOfMemory = types.StringValue(*rp.Spec.JVM.MinHeapXmsRatioOfMemory)
		}
		if rp.Spec.JVM.MinHeapXms != nil {
			m.JVMMinHeapXms = types.StringValue(*rp.Spec.JVM.MinHeapXms)
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
