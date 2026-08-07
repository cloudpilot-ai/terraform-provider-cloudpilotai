package api

import (
	"context"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/robfig/cron/v3"

	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

const (
	ScheduledRebalanceSelectionOrderOldestFirst            = "oldest_first"
	ScheduledRebalanceSelectionOrderNewestFirst            = "newest_first"
	ScheduledRebalanceSelectionOrderNameAsc                = "name_asc"
	ScheduledRebalanceSelectionOrderLowestUtilizationFirst = "lowest_utilization_first"
)

type ScheduledRebalanceLabelSelectorRequirement struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

type ScheduledRebalanceNodeSelectorTerm struct {
	MatchExpressions []ScheduledRebalanceLabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

type ScheduledRebalanceScope struct {
	NodePoolName             string                               `json:"nodePoolName,omitempty"`
	NodeNames                []string                             `json:"nodeNames,omitempty"`
	NodeSelectorTerms        []ScheduledRebalanceNodeSelectorTerm `json:"nodeSelectorTerms,omitempty"`
	ExcludeNodeNames         []string                             `json:"excludeNodeNames,omitempty"`
	ExcludeNodeSelectorTerms []ScheduledRebalanceNodeSelectorTerm `json:"excludeNodeSelectorTerms,omitempty"`
	CapacityTypes            []string                             `json:"capacityTypes,omitempty"`
}

type ScheduledRebalanceNodeConstraints struct {
	MinAgeSeconds  int64 `json:"minAgeSeconds,omitempty"`
	MaxNodes       int64 `json:"maxNodes,omitempty"`
	MinClusterSize int64 `json:"minClusterSize,omitempty"`
}

type ScheduledRebalancePolicy struct {
	ID              string                            `json:"id"`
	Name            string                            `json:"name"`
	Cron            string                            `json:"cron"`
	Timezone        string                            `json:"timezone"`
	Enabled         bool                              `json:"enabled"`
	Scope           ScheduledRebalanceScope           `json:"scope"`
	NodeConstraints ScheduledRebalanceNodeConstraints `json:"nodeConstraints"`
	SelectionOrder  string                            `json:"selectionOrder"`
	ForceDrain      bool                              `json:"forceDrain"`
}

type ScheduledRebalancePolicyList struct {
	ScheduledRebalancePolicies []ScheduledRebalancePolicy `json:"scheduledRebalancePolicies"`
}

type ApplyScheduledRebalancePolicyRequest struct {
	Name            string                            `json:"name"`
	Cron            string                            `json:"cron"`
	Timezone        *string                           `json:"timezone,omitempty"`
	Enabled         *bool                             `json:"enabled,omitempty"`
	Scope           ScheduledRebalanceScope           `json:"scope"`
	NodeConstraints ScheduledRebalanceNodeConstraints `json:"nodeConstraints"`
	SelectionOrder  string                            `json:"selectionOrder"`
	ForceDrain      bool                              `json:"forceDrain"`
}

type ScheduledRebalanceLabelSelectorRequirementModel struct {
	Key      types.String                   `tfsdk:"key"`
	Operator types.String                   `tfsdk:"operator"`
	Values   customfield.List[types.String] `tfsdk:"values"`
}

type ScheduledRebalanceNodeSelectorTermModel struct {
	MatchExpressions customfield.NestedObjectList[ScheduledRebalanceLabelSelectorRequirementModel] `tfsdk:"match_expressions"`
}

type ScheduledRebalanceScopeModel struct {
	NodePoolName             types.String                                                          `tfsdk:"node_pool_name"`
	NodeNames                customfield.List[types.String]                                        `tfsdk:"node_names"`
	NodeSelectorTerms        customfield.NestedObjectList[ScheduledRebalanceNodeSelectorTermModel] `tfsdk:"node_selector_terms"`
	ExcludeNodeNames         customfield.List[types.String]                                        `tfsdk:"exclude_node_names"`
	ExcludeNodeSelectorTerms customfield.NestedObjectList[ScheduledRebalanceNodeSelectorTermModel] `tfsdk:"exclude_node_selector_terms"`
	CapacityTypes            customfield.List[types.String]                                        `tfsdk:"capacity_types"`
}

type ScheduledRebalanceNodeConstraintsModel struct {
	MinAgeSeconds  types.Int64 `tfsdk:"min_age_seconds"`
	MaxNodes       types.Int64 `tfsdk:"max_nodes"`
	MinClusterSize types.Int64 `tfsdk:"min_cluster_size"`
}

type ScheduledRebalancePolicyModel struct {
	Name            types.String                                                     `tfsdk:"name"`
	Cron            types.String                                                     `tfsdk:"cron"`
	Timezone        types.String                                                     `tfsdk:"timezone"`
	Enabled         types.Bool                                                       `tfsdk:"enabled"`
	Scope           customfield.NestedObject[ScheduledRebalanceScopeModel]           `tfsdk:"scope"`
	NodeConstraints customfield.NestedObject[ScheduledRebalanceNodeConstraintsModel] `tfsdk:"node_constraints"`
	SelectionOrder  types.String                                                     `tfsdk:"selection_order"`
	ForceDrain      types.Bool                                                       `tfsdk:"force_drain"`
}

func (m ScheduledRebalancePolicyModel) ToRequest(ctx context.Context, base *ScheduledRebalancePolicy) (*ApplyScheduledRebalancePolicyRequest, error) {
	if base == nil && (m.Cron.IsNull() || m.Cron.IsUnknown() || strings.TrimSpace(m.Cron.ValueString()) == "") {
		return nil, fmt.Errorf("cron is required for a new scheduled rebalance policy")
	}
	request := &ApplyScheduledRebalancePolicyRequest{Name: m.Name.ValueString()}
	if base != nil {
		request.Cron = base.Cron
		if strings.TrimSpace(base.Timezone) != "" {
			request.Timezone = stringPointer(base.Timezone)
		}
		request.Enabled = boolPointer(base.Enabled)
		request.Scope = base.Scope
		request.NodeConstraints = base.NodeConstraints
		request.SelectionOrder = base.SelectionOrder
		request.ForceDrain = base.ForceDrain
	}
	if !m.Cron.IsNull() && !m.Cron.IsUnknown() {
		request.Cron = m.Cron.ValueString()
	}
	if !m.Timezone.IsNull() && !m.Timezone.IsUnknown() {
		request.Timezone = stringPointer(m.Timezone.ValueString())
	}
	if !m.Enabled.IsNull() && !m.Enabled.IsUnknown() {
		request.Enabled = boolPointer(m.Enabled.ValueBool())
	}
	if !m.SelectionOrder.IsNull() && !m.SelectionOrder.IsUnknown() {
		selectionOrder := m.SelectionOrder.ValueString()
		if selectionOrder != strings.TrimSpace(selectionOrder) {
			return nil, fmt.Errorf("selection_order cannot contain leading or trailing whitespace")
		}
		switch selectionOrder {
		case ScheduledRebalanceSelectionOrderOldestFirst,
			ScheduledRebalanceSelectionOrderNewestFirst,
			ScheduledRebalanceSelectionOrderNameAsc,
			ScheduledRebalanceSelectionOrderLowestUtilizationFirst:
			request.SelectionOrder = selectionOrder
		default:
			return nil, fmt.Errorf("unsupported selection_order %q", selectionOrder)
		}
	}
	if !m.ForceDrain.IsNull() && !m.ForceDrain.IsUnknown() {
		request.ForceDrain = m.ForceDrain.ValueBool()
	}
	if !m.Scope.IsNull() && !m.Scope.IsUnknown() {
		scope, diags := m.Scope.Value(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("scope: %v", diags)
		}
		if scope != nil {
			if err := overlayScheduledRebalanceScope(ctx, &request.Scope, *scope); err != nil {
				return nil, err
			}
		}
	}
	if !m.NodeConstraints.IsNull() && !m.NodeConstraints.IsUnknown() {
		constraints, diags := m.NodeConstraints.Value(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("node_constraints: %v", diags)
		}
		if constraints != nil {
			if !constraints.MinAgeSeconds.IsNull() && !constraints.MinAgeSeconds.IsUnknown() {
				request.NodeConstraints.MinAgeSeconds = constraints.MinAgeSeconds.ValueInt64()
			}
			if !constraints.MaxNodes.IsNull() && !constraints.MaxNodes.IsUnknown() {
				request.NodeConstraints.MaxNodes = constraints.MaxNodes.ValueInt64()
			}
			if !constraints.MinClusterSize.IsNull() && !constraints.MinClusterSize.IsUnknown() {
				request.NodeConstraints.MinClusterSize = constraints.MinClusterSize.ValueInt64()
			}
		}
	}
	if err := validateScheduledRebalanceRequest(request); err != nil {
		return nil, err
	}
	return request, nil
}

func validateScheduledRebalanceRequest(request *ApplyScheduledRebalancePolicyRequest) error {
	if request == nil {
		return fmt.Errorf("request is required")
	}
	if err := validateScheduledRebalanceText("name", request.Name); err != nil {
		return err
	}
	if err := validateScheduledRebalanceText("cron", request.Cron); err != nil {
		return err
	}
	if len(strings.Fields(request.Cron)) != 5 {
		return fmt.Errorf("invalid cron: expected exactly 5 fields")
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(request.Cron); err != nil {
		return fmt.Errorf("invalid cron: %w", err)
	}
	if request.Timezone != nil {
		if err := validateScheduledRebalanceText("timezone", *request.Timezone); err != nil {
			return err
		}
		if _, err := time.LoadLocation(*request.Timezone); err != nil {
			return fmt.Errorf("invalid timezone: %w", err)
		}
	}
	if request.NodeConstraints.MinAgeSeconds < 0 {
		return fmt.Errorf("node_constraints.min_age_seconds cannot be negative")
	}
	if request.NodeConstraints.MaxNodes < 0 {
		return fmt.Errorf("node_constraints.max_nodes cannot be negative")
	}
	if request.NodeConstraints.MinClusterSize < 0 {
		return fmt.Errorf("node_constraints.min_cluster_size cannot be negative")
	}
	if len(request.Scope.NodeNames) > 0 && (request.Scope.NodePoolName != "" || len(request.Scope.NodeSelectorTerms) > 0) {
		return fmt.Errorf("scope.node_names cannot be combined with scope.node_pool_name or scope.node_selector_terms")
	}
	if request.Scope.NodePoolName != "" {
		if err := validateScheduledRebalanceText("scope.node_pool_name", request.Scope.NodePoolName); err != nil {
			return err
		}
	}
	if err := validateScheduledRebalanceStringList("scope.node_names", request.Scope.NodeNames); err != nil {
		return err
	}
	if err := validateScheduledRebalanceStringList("scope.exclude_node_names", request.Scope.ExcludeNodeNames); err != nil {
		return err
	}
	if err := validateScheduledRebalanceStringList("scope.capacity_types", request.Scope.CapacityTypes); err != nil {
		return err
	}
	if err := validateScheduledRebalanceSelectorTerms("scope.node_selector_terms", request.Scope.NodeSelectorTerms); err != nil {
		return err
	}
	if err := validateScheduledRebalanceSelectorTerms("scope.exclude_node_selector_terms", request.Scope.ExcludeNodeSelectorTerms); err != nil {
		return err
	}
	return nil
}

func validateScheduledRebalanceStringList(field string, values []string) error {
	for index, value := range values {
		if err := validateScheduledRebalanceText(fmt.Sprintf("%s[%d]", field, index), value); err != nil {
			return err
		}
	}
	return nil
}

func validateScheduledRebalanceSelectorTerms(field string, terms []ScheduledRebalanceNodeSelectorTerm) error {
	for termIndex, term := range terms {
		for expressionIndex, expression := range term.MatchExpressions {
			prefix := fmt.Sprintf("%s[%d].match_expressions[%d]", field, termIndex, expressionIndex)
			if err := validateScheduledRebalanceText(prefix+".key", expression.Key); err != nil {
				return err
			}
			switch expression.Operator {
			case "Exists", "DoesNotExist":
				if len(expression.Values) > 0 {
					return fmt.Errorf("%s operator %s does not accept values", prefix, expression.Operator)
				}
			case "In", "NotIn":
				if len(expression.Values) == 0 {
					return fmt.Errorf("%s operator %s requires values", prefix, expression.Operator)
				}
			default:
				return fmt.Errorf("%s has unsupported operator %q", prefix, expression.Operator)
			}
			for valueIndex, value := range expression.Values {
				if err := validateScheduledRebalanceText(fmt.Sprintf("%s.values[%d]", prefix, valueIndex), value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateScheduledRebalanceText(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s is required", field)
	}
	if value != trimmed {
		return fmt.Errorf("%s cannot contain leading or trailing whitespace", field)
	}
	return nil
}

func overlayScheduledRebalanceScope(ctx context.Context, out *ScheduledRebalanceScope, model ScheduledRebalanceScopeModel) error {
	if !model.NodePoolName.IsNull() && !model.NodePoolName.IsUnknown() {
		out.NodePoolName = model.NodePoolName.ValueString()
	}
	var err error
	if out.NodeNames, err = scheduledStringList(ctx, model.NodeNames, out.NodeNames); err != nil {
		return fmt.Errorf("scope.node_names: %w", err)
	}
	if out.ExcludeNodeNames, err = scheduledStringList(ctx, model.ExcludeNodeNames, out.ExcludeNodeNames); err != nil {
		return fmt.Errorf("scope.exclude_node_names: %w", err)
	}
	if out.CapacityTypes, err = scheduledStringList(ctx, model.CapacityTypes, out.CapacityTypes); err != nil {
		return fmt.Errorf("scope.capacity_types: %w", err)
	}
	if out.NodeSelectorTerms, err = scheduledSelectorTerms(ctx, model.NodeSelectorTerms, out.NodeSelectorTerms); err != nil {
		return fmt.Errorf("scope.node_selector_terms: %w", err)
	}
	if out.ExcludeNodeSelectorTerms, err = scheduledSelectorTerms(ctx, model.ExcludeNodeSelectorTerms, out.ExcludeNodeSelectorTerms); err != nil {
		return fmt.Errorf("scope.exclude_node_selector_terms: %w", err)
	}
	return nil
}

func scheduledStringList(ctx context.Context, list customfield.List[types.String], current []string) ([]string, error) {
	if list.IsNullOrUnknown() {
		return current, nil
	}
	values, diags := list.Value(ctx)
	if diags.HasError() {
		return nil, fmt.Errorf("%v", diags)
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.ValueString())
	}
	return out, nil
}

func scheduledSelectorTerms(ctx context.Context, list customfield.NestedObjectList[ScheduledRebalanceNodeSelectorTermModel], current []ScheduledRebalanceNodeSelectorTerm) ([]ScheduledRebalanceNodeSelectorTerm, error) {
	if list.IsNullOrUnknown() {
		return current, nil
	}
	models, diags := list.AsStructSliceT(ctx)
	if diags.HasError() {
		return nil, fmt.Errorf("%v", diags)
	}
	out := make([]ScheduledRebalanceNodeSelectorTerm, 0, len(models))
	for termIndex, model := range models {
		term := ScheduledRebalanceNodeSelectorTerm{}
		if termIndex < len(current) {
			term = current[termIndex]
		}
		if !model.MatchExpressions.IsNullOrUnknown() {
			requirements, requirementDiags := model.MatchExpressions.AsStructSliceT(ctx)
			if requirementDiags.HasError() {
				return nil, fmt.Errorf("match_expressions: %v", requirementDiags)
			}
			term.MatchExpressions = make([]ScheduledRebalanceLabelSelectorRequirement, 0, len(requirements))
			for requirementIndex, requirement := range requirements {
				baseRequirement := ScheduledRebalanceLabelSelectorRequirement{}
				if termIndex < len(current) && requirementIndex < len(current[termIndex].MatchExpressions) {
					baseRequirement = current[termIndex].MatchExpressions[requirementIndex]
				}
				var currentValues []string
				if requirement.Key.ValueString() == baseRequirement.Key && requirement.Operator.ValueString() == baseRequirement.Operator {
					currentValues = baseRequirement.Values
				}
				values, err := scheduledStringList(ctx, requirement.Values, currentValues)
				if err != nil {
					return nil, err
				}
				term.MatchExpressions = append(term.MatchExpressions, ScheduledRebalanceLabelSelectorRequirement{Key: requirement.Key.ValueString(), Operator: requirement.Operator.ValueString(), Values: values})
			}
		}
		out = append(out, term)
	}
	return out, nil
}

func stringPointer(value string) *string { return &value }
func boolPointer(value bool) *bool       { return &value }

func ScheduledRebalancePolicyModelFromPolicy(ctx context.Context, policy ScheduledRebalancePolicy, state ScheduledRebalancePolicyModel) ScheduledRebalancePolicyModel {
	remote := scheduledRebalancePolicyModelFromPolicy(ctx, policy)
	remote.Cron = preserveScheduledString(state.Cron, remote.Cron)
	remote.Timezone = preserveScheduledString(state.Timezone, remote.Timezone)
	remote.Enabled = preserveScheduledBool(state.Enabled, remote.Enabled)
	remote.SelectionOrder = preserveScheduledString(state.SelectionOrder, remote.SelectionOrder)
	remote.ForceDrain = preserveScheduledBool(state.ForceDrain, remote.ForceDrain)
	remote.Scope = preserveScheduledScope(ctx, state.Scope, remote.Scope)
	remote.NodeConstraints = preserveScheduledConstraints(ctx, state.NodeConstraints, remote.NodeConstraints)
	return remote
}

func ScheduledRebalancePolicyModelForImport(ctx context.Context, policy ScheduledRebalancePolicy) ScheduledRebalancePolicyModel {
	return scheduledRebalancePolicyModelFromPolicy(ctx, policy)
}

func scheduledRebalancePolicyModelFromPolicy(ctx context.Context, policy ScheduledRebalancePolicy) ScheduledRebalancePolicyModel {
	timezone := types.StringValue(policy.Timezone)
	if strings.TrimSpace(policy.Timezone) == "" {
		timezone = types.StringNull()
	}
	selectionOrder := types.StringValue(policy.SelectionOrder)
	if strings.TrimSpace(policy.SelectionOrder) == "" {
		selectionOrder = types.StringNull()
	}
	return ScheduledRebalancePolicyModel{
		Name:     types.StringValue(policy.Name),
		Cron:     types.StringValue(policy.Cron),
		Timezone: timezone,
		Enabled:  types.BoolValue(policy.Enabled),
		Scope:    scheduledScopeModelFromAPI(ctx, policy.Scope),
		NodeConstraints: customfield.NewObjectMust(ctx, &ScheduledRebalanceNodeConstraintsModel{
			MinAgeSeconds:  types.Int64Value(policy.NodeConstraints.MinAgeSeconds),
			MaxNodes:       types.Int64Value(policy.NodeConstraints.MaxNodes),
			MinClusterSize: types.Int64Value(policy.NodeConstraints.MinClusterSize),
		}),
		SelectionOrder: selectionOrder,
		ForceDrain:     types.BoolValue(policy.ForceDrain),
	}
}

func scheduledScopeModelFromAPI(ctx context.Context, scope ScheduledRebalanceScope) customfield.NestedObject[ScheduledRebalanceScopeModel] {
	return customfield.NewObjectMust(ctx, &ScheduledRebalanceScopeModel{
		NodePoolName:             types.StringValue(scope.NodePoolName),
		NodeNames:                scheduledStringListFromAPI(ctx, scope.NodeNames),
		NodeSelectorTerms:        scheduledSelectorTermsFromAPI(ctx, scope.NodeSelectorTerms),
		ExcludeNodeNames:         scheduledStringListFromAPI(ctx, scope.ExcludeNodeNames),
		ExcludeNodeSelectorTerms: scheduledSelectorTermsFromAPI(ctx, scope.ExcludeNodeSelectorTerms),
		CapacityTypes:            scheduledStringListFromAPI(ctx, scope.CapacityTypes),
	})
}

func scheduledStringListFromAPI(ctx context.Context, values []string) customfield.List[types.String] {
	items := make([]types.String, 0, len(values))
	for _, value := range values {
		items = append(items, types.StringValue(value))
	}
	list, _ := customfield.NewList[types.String](ctx, items)
	return list
}

func scheduledSelectorTermsFromAPI(ctx context.Context, terms []ScheduledRebalanceNodeSelectorTerm) customfield.NestedObjectList[ScheduledRebalanceNodeSelectorTermModel] {
	models := make([]ScheduledRebalanceNodeSelectorTermModel, 0, len(terms))
	for _, term := range terms {
		requirements := make([]ScheduledRebalanceLabelSelectorRequirementModel, 0, len(term.MatchExpressions))
		for _, requirement := range term.MatchExpressions {
			requirements = append(requirements, ScheduledRebalanceLabelSelectorRequirementModel{
				Key: types.StringValue(requirement.Key), Operator: types.StringValue(requirement.Operator), Values: scheduledStringListFromAPI(ctx, requirement.Values),
			})
		}
		models = append(models, ScheduledRebalanceNodeSelectorTermModel{MatchExpressions: customfield.NewObjectListMust(ctx, requirements)})
	}
	return customfield.NewObjectListMust(ctx, models)
}

func preserveScheduledString(state, remote types.String) types.String {
	if state.IsNull() {
		return state
	}
	return remote
}
func preserveScheduledBool(state, remote types.Bool) types.Bool {
	if state.IsNull() {
		return state
	}
	return remote
}
func preserveScheduledInt64(state, remote types.Int64) types.Int64 {
	if state.IsNull() {
		return state
	}
	return remote
}

func preserveScheduledScope(ctx context.Context, state, remote customfield.NestedObject[ScheduledRebalanceScopeModel]) customfield.NestedObject[ScheduledRebalanceScopeModel] {
	if state.IsNull() || state.IsUnknown() || remote.IsNull() || remote.IsUnknown() {
		return state
	}
	stateValue, stateDiags := state.Value(ctx)
	remoteValue, remoteDiags := remote.Value(ctx)
	if stateDiags.HasError() || remoteDiags.HasError() || stateValue == nil || remoteValue == nil {
		return state
	}
	remoteValue.NodePoolName = preserveScheduledString(stateValue.NodePoolName, remoteValue.NodePoolName)
	if stateValue.NodeNames.IsNull() {
		remoteValue.NodeNames = stateValue.NodeNames
	}
	remoteValue.NodeSelectorTerms = preserveScheduledSelectorTerms(ctx, stateValue.NodeSelectorTerms, remoteValue.NodeSelectorTerms)
	if stateValue.ExcludeNodeNames.IsNull() {
		remoteValue.ExcludeNodeNames = stateValue.ExcludeNodeNames
	}
	remoteValue.ExcludeNodeSelectorTerms = preserveScheduledSelectorTerms(ctx, stateValue.ExcludeNodeSelectorTerms, remoteValue.ExcludeNodeSelectorTerms)
	if stateValue.CapacityTypes.IsNull() {
		remoteValue.CapacityTypes = stateValue.CapacityTypes
	}
	return customfield.NewObjectMust(ctx, remoteValue)
}

func preserveScheduledSelectorTerms(ctx context.Context, state, remote customfield.NestedObjectList[ScheduledRebalanceNodeSelectorTermModel]) customfield.NestedObjectList[ScheduledRebalanceNodeSelectorTermModel] {
	if state.IsNull() || state.IsUnknown() || remote.IsNull() || remote.IsUnknown() {
		return state
	}
	stateTerms, stateDiags := state.AsStructSliceT(ctx)
	remoteTerms, remoteDiags := remote.AsStructSliceT(ctx)
	if stateDiags.HasError() || remoteDiags.HasError() {
		return state
	}
	for termIndex := range stateTerms {
		if termIndex >= len(remoteTerms) {
			break
		}
		if stateTerms[termIndex].MatchExpressions.IsNull() {
			remoteTerms[termIndex].MatchExpressions = stateTerms[termIndex].MatchExpressions
			continue
		}
		stateRequirements, stateRequirementDiags := stateTerms[termIndex].MatchExpressions.AsStructSliceT(ctx)
		remoteRequirements, remoteRequirementDiags := remoteTerms[termIndex].MatchExpressions.AsStructSliceT(ctx)
		if stateRequirementDiags.HasError() || remoteRequirementDiags.HasError() {
			return state
		}
		for requirementIndex := range stateRequirements {
			if requirementIndex >= len(remoteRequirements) {
				break
			}
			if stateRequirements[requirementIndex].Values.IsNull() {
				remoteRequirements[requirementIndex].Values = stateRequirements[requirementIndex].Values
			}
		}
		remoteTerms[termIndex].MatchExpressions = customfield.NewObjectListMust(ctx, remoteRequirements)
	}
	return customfield.NewObjectListMust(ctx, remoteTerms)
}

func preserveScheduledConstraints(ctx context.Context, state, remote customfield.NestedObject[ScheduledRebalanceNodeConstraintsModel]) customfield.NestedObject[ScheduledRebalanceNodeConstraintsModel] {
	if state.IsNull() || state.IsUnknown() || remote.IsNull() || remote.IsUnknown() {
		return state
	}
	stateValue, stateDiags := state.Value(ctx)
	remoteValue, remoteDiags := remote.Value(ctx)
	if stateDiags.HasError() || remoteDiags.HasError() || stateValue == nil || remoteValue == nil {
		return state
	}
	remoteValue.MinAgeSeconds = preserveScheduledInt64(stateValue.MinAgeSeconds, remoteValue.MinAgeSeconds)
	remoteValue.MaxNodes = preserveScheduledInt64(stateValue.MaxNodes, remoteValue.MaxNodes)
	remoteValue.MinClusterSize = preserveScheduledInt64(stateValue.MinClusterSize, remoteValue.MinClusterSize)
	return customfield.NewObjectMust(ctx, remoteValue)
}
