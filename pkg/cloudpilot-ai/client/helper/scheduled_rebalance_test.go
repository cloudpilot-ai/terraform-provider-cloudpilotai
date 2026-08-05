package helper

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type fakeScheduledRebalanceClient struct {
	policies []api.ScheduledRebalancePolicy
	created  []*api.ApplyScheduledRebalancePolicyRequest
	updated  []*api.ApplyScheduledRebalancePolicyRequest
	deleted  []string
	listed   int
}

func (f *fakeScheduledRebalanceClient) ListScheduledRebalances(string) (api.ScheduledRebalancePolicyList, error) {
	f.listed++
	return api.ScheduledRebalancePolicyList{ScheduledRebalancePolicies: f.policies}, nil
}

func (f *fakeScheduledRebalanceClient) CreateScheduledRebalance(_ string, request *api.ApplyScheduledRebalancePolicyRequest) (*api.ScheduledRebalancePolicy, error) {
	f.created = append(f.created, request)
	return &api.ScheduledRebalancePolicy{Name: request.Name}, nil
}

func (f *fakeScheduledRebalanceClient) UpdateScheduledRebalance(_, _ string, request *api.ApplyScheduledRebalancePolicyRequest) (*api.ScheduledRebalancePolicy, error) {
	f.updated = append(f.updated, request)
	return &api.ScheduledRebalancePolicy{Name: request.Name}, nil
}

func (f *fakeScheduledRebalanceClient) DeleteScheduledRebalance(_, policyID string) error {
	f.deleted = append(f.deleted, policyID)
	return nil
}

func TestSyncScheduledRebalanceNullIsUnmanaged(t *testing.T) {
	ctx := context.Background()
	client := &fakeScheduledRebalanceClient{}
	if err := SyncScheduledRebalanceConfiguration(ctx, client, "cluster-1", customfield.NullObjectList[api.ScheduledRebalancePolicyModel](ctx), map[string]struct{}{"old": {}}); err != nil {
		t.Fatalf("SyncScheduledRebalanceConfiguration() error = %v", err)
	}
	if client.listed != 0 || len(client.created)+len(client.updated)+len(client.deleted) != 0 {
		t.Fatalf("null configuration made remote calls: %#v", client)
	}
}

func TestSyncScheduledRebalanceRejectsNewPolicyWithoutCronBeforeCreate(t *testing.T) {
	ctx := context.Background()
	client := &fakeScheduledRebalanceClient{}
	desired := customfield.NewObjectListMust(ctx, []api.ScheduledRebalancePolicyModel{{
		Name: types.StringValue("missing-cron"),
	}})

	err := SyncScheduledRebalanceConfiguration(ctx, client, "cluster-1", desired, nil)
	if err == nil || !strings.Contains(err.Error(), "cron is required") {
		t.Fatalf("SyncScheduledRebalanceConfiguration() error = %v, want missing cron error", err)
	}
	if len(client.created) != 0 {
		t.Fatalf("invalid policy reached create API: %#v", client.created)
	}
}

func TestSyncScheduledRebalancePrevalidatesAllPoliciesBeforeWrites(t *testing.T) {
	ctx := context.Background()
	client := &fakeScheduledRebalanceClient{}
	desired := customfield.NewObjectListMust(ctx, []api.ScheduledRebalancePolicyModel{
		{Name: types.StringValue("valid"), Cron: types.StringValue("0 2 * * *")},
		{Name: types.StringValue("missing-cron")},
	})

	err := SyncScheduledRebalanceConfiguration(ctx, client, "cluster-1", desired, nil)
	if err == nil || !strings.Contains(err.Error(), "cron is required") {
		t.Fatalf("SyncScheduledRebalanceConfiguration() error = %v, want missing cron error", err)
	}
	if client.listed != 1 || len(client.created)+len(client.updated)+len(client.deleted) != 0 {
		t.Fatalf("invalid policy list made writes: %#v", client)
	}
}

func TestSyncScheduledRebalanceRejectsMalformedCronBeforeWrites(t *testing.T) {
	ctx := context.Background()
	client := &fakeScheduledRebalanceClient{}
	desired := customfield.NewObjectListMust(ctx, []api.ScheduledRebalancePolicyModel{
		{Name: types.StringValue("valid"), Cron: types.StringValue("0 2 * * *")},
		{Name: types.StringValue("invalid"), Cron: types.StringValue("61 2 * * *")},
	})

	err := SyncScheduledRebalanceConfiguration(ctx, client, "cluster-1", desired, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid cron") {
		t.Fatalf("SyncScheduledRebalanceConfiguration() error = %v, want invalid cron error", err)
	}
	if len(client.created) != 0 || len(client.updated) != 0 || len(client.deleted) != 0 {
		t.Fatalf("invalid scheduled rebalance list made writes: created=%d updated=%d deleted=%d", len(client.created), len(client.updated), len(client.deleted))
	}
}

func TestSyncScheduledRebalanceRejectsDuplicateNamesBeforeRemoteCalls(t *testing.T) {
	ctx := context.Background()
	client := &fakeScheduledRebalanceClient{}
	desired := customfield.NewObjectListMust(ctx, []api.ScheduledRebalancePolicyModel{
		{Name: types.StringValue("duplicate"), Cron: types.StringValue("0 2 * * *")},
		{Name: types.StringValue("duplicate"), Cron: types.StringValue("0 3 * * *")},
	})

	err := SyncScheduledRebalanceConfiguration(ctx, client, "cluster-1", desired, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("SyncScheduledRebalanceConfiguration() error = %v, want duplicate name error", err)
	}
	if client.listed != 0 || len(client.created)+len(client.updated)+len(client.deleted) != 0 {
		t.Fatalf("duplicate policies made remote calls: %#v", client)
	}
}

func TestSyncScheduledRebalanceSupportsLowestUtilizationFirst(t *testing.T) {
	ctx := context.Background()
	client := &fakeScheduledRebalanceClient{}
	desired := customfield.NewObjectListMust(ctx, []api.ScheduledRebalancePolicyModel{{
		Name:           types.StringValue("lowest-utilization"),
		Cron:           types.StringValue("0 2 * * *"),
		SelectionOrder: types.StringValue(api.ScheduledRebalanceSelectionOrderLowestUtilizationFirst),
	}})

	if err := SyncScheduledRebalanceConfiguration(ctx, client, "cluster-1", desired, nil); err != nil {
		t.Fatalf("SyncScheduledRebalanceConfiguration() error = %v", err)
	}
	if len(client.created) != 1 || client.created[0].SelectionOrder != api.ScheduledRebalanceSelectionOrderLowestUtilizationFirst {
		t.Fatalf("created = %#v, want lowest_utilization_first", client.created)
	}
}

func TestSyncScheduledRebalanceDeletesOnlyPreviouslyManagedPolicies(t *testing.T) {
	ctx := context.Background()
	client := &fakeScheduledRebalanceClient{policies: []api.ScheduledRebalancePolicy{
		{ID: "managed-id", Name: "managed-old"},
		{ID: "unmanaged-id", Name: "server-only"},
	}}
	desired := customfield.NewObjectListMust(ctx, []api.ScheduledRebalancePolicyModel{})
	if err := SyncScheduledRebalanceConfiguration(ctx, client, "cluster-1", desired, map[string]struct{}{"managed-old": {}}); err != nil {
		t.Fatalf("SyncScheduledRebalanceConfiguration() error = %v", err)
	}
	if len(client.deleted) != 1 || client.deleted[0] != "managed-id" {
		t.Fatalf("deleted = %#v", client.deleted)
	}
}

func TestSyncScheduledRebalancePreservesNullPolicyFields(t *testing.T) {
	ctx := context.Background()
	client := &fakeScheduledRebalanceClient{policies: []api.ScheduledRebalancePolicy{{
		ID: "policy-id", Name: "nightly", Cron: "0 2 * * *", Timezone: "Asia/Shanghai", Enabled: true,
		SelectionOrder: "oldest_first", ForceDrain: true,
	}}}
	desired := customfield.NewObjectListMust(ctx, []api.ScheduledRebalancePolicyModel{{
		Name: types.StringValue("nightly"),
		Cron: types.StringValue("0 3 * * *"),
	}})
	if err := SyncScheduledRebalanceConfiguration(ctx, client, "cluster-1", desired, map[string]struct{}{"nightly": {}}); err != nil {
		t.Fatalf("SyncScheduledRebalanceConfiguration() error = %v", err)
	}
	if len(client.updated) != 1 {
		t.Fatalf("updated = %#v", client.updated)
	}
	request := client.updated[0]
	if request.Cron != "0 3 * * *" || request.Timezone == nil || *request.Timezone != "Asia/Shanghai" || request.Enabled == nil || !*request.Enabled || !request.ForceDrain {
		t.Fatalf("request did not preserve unmanaged fields: %#v", request)
	}
}

func TestSyncScheduledRebalanceSkipsUnchangedPolicy(t *testing.T) {
	ctx := context.Background()
	client := &fakeScheduledRebalanceClient{policies: []api.ScheduledRebalancePolicy{{
		ID: "policy-id", Name: "nightly", Cron: "0 2 * * *", Timezone: "UTC", Enabled: true,
		SelectionOrder: "oldest_first",
	}}}
	desired := customfield.NewObjectListMust(ctx, []api.ScheduledRebalancePolicyModel{{
		Name: types.StringValue("nightly"),
	}})
	if err := SyncScheduledRebalanceConfiguration(ctx, client, "cluster-1", desired, map[string]struct{}{"nightly": {}}); err != nil {
		t.Fatalf("SyncScheduledRebalanceConfiguration() error = %v", err)
	}
	if len(client.updated) != 0 {
		t.Fatalf("unchanged policy was updated: %#v", client.updated)
	}
}

func TestSyncScheduledRebalanceSkipsUnchangedPolicyWithoutTimezone(t *testing.T) {
	ctx := context.Background()
	policy := api.ScheduledRebalancePolicy{
		ID: "policy-id", Name: "nightly", Cron: "0 2 * * *", Enabled: true,
	}
	client := &fakeScheduledRebalanceClient{policies: []api.ScheduledRebalancePolicy{policy}}
	desired := customfield.NewObjectListMust(ctx, []api.ScheduledRebalancePolicyModel{
		api.ScheduledRebalancePolicyModelForImport(ctx, policy),
	})

	if err := SyncScheduledRebalanceConfiguration(ctx, client, "cluster-1", desired, map[string]struct{}{"nightly": {}}); err != nil {
		t.Fatalf("SyncScheduledRebalanceConfiguration() error = %v", err)
	}
	if len(client.updated) != 0 {
		t.Fatalf("unchanged policy was updated: %#v", client.updated)
	}
}

func TestSyncScheduledRebalanceTreatsNilAndEmptyScopeSlicesAsEqual(t *testing.T) {
	ctx := context.Background()
	policy := api.ScheduledRebalancePolicy{
		ID: "policy-id", Name: "nightly", Cron: "0 2 * * *", Timezone: "UTC", Enabled: true,
		Scope: api.ScheduledRebalanceScope{
			NodeSelectorTerms: []api.ScheduledRebalanceNodeSelectorTerm{{
				MatchExpressions: []api.ScheduledRebalanceLabelSelectorRequirement{{Key: "team", Operator: "Exists"}},
			}},
		},
	}
	client := &fakeScheduledRebalanceClient{policies: []api.ScheduledRebalancePolicy{policy}}
	desired := customfield.NewObjectListMust(ctx, []api.ScheduledRebalancePolicyModel{
		api.ScheduledRebalancePolicyModelForImport(ctx, policy),
	})

	if err := SyncScheduledRebalanceConfiguration(ctx, client, "cluster-1", desired, map[string]struct{}{"nightly": {}}); err != nil {
		t.Fatalf("SyncScheduledRebalanceConfiguration() error = %v", err)
	}
	if len(client.updated) != 0 {
		t.Fatalf("nil and empty scope slices caused an update: %#v", client.updated)
	}
}

func TestRefreshScheduledRebalancesDropsMissingPolicy(t *testing.T) {
	ctx := context.Background()
	client := &fakeScheduledRebalanceClient{policies: []api.ScheduledRebalancePolicy{{Name: "existing", Cron: "0 2 * * *"}}}
	current := customfield.NewObjectListMust(ctx, []api.ScheduledRebalancePolicyModel{
		{Name: types.StringValue("existing"), Cron: types.StringValue("0 1 * * *")},
		{Name: types.StringValue("deleted"), Cron: types.StringValue("0 3 * * *")},
	})

	refreshed, err := RefreshScheduledRebalances(ctx, client, "cluster-1", current)
	if err != nil {
		t.Fatalf("RefreshScheduledRebalances() error = %v", err)
	}
	models, diags := refreshed.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("refreshed diagnostics = %v", diags)
	}
	if len(models) != 1 || models[0].Name.ValueString() != "existing" {
		t.Fatalf("refreshed = %#v, want only existing policy", models)
	}
}

func TestRefreshScheduledRebalancesNullIsUnmanaged(t *testing.T) {
	ctx := context.Background()
	client := &fakeScheduledRebalanceClient{policies: []api.ScheduledRebalancePolicy{{Name: "server-only"}}}
	current := customfield.NullObjectList[api.ScheduledRebalancePolicyModel](ctx)

	refreshed, err := RefreshScheduledRebalances(ctx, client, "cluster-1", current)
	if err != nil {
		t.Fatalf("RefreshScheduledRebalances() error = %v", err)
	}
	if !refreshed.IsNull() || client.listed != 0 {
		t.Fatalf("null refresh = %#v, list calls = %d; want unmanaged null with no remote read", refreshed, client.listed)
	}
}

func TestHydrateScheduledRebalancesPostWritePreservesMissingPolicy(t *testing.T) {
	ctx := context.Background()
	client := &fakeScheduledRebalanceClient{}
	current := customfield.NewObjectListMust(ctx, []api.ScheduledRebalancePolicyModel{{
		Name: types.StringValue("pending"), Cron: types.StringValue("0 2 * * *"),
	}})

	hydrated, err := HydrateScheduledRebalancesPostWrite(ctx, client, "cluster-1", current)
	if err != nil {
		t.Fatalf("HydrateScheduledRebalancesPostWrite() error = %v", err)
	}
	models, diags := hydrated.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("hydrated diagnostics = %v", diags)
	}
	if len(models) != 1 || models[0].Name.ValueString() != "pending" {
		t.Fatalf("hydrated = %#v, want pending policy preserved", models)
	}
}

func TestImportScheduledRebalancesHydratesAllFields(t *testing.T) {
	ctx := context.Background()
	client := &fakeScheduledRebalanceClient{policies: []api.ScheduledRebalancePolicy{{
		Name: "nightly", Cron: "0 2 * * *", Timezone: "UTC", Enabled: true,
		SelectionOrder: "oldest_first", ForceDrain: true,
		NodeConstraints: api.ScheduledRebalanceNodeConstraints{MinAgeSeconds: 300, MaxNodes: 2, MinClusterSize: 3},
	}}}

	imported, err := ImportScheduledRebalances(ctx, client, "cluster-1")
	if err != nil {
		t.Fatalf("ImportScheduledRebalances() error = %v", err)
	}
	models, diags := imported.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("imported diagnostics = %v", diags)
	}
	if len(models) != 1 || models[0].Name.ValueString() != "nightly" || models[0].Cron.ValueString() != "0 2 * * *" || !models[0].Enabled.ValueBool() || !models[0].ForceDrain.ValueBool() {
		t.Fatalf("imported = %#v, want complete nightly policy", models)
	}
	constraints, constraintDiags := models[0].NodeConstraints.Value(ctx)
	if constraintDiags.HasError() || constraints == nil || constraints.MaxNodes.ValueInt64() != 2 {
		t.Fatalf("imported constraints = %#v, diagnostics = %v", constraints, constraintDiags)
	}
}
