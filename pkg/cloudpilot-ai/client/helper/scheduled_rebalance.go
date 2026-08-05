package helper

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type scheduledRebalanceClient interface {
	ListScheduledRebalances(clusterID string) (api.ScheduledRebalancePolicyList, error)
	CreateScheduledRebalance(clusterID string, request *api.ApplyScheduledRebalancePolicyRequest) (*api.ScheduledRebalancePolicy, error)
	UpdateScheduledRebalance(clusterID, policyID string, request *api.ApplyScheduledRebalancePolicyRequest) (*api.ScheduledRebalancePolicy, error)
	DeleteScheduledRebalance(clusterID, policyID string) error
}

type scheduledRebalanceReader interface {
	ListScheduledRebalances(clusterID string) (api.ScheduledRebalancePolicyList, error)
}

type scheduledRebalanceOperation struct {
	name    string
	base    api.ScheduledRebalancePolicy
	exists  bool
	request *api.ApplyScheduledRebalancePolicyRequest
}

type scheduledRebalanceDeletion struct {
	name     string
	policyID string
}

// ScheduledRebalancePlan contains a fully validated set of remote operations.
type ScheduledRebalancePlan struct {
	clusterID  string
	operations []scheduledRebalanceOperation
	deletions  []scheduledRebalanceDeletion
}

func PrepareScheduledRebalanceConfiguration(ctx context.Context, client scheduledRebalanceReader, clusterID string, desired customfield.NestedObjectList[api.ScheduledRebalancePolicyModel], previousNames map[string]struct{}) (*ScheduledRebalancePlan, error) {
	plan := &ScheduledRebalancePlan{clusterID: clusterID}
	if desired.IsNullOrUnknown() {
		return plan, nil
	}
	models, diags := desired.AsStructSliceT(ctx)
	if diags.HasError() {
		return nil, fmt.Errorf("scheduled_rebalances: %v", diags)
	}
	desiredNames := make(map[string]struct{}, len(models))
	for _, model := range models {
		name := model.Name.ValueString()
		if _, duplicate := desiredNames[name]; duplicate {
			return nil, fmt.Errorf("scheduled rebalance policy name %q is duplicated", name)
		}
		desiredNames[name] = struct{}{}
	}
	remoteList, err := client.ListScheduledRebalances(clusterID)
	if err != nil {
		if errors.Is(err, cloudpilotaiclient.ErrNotFound) && len(models) == 0 {
			return plan, nil
		}
		return nil, err
	}
	remoteByName := make(map[string]api.ScheduledRebalancePolicy, len(remoteList.ScheduledRebalancePolicies))
	for _, policy := range remoteList.ScheduledRebalancePolicies {
		remoteByName[policy.Name] = policy
	}
	plan.operations = make([]scheduledRebalanceOperation, 0, len(models))
	for _, model := range models {
		name := model.Name.ValueString()
		base, exists := remoteByName[name]
		var basePointer *api.ScheduledRebalancePolicy
		if exists {
			basePointer = &base
		}
		request, err := model.ToRequest(ctx, basePointer)
		if err != nil {
			return nil, fmt.Errorf("scheduled rebalance policy %q: %w", name, err)
		}
		plan.operations = append(plan.operations, scheduledRebalanceOperation{name: name, base: base, exists: exists, request: request})
	}
	for name := range previousNames {
		if _, keep := desiredNames[name]; keep {
			continue
		}
		policy, exists := remoteByName[name]
		if !exists {
			continue
		}
		plan.deletions = append(plan.deletions, scheduledRebalanceDeletion{name: name, policyID: policy.ID})
	}
	return plan, nil
}

func ApplyScheduledRebalanceConfiguration(client scheduledRebalanceClient, plan *ScheduledRebalancePlan) error {
	if plan == nil {
		return nil
	}
	for _, operation := range plan.operations {
		if operation.exists {
			if scheduledRebalanceRequestMatchesPolicy(operation.request, operation.base) {
				continue
			}
			if _, err := client.UpdateScheduledRebalance(plan.clusterID, operation.base.ID, operation.request); err != nil {
				return fmt.Errorf("update scheduled rebalance policy %q: %w", operation.name, err)
			}
		} else {
			if _, err := client.CreateScheduledRebalance(plan.clusterID, operation.request); err != nil {
				return fmt.Errorf("create scheduled rebalance policy %q: %w", operation.name, err)
			}
		}
	}
	for _, deletion := range plan.deletions {
		if err := client.DeleteScheduledRebalance(plan.clusterID, deletion.policyID); err != nil && !errors.Is(err, cloudpilotaiclient.ErrNotFound) {
			return fmt.Errorf("delete scheduled rebalance policy %q: %w", deletion.name, err)
		}
	}
	return nil
}

func SyncScheduledRebalanceConfiguration(ctx context.Context, client scheduledRebalanceClient, clusterID string, desired customfield.NestedObjectList[api.ScheduledRebalancePolicyModel], previousNames map[string]struct{}) error {
	plan, err := PrepareScheduledRebalanceConfiguration(ctx, client, clusterID, desired, previousNames)
	if err != nil {
		return err
	}
	return ApplyScheduledRebalanceConfiguration(client, plan)
}

func scheduledRebalanceRequestMatchesPolicy(request *api.ApplyScheduledRebalancePolicyRequest, policy api.ScheduledRebalancePolicy) bool {
	if request == nil || request.Enabled == nil {
		return false
	}
	timezoneMatches := request.Timezone == nil && policy.Timezone == ""
	if request.Timezone != nil {
		timezoneMatches = *request.Timezone == policy.Timezone
	}
	return request.Name == policy.Name &&
		request.Cron == policy.Cron &&
		timezoneMatches &&
		*request.Enabled == policy.Enabled &&
		scheduledRebalanceScopesEqual(request.Scope, policy.Scope) &&
		request.NodeConstraints == policy.NodeConstraints &&
		request.SelectionOrder == policy.SelectionOrder &&
		request.ForceDrain == policy.ForceDrain
}

func scheduledRebalanceScopesEqual(left, right api.ScheduledRebalanceScope) bool {
	return left.NodePoolName == right.NodePoolName &&
		slices.Equal(left.NodeNames, right.NodeNames) &&
		scheduledRebalanceSelectorTermsEqual(left.NodeSelectorTerms, right.NodeSelectorTerms) &&
		slices.Equal(left.ExcludeNodeNames, right.ExcludeNodeNames) &&
		scheduledRebalanceSelectorTermsEqual(left.ExcludeNodeSelectorTerms, right.ExcludeNodeSelectorTerms) &&
		slices.Equal(left.CapacityTypes, right.CapacityTypes)
}

func scheduledRebalanceSelectorTermsEqual(left, right []api.ScheduledRebalanceNodeSelectorTerm) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftExpressions := left[index].MatchExpressions
		rightExpressions := right[index].MatchExpressions
		if len(leftExpressions) != len(rightExpressions) {
			return false
		}
		for expressionIndex := range leftExpressions {
			leftExpression := leftExpressions[expressionIndex]
			rightExpression := rightExpressions[expressionIndex]
			if leftExpression.Key != rightExpression.Key ||
				leftExpression.Operator != rightExpression.Operator ||
				!slices.Equal(leftExpression.Values, rightExpression.Values) {
				return false
			}
		}
	}
	return true
}

func RefreshScheduledRebalances(ctx context.Context, client scheduledRebalanceReader, clusterID string, current customfield.NestedObjectList[api.ScheduledRebalancePolicyModel]) (customfield.NestedObjectList[api.ScheduledRebalancePolicyModel], error) {
	return hydrateScheduledRebalances(ctx, client, clusterID, current, false)
}

func HydrateScheduledRebalancesPostWrite(ctx context.Context, client scheduledRebalanceReader, clusterID string, current customfield.NestedObjectList[api.ScheduledRebalancePolicyModel]) (customfield.NestedObjectList[api.ScheduledRebalancePolicyModel], error) {
	return hydrateScheduledRebalances(ctx, client, clusterID, current, true)
}

func hydrateScheduledRebalances(ctx context.Context, client scheduledRebalanceReader, clusterID string, current customfield.NestedObjectList[api.ScheduledRebalancePolicyModel], preserveMissing bool) (customfield.NestedObjectList[api.ScheduledRebalancePolicyModel], error) {
	if current.IsNullOrUnknown() {
		return current, nil
	}
	state, diags := current.AsStructSliceT(ctx)
	if diags.HasError() {
		return current, fmt.Errorf("scheduled_rebalances: %v", diags)
	}
	remoteList, err := client.ListScheduledRebalances(clusterID)
	if err != nil {
		if errors.Is(err, cloudpilotaiclient.ErrNotFound) {
			if preserveMissing {
				return current, nil
			}
			remoteList = api.ScheduledRebalancePolicyList{}
		} else {
			return current, err
		}
	}
	remoteByName := make(map[string]api.ScheduledRebalancePolicy, len(remoteList.ScheduledRebalancePolicies))
	for _, policy := range remoteList.ScheduledRebalancePolicies {
		remoteByName[policy.Name] = policy
	}
	hydrated := make([]api.ScheduledRebalancePolicyModel, 0, len(state))
	for _, model := range state {
		if policy, ok := remoteByName[model.Name.ValueString()]; ok {
			hydrated = append(hydrated, api.ScheduledRebalancePolicyModelFromPolicy(ctx, policy, model))
		} else if preserveMissing {
			hydrated = append(hydrated, model)
		}
	}
	list, listDiags := customfield.NewObjectList(ctx, hydrated)
	if listDiags.HasError() {
		return current, fmt.Errorf("scheduled_rebalances: %v", listDiags)
	}
	return list, nil
}

func ImportScheduledRebalances(ctx context.Context, client scheduledRebalanceReader, clusterID string) (customfield.NestedObjectList[api.ScheduledRebalancePolicyModel], error) {
	remoteList, err := client.ListScheduledRebalances(clusterID)
	if err != nil && !errors.Is(err, cloudpilotaiclient.ErrNotFound) {
		return customfield.NullObjectList[api.ScheduledRebalancePolicyModel](ctx), err
	}
	if len(remoteList.ScheduledRebalancePolicies) == 0 {
		return customfield.NullObjectList[api.ScheduledRebalancePolicyModel](ctx), nil
	}
	sort.SliceStable(remoteList.ScheduledRebalancePolicies, func(i, j int) bool {
		return remoteList.ScheduledRebalancePolicies[i].Name < remoteList.ScheduledRebalancePolicies[j].Name
	})
	models := make([]api.ScheduledRebalancePolicyModel, 0, len(remoteList.ScheduledRebalancePolicies))
	for _, policy := range remoteList.ScheduledRebalancePolicies {
		models = append(models, api.ScheduledRebalancePolicyModelForImport(ctx, policy))
	}
	list, diags := customfield.NewObjectList(ctx, models)
	if diags.HasError() {
		return customfield.NullObjectList[api.ScheduledRebalancePolicyModel](ctx), fmt.Errorf("scheduled_rebalances: %v", diags)
	}
	return list, nil
}
