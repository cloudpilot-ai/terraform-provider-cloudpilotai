package api

import (
	"context"
	"strings"
	"testing"

	gcpcorev1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter/apis/v1"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
)

func TestGCENodePoolModelRoundTripPreservesRequirements(t *testing.T) {
	ctx := context.Background()
	remote := GCENodePool{
		Name:   "cloudpilot-general",
		Enable: true,
		NodePoolSpec: &gcpcorev1.NodePoolSpec{
			Template: gcpcorev1.NodeClaimTemplate{
				ObjectMeta: gcpcorev1.ObjectMeta{
					Labels: map[string]string{"team": "platform"},
				},
				Spec: gcpcorev1.NodeClaimTemplateSpec{
					NodeClassRef: &gcpcorev1.NodeClassReference{
						Group: gceNodeClassRefGroup,
						Kind:  gceNodeClassRefKind,
						Name:  "cloudpilot",
					},
					Requirements: []gcpcorev1.NodeSelectorRequirementWithMinValues{
						{Key: gceLabelInstanceFamily, Operator: corev1.NodeSelectorOpIn, Values: []string{"n4"}},
						{Key: corev1.LabelArchStable, Operator: corev1.NodeSelectorOpIn, Values: []string{"amd64"}},
						{Key: gcpcorev1.CapacityTypeLabelKey, Operator: corev1.NodeSelectorOpIn, Values: []string{"spot", "on-demand"}},
						{Key: gceLabelInstanceCPU, Operator: corev1.NodeSelectorOpLt, Values: []string{"17"}},
						{Key: gceLabelInstanceMemory, Operator: corev1.NodeSelectorOpLt, Values: []string{"32769"}},
						{Key: gceLabelInstanceGPUCount, Operator: corev1.NodeSelectorOpExists},
					},
					Taints:      []corev1.Taint{{Key: "dedicated", Value: "wa", Effect: corev1.TaintEffectNoSchedule}},
					ExpireAfter: gcpcorev1.MustParseNillableDuration("Never"),
				},
			},
			Disruption: gcpcorev1.Disruption{
				ConsolidateAfter: gcpcorev1.MustParseNillableDuration("0s"),
				Budgets:          []gcpcorev1.Budget{{Nodes: "10%"}},
			},
			Weight: lo.ToPtr[int32](2),
		},
	}

	model, err := remote.ToGCENodePoolModel(ctx)
	if err != nil {
		t.Fatalf("ToGCENodePoolModel() error = %v", err)
	}
	if got := model.NodeClass.ValueString(); got != "cloudpilot" {
		t.Fatalf("NodeClass = %q", got)
	}
	if !model.EnableGPU.ValueBool() {
		t.Fatalf("EnableGPU = %v, want true", model.EnableGPU)
	}

	roundTrip, err := model.ToGCENodePool(ctx, GCENodePool{Name: "cloudpilot-general"})
	if err != nil {
		t.Fatalf("ToGCENodePool() error = %v", err)
	}
	if roundTrip.NodePoolSpec.Template.Spec.NodeClassRef.Name != "cloudpilot" {
		t.Fatalf("NodeClassRef = %#v", roundTrip.NodePoolSpec.Template.Spec.NodeClassRef)
	}
	if len(roundTrip.NodePoolSpec.Template.Spec.Taints) != 1 {
		t.Fatalf("Taints = %#v", roundTrip.NodePoolSpec.Template.Spec.Taints)
	}
	if got := gcpRequirementsToStrings(roundTrip.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceFamily, corev1.NodeSelectorOpIn); got == nil || len(*got) != 1 || (*got)[0].ValueString() != "n4" {
		t.Fatalf("InstanceFamily requirements = %#v", roundTrip.NodePoolSpec.Template.Spec.Requirements)
	}
	if !gcpEnableGPUToBoolByKey(roundTrip.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceGPUCount).ValueBool() {
		t.Fatalf("EnableGPU requirements = %#v", roundTrip.NodePoolSpec.Template.Spec.Requirements)
	}
}

func TestGCENodePoolModelRejectsEmptyNodeClass(t *testing.T) {
	model := GCENodePoolModel{
		Name:      types.StringValue("cloudpilot-general"),
		NodeClass: types.StringValue("  "),
	}

	_, err := model.ToGCENodePool(context.Background(), GCENodePool{Name: "cloudpilot-general"})
	if err == nil || !strings.Contains(err.Error(), "must reference a valid nodeclass") {
		t.Fatalf("ToGCENodePool() error = %v, want empty nodeclass validation", err)
	}
}

func TestGCENodePoolModelWhitespaceOriginJSONStillAttemptsOverride(t *testing.T) {
	model := GCENodePoolModel{
		Name:               types.StringValue("cloudpilot-general"),
		OriginNodePoolJSON: types.StringValue("   "),
	}

	_, err := model.ToGCENodePool(context.Background(), GCENodePool{Name: "cloudpilot-general"})
	if err == nil {
		t.Fatal("ToGCENodePool() error = nil, want JSON unmarshal failure for whitespace origin_nodepool_json")
	}
}

func TestGCENodePoolModelOriginJSONPrecedenceWins(t *testing.T) {
	ctx := context.Background()
	model := GCENodePoolModel{
		Name:               types.StringValue("ignored"),
		NodeClass:          types.StringValue("ignored"),
		OriginNodePoolJSON: types.StringValue(`{"name":"from-origin","enable":true,"nodePoolSpec":{"template":{"metadata":{"labels":{"source":"origin"}},"spec":{"nodeClassRef":{"group":"karpenter.k8s.gcp","kind":"GCENodeClass","name":"origin-nodeclass"},"requirements":[]}},"disruption":{"consolidateAfter":"0s"}}}`),
	}

	got, err := model.ToGCENodePool(ctx, GCENodePool{Name: "current"})
	if err != nil {
		t.Fatalf("ToGCENodePool() error = %v", err)
	}
	if got.Name != "from-origin" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.NodePoolSpec == nil || got.NodePoolSpec.Template.Spec.NodeClassRef == nil || got.NodePoolSpec.Template.Spec.NodeClassRef.Name != "origin-nodeclass" {
		t.Fatalf("NodePoolSpec = %#v", got.NodePoolSpec)
	}
}

func TestGCENodePoolModelWritesZoneRequirementWithStandardKey(t *testing.T) {
	ctx := context.Background()
	model := GCENodePoolModel{
		Name:      types.StringValue("cloudpilot-general"),
		Enable:    types.BoolValue(true),
		NodeClass: types.StringValue("cloudpilot"),
		Zone: &[]types.String{
			types.StringValue("us-central1-a"),
		},
	}

	current := GCENodePool{
		Name:         "cloudpilot-general",
		NodePoolSpec: defaultGCPNodePoolSpec(),
	}
	current.NodePoolSpec.Template.Spec.Requirements = append(current.NodePoolSpec.Template.Spec.Requirements,
		gcpcorev1.NodeSelectorRequirementWithMinValues{
			Key:      gceLegacyLabelTopologyZoneID,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{"legacy-zone"},
		},
	)

	updated, err := model.ToGCENodePool(ctx, current)
	if err != nil {
		t.Fatalf("ToGCENodePool() error = %v", err)
	}
	if got := gcpRequirementsToStrings(updated.NodePoolSpec.Template.Spec.Requirements, corev1.LabelTopologyZone, corev1.NodeSelectorOpIn); got == nil || len(*got) != 1 || (*got)[0].ValueString() != "us-central1-a" {
		t.Fatalf("standard zone requirement = %#v", updated.NodePoolSpec.Template.Spec.Requirements)
	}
	if got := gcpRequirementsToStrings(updated.NodePoolSpec.Template.Spec.Requirements, gceLegacyLabelTopologyZoneID, corev1.NodeSelectorOpIn); got != nil {
		t.Fatalf("legacy GCP zone requirement should be absent, got %#v", *got)
	}
}

func TestGCENodePoolModelPrefersStandardZoneRequirementOnRead(t *testing.T) {
	ctx := context.Background()
	remote := GCENodePool{
		Name:   "cloudpilot-general",
		Enable: true,
		NodePoolSpec: &gcpcorev1.NodePoolSpec{
			Template: gcpcorev1.NodeClaimTemplate{
				Spec: gcpcorev1.NodeClaimTemplateSpec{
					NodeClassRef: &gcpcorev1.NodeClassReference{
						Group: gceNodeClassRefGroup,
						Kind:  gceNodeClassRefKind,
						Name:  "cloudpilot",
					},
					Requirements: []gcpcorev1.NodeSelectorRequirementWithMinValues{
						{Key: corev1.LabelTopologyZone, Operator: corev1.NodeSelectorOpIn, Values: []string{"standard-zone"}},
						{Key: gceLegacyLabelTopologyZoneID, Operator: corev1.NodeSelectorOpIn, Values: []string{"legacy-zone"}},
					},
				},
			},
		},
	}

	model, err := remote.ToGCENodePoolModel(ctx)
	if err != nil {
		t.Fatalf("ToGCENodePoolModel() error = %v", err)
	}
	if model.Zone == nil || len(*model.Zone) != 1 || (*model.Zone)[0].ValueString() != "standard-zone" {
		t.Fatalf("Zone = %#v", model.Zone)
	}
}

func TestGCENodePoolModelReadsLegacyZoneRequirement(t *testing.T) {
	ctx := context.Background()
	remote := GCENodePool{
		Name:   "cloudpilot-general",
		Enable: true,
		NodePoolSpec: &gcpcorev1.NodePoolSpec{
			Template: gcpcorev1.NodeClaimTemplate{
				Spec: gcpcorev1.NodeClaimTemplateSpec{
					NodeClassRef: &gcpcorev1.NodeClassReference{
						Group: gceNodeClassRefGroup,
						Kind:  gceNodeClassRefKind,
						Name:  "cloudpilot",
					},
					Requirements: []gcpcorev1.NodeSelectorRequirementWithMinValues{
						{Key: gceLegacyLabelTopologyZoneID, Operator: corev1.NodeSelectorOpIn, Values: []string{"legacy-zone"}},
					},
				},
			},
		},
	}

	model, err := remote.ToGCENodePoolModel(ctx)
	if err != nil {
		t.Fatalf("ToGCENodePoolModel() error = %v", err)
	}
	if model.Zone == nil || len(*model.Zone) != 1 || (*model.Zone)[0].ValueString() != "legacy-zone" {
		t.Fatalf("Zone = %#v", model.Zone)
	}
}

func TestGCENodePoolModelLeavesZoneRequirementsUnmanagedWhenZoneIsNull(t *testing.T) {
	ctx := context.Background()
	current := GCENodePool{
		Name:         "cloudpilot-general",
		NodePoolSpec: defaultGCPNodePoolSpec(),
	}
	current.NodePoolSpec.Template.Spec.Requirements = append(current.NodePoolSpec.Template.Spec.Requirements,
		gcpcorev1.NodeSelectorRequirementWithMinValues{
			Key:      gceLegacyLabelTopologyZoneID,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{"legacy-zone"},
		},
	)
	model := GCENodePoolModel{
		Name:      types.StringValue("cloudpilot-general"),
		NodeClass: types.StringValue("cloudpilot"),
	}

	updated, err := model.ToGCENodePool(ctx, current)
	if err != nil {
		t.Fatalf("ToGCENodePool() error = %v", err)
	}
	if got := gcpRequirementsToStrings(updated.NodePoolSpec.Template.Spec.Requirements, gceLegacyLabelTopologyZoneID, corev1.NodeSelectorOpIn); got == nil || len(*got) != 1 || (*got)[0].ValueString() != "legacy-zone" {
		t.Fatalf("legacy zone requirement was unexpectedly changed: %#v", updated.NodePoolSpec.Template.Spec.Requirements)
	}
}

func TestGCENodePoolModelLeavesUnsetWeightNullOnRead(t *testing.T) {
	ctx := context.Background()
	remote := GCENodePool{
		Name:   "cloudpilot-general",
		Enable: true,
		NodePoolSpec: &gcpcorev1.NodePoolSpec{
			Template: gcpcorev1.NodeClaimTemplate{
				Spec: gcpcorev1.NodeClaimTemplateSpec{
					NodeClassRef: &gcpcorev1.NodeClassReference{
						Group: gceNodeClassRefGroup,
						Kind:  gceNodeClassRefKind,
						Name:  "cloudpilot",
					},
				},
			},
		},
	}

	model, err := remote.ToGCENodePoolModel(ctx)
	if err != nil {
		t.Fatalf("ToGCENodePoolModel() error = %v", err)
	}
	if !model.ProvisionPriority.IsNull() {
		t.Fatalf("ProvisionPriority = %#v, want null", model.ProvisionPriority)
	}
}

func TestGCENodePoolModelPreservesNeverDisruptionDelayOnRead(t *testing.T) {
	ctx := context.Background()
	remote := GCENodePool{
		Name:   "cloudpilot-general",
		Enable: true,
		NodePoolSpec: &gcpcorev1.NodePoolSpec{
			Template: gcpcorev1.NodeClaimTemplate{
				Spec: gcpcorev1.NodeClaimTemplateSpec{
					NodeClassRef: &gcpcorev1.NodeClassReference{
						Group: gceNodeClassRefGroup,
						Kind:  gceNodeClassRefKind,
						Name:  "cloudpilot",
					},
				},
			},
			Disruption: gcpcorev1.Disruption{
				ConsolidateAfter: gcpcorev1.MustParseNillableDuration("Never"),
			},
		},
	}

	model, err := remote.ToGCENodePoolModel(ctx)
	if err != nil {
		t.Fatalf("ToGCENodePoolModel() error = %v", err)
	}
	if model.NodeDisruptionDelay.IsNull() || model.NodeDisruptionDelay.ValueString() != gcpcorev1.Never {
		t.Fatalf("NodeDisruptionDelay = %#v, want %q", model.NodeDisruptionDelay, gcpcorev1.Never)
	}
}

func TestGCENodePoolModelSupportsMultipleDisruptionBudgets(t *testing.T) {
	ctx := context.Background()
	reasons := []types.String{types.StringValue("Empty"), types.StringValue("Underutilized")}
	model := GCENodePoolModel{
		Name:      types.StringValue("cloudpilot-general"),
		NodeClass: types.StringValue("cloudpilot"),
		NodeDisruptionBudgets: customfield.NewObjectListMust(ctx, []DisruptionBudgetModel{
			{Nodes: types.StringValue("20%")},
			{
				Nodes:    types.StringValue("0"),
				Reasons:  &reasons,
				Schedule: types.StringValue("0 9 * * mon-fri"),
				Duration: types.StringValue("8h"),
			},
		}),
	}

	got, err := model.ToGCENodePool(ctx, GCENodePool{})
	if err != nil {
		t.Fatalf("ToGCENodePool() error = %v", err)
	}
	if len(got.NodePoolSpec.Disruption.Budgets) != 2 {
		t.Fatalf("budgets = %#v", got.NodePoolSpec.Disruption.Budgets)
	}
	if got.NodePoolSpec.Disruption.Budgets[1].Schedule == nil || *got.NodePoolSpec.Disruption.Budgets[1].Schedule != "0 9 * * mon-fri" {
		t.Fatalf("scheduled budget = %#v", got.NodePoolSpec.Disruption.Budgets[1])
	}
	if len(got.NodePoolSpec.Disruption.Budgets[1].Reasons) != 2 {
		t.Fatalf("budget reasons = %#v", got.NodePoolSpec.Disruption.Budgets[1].Reasons)
	}

	readModel, err := got.ToGCENodePoolModel(ctx)
	if err != nil {
		t.Fatalf("ToGCENodePoolModel() error = %v", err)
	}
	readBudgets, diags := readModel.NodeDisruptionBudgets.AsStructSliceT(ctx)
	if diags.HasError() || len(readBudgets) != 2 {
		t.Fatalf("read budgets = %#v, diagnostics = %v", readBudgets, diags)
	}
	if readBudgets[1].Duration.ValueString() != "8h" {
		t.Fatalf("read duration = %#v", readBudgets[1].Duration)
	}
}

func TestGCENodePoolModelRejectsEmptyDisruptionBudgets(t *testing.T) {
	ctx := context.Background()
	model := GCENodePoolModel{
		Name:                  types.StringValue("cloudpilot-general"),
		NodeClass:             types.StringValue("cloudpilot"),
		NodeDisruptionBudgets: customfield.NewObjectListMust(ctx, []DisruptionBudgetModel{}),
	}

	if _, err := model.ToGCENodePool(ctx, GCENodePool{}); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("ToGCENodePool() error = %v, want empty budgets error", err)
	}
}

func TestGCENodePoolModelRejectsTooManyDisruptionBudgets(t *testing.T) {
	ctx := context.Background()
	budgets := make([]DisruptionBudgetModel, maxDisruptionBudgets+1)
	for i := range budgets {
		budgets[i].Nodes = types.StringValue("10%")
	}
	model := GCENodePoolModel{
		Name:                  types.StringValue("cloudpilot-general"),
		NodeClass:             types.StringValue("cloudpilot"),
		NodeDisruptionBudgets: customfield.NewObjectListMust(ctx, budgets),
	}

	if _, err := model.ToGCENodePool(ctx, GCENodePool{}); err == nil || !strings.Contains(err.Error(), "at most 50") {
		t.Fatalf("ToGCENodePool() error = %v, want maximum budgets error", err)
	}
}

func TestGCENodePoolModelRejectsTooManyDisruptionBudgetReasons(t *testing.T) {
	ctx := context.Background()
	reasons := make([]types.String, maxGCPDisruptionBudgetReasons+1)
	for i := range reasons {
		reasons[i] = types.StringValue("Drifted")
	}
	model := GCENodePoolModel{
		Name:      types.StringValue("cloudpilot-general"),
		NodeClass: types.StringValue("cloudpilot"),
		NodeDisruptionBudgets: customfield.NewObjectListMust(ctx, []DisruptionBudgetModel{{
			Nodes:   types.StringValue("10%"),
			Reasons: &reasons,
		}}),
	}

	if _, err := model.ToGCENodePool(ctx, GCENodePool{}); err == nil || !strings.Contains(err.Error(), "reasons must contain at most 50") {
		t.Fatalf("ToGCENodePool() error = %v, want maximum reasons error", err)
	}
}

func TestGCENodePoolToModelKeepsAbsentDisruptionBudgetsNull(t *testing.T) {
	model, err := (&GCENodePool{Name: "cloudpilot-general", NodePoolSpec: &gcpcorev1.NodePoolSpec{}}).ToGCENodePoolModel(context.Background())
	if err != nil {
		t.Fatalf("ToGCENodePoolModel() error = %v", err)
	}
	if !model.NodeDisruptionBudgets.IsNull() {
		t.Fatalf("NodeDisruptionBudgets = %#v, want null for absent remote budgets", model.NodeDisruptionBudgets)
	}
}

func TestGCPUpdateRequirementsRemovesOnlyMatchedRequirement(t *testing.T) {
	requirements := []gcpcorev1.NodeSelectorRequirementWithMinValues{
		{Key: "first", Operator: corev1.NodeSelectorOpIn, Values: []string{"a"}},
		{Key: gceLegacyLabelTopologyZoneID, Operator: corev1.NodeSelectorOpIn, Values: []string{"us-central1-a"}},
		{Key: "last", Operator: corev1.NodeSelectorOpIn, Values: []string{"z"}},
	}

	updated := gcpUpdateRequirements(gceLegacyLabelTopologyZoneID, corev1.NodeSelectorOpIn, nil, requirements)
	if len(updated) != 2 {
		t.Fatalf("updated requirements = %#v", updated)
	}
	if updated[0].Key != "first" || updated[1].Key != "last" {
		t.Fatalf("unexpected remaining requirements = %#v", updated)
	}
}

func TestGCENodePoolModelTreatsZeroMaxConstraintsAsUnlimited(t *testing.T) {
	ctx := context.Background()
	current := GCENodePool{
		Name:         "cloudpilot-general",
		NodePoolSpec: defaultGCPNodePoolSpec(),
	}
	current.NodePoolSpec.Template.Spec.Requirements = append(current.NodePoolSpec.Template.Spec.Requirements,
		gcpcorev1.NodeSelectorRequirementWithMinValues{
			Key:      gceLabelInstanceCPU,
			Operator: corev1.NodeSelectorOpLt,
			Values:   []string{"64"},
		},
		gcpcorev1.NodeSelectorRequirementWithMinValues{
			Key:      gceLabelInstanceMemory,
			Operator: corev1.NodeSelectorOpLt,
			Values:   []string{"262144"},
		},
	)
	model := GCENodePoolModel{
		Name:              types.StringValue("cloudpilot-general"),
		Enable:            types.BoolValue(true),
		NodeClass:         types.StringValue("cloudpilot"),
		InstanceCPUMAX:    types.Int64Value(0),
		InstanceMemoryMAX: types.Int64Value(0),
	}

	updated, err := model.ToGCENodePool(ctx, current)
	if err != nil {
		t.Fatalf("ToGCENodePool() error = %v", err)
	}
	if got, err := gcpRequirementsToOptionalInt64(updated.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceCPU, corev1.NodeSelectorOpLt); err != nil || !got.IsNull() {
		t.Fatalf("instance cpu max = %#v, err = %v, want null", got, err)
	}
	if got, err := gcpRequirementsToOptionalInt64(updated.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceMemory, corev1.NodeSelectorOpLt); err != nil || !got.IsNull() {
		t.Fatalf("instance memory max = %#v, err = %v, want null", got, err)
	}
}

func TestGCENodePoolModelPreservesConfiguredManagedLabel(t *testing.T) {
	ctx := context.Background()
	model := GCENodePoolModel{
		Name:      types.StringValue("cloudpilot-general"),
		Enable:    types.BoolValue(true),
		NodeClass: types.StringValue("cloudpilot"),
		Labels: customfield.NewMapMust[types.String](ctx, map[string]types.String{
			"team":                        types.StringValue("platform"),
			CloudPilotManagedNodeLabelKey: types.StringValue("false"),
		}),
	}

	updated, err := model.ToGCENodePool(ctx, GCENodePool{Name: "cloudpilot-general"})
	if err != nil {
		t.Fatalf("ToGCENodePool() error = %v", err)
	}
	if updated.NodePoolSpec.Template.ObjectMeta.Labels[CloudPilotManagedNodeLabelKey] != "false" {
		t.Fatalf("managed label = %#v, want configured value", updated.NodePoolSpec.Template.ObjectMeta.Labels)
	}
}

func TestGCENodePoolModelAddsManagedLabelWhenApplyingUserLabels(t *testing.T) {
	ctx := context.Background()
	model := GCENodePoolModel{
		Name:      types.StringValue("cloudpilot-general"),
		Enable:    types.BoolValue(true),
		NodeClass: types.StringValue("cloudpilot"),
		Labels: customfield.NewMapMust[types.String](ctx, map[string]types.String{
			"team": types.StringValue("platform"),
		}),
	}

	updated, err := model.ToGCENodePool(ctx, GCENodePool{Name: "cloudpilot-general"})
	if err != nil {
		t.Fatalf("ToGCENodePool() error = %v", err)
	}
	if updated.NodePoolSpec.Template.ObjectMeta.Labels[CloudPilotManagedNodeLabelKey] != "true" {
		t.Fatalf("managed label = %#v", updated.NodePoolSpec.Template.ObjectMeta.Labels)
	}
	if updated.NodePoolSpec.Template.ObjectMeta.Labels["team"] != "platform" {
		t.Fatalf("user labels = %#v", updated.NodePoolSpec.Template.ObjectMeta.Labels)
	}
}
