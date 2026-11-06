package api

import (
	"context"
	"fmt"
	"reflect"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func applyStringValue(target *string, value types.String) {
	if !value.IsNull() && !value.IsUnknown() {
		*target = value.ValueString()
	}
}

func applyStringMapValues(current map[string]string, values map[string]types.String) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		if value.IsNull() || value.IsUnknown() {
			if existing, ok := current[key]; ok {
				result[key] = existing
			}
		} else {
			result[key] = value.ValueString()
		}
	}
	return result
}

func matchingTargetReference(ctx context.Context, previous []TypedObjectReference, used []bool, target TargetRefModel) TypedObjectReference {
	first := -1
	selector, _ := labelSelectorModelToAPI(ctx, target.LabelSelector)
	for i, item := range previous {
		if used[i] || !targetReferenceMatches(item, target) {
			continue
		}
		if first == -1 {
			first = i
		}
		if !target.LabelSelector.IsNull() && !target.LabelSelector.IsUnknown() && reflect.DeepEqual(item.LabelSelector, selector) {
			first = i
			break
		}
	}
	if first < 0 {
		return TypedObjectReference{}
	}
	used[first] = true
	return previous[first]
}

func targetReferenceMatches(item TypedObjectReference, target TargetRefModel) bool {
	for _, field := range []struct {
		current string
		value   types.String
	}{
		{item.APIVersion, target.APIVersion}, {item.Kind, target.Kind},
		{item.Name, target.Name}, {item.Namespace, target.Namespace},
	} {
		if !field.value.IsNull() && !field.value.IsUnknown() && field.current != field.value.ValueString() {
			return false
		}
	}
	return true
}

func labelSelectorModelToAPIFromBase(ctx context.Context, model customfield.NestedObject[LabelSelectorModel], current *LabelSelector) (*LabelSelector, error) {
	if model.IsNull() || model.IsUnknown() {
		return current, nil
	}
	value, diags := model.Value(ctx)
	if diags.HasError() {
		return nil, fmt.Errorf("label_selector: %v", diags)
	}
	if value == nil {
		return current, nil
	}
	// A present empty object clears the selector; null leaves it unmanaged.
	if value.MatchLabels.IsNull() && value.MatchExpressions.IsNull() {
		return nil, nil
	}
	out := &LabelSelector{}
	if current != nil {
		*out = *current
	}
	if !value.MatchLabels.IsNull() && !value.MatchLabels.IsUnknown() {
		labels, labelDiags := value.MatchLabels.Value(ctx)
		if labelDiags.HasError() {
			return nil, fmt.Errorf("label_selector.match_labels: %v", labelDiags)
		}
		out.MatchLabels = applyStringMapValues(out.MatchLabels, labels)
	}
	if !value.MatchExpressions.IsNullOrUnknown() {
		expressions, exprDiags := value.MatchExpressions.AsStructSliceT(ctx)
		if exprDiags.HasError() {
			return nil, fmt.Errorf("label_selector.match_expressions: %v", exprDiags)
		}
		out.MatchExpressions = mergeLabelExpressions(out.MatchExpressions, expressions)
	}
	if len(out.MatchLabels) == 0 && len(out.MatchExpressions) == 0 {
		return nil, nil
	}
	return out, nil
}

func mergeLabelExpressions(previous []LabelSelectorRequirement, models []LabelSelectorRequirementModel) []LabelSelectorRequirement {
	result := make([]LabelSelectorRequirement, 0, len(models))
	used := make([]bool, len(previous))
	for _, model := range models {
		item := LabelSelectorRequirement{Key: model.Key.ValueString(), Operator: model.Operator.ValueString()}
		for i, existing := range previous {
			if !used[i] && existing.Key == item.Key && existing.Operator == item.Operator {
				item = existing
				used[i] = true
				break
			}
		}
		if model.Values != nil {
			item.Values = stringSliceValues(model.Values)
		}
		result = append(result, item)
	}
	return result
}
