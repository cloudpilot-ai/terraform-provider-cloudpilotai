package api

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func TestScheduledSelectorTermsInheritsValuesOnlyForSameRequirement(t *testing.T) {
	ctx := context.Background()
	current := []ScheduledRebalanceNodeSelectorTerm{{
		MatchExpressions: []ScheduledRebalanceLabelSelectorRequirement{{
			Key: "zone", Operator: "In", Values: []string{"us-central1-a"},
		}},
	}}
	tests := []struct {
		name       string
		key        string
		operator   string
		wantValues []string
	}{
		{name: "same identity", key: "zone", operator: "In", wantValues: []string{"us-central1-a"}},
		{name: "changed key", key: "team", operator: "In"},
		{name: "changed operator", key: "zone", operator: "Exists"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			models := customfield.NewObjectListMust(ctx, []ScheduledRebalanceNodeSelectorTermModel{{
				MatchExpressions: customfield.NewObjectListMust(ctx, []ScheduledRebalanceLabelSelectorRequirementModel{{
					Key: types.StringValue(test.key), Operator: types.StringValue(test.operator), Values: customfield.NullList[types.String](ctx),
				}}),
			}})

			terms, err := scheduledSelectorTerms(ctx, models, current)
			if err != nil {
				t.Fatalf("scheduledSelectorTerms() error = %v", err)
			}
			if got := terms[0].MatchExpressions[0].Values; !slices.Equal(got, test.wantValues) {
				t.Fatalf("values = %#v, want %#v", got, test.wantValues)
			}
		})
	}
}

func TestValidateScheduledRebalanceRequestRejectsServerInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ApplyScheduledRebalancePolicyRequest)
		want   string
	}{
		{name: "malformed cron", mutate: func(request *ApplyScheduledRebalancePolicyRequest) { request.Cron = "61 2 * * *" }, want: "invalid cron"},
		{name: "invalid timezone", mutate: func(request *ApplyScheduledRebalancePolicyRequest) { request.Timezone = stringPointer("Mars/Olympus") }, want: "invalid timezone"},
		{name: "negative minimum age", mutate: func(request *ApplyScheduledRebalancePolicyRequest) { request.NodeConstraints.MinAgeSeconds = -1 }, want: "min_age_seconds"},
		{name: "conflicting node names", mutate: func(request *ApplyScheduledRebalancePolicyRequest) {
			request.Scope.NodeNames = []string{"node-1"}
			request.Scope.NodePoolName = "general"
		}, want: "cannot be combined"},
		{name: "invalid selector values", mutate: func(request *ApplyScheduledRebalancePolicyRequest) {
			request.Scope.NodeSelectorTerms = []ScheduledRebalanceNodeSelectorTerm{{MatchExpressions: []ScheduledRebalanceLabelSelectorRequirement{{Key: "team", Operator: "Exists", Values: []string{"platform"}}}}}
		}, want: "does not accept values"},
		{name: "padded node name", mutate: func(request *ApplyScheduledRebalancePolicyRequest) { request.Scope.NodeNames = []string{" node-1"} }, want: "whitespace"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := &ApplyScheduledRebalancePolicyRequest{Name: "nightly", Cron: "0 2 * * *", Timezone: stringPointer("UTC")}
			test.mutate(request)
			if err := validateScheduledRebalanceRequest(request); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateScheduledRebalanceRequest() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestScheduledRebalancePolicyModelRejectsPaddedSelectionOrder(t *testing.T) {
	model := ScheduledRebalancePolicyModel{
		Name:           types.StringValue("nightly"),
		Cron:           types.StringValue("0 2 * * *"),
		SelectionOrder: types.StringValue(" oldest_first "),
	}
	if _, err := model.ToRequest(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "whitespace") {
		t.Fatalf("ToRequest() error = %v, want whitespace error", err)
	}
}

func TestScheduledRebalancePolicyModelKeepsAbsentTimezoneUnmanaged(t *testing.T) {
	ctx := context.Background()
	policy := ScheduledRebalancePolicy{Name: "nightly", Cron: "0 2 * * *", Enabled: true}
	model := ScheduledRebalancePolicyModelForImport(ctx, policy)
	if !model.Timezone.IsNull() {
		t.Fatalf("Timezone = %#v, want null", model.Timezone)
	}

	request, err := model.ToRequest(ctx, &policy)
	if err != nil {
		t.Fatalf("ToRequest() error = %v", err)
	}
	if request.Timezone != nil {
		t.Fatalf("request.Timezone = %q, want nil", *request.Timezone)
	}
}

func TestScheduledRebalancePolicyModelPreservesLargeNodeConstraints(t *testing.T) {
	ctx := context.Background()
	const largeNodeCount int64 = 1 << 32
	model := ScheduledRebalancePolicyModel{
		Name: types.StringValue("nightly"),
		Cron: types.StringValue("0 2 * * *"),
		NodeConstraints: customfield.NewObjectMust(ctx, &ScheduledRebalanceNodeConstraintsModel{
			MaxNodes:       types.Int64Value(largeNodeCount),
			MinClusterSize: types.Int64Value(largeNodeCount),
		}),
	}

	request, err := model.ToRequest(ctx, nil)
	if err != nil {
		t.Fatalf("ToRequest() error = %v", err)
	}
	if request.NodeConstraints.MaxNodes != largeNodeCount || request.NodeConstraints.MinClusterSize != largeNodeCount {
		t.Fatalf("NodeConstraints = %#v, want large values preserved", request.NodeConstraints)
	}
}
