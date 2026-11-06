package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	gcpcorev1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter/apis/v1"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
)

func (g *GCENodePool) ToGCENodePoolModel(ctx context.Context) (*GCENodePoolModel, error) {
	if g == nil {
		return nil, nil
	}

	model := &GCENodePoolModel{
		Name:                   types.StringValue(g.Name),
		Enable:                 types.BoolValue(g.Enable),
		EnableImageAccelerator: types.BoolValue(g.EnableImageAccelerator),
		Labels:                 customfield.NullMap[types.String](ctx),
		Taints:                 customfield.NullObjectList[TaintModel](ctx),
		OriginNodePoolJSON:     types.StringNull(),
	}
	model.JobNodePoolOptimization = jobOptimizationModelFromAPI(ctx, g.JobNodePoolOptimization)
	if g.NodePoolSpec == nil {
		return model, nil
	}

	if g.NodePoolSpec.Template.Spec.NodeClassRef != nil && g.NodePoolSpec.Template.Spec.NodeClassRef.Name != "" {
		model.NodeClass = types.StringValue(g.NodePoolSpec.Template.Spec.NodeClassRef.Name)
	}
	model.DisruptionCooldown = durationModel(g.NodePoolSpec.Disruption.DisruptionCooldown)
	model.EnableGPU = gcpEnableGPUToBoolByKey(g.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceGPUCount)
	if g.NodePoolSpec.Weight != nil {
		model.ProvisionPriority = types.Int32Value(*g.NodePoolSpec.Weight)
	} else {
		model.ProvisionPriority = types.Int32Null()
	}
	model.InstanceFamily = gcpRequirementsToStrings(g.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceFamily, corev1.NodeSelectorOpIn)
	model.InstanceArch = gcpRequirementsToStrings(g.NodePoolSpec.Template.Spec.Requirements, corev1.LabelArchStable, corev1.NodeSelectorOpIn)
	model.CapacityType = gcpRequirementsToStrings(g.NodePoolSpec.Template.Spec.Requirements, gcpcorev1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn)
	model.Zone = gcpRequirementsToStringsByKeys(g.NodePoolSpec.Template.Spec.Requirements, corev1.NodeSelectorOpIn, corev1.LabelTopologyZone, gceLegacyLabelTopologyZoneID)

	var err error
	model.InstanceCPUMAX, err = gcpRequirementsToOptionalInt64(g.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceCPU, corev1.NodeSelectorOpLt)
	if err != nil {
		return nil, err
	}
	model.InstanceCPUMIN, err = gcpRequirementsToOptionalInt64(g.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceCPU, corev1.NodeSelectorOpGt)
	if err != nil {
		return nil, err
	}
	model.InstanceMemoryMAX, err = gcpRequirementsToOptionalInt64(g.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceMemory, corev1.NodeSelectorOpLt)
	if err != nil {
		return nil, err
	}
	model.InstanceMemoryMIN, err = gcpRequirementsToOptionalInt64(g.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceMemory, corev1.NodeSelectorOpGt)
	if err != nil {
		return nil, err
	}
	if len(g.NodePoolSpec.Template.ObjectMeta.Labels) > 0 {
		model.Labels = stringMapToTerraformMap(ctx, g.NodePoolSpec.Template.ObjectMeta.Labels)
	}
	if len(g.NodePoolSpec.Template.Spec.Taints) > 0 {
		model.Taints = customfield.NewObjectListMust(ctx, lo.Map(g.NodePoolSpec.Template.Spec.Taints, func(taint corev1.Taint, _ int) TaintModel {
			return TaintModel{
				Key:    types.StringValue(taint.Key),
				Value:  types.StringValue(taint.Value),
				Effect: types.StringValue(string(taint.Effect)),
			}
		}))
	}
	if len(g.NodePoolSpec.Disruption.Budgets) > 0 {
		model.NodeDisruptionLimit = types.StringValue(g.NodePoolSpec.Disruption.Budgets[0].Nodes)
	}
	if len(g.NodePoolSpec.Disruption.Budgets) > 0 {
		budgetModels := make([]DisruptionBudgetModel, 0, len(g.NodePoolSpec.Disruption.Budgets))
		for _, budget := range g.NodePoolSpec.Disruption.Budgets {
			budgetModels = append(budgetModels, newDisruptionBudgetModel(
				budget.Nodes,
				lo.Map(budget.Reasons, func(reason gcpcorev1.DisruptionReason, _ int) string { return string(reason) }),
				budget.Schedule,
				budget.Duration,
			))
		}
		model.NodeDisruptionBudgets = customfield.NewObjectListMust(ctx, budgetModels)
	} else {
		model.NodeDisruptionBudgets = customfield.NullObjectList[DisruptionBudgetModel](ctx)
	}
	if consolidateAfter, ok := gcpNillableDurationToString(g.NodePoolSpec.Disruption.ConsolidateAfter); ok {
		model.NodeDisruptionDelay = types.StringValue(consolidateAfter)
	}

	return model, nil
}

func (m *GCENodePoolModel) ToGCENodePool(ctx context.Context, current GCENodePool) (*GCENodePool, error) {
	if !m.OriginNodePoolJSON.IsNull() && !m.OriginNodePoolJSON.IsUnknown() && m.OriginNodePoolJSON.ValueString() != "" {
		var raw GCENodePool
		if err := json.Unmarshal([]byte(m.OriginNodePoolJSON.ValueString()), &raw); err != nil {
			return nil, err
		}
		raw.originJSON = true
		return &raw, nil
	}
	out := current
	out.originJSON = false
	if current.NodePoolSpec != nil {
		out.NodePoolSpec = current.NodePoolSpec.DeepCopy()
	}
	out.Name = m.Name.ValueString()
	if out.NodePoolSpec == nil {
		enableGPU := false
		if !m.EnableGPU.IsNull() && !m.EnableGPU.IsUnknown() {
			enableGPU = m.EnableGPU.ValueBool()
		}
		out.NodePoolSpec = enableGPUGCPNodePoolSpec(nil, enableGPU)
	}
	if !m.Enable.IsNull() && !m.Enable.IsUnknown() {
		out.Enable = m.Enable.ValueBool()
	}
	if !m.EnableImageAccelerator.IsNull() && !m.EnableImageAccelerator.IsUnknown() {
		out.EnableImageAccelerator = m.EnableImageAccelerator.ValueBool()
	}
	if !m.EnableGPU.IsNull() && !m.EnableGPU.IsUnknown() {
		out.NodePoolSpec = enableGPUGCPNodePoolSpec(out.NodePoolSpec, m.EnableGPU.ValueBool())
	}
	if out.NodePoolSpec.Template.Spec.NodeClassRef == nil {
		out.NodePoolSpec.Template.Spec.NodeClassRef = &gcpcorev1.NodeClassReference{
			Group: gceNodeClassRefGroup,
			Kind:  gceNodeClassRefKind,
		}
	}
	out.NodePoolSpec.Template.Spec.NodeClassRef.Group = gceNodeClassRefGroup
	out.NodePoolSpec.Template.Spec.NodeClassRef.Kind = gceNodeClassRefKind
	if !m.NodeClass.IsNull() && !m.NodeClass.IsUnknown() {
		nodeClassName := strings.TrimSpace(m.NodeClass.ValueString())
		if nodeClassName == "" {
			return nil, fmt.Errorf("nodepool %s must reference a valid nodeclass", out.Name)
		}
		out.NodePoolSpec.Template.Spec.NodeClassRef.Name = nodeClassName
	}
	if strings.TrimSpace(out.NodePoolSpec.Template.Spec.NodeClassRef.Name) == "" {
		return nil, fmt.Errorf("nodepool %s must reference a valid nodeclass", out.Name)
	}
	m.applyGCEPoolRequirements(&out)
	if !m.NodeDisruptionBudgets.IsNullOrUnknown() {
		budgets, diags := m.NodeDisruptionBudgets.AsStructSliceT(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("node_disruption_budgets: %v", diags)
		}
		if err := validateDisruptionBudgetCount(len(budgets)); err != nil {
			return nil, err
		}
		if !m.NodeDisruptionLimit.IsNull() && !m.NodeDisruptionLimit.IsUnknown() &&
			m.NodeDisruptionLimit.ValueString() != budgets[0].Nodes.ValueString() {
			return nil, fmt.Errorf("node_disruption_limit must match node_disruption_budgets[0].nodes when both are configured")
		}
		out.NodePoolSpec.Disruption.Budgets = make([]gcpcorev1.Budget, 0, len(budgets))
		for i, budget := range budgets {
			values, err := budget.values(ctx)
			if err != nil {
				return nil, fmt.Errorf("node_disruption_budgets[%d]: %w", i, err)
			}
			if len(values.reasons) > maxGCPDisruptionBudgetReasons {
				return nil, fmt.Errorf("node_disruption_budgets[%d]: reasons must contain at most %d values", i, maxGCPDisruptionBudgetReasons)
			}
			out.NodePoolSpec.Disruption.Budgets = append(out.NodePoolSpec.Disruption.Budgets, gcpcorev1.Budget{
				Nodes:    values.nodes,
				Reasons:  lo.Map(values.reasons, func(reason string, _ int) gcpcorev1.DisruptionReason { return gcpcorev1.DisruptionReason(reason) }),
				Schedule: values.schedule,
				Duration: values.duration,
			})
		}
	} else if !m.NodeDisruptionLimit.IsNull() && !m.NodeDisruptionLimit.IsUnknown() {
		ensureFirstGCPBudget(out.NodePoolSpec).Nodes = m.NodeDisruptionLimit.ValueString()
	}
	if !m.NodeDisruptionDelay.IsNull() && !m.NodeDisruptionDelay.IsUnknown() {
		out.NodePoolSpec.Disruption.ConsolidateAfter = gcpcorev1.MustParseNillableDuration(m.NodeDisruptionDelay.ValueString())
	}
	if err := applyGCPNodePoolLabels(ctx, out.NodePoolSpec, m.Labels); err != nil {
		return nil, err
	}
	if err := applyGCPNodePoolTaints(ctx, out.NodePoolSpec, m.Taints); err != nil {
		return nil, err
	}
	if err := applyGCENodePoolPolicies(ctx, &out, m); err != nil {
		return nil, err
	}
	return &out, nil
}

func gcpRequirementsToStringsByKeys(requirements []gcpcorev1.NodeSelectorRequirementWithMinValues, operator corev1.NodeSelectorOperator, keys ...string) *[]types.String {
	for _, key := range keys {
		if values := gcpRequirementsToStrings(requirements, key, operator); values != nil {
			return values
		}
	}
	return nil
}

func gcpEnableGPUToBoolByKey(requirements []gcpcorev1.NodeSelectorRequirementWithMinValues, key string) types.Bool {
	_, found := lo.Find(requirements, func(r gcpcorev1.NodeSelectorRequirementWithMinValues) bool {
		return r.Key == key && r.Operator == corev1.NodeSelectorOpExists
	})
	return lo.Ternary(found, types.BoolValue(true), types.BoolValue(false))
}

func defaultGCPNodePoolSpec() *gcpcorev1.NodePoolSpec {
	return &gcpcorev1.NodePoolSpec{
		Template: gcpcorev1.NodeClaimTemplate{
			ObjectMeta: gcpcorev1.ObjectMeta{
				Labels: map[string]string{
					CloudPilotManagedNodeLabelKey: "true",
				},
			},
			Spec: gcpcorev1.NodeClaimTemplateSpec{
				Requirements: []gcpcorev1.NodeSelectorRequirementWithMinValues{
					{
						Key:      corev1.LabelArchStable,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"amd64"},
					},
					{
						Key:      corev1.LabelOSStable,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"linux"},
					},
					{
						Key:      gcpcorev1.CapacityTypeLabelKey,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"spot", "on-demand"},
					},
				},
				NodeClassRef: &gcpcorev1.NodeClassReference{
					Group: gceNodeClassRefGroup,
					Kind:  gceNodeClassRefKind,
					Name:  DefaultNodeClassName,
				},
				ExpireAfter: gcpcorev1.MustParseNillableDuration("Never"),
			},
		},
		Disruption: gcpcorev1.Disruption{
			ConsolidationPolicy: gcpcorev1.ConsolidationPolicyWhenEmptyOrUnderutilized,
			ConsolidateAfter:    gcpcorev1.MustParseNillableDuration("0s"),
			Budgets: []gcpcorev1.Budget{{
				Nodes: "10%",
			}},
		},
	}
}

func enableGPUGCPNodePoolSpec(nodePoolSpec *gcpcorev1.NodePoolSpec, enableGPU bool) *gcpcorev1.NodePoolSpec {
	if nodePoolSpec == nil {
		nodePoolSpec = defaultGCPNodePoolSpec()
	}

	_, index, found := lo.FindIndexOf(nodePoolSpec.Template.Spec.Requirements, func(req gcpcorev1.NodeSelectorRequirementWithMinValues) bool {
		return req.Key == gceLabelInstanceGPUCount
	})

	operator := corev1.NodeSelectorOpDoesNotExist
	if enableGPU {
		operator = corev1.NodeSelectorOpExists
	}

	if found {
		nodePoolSpec.Template.Spec.Requirements[index].Operator = operator
		nodePoolSpec.Template.Spec.Requirements[index].Values = nil
		return nodePoolSpec
	}

	nodePoolSpec.Template.Spec.Requirements = append(nodePoolSpec.Template.Spec.Requirements, gcpcorev1.NodeSelectorRequirementWithMinValues{
		Key:      gceLabelInstanceGPUCount,
		Operator: operator,
	})
	return nodePoolSpec
}

func ensureFirstGCPBudget(nodePoolSpec *gcpcorev1.NodePoolSpec) *gcpcorev1.Budget {
	if len(nodePoolSpec.Disruption.Budgets) == 0 {
		nodePoolSpec.Disruption.Budgets = []gcpcorev1.Budget{{}}
	}
	return &nodePoolSpec.Disruption.Budgets[0]
}

func applyGCPNodePoolLabels(ctx context.Context, nodePoolSpec *gcpcorev1.NodePoolSpec, labels customfield.Map[types.String]) error {
	if labels.IsNull() || labels.IsUnknown() {
		return nil
	}
	values, diags := labels.Value(ctx)
	if diags.HasError() {
		return fmt.Errorf("labels: %v", diags)
	}
	nodePoolSpec.Template.ObjectMeta.Labels = map[string]string{}
	for key, value := range values {
		nodePoolSpec.Template.ObjectMeta.Labels[key] = value.ValueString()
	}
	if _, ok := nodePoolSpec.Template.ObjectMeta.Labels[CloudPilotManagedNodeLabelKey]; !ok {
		nodePoolSpec.Template.ObjectMeta.Labels[CloudPilotManagedNodeLabelKey] = "true"
	}
	return nil
}

func applyGCPNodePoolTaints(ctx context.Context, nodePoolSpec *gcpcorev1.NodePoolSpec, taints customfield.NestedObjectList[TaintModel]) error {
	if taints.IsNullOrUnknown() {
		return nil
	}
	values, diags := taints.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("taints: %v", diags)
	}
	nodePoolSpec.Template.Spec.Taints = lo.Map(values, func(taint TaintModel, _ int) corev1.Taint {
		return corev1.Taint{
			Key:    taint.Key.ValueString(),
			Value:  taint.Value.ValueString(),
			Effect: corev1.TaintEffect(taint.Effect.ValueString()),
		}
	})
	return nil
}

func gcpRequirementsToStrings(requirements []gcpcorev1.NodeSelectorRequirementWithMinValues, key string, operator corev1.NodeSelectorOperator) *[]types.String {
	v, found := lo.Find(requirements, func(r gcpcorev1.NodeSelectorRequirementWithMinValues) bool {
		return r.Key == key && r.Operator == operator
	})
	if !found {
		return nil
	}

	values := make([]types.String, len(v.Values))
	for i, value := range v.Values {
		values[i] = types.StringValue(value)
	}
	return &values
}

func gcpRequirementsToOptionalInt64(requirements []gcpcorev1.NodeSelectorRequirementWithMinValues, key string, operator corev1.NodeSelectorOperator) (types.Int64, error) {
	v, found := lo.Find(requirements, func(r gcpcorev1.NodeSelectorRequirementWithMinValues) bool {
		return r.Key == key && r.Operator == operator
	})
	if !found || len(v.Values) == 0 {
		return types.Int64Null(), nil
	}

	intValue, err := strconv.ParseInt(v.Values[0], 10, 64)
	if err != nil {
		return types.Int64Null(), err
	}
	return types.Int64Value(intValue), nil
}

func gcpUpdateRequirements(key string, operator corev1.NodeSelectorOperator, values []string, requirements []gcpcorev1.NodeSelectorRequirementWithMinValues) []gcpcorev1.NodeSelectorRequirementWithMinValues {
	_, index, found := lo.FindIndexOf(requirements, func(item gcpcorev1.NodeSelectorRequirementWithMinValues) bool {
		return item.Key == key && item.Operator == operator
	})

	if found {
		if len(values) == 0 {
			requirements = append(requirements[:index], requirements[index+1:]...)
			return requirements
		}

		requirements[index].Values = values
		return requirements
	}

	if len(values) == 0 {
		return requirements
	}

	requirements = append(requirements, gcpcorev1.NodeSelectorRequirementWithMinValues{
		Key:      key,
		Operator: operator,
		Values:   values,
	})
	return requirements
}

func gcpNillableDurationToString(value gcpcorev1.NillableDuration) (string, bool) {
	if len(value.Raw) > 0 {
		var raw string
		if err := json.Unmarshal(value.Raw, &raw); err == nil && raw != "" {
			if raw == gcpcorev1.Never {
				return gcpcorev1.Never, true
			}
			return NormalizeDuration(raw), true
		}
	}
	if value.Duration != nil {
		return NormalizeDuration(value.Duration.String()), true
	}
	return gcpcorev1.Never, true
}

func (m *GCENodePoolModel) applyGCEPoolRequirements(out *GCENodePool) {
	if !m.ProvisionPriority.IsNull() && !m.ProvisionPriority.IsUnknown() {
		weight := m.ProvisionPriority.ValueInt32()
		out.NodePoolSpec.Weight = &weight
	}
	if m.InstanceFamily != nil {
		values := lo.Map(*m.InstanceFamily, func(item types.String, _ int) string { return item.ValueString() })
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gceLabelInstanceFamily, corev1.NodeSelectorOpIn, values, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if m.InstanceArch != nil {
		values := lo.Map(*m.InstanceArch, func(item types.String, _ int) string { return item.ValueString() })
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(corev1.LabelArchStable, corev1.NodeSelectorOpIn, values, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if m.CapacityType != nil {
		values := lo.Map(*m.CapacityType, func(item types.String, _ int) string { return item.ValueString() })
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gcpcorev1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, values, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if m.Zone != nil {
		values := lo.Map(*m.Zone, func(item types.String, _ int) string { return item.ValueString() })
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, values, out.NodePoolSpec.Template.Spec.Requirements)
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gceLegacyLabelTopologyZoneID, corev1.NodeSelectorOpIn, nil, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if !m.InstanceCPUMAX.IsNull() && !m.InstanceCPUMAX.IsUnknown() {
		values := []string{strconv.FormatInt(m.InstanceCPUMAX.ValueInt64(), 10)}
		if m.InstanceCPUMAX.ValueInt64() == 0 {
			values = nil
		}
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gceLabelInstanceCPU, corev1.NodeSelectorOpLt, values, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if !m.InstanceCPUMIN.IsNull() && !m.InstanceCPUMIN.IsUnknown() {
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gceLabelInstanceCPU, corev1.NodeSelectorOpGt, []string{strconv.FormatInt(m.InstanceCPUMIN.ValueInt64(), 10)}, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if !m.InstanceMemoryMAX.IsNull() && !m.InstanceMemoryMAX.IsUnknown() {
		values := []string{strconv.FormatInt(m.InstanceMemoryMAX.ValueInt64(), 10)}
		if m.InstanceMemoryMAX.ValueInt64() == 0 {
			values = nil
		}
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gceLabelInstanceMemory, corev1.NodeSelectorOpLt, values, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if !m.InstanceMemoryMIN.IsNull() && !m.InstanceMemoryMIN.IsUnknown() {
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gceLabelInstanceMemory, corev1.NodeSelectorOpGt, []string{strconv.FormatInt(m.InstanceMemoryMIN.ValueInt64(), 10)}, out.NodePoolSpec.Template.Spec.Requirements)
	}

}
