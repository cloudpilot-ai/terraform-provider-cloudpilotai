package workloadautoscaler

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/statevalue"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func hydratePostWriteState(ctx context.Context, client postWriteStateHydratorClient, clusterID string, data *WorkloadAutoscalerModel) error {
	conf, err := client.GetWAConfiguration(clusterID)
	if err != nil {
		return fmt.Errorf("failed to get Workload Autoscaler configuration: %w", err)
	}
	mergeWAConfigurationFromAPI(data, conf, false)

	rps, err := client.ListRecommendationPolicies(clusterID)
	if err != nil {
		return fmt.Errorf("failed to list recommendation policies: %w", err)
	}
	if err := hydrateRecommendationPoliciesPostWrite(ctx, data, rps); err != nil {
		return err
	}

	aps, err := client.ListAutoscalingPolicies(clusterID)
	if err != nil {
		return fmt.Errorf("failed to list autoscaling policies: %w", err)
	}
	if err := hydrateAutoscalingPoliciesPostWrite(ctx, data, aps); err != nil {
		return err
	}

	return nil
}

func hydrateRecommendationPoliciesPostWrite(ctx context.Context, data *WorkloadAutoscalerModel, rps []api.RecommendationPolicyResource) error {
	if data.RecommendationPolicies.IsNullOrUnknown() {
		return nil
	}

	stateRPs, diags := data.RecommendationPolicies.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("recommendation policies diagnostics = %v", diags)
	}

	rpByName := make(map[string]api.RecommendationPolicyModel, len(rps))
	for i := range rps {
		rpByName[rps[i].Name] = preserveRecommendationPolicyStateRepresentation(
			api.RecommendationPolicyModelFromResource(&rps[i]),
			findRecommendationPolicyStateModel(stateRPs, rps[i].Name),
		)
	}

	ordered := orderByState(stateRPs, rpByName, func(m api.RecommendationPolicyModel) string {
		return m.Name.ValueString()
	})
	data.RecommendationPolicies = customfield.NewObjectListMust(ctx, ordered)
	return nil
}

func hydrateAutoscalingPoliciesPostWrite(ctx context.Context, data *WorkloadAutoscalerModel, aps []api.AutoscalingPolicyResource) error {
	return mergeAutoscalingPoliciesFromAPI(ctx, data, aps, false)
}

func findAutoscalingPolicyStateModel(items []api.AutoscalingPolicyModel, name string) api.AutoscalingPolicyModel {
	for _, item := range items {
		if item.Name.ValueString() == name {
			return item
		}
	}
	return api.AutoscalingPolicyModel{}
}

func findRecommendationPolicyStateModel(items []api.RecommendationPolicyModel, name string) api.RecommendationPolicyModel {
	for _, item := range items {
		if item.Name.ValueString() == name {
			return item
		}
	}
	return api.RecommendationPolicyModel{}
}

func mergeAutoscalingPoliciesFromAPI(ctx context.Context, data *WorkloadAutoscalerModel, aps []api.AutoscalingPolicyResource, importAll bool) error {
	if data.AutoscalingPolicies.IsNullOrUnknown() {
		if importAll && len(aps) > 0 {
			allAPs := make([]api.AutoscalingPolicyModel, 0, len(aps))
			for i := range aps {
				allAPs = append(allAPs, api.AutoscalingPolicyModelFromResource(ctx, &aps[i]))
			}
			data.AutoscalingPolicies = customfield.NewObjectListMust(ctx, allAPs)
		}
		return nil
	}

	stateAPs, diags := data.AutoscalingPolicies.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("autoscaling policies diagnostics = %v", diags)
	}

	apByName := make(map[string]api.AutoscalingPolicyModel, len(aps))
	for i := range aps {
		model := api.AutoscalingPolicyModelFromResource(ctx, &aps[i])
		if !importAll {
			model = preserveAutoscalingPolicyStateRepresentation(ctx, model, findAutoscalingPolicyStateModel(stateAPs, aps[i].Name))
		}
		apByName[aps[i].Name] = model
	}

	ordered := orderByState(stateAPs, apByName, func(m api.AutoscalingPolicyModel) string {
		return m.Name.ValueString()
	})
	list, diags := customfield.NewObjectList(ctx, ordered)
	if diags.HasError() {
		return fmt.Errorf("autoscaling policies diagnostics = %v", diags)
	}
	data.AutoscalingPolicies = list
	return nil
}

func preserveRecommendationPolicyStateRepresentation(remote, state api.RecommendationPolicyModel) api.RecommendationPolicyModel {
	remote.StrategyType = preserveManagedString(state.StrategyType, remote.StrategyType)
	remote.PercentileCPU = preserveManagedInt32(state.PercentileCPU, remote.PercentileCPU)
	remote.PercentileMemory = preserveManagedInt32(state.PercentileMemory, remote.PercentileMemory)
	remote.BufferCPU = preserveExplicitEmptyString(state.BufferCPU, remote.BufferCPU)
	remote.BufferMemory = preserveExplicitEmptyString(state.BufferMemory, remote.BufferMemory)
	remote.RequestMinCPU = preserveExplicitEmptyString(state.RequestMinCPU, remote.RequestMinCPU)
	remote.RequestMinMemory = preserveExplicitEmptyString(state.RequestMinMemory, remote.RequestMinMemory)
	remote.RequestMaxCPU = preserveExplicitEmptyString(state.RequestMaxCPU, remote.RequestMaxCPU)
	remote.RequestMaxMemory = preserveExplicitEmptyString(state.RequestMaxMemory, remote.RequestMaxMemory)
	remote.JVMHeapBuffer = preserveExplicitEmptyString(state.JVMHeapBuffer, remote.JVMHeapBuffer)
	remote.JVMMinHeapXmsRatioOfMemory = preserveExplicitEmptyString(state.JVMMinHeapXmsRatioOfMemory, remote.JVMMinHeapXmsRatioOfMemory)
	remote.JVMMinHeapXms = preserveHeapBound(state.JVMMinHeapXms, remote.JVMMinHeapXms)
	remote.JVMMaxHeapXms = preserveHeapBound(state.JVMMaxHeapXms, remote.JVMMaxHeapXms)
	remote.JVMMinHeapXmx = preserveHeapBound(state.JVMMinHeapXmx, remote.JVMMinHeapXmx)
	remote.JVMMaxHeapXmx = preserveHeapBound(state.JVMMaxHeapXmx, remote.JVMMaxHeapXmx)
	remote.JVMRecentNonHeapWindow = preserveManagedDuration(state.JVMRecentNonHeapWindow, remote.JVMRecentNonHeapWindow)
	remote.JVMHeapUsedPercentile = preserveManagedInt32(state.JVMHeapUsedPercentile, remote.JVMHeapUsedPercentile)
	return remote
}

func preserveAutoscalingPolicyStateRepresentation(ctx context.Context, remote, state api.AutoscalingPolicyModel) api.AutoscalingPolicyModel {
	remote.Enable = preserveManagedBool(state.Enable, remote.Enable)
	remote.Priority = preserveManagedInt64(state.Priority, remote.Priority)
	remote.DisableRuntimeOptimization = preserveManagedBool(state.DisableRuntimeOptimization, remote.DisableRuntimeOptimization)
	remote.UpdateResources = preserveManagedStringList(state.UpdateResources, remote.UpdateResources)
	remote.DriftThresholdCPU = preserveExplicitEmptyString(state.DriftThresholdCPU, remote.DriftThresholdCPU)
	remote.DriftThresholdMemory = preserveExplicitEmptyString(state.DriftThresholdMemory, remote.DriftThresholdMemory)
	remote.OnPolicyRemoval = preserveManagedString(state.OnPolicyRemoval, remote.OnPolicyRemoval)
	remote.TargetRefs = preserveTargetRefsStateRepresentation(ctx, state.TargetRefs, remote.TargetRefs)
	remote.UpdateSchedules = preserveUpdateSchedulesStateRepresentation(ctx, state.UpdateSchedules, remote.UpdateSchedules)
	remote.LimitPolicies = preserveLimitPoliciesStateRepresentation(ctx, state.LimitPolicies, remote.LimitPolicies)
	remote.StartupBoostRecommendationPolicyName = preserveExplicitEmptyString(state.StartupBoostRecommendationPolicyName, remote.StartupBoostRecommendationPolicyName)
	remote.StartupBoostUpdateResources = preserveManagedStringList(state.StartupBoostUpdateResources, remote.StartupBoostUpdateResources)
	remote.StartupBoostEnabled = preserveOptionalFalse(state.StartupBoostEnabled, remote.StartupBoostEnabled)
	remote.StartupBoostMinBoostDuration = preserveManagedDuration(state.StartupBoostMinBoostDuration, remote.StartupBoostMinBoostDuration)
	remote.StartupBoostMinReadyDuration = preserveManagedDuration(state.StartupBoostMinReadyDuration, remote.StartupBoostMinReadyDuration)
	remote.StartupBoostMultiplierCPU = preserveExplicitEmptyString(state.StartupBoostMultiplierCPU, remote.StartupBoostMultiplierCPU)
	remote.StartupBoostMultiplierMemory = preserveExplicitEmptyString(state.StartupBoostMultiplierMemory, remote.StartupBoostMultiplierMemory)
	remote.InPlaceFallbackDefaultPolicy = preserveManagedString(state.InPlaceFallbackDefaultPolicy, remote.InPlaceFallbackDefaultPolicy)
	remote.InPlaceFallbackReasonPolicies = preserveManagedStringMap(ctx, state.InPlaceFallbackReasonPolicies, remote.InPlaceFallbackReasonPolicies)
	return remote
}

// orderByState returns server items that are tracked in state, preserving
// state order. Items on the server but NOT in state are ignored — Terraform
// only manages resources it declared.
func orderByState[T any](stateItems []T, serverByName map[string]T, getName func(T) string) []T {
	result := make([]T, 0, len(stateItems))
	for _, stateItem := range stateItems {
		name := getName(stateItem)
		if serverItem, ok := serverByName[name]; ok {
			result = append(result, serverItem)
		}
	}
	return result
}

func mergeWAConfigurationFromAPI(data *WorkloadAutoscalerModel, conf *api.WAConfiguration, importAll bool) {
	if conf == nil {
		return
	}

	if importAll || !data.EnableNewWorkloadsProactiveUpdate.IsNull() {
		if conf.EnableNewWorkloadsProactiveUpdate != nil {
			data.EnableNewWorkloadsProactiveUpdate = types.BoolValue(*conf.EnableNewWorkloadsProactiveUpdate)
		} else {
			data.EnableNewWorkloadsProactiveUpdate = types.BoolNull()
		}
	}
	if importAll || !data.LimiterQuotaPerWindow.IsNull() {
		if conf.LimiterQuotaPerWindow != nil {
			data.LimiterQuotaPerWindow = types.Int64Value(int64(*conf.LimiterQuotaPerWindow))
		} else {
			data.LimiterQuotaPerWindow = types.Int64Null()
		}
	}
	if importAll || !data.LimiterBurst.IsNull() {
		if conf.LimiterBurst != nil {
			data.LimiterBurst = types.Int64Value(int64(*conf.LimiterBurst))
		} else {
			data.LimiterBurst = types.Int64Null()
		}
	}
	if importAll || !data.LimiterWindowSeconds.IsNull() {
		if conf.LimiterWindowSeconds != nil {
			data.LimiterWindowSeconds = types.Int64Value(int64(*conf.LimiterWindowSeconds))
		} else {
			data.LimiterWindowSeconds = types.Int64Null()
		}
	}
	if importAll || !data.EnablePreemptedPodGC.IsNull() {
		if conf.EnablePreemptedPodGC != nil {
			data.EnablePreemptedPodGC = types.BoolValue(*conf.EnablePreemptedPodGC)
		} else {
			data.EnablePreemptedPodGC = types.BoolNull()
		}
	}
	if importAll || !data.PreemptedPodGCTTL.IsNull() {
		if conf.PreemptedPodGCTTL != nil {
			data.PreemptedPodGCTTL = types.StringValue(*conf.PreemptedPodGCTTL)
		} else {
			data.PreemptedPodGCTTL = types.StringNull()
		}
	}
	if importAll || !data.EnableInitialOptimizationDataWindowCheck.IsNull() {
		if conf.EnableInitialOptimizationDataWindowCheck != nil {
			data.EnableInitialOptimizationDataWindowCheck = types.BoolValue(*conf.EnableInitialOptimizationDataWindowCheck)
		} else {
			data.EnableInitialOptimizationDataWindowCheck = types.BoolNull()
		}
	}
}

func isManagedBool(value types.Bool) bool {
	return !value.IsNull()
}

func isManagedInt64(value types.Int64) bool {
	return !value.IsNull()
}

func isManagedString(value types.String) bool {
	return !value.IsNull()
}

func boolValueOrDefault(value types.Bool, fallback bool) bool {
	if !isManagedBool(value) {
		return fallback
	}
	return value.ValueBool()
}

func stringValueOrDefault(value types.String, fallback string) string {
	if !isManagedString(value) {
		return fallback
	}
	return value.ValueString()
}

func stringValue(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}

func managedBoolChanged(plan, state types.Bool) bool {
	if !isManagedBool(plan) {
		return false
	}
	if !isManagedBool(state) {
		return true
	}
	return plan.ValueBool() != state.ValueBool()
}

func managedStringChanged(plan, state types.String) bool {
	if !isManagedString(plan) {
		return false
	}
	if !isManagedString(state) {
		return true
	}
	return plan.ValueString() != state.ValueString()
}

func preserveManagedBool(stateVal, remoteVal types.Bool) types.Bool {
	if stateVal.IsNull() {
		return types.BoolNull()
	}
	return remoteVal
}

func preserveOptionalFalse(stateVal, remoteVal types.Bool) types.Bool {
	if stateVal.IsNull() || stateVal.IsUnknown() {
		return types.BoolNull()
	}
	if !stateVal.ValueBool() && (remoteVal.IsNull() || remoteVal.IsUnknown()) {
		return stateVal
	}
	return remoteVal
}

func preserveManagedInt64(stateVal, remoteVal types.Int64) types.Int64 {
	if stateVal.IsNull() {
		return types.Int64Null()
	}
	return remoteVal
}

func preserveManagedInt32(stateVal, remoteVal types.Int32) types.Int32 {
	if stateVal.IsNull() {
		return types.Int32Null()
	}
	return remoteVal
}

func preserveManagedString(stateVal, remoteVal types.String) types.String {
	if stateVal.IsNull() {
		return types.StringNull()
	}
	return remoteVal
}

func preserveExplicitEmptyString(stateVal, remoteVal types.String) types.String {
	if stateVal.IsNull() {
		return types.StringNull()
	}
	if !stateVal.IsUnknown() && stateVal.ValueString() == "" && remoteVal.IsNull() {
		return stateVal
	}
	return remoteVal
}

func preserveHeapBound(stateVal, remoteVal types.String) types.String {
	return statevalue.PreserveQuantity(stateVal, preserveExplicitEmptyString(stateVal, remoteVal))
}

func preserveManagedDuration(stateVal, remoteVal types.String) types.String {
	if stateVal.IsNull() || stateVal.IsUnknown() {
		return types.StringNull()
	}
	if remoteVal.IsNull() || remoteVal.IsUnknown() {
		return preserveExplicitEmptyString(stateVal, remoteVal)
	}

	stateDuration, stateErr := time.ParseDuration(stateVal.ValueString())
	remoteDuration, remoteErr := time.ParseDuration(remoteVal.ValueString())
	if stateVal.ValueString() == "" && remoteErr == nil && remoteDuration == 0 {
		return stateVal
	}
	if stateErr == nil && remoteErr == nil && stateDuration == remoteDuration {
		return stateVal
	}
	return remoteVal
}

func mergeNestedObjectsByState[T any](
	stateItems, remoteItems []T,
	key func(T) string,
	preserve func(remote, state T) T,
) []T {
	result := make([]T, 0, len(remoteItems))
	matchedRemote := make([]bool, len(remoteItems))
	for _, stateItem := range stateItems {
		stateKey := key(stateItem)
		for i, remoteItem := range remoteItems {
			if matchedRemote[i] || key(remoteItem) != stateKey {
				continue
			}
			result = append(result, preserve(remoteItem, stateItem))
			matchedRemote[i] = true
			break
		}
	}
	for i, remoteItem := range remoteItems {
		if !matchedRemote[i] {
			result = append(result, remoteItem)
		}
	}
	return result
}

func preserveTargetRefsStateRepresentation(
	ctx context.Context,
	stateVal customfield.NestedObjectList[api.TargetRefModel],
	remoteVal customfield.NestedObjectList[api.TargetRefModel],
) customfield.NestedObjectList[api.TargetRefModel] {
	if stateVal.IsNull() {
		return customfield.NullObjectList[api.TargetRefModel](ctx)
	}
	if remoteVal.IsNullOrUnknown() {
		return preserveManagedObjectList(ctx, stateVal, remoteVal)
	}

	stateItems, stateDiags := stateVal.AsStructSliceT(ctx)
	remoteItems, remoteDiags := remoteVal.AsStructSliceT(ctx)
	if stateDiags.HasError() || remoteDiags.HasError() {
		return remoteVal
	}

	result := make([]api.TargetRefModel, 0, len(remoteItems))
	used := make([]bool, len(remoteItems))
	for _, stateItem := range stateItems {
		index := matchingTargetRefStateIndex(ctx, stateItem, remoteItems, used)
		if index < 0 {
			continue
		}
		remoteItem := remoteItems[index]
		remoteItem.Name = preserveExplicitEmptyString(stateItem.Name, remoteItem.Name)
		remoteItem.Namespace = preserveExplicitEmptyString(stateItem.Namespace, remoteItem.Namespace)
		remoteItem.LabelSelector = preserveLabelSelectorStateRepresentation(ctx, stateItem.LabelSelector, remoteItem.LabelSelector)
		result = append(result, remoteItem)
		used[index] = true
	}
	for i, item := range remoteItems {
		if !used[i] {
			result = append(result, item)
		}
	}
	return customfield.NewObjectListMust(ctx, result)
}

func matchingTargetRefStateIndex(ctx context.Context, state api.TargetRefModel, remote []api.TargetRefModel, used []bool) int {
	first := -1
	selector, selectorOK := labelSelectorSemanticValue(ctx, state.LabelSelector)
	for i, item := range remote {
		if used[i] || state.APIVersion.ValueString() != item.APIVersion.ValueString() || state.Kind.ValueString() != item.Kind.ValueString() {
			continue
		}
		if !state.Name.IsNull() && !state.Name.IsUnknown() && state.Name.ValueString() != item.Name.ValueString() {
			continue
		}
		if !state.Namespace.IsNull() && !state.Namespace.IsUnknown() && state.Namespace.ValueString() != item.Namespace.ValueString() {
			continue
		}
		if first < 0 {
			first = i
		}
		other, otherOK := labelSelectorSemanticValue(ctx, item.LabelSelector)
		if selectorOK && otherOK && !state.LabelSelector.IsNull() && reflect.DeepEqual(selector, other) {
			return i
		}
	}
	return first
}

func preserveLabelSelectorStateRepresentation(
	ctx context.Context,
	stateVal customfield.NestedObject[api.LabelSelectorModel],
	remoteVal customfield.NestedObject[api.LabelSelectorModel],
) customfield.NestedObject[api.LabelSelectorModel] {
	if stateVal.IsNull() || stateVal.IsUnknown() {
		return customfield.NullObject[api.LabelSelectorModel](ctx)
	}
	stateSelector, stateOK := labelSelectorSemanticValue(ctx, stateVal)
	remoteSelector, remoteOK := labelSelectorSemanticValue(ctx, remoteVal)
	if stateOK && remoteOK && reflect.DeepEqual(stateSelector, remoteSelector) {
		return stateVal
	}
	state, stateDiags := stateVal.Value(ctx)
	remote, remoteDiags := remoteVal.Value(ctx)
	if stateDiags.HasError() || remoteDiags.HasError() || state == nil {
		return remoteVal
	}
	if state.MatchLabels.IsNull() && state.MatchExpressions.IsNull() {
		// An empty selector manages the whole object, so retain remote drift.
		return remoteVal
	}
	if remote == nil {
		remote = &api.LabelSelectorModel{}
	}
	remote.MatchLabels = preserveManagedStringMap(ctx, state.MatchLabels, remote.MatchLabels)
	if state.MatchExpressions.IsNullOrUnknown() {
		remote.MatchExpressions = customfield.NullObjectList[api.LabelSelectorRequirementModel](ctx)
	} else if !remote.MatchExpressions.IsNullOrUnknown() {
		stateItems, stateDiags := state.MatchExpressions.AsStructSliceT(ctx)
		remoteItems, remoteDiags := remote.MatchExpressions.AsStructSliceT(ctx)
		if !stateDiags.HasError() && !remoteDiags.HasError() {
			items := mergeNestedObjectsByState(stateItems, remoteItems,
				func(item api.LabelSelectorRequirementModel) string {
					return item.Key.ValueString() + "\x00" + item.Operator.ValueString()
				},
				func(remote, state api.LabelSelectorRequirementModel) api.LabelSelectorRequirementModel {
					remote.Values = preserveManagedStringList(state.Values, remote.Values)
					return remote
				})
			remote.MatchExpressions = customfield.NewObjectListMust(ctx, items)
		}
	}
	return customfield.NewObjectMust(ctx, remote)
}

func labelSelectorSemanticValue(
	ctx context.Context,
	value customfield.NestedObject[api.LabelSelectorModel],
) (api.LabelSelector, bool) {
	result := api.LabelSelector{}
	if value.IsNull() {
		return result, true
	}
	if value.IsUnknown() {
		return result, false
	}
	model, diags := value.Value(ctx)
	if diags.HasError() || model == nil {
		return result, false
	}

	labels, diags := model.MatchLabels.Value(ctx)
	if diags.HasError() {
		return result, false
	}
	if len(labels) > 0 {
		result.MatchLabels = make(map[string]string, len(labels))
		for key, labelValue := range labels {
			result.MatchLabels[key] = labelValue.ValueString()
		}
	}

	expressions, diags := model.MatchExpressions.AsStructSliceT(ctx)
	if diags.HasError() {
		return result, false
	}
	for _, expression := range expressions {
		item := api.LabelSelectorRequirement{
			Key:      expression.Key.ValueString(),
			Operator: expression.Operator.ValueString(),
		}
		if expression.Values != nil && len(*expression.Values) > 0 {
			item.Values = make([]string, 0, len(*expression.Values))
			for _, expressionValue := range *expression.Values {
				item.Values = append(item.Values, expressionValue.ValueString())
			}
		}
		result.MatchExpressions = append(result.MatchExpressions, item)
	}
	return result, true
}

func preserveUpdateSchedulesStateRepresentation(
	ctx context.Context,
	stateVal customfield.NestedObjectList[api.UpdateScheduleModel],
	remoteVal customfield.NestedObjectList[api.UpdateScheduleModel],
) customfield.NestedObjectList[api.UpdateScheduleModel] {
	if stateVal.IsNull() {
		return customfield.NullObjectList[api.UpdateScheduleModel](ctx)
	}
	if remoteVal.IsNullOrUnknown() {
		return preserveManagedObjectList(ctx, stateVal, remoteVal)
	}

	stateItems, stateDiags := stateVal.AsStructSliceT(ctx)
	remoteItems, remoteDiags := remoteVal.AsStructSliceT(ctx)
	if stateDiags.HasError() || remoteDiags.HasError() {
		return remoteVal
	}

	result := mergeNestedObjectsByState(
		stateItems,
		remoteItems,
		func(item api.UpdateScheduleModel) string { return item.Name.ValueString() },
		func(remoteItem, stateItem api.UpdateScheduleModel) api.UpdateScheduleModel {
			remoteItem.Schedule = preserveExplicitEmptyString(stateItem.Schedule, remoteItem.Schedule)
			remoteItem.Duration = preserveManagedDuration(stateItem.Duration, remoteItem.Duration)
			remoteItem.Mode = preserveManagedString(stateItem.Mode, remoteItem.Mode)
			return remoteItem
		},
	)
	return customfield.NewObjectListMust(ctx, result)
}

func preserveLimitPoliciesStateRepresentation(
	ctx context.Context,
	stateVal customfield.NestedObjectList[api.LimitPolicyModel],
	remoteVal customfield.NestedObjectList[api.LimitPolicyModel],
) customfield.NestedObjectList[api.LimitPolicyModel] {
	if stateVal.IsNull() {
		return customfield.NullObjectList[api.LimitPolicyModel](ctx)
	}
	if remoteVal.IsNullOrUnknown() {
		return preserveManagedObjectList(ctx, stateVal, remoteVal)
	}

	stateItems, stateDiags := stateVal.AsStructSliceT(ctx)
	remoteItems, remoteDiags := remoteVal.AsStructSliceT(ctx)
	if stateDiags.HasError() || remoteDiags.HasError() {
		return remoteVal
	}

	result := mergeNestedObjectsByState(
		stateItems,
		remoteItems,
		func(item api.LimitPolicyModel) string { return item.Resource.ValueString() },
		func(remoteItem, stateItem api.LimitPolicyModel) api.LimitPolicyModel {
			remoteItem.RemoveLimit = preserveOptionalFalse(stateItem.RemoveLimit, remoteItem.RemoveLimit)
			remoteItem.KeepLimit = preserveOptionalFalse(stateItem.KeepLimit, remoteItem.KeepLimit)
			remoteItem.Multiplier = preserveExplicitEmptyString(stateItem.Multiplier, remoteItem.Multiplier)
			remoteItem.AutoHeadroom = preserveExplicitEmptyString(stateItem.AutoHeadroom, remoteItem.AutoHeadroom)
			return remoteItem
		},
	)
	return customfield.NewObjectListMust(ctx, result)
}

func preserveManagedStringList(stateVal, remoteVal *[]types.String) *[]types.String {
	if stateVal == nil {
		return nil
	}
	if remoteVal == nil && len(*stateVal) == 0 {
		return stateVal
	}
	return remoteVal
}

func preserveManagedStringMap(ctx context.Context, stateVal, remoteVal customfield.Map[types.String]) customfield.Map[types.String] {
	if stateVal.IsNull() || stateVal.IsUnknown() {
		return customfield.NullMap[types.String](ctx)
	}
	stateValues, stateDiags := stateVal.Value(ctx)
	if stateDiags.HasError() {
		return remoteVal
	}
	if remoteVal.IsNull() || remoteVal.IsUnknown() {
		if len(stateValues) == 0 {
			return stateVal
		}
	}
	remoteValues, remoteDiags := remoteVal.Value(ctx)
	if remoteDiags.HasError() {
		return remoteVal
	}
	values := make(map[string]types.String, len(remoteValues))
	for key, value := range remoteValues {
		values[key] = value
	}
	for key, value := range stateValues {
		if value.IsNull() {
			values[key] = types.StringNull()
		}
	}
	if len(values) == 0 {
		return remoteVal
	}
	return customfield.NewMapMust[types.String](ctx, values)
}

func preserveManagedObjectList[T any](ctx context.Context, stateVal, remoteVal customfield.NestedObjectList[T]) customfield.NestedObjectList[T] {
	if stateVal.IsNull() {
		return customfield.NullObjectList[T](ctx)
	}
	if remoteVal.IsNullOrUnknown() {
		values, diags := stateVal.AsStructSliceT(ctx)
		if !diags.HasError() && len(values) == 0 {
			return stateVal
		}
	}
	return remoteVal
}
