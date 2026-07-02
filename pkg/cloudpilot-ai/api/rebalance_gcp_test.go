package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	gcpproviderv1alpha1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter-provider-gcp/apis/v1alpha1"
	gcpcorev1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter/apis/v1"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func TestGCENodeClassModelRoundTripPreservesTypedFields(t *testing.T) {
	ctx := context.Background()
	enablePrivateNodes := true
	remote := GCENodeClass{
		Name: "cloudpilot",
		NodeClassSpec: &GCENodeClassSpec{
			ServiceAccount: "nodes@test-project.iam.gserviceaccount.com",
			ImageSelectorTerms: []GCEImageSelectorTerm{{
				Family:  "ContainerOptimizedOS",
				Channel: "cluster",
			}},
			Disks: []GCEDisk{{
				Boot:     true,
				Category: "pd-balanced",
				SizeGiB:  80,
			}},
			Labels:      map[string]string{"cloudpilot.ai/managed": "true"},
			Metadata:    map[string]string{"startup-script": "echo hi"},
			NetworkTags: []gcpproviderv1alpha1.NetworkTag{"cloudpilot"},
			NetworkConfig: &GCENetworkConfig{
				EnablePrivateNodes: &enablePrivateNodes,
				Subnetwork:         "projects/test/regions/us-central1/subnetworks/default",
			},
			GPUDriverVersion: "default",
		},
	}

	model, err := remote.ToGCENodeClassModel(ctx)
	if err != nil {
		t.Fatalf("ToGCENodeClassModel() error = %v", err)
	}
	if got := model.ServiceAccount.ValueString(); got != "nodes@test-project.iam.gserviceaccount.com" {
		t.Fatalf("ServiceAccount = %q", got)
	}

	roundTrip, err := model.ToGCENodeClass(ctx, GCENodeClass{Name: "cloudpilot"})
	if err != nil {
		t.Fatalf("ToGCENodeClass() error = %v", err)
	}
	if roundTrip.NodeClassSpec.ServiceAccount != remote.NodeClassSpec.ServiceAccount {
		t.Fatalf("ServiceAccount = %q, want %q", roundTrip.NodeClassSpec.ServiceAccount, remote.NodeClassSpec.ServiceAccount)
	}
	if len(roundTrip.NodeClassSpec.ImageSelectorTerms) != 1 || roundTrip.NodeClassSpec.ImageSelectorTerms[0].Channel != "cluster" {
		t.Fatalf("ImageSelectorTerms = %#v", roundTrip.NodeClassSpec.ImageSelectorTerms)
	}
	if roundTrip.NodeClassSpec.NetworkConfig == nil || roundTrip.NodeClassSpec.NetworkConfig.EnablePrivateNodes == nil || !*roundTrip.NodeClassSpec.NetworkConfig.EnablePrivateNodes {
		t.Fatalf("NetworkConfig = %#v", roundTrip.NodeClassSpec.NetworkConfig)
	}
}

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

func TestGCENodeClassModelWhitespaceOriginJSONStillAttemptsOverride(t *testing.T) {
	model := GCENodeClassModel{
		Name:                types.StringValue("cloudpilot"),
		OriginNodeClassJSON: types.StringValue("   "),
	}

	_, err := model.ToGCENodeClass(context.Background(), GCENodeClass{Name: "cloudpilot"})
	if err == nil {
		t.Fatal("ToGCENodeClass() error = nil, want JSON unmarshal failure for whitespace origin_nodeclass_json")
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

func TestGCENodeClassModelTypedRoundTripPreservesServerOnlyFields(t *testing.T) {
	ctx := context.Background()
	var current GCENodeClass
	raw := []byte(`{
		"name":"cloudpilot",
		"nodeClassSpec":{
			"serviceAccount":"old@test-project.iam.gserviceaccount.com",
			"imageFamily":"ContainerOptimizedOS",
			"imageSelectorTerms":[{"alias":"ContainerOptimizedOS@latest"}],
			"shieldedInstanceConfig":{"enableSecureBoot":true},
			"metadata":{"preserve":"me"}
		}
	}`)
	if err := json.Unmarshal(raw, &current); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	model := GCENodeClassModel{
		Name:           types.StringValue("cloudpilot"),
		ServiceAccount: types.StringValue("new@test-project.iam.gserviceaccount.com"),
	}

	updated, err := model.ToGCENodeClass(ctx, current)
	if err != nil {
		t.Fatalf("ToGCENodeClass() error = %v", err)
	}

	marshaled, err := json.Marshal(updated)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(marshaled, &payload); err != nil {
		t.Fatalf("json.Unmarshal(marshaled) error = %v", err)
	}
	spec, ok := payload["nodeClassSpec"].(map[string]any)
	if !ok {
		t.Fatalf("nodeClassSpec = %#v", payload["nodeClassSpec"])
	}
	if got := spec["serviceAccount"]; got != "new@test-project.iam.gserviceaccount.com" {
		t.Fatalf("serviceAccount = %#v", got)
	}
	if got := spec["imageFamily"]; got != "ContainerOptimizedOS" {
		t.Fatalf("imageFamily = %#v", got)
	}
	shielded, ok := spec["shieldedInstanceConfig"].(map[string]any)
	if !ok || shielded["enableSecureBoot"] != true {
		t.Fatalf("shieldedInstanceConfig = %#v", spec["shieldedInstanceConfig"])
	}
	metadata, ok := spec["metadata"].(map[string]any)
	if !ok || metadata["preserve"] != "me" {
		t.Fatalf("metadata = %#v", spec["metadata"])
	}
}

func TestGCENodeClassModelOriginJSONPrecedenceWins(t *testing.T) {
	ctx := context.Background()
	model := GCENodeClassModel{
		Name:                types.StringValue("ignored"),
		ServiceAccount:      types.StringValue("ignored@test-project.iam.gserviceaccount.com"),
		OriginNodeClassJSON: types.StringValue(`{"name":"from-origin","nodeClassSpec":{"serviceAccount":"origin@test-project.iam.gserviceaccount.com","metadata":{"keep":"origin"}}}`),
	}

	got, err := model.ToGCENodeClass(ctx, GCENodeClass{Name: "current"})
	if err != nil {
		t.Fatalf("ToGCENodeClass() error = %v", err)
	}
	if got.Name != "from-origin" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.NodeClassSpec == nil || got.NodeClassSpec.ServiceAccount != "origin@test-project.iam.gserviceaccount.com" {
		t.Fatalf("NodeClassSpec = %#v", got.NodeClassSpec)
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

func TestGCENodeClassModelAliasCompatibilityRoundTrip(t *testing.T) {
	ctx := context.Background()
	remote := GCENodeClass{
		Name: "cloudpilot",
		NodeClassSpec: &GCENodeClassSpec{
			ImageSelectorTerms: []GCEImageSelectorTerm{{
				Alias: "ContainerOptimizedOS@latest",
			}},
		},
	}

	model, err := remote.ToGCENodeClassModel(ctx)
	if err != nil {
		t.Fatalf("ToGCENodeClassModel() error = %v", err)
	}
	terms, diags := model.ImageSelectorTerms.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("ImageSelectorTerms diagnostics = %v", diags)
	}
	if len(terms) != 1 || terms[0].Family.ValueString() != "ContainerOptimizedOS" || terms[0].Version.ValueString() != "latest" {
		t.Fatalf("ImageSelectorTerms model = %#v", terms)
	}

	roundTrip, err := model.ToGCENodeClass(ctx, GCENodeClass{Name: "cloudpilot"})
	if err != nil {
		t.Fatalf("ToGCENodeClass() error = %v", err)
	}
	if len(roundTrip.NodeClassSpec.ImageSelectorTerms) != 1 {
		t.Fatalf("ImageSelectorTerms = %#v", roundTrip.NodeClassSpec.ImageSelectorTerms)
	}
	got := roundTrip.NodeClassSpec.ImageSelectorTerms[0]
	if got.Alias != "" || got.Family != "ContainerOptimizedOS" || got.Version != "latest" {
		t.Fatalf("ImageSelectorTerms[0] = %#v", got)
	}
}

func TestGCENodeClassModelClearsLegacyImageFamilyWhenSelectorsTouched(t *testing.T) {
	ctx := context.Background()
	var current GCENodeClass
	raw := []byte(`{
		"name":"cloudpilot",
		"nodeClassSpec":{
			"imageFamily":"ContainerOptimizedOS",
			"imageSelectorTerms":[{"alias":"ContainerOptimizedOS@latest"}]
		}
	}`)
	if err := json.Unmarshal(raw, &current); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	model := GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		ImageSelectorTerms: customfield.NewObjectListMust(ctx, []GCEImageSelectorTermModel{{
			Family:  types.StringValue("ContainerOptimizedOS"),
			Channel: types.StringValue("cluster"),
		}}),
	}

	updated, err := model.ToGCENodeClass(ctx, current)
	if err != nil {
		t.Fatalf("ToGCENodeClass() error = %v", err)
	}
	marshaled, err := json.Marshal(updated)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(marshaled, &payload); err != nil {
		t.Fatalf("json.Unmarshal(marshaled) error = %v", err)
	}
	spec := payload["nodeClassSpec"].(map[string]any)
	if _, exists := spec["imageFamily"]; exists {
		t.Fatalf("imageFamily should be cleared when typed image selector terms are set: %#v", spec["imageFamily"])
	}
}

func TestGCENodeClassModelRejectsNewTypedNodeClassWithoutImageSelectorTerms(t *testing.T) {
	model := GCENodeClassModel{
		Name:           types.StringValue("cloudpilot"),
		ServiceAccount: types.StringValue("nodes@test-project.iam.gserviceaccount.com"),
	}

	_, err := model.ToGCENodeClass(context.Background(), GCENodeClass{Name: "cloudpilot"})
	if err == nil || !strings.Contains(err.Error(), "image_selector_terms must be set") {
		t.Fatalf("ToGCENodeClass() error = %v, want missing image_selector_terms validation", err)
	}
}

func TestGCENodeClassModelRejectsExplicitEmptyImageSelectorTerms(t *testing.T) {
	ctx := context.Background()
	current := GCENodeClass{
		Name: "cloudpilot",
		NodeClassSpec: &GCENodeClassSpec{
			ImageSelectorTerms: []GCEImageSelectorTerm{{
				Family:  "ContainerOptimizedOS",
				Version: "latest",
			}},
		},
	}
	model := GCENodeClassModel{
		Name:               types.StringValue("cloudpilot"),
		ImageSelectorTerms: customfield.NewObjectListMust(ctx, []GCEImageSelectorTermModel{}),
	}

	_, err := model.ToGCENodeClass(ctx, current)
	if err == nil || !strings.Contains(err.Error(), "image_selector_terms cannot be empty") {
		t.Fatalf("ToGCENodeClass() error = %v, want explicit empty image_selector_terms validation", err)
	}
}

func TestGCENodeClassModelPreservesNonExposedKubeletFields(t *testing.T) {
	ctx := context.Background()
	maxPods := int32(110)
	current := GCENodeClass{
		Name: "cloudpilot",
		NodeClassSpec: &GCENodeClassSpec{
			KubeletConfiguration: &GCEKubeletConfiguration{
				MaxPods:        &maxPods,
				KubeReserved:   map[string]gcpproviderv1alpha1.KubeletQuantity{"cpu": "100m"},
				EvictionHard:   map[string]gcpproviderv1alpha1.KubeletQuantity{"memory.available": "10%"},
				SystemReserved: map[string]gcpproviderv1alpha1.KubeletQuantity{"memory": "1Gi"},
			},
		},
	}
	model := GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		KubeletConfiguration: customfield.NewObjectMust(ctx, &GCEKubeletConfigurationModel{
			KubeReserved:   customfield.NewMapMust[types.String](ctx, map[string]types.String{"cpu": types.StringValue("200m")}),
			SystemReserved: customfield.NewMapMust[types.String](ctx, map[string]types.String{"memory": types.StringValue("2Gi")}),
			EvictionHard:   customfield.NewMapMust[types.String](ctx, map[string]types.String{"memory.available": types.StringValue("15%")}),
			EvictionSoft:   customfield.NewMapMust[types.String](ctx, map[string]types.String{"memory.available": types.StringValue("20%")}),
		}),
	}

	updated, err := model.ToGCENodeClass(ctx, current)
	if err != nil {
		t.Fatalf("ToGCENodeClass() error = %v", err)
	}
	if updated.NodeClassSpec.KubeletConfiguration == nil {
		t.Fatal("KubeletConfiguration is nil")
	}
	if updated.NodeClassSpec.KubeletConfiguration.MaxPods == nil || *updated.NodeClassSpec.KubeletConfiguration.MaxPods != 110 {
		t.Fatalf("MaxPods = %#v", updated.NodeClassSpec.KubeletConfiguration.MaxPods)
	}
	if got := string(updated.NodeClassSpec.KubeletConfiguration.KubeReserved["cpu"]); got != "200m" {
		t.Fatalf("KubeReserved[cpu] = %q", got)
	}
	if got := string(updated.NodeClassSpec.KubeletConfiguration.SystemReserved["memory"]); got != "2Gi" {
		t.Fatalf("SystemReserved[memory] = %q", got)
	}
	if got := string(updated.NodeClassSpec.KubeletConfiguration.EvictionHard["memory.available"]); got != "15%" {
		t.Fatalf("EvictionHard[memory.available] = %q", got)
	}
	if got := string(updated.NodeClassSpec.KubeletConfiguration.EvictionSoft["memory.available"]); got != "20%" {
		t.Fatalf("EvictionSoft[memory.available] = %q", got)
	}
}

func TestGCENodeClassModelPreservesNonExposedDiskFields(t *testing.T) {
	ctx := context.Background()
	current := GCENodeClass{
		Name: "cloudpilot",
		NodeClassSpec: &GCENodeClassSpec{
			Disks: []GCEDisk{{
				SizeGiB:               80,
				Category:              "pd-balanced",
				Boot:                  true,
				SecondaryBootImage:    "global/images/cache-image",
				SecondaryBootMode:     "CONTAINER_IMAGE_CACHE",
				KMSKeyName:            "projects/test/locations/us/keyRings/ring/cryptoKeys/key",
				KMSKeyServiceAccount:  "kms@test-project.iam.gserviceaccount.com",
				ProvisionedIOPS:       lo.ToPtr[int64](5000),
				ProvisionedThroughput: lo.ToPtr[int64](200),
			}},
			ImageSelectorTerms: []GCEImageSelectorTerm{{
				Family:  "ContainerOptimizedOS",
				Version: "latest",
			}},
		},
	}
	model := GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		Disks: customfield.NewObjectListMust(ctx, []GCEDiskModel{{
			SizeGiB:  types.Int64Value(120),
			Category: types.StringValue("pd-ssd"),
			Boot:     types.BoolValue(true),
		}}),
	}

	updated, err := model.ToGCENodeClass(ctx, current)
	if err != nil {
		t.Fatalf("ToGCENodeClass() error = %v", err)
	}
	if len(updated.NodeClassSpec.Disks) != 1 {
		t.Fatalf("Disks = %#v", updated.NodeClassSpec.Disks)
	}
	got := updated.NodeClassSpec.Disks[0]
	if got.SizeGiB != 120 || got.Category != "pd-ssd" || !got.Boot {
		t.Fatalf("typed disk fields = %#v", got)
	}
	if got.SecondaryBootImage != "global/images/cache-image" || got.SecondaryBootMode != "CONTAINER_IMAGE_CACHE" {
		t.Fatalf("secondary boot fields were lost: %#v", got)
	}
	if got.KMSKeyName == "" || got.KMSKeyServiceAccount == "" {
		t.Fatalf("kms fields were lost: %#v", got)
	}
	if got.ProvisionedIOPS == nil || *got.ProvisionedIOPS != 5000 || got.ProvisionedThroughput == nil || *got.ProvisionedThroughput != 200 {
		t.Fatalf("provisioned performance fields were lost: %#v", got)
	}
}

func TestGCENodeClassModelNetworkConfigPreservesUntouchedFields(t *testing.T) {
	ctx := context.Background()
	enablePrivateNodes := true
	current := GCENodeClass{
		Name: "cloudpilot",
		NodeClassSpec: &GCENodeClassSpec{
			NetworkConfig: &GCENetworkConfig{
				EnablePrivateNodes: &enablePrivateNodes,
				Subnetwork:         "projects/test/regions/us-central1/subnetworks/original",
				AdditionalNetworkInterfaces: []GCEAdditionalNetworkInterface{{
					Network:    "projects/test/global/networks/default",
					Subnetwork: "projects/test/regions/us-central1/subnetworks/secondary",
				}},
			},
			ImageSelectorTerms: []GCEImageSelectorTerm{{
				Family:  "ContainerOptimizedOS",
				Version: "latest",
			}},
		},
	}
	model := GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		NetworkConfig: customfield.NewObjectMust(ctx, &GCENetworkConfigModel{
			Subnetwork: types.StringValue("projects/test/regions/us-central1/subnetworks/updated"),
		}),
	}

	updated, err := model.ToGCENodeClass(ctx, current)
	if err != nil {
		t.Fatalf("ToGCENodeClass() error = %v", err)
	}
	if updated.NodeClassSpec.NetworkConfig == nil {
		t.Fatal("NetworkConfig is nil")
	}
	if updated.NodeClassSpec.NetworkConfig.Subnetwork != "projects/test/regions/us-central1/subnetworks/updated" {
		t.Fatalf("Subnetwork = %q", updated.NodeClassSpec.NetworkConfig.Subnetwork)
	}
	if updated.NodeClassSpec.NetworkConfig.EnablePrivateNodes == nil || !*updated.NodeClassSpec.NetworkConfig.EnablePrivateNodes {
		t.Fatalf("EnablePrivateNodes = %#v", updated.NodeClassSpec.NetworkConfig.EnablePrivateNodes)
	}
	if len(updated.NodeClassSpec.NetworkConfig.AdditionalNetworkInterfaces) != 1 || updated.NodeClassSpec.NetworkConfig.AdditionalNetworkInterfaces[0].Subnetwork != "projects/test/regions/us-central1/subnetworks/secondary" {
		t.Fatalf("AdditionalNetworkInterfaces = %#v", updated.NodeClassSpec.NetworkConfig.AdditionalNetworkInterfaces)
	}
}

func TestGCENodeClassModelRejectsInvalidTypedImageSelectorCombination(t *testing.T) {
	ctx := context.Background()
	model := GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		ImageSelectorTerms: customfield.NewObjectListMust(ctx, []GCEImageSelectorTermModel{{
			ID:      types.StringValue("projects/test/global/images/image"),
			Family:  types.StringValue("ContainerOptimizedOS"),
			Version: types.StringValue("latest"),
		}}),
	}

	_, err := model.ToGCENodeClass(ctx, GCENodeClass{Name: "cloudpilot"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("ToGCENodeClass() error = %v, want invalid image selector validation", err)
	}
}

func TestGCENodeClassModelDoesNotInferUbuntu2204FromLegacyUbuntuAlias(t *testing.T) {
	ctx := context.Background()
	remote := GCENodeClass{
		Name: "cloudpilot",
		NodeClassSpec: &GCENodeClassSpec{
			ImageSelectorTerms: []GCEImageSelectorTerm{{
				Alias: "Ubuntu@latest",
			}},
		},
	}

	model, err := remote.ToGCENodeClassModel(ctx)
	if err != nil {
		t.Fatalf("ToGCENodeClassModel() error = %v", err)
	}
	terms, diags := model.ImageSelectorTerms.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("ImageSelectorTerms diagnostics = %v", diags)
	}
	if len(terms) != 1 {
		t.Fatalf("ImageSelectorTerms = %#v", terms)
	}
	if !terms[0].Family.IsNull() || !terms[0].Version.IsNull() {
		t.Fatalf("legacy Ubuntu alias should not be rewritten to a typed family/version: %#v", terms[0])
	}
}

func TestGCENodePoolModelWritesZoneRequirementWithGCPKey(t *testing.T) {
	ctx := context.Background()
	model := GCENodePoolModel{
		Name:      types.StringValue("cloudpilot-general"),
		Enable:    types.BoolValue(true),
		NodeClass: types.StringValue("cloudpilot"),
		Zone: &[]types.String{
			types.StringValue("us-central1-a"),
		},
	}

	updated, err := model.ToGCENodePool(ctx, GCENodePool{Name: "cloudpilot-general"})
	if err != nil {
		t.Fatalf("ToGCENodePool() error = %v", err)
	}
	if got := gcpRequirementsToStrings(updated.NodePoolSpec.Template.Spec.Requirements, gceLabelTopologyZoneID, corev1.NodeSelectorOpIn); got == nil || len(*got) != 1 || (*got)[0].ValueString() != "us-central1-a" {
		t.Fatalf("GCP zone requirement = %#v", updated.NodePoolSpec.Template.Spec.Requirements)
	}
	if got := gcpRequirementsToStrings(updated.NodePoolSpec.Template.Spec.Requirements, corev1.LabelTopologyZone, corev1.NodeSelectorOpIn); got != nil {
		t.Fatalf("generic zone requirement should be absent, got %#v", *got)
	}
}

func TestGCENodePoolModelPrefersGCPZoneRequirementOnRead(t *testing.T) {
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
						{Key: corev1.LabelTopologyZone, Operator: corev1.NodeSelectorOpIn, Values: []string{"generic-zone"}},
						{Key: gceLabelTopologyZoneID, Operator: corev1.NodeSelectorOpIn, Values: []string{"gcp-zone"}},
					},
				},
			},
		},
	}

	model, err := remote.ToGCENodePoolModel(ctx)
	if err != nil {
		t.Fatalf("ToGCENodePoolModel() error = %v", err)
	}
	if model.Zone == nil || len(*model.Zone) != 1 || (*model.Zone)[0].ValueString() != "gcp-zone" {
		t.Fatalf("Zone = %#v", model.Zone)
	}
}

func TestGCENodeClassModelRejectsUnsupportedImageSelectorFamilyAndChannelAndVersion(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		term GCEImageSelectorTermModel
	}{
		{
			name: "unsupported family",
			term: GCEImageSelectorTermModel{
				Family:  types.StringValue("Ubuntu"),
				Version: types.StringValue("latest"),
			},
		},
		{
			name: "unsupported channel",
			term: GCEImageSelectorTermModel{
				Family:  types.StringValue("ContainerOptimizedOS"),
				Channel: types.StringValue("beta"),
			},
		},
		{
			name: "invalid cos version",
			term: GCEImageSelectorTermModel{
				Family:  types.StringValue("ContainerOptimizedOS"),
				Version: types.StringValue("bad-version"),
			},
		},
		{
			name: "invalid ubuntu version",
			term: GCEImageSelectorTermModel{
				Family:  types.StringValue("Ubuntu2404"),
				Version: types.StringValue("20260416"),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := GCENodeClassModel{
				Name: types.StringValue("cloudpilot"),
				ImageSelectorTerms: customfield.NewObjectListMust(ctx, []GCEImageSelectorTermModel{
					tc.term,
				}),
			}
			_, err := model.ToGCENodeClass(ctx, GCENodeClass{Name: "cloudpilot"})
			if err == nil {
				t.Fatalf("ToGCENodeClass() error = nil for case %q", tc.name)
			}
		})
	}
}

func TestGCENodeClassModelRejectsMixedChannelAndCOSLatestSelectorList(t *testing.T) {
	ctx := context.Background()
	model := GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		ImageSelectorTerms: customfield.NewObjectListMust(ctx, []GCEImageSelectorTermModel{
			{
				Family:  types.StringValue("ContainerOptimizedOS"),
				Channel: types.StringValue("cluster"),
			},
			{
				Family:  types.StringValue("ContainerOptimizedOS"),
				Version: types.StringValue("latest"),
			},
		}),
	}

	_, err := model.ToGCENodeClass(ctx, GCENodeClass{Name: "cloudpilot"})
	if err == nil || !strings.Contains(err.Error(), "channel-based terms cannot be mixed") {
		t.Fatalf("ToGCENodeClass() error = %v, want mixed-list validation", err)
	}
}

func TestGCENodeClassModelLegacyAliasMixedSelectorListStillRoundTrips(t *testing.T) {
	ctx := context.Background()
	remote := GCENodeClass{
		Name: "cloudpilot",
		NodeClassSpec: &GCENodeClassSpec{
			ImageSelectorTerms: []GCEImageSelectorTerm{
				{Alias: "ContainerOptimizedOS@latest"},
				{Family: "ContainerOptimizedOS", Channel: "cluster"},
			},
		},
	}

	model, err := remote.ToGCENodeClassModel(ctx)
	if err != nil {
		t.Fatalf("ToGCENodeClassModel() error = %v", err)
	}

	roundTrip, err := model.ToGCENodeClass(ctx, remote)
	if err != nil {
		t.Fatalf("ToGCENodeClass() error = %v", err)
	}
	if len(roundTrip.NodeClassSpec.ImageSelectorTerms) != 2 {
		t.Fatalf("ImageSelectorTerms = %#v", roundTrip.NodeClassSpec.ImageSelectorTerms)
	}
	if roundTrip.NodeClassSpec.ImageSelectorTerms[0].Alias != "ContainerOptimizedOS@latest" {
		t.Fatalf("legacy alias term was not preserved: %#v", roundTrip.NodeClassSpec.ImageSelectorTerms)
	}
	if roundTrip.NodeClassSpec.ImageSelectorTerms[1].Channel != "cluster" {
		t.Fatalf("channel term was not preserved: %#v", roundTrip.NodeClassSpec.ImageSelectorTerms)
	}
}

func TestGCENodeClassModelRejectsAdditionalNetworkInterfaceWithoutSubnetwork(t *testing.T) {
	ctx := context.Background()
	model := GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		ImageSelectorTerms: customfield.NewObjectListMust(ctx, []GCEImageSelectorTermModel{{
			Family:  types.StringValue("ContainerOptimizedOS"),
			Version: types.StringValue("latest"),
		}}),
		NetworkConfig: customfield.NewObjectMust(ctx, &GCENetworkConfigModel{
			AdditionalNetworkInterfaces: customfield.NewObjectListMust(ctx, []GCEAdditionalNetworkInterfaceModel{{
				Network: types.StringValue("projects/test/global/networks/default"),
			}}),
		}),
	}

	_, err := model.ToGCENodeClass(ctx, GCENodeClass{Name: "cloudpilot"})
	if err == nil || !strings.Contains(err.Error(), "additional_network_interfaces[0].subnetwork is required") {
		t.Fatalf("ToGCENodeClass() error = %v, want missing subnetwork validation", err)
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

func TestGCPUpdateRequirementsRemovesOnlyMatchedRequirement(t *testing.T) {
	requirements := []gcpcorev1.NodeSelectorRequirementWithMinValues{
		{Key: "first", Operator: corev1.NodeSelectorOpIn, Values: []string{"a"}},
		{Key: gceLabelTopologyZoneID, Operator: corev1.NodeSelectorOpIn, Values: []string{"us-central1-a"}},
		{Key: "last", Operator: corev1.NodeSelectorOpIn, Values: []string{"z"}},
	}

	updated := gcpUpdateRequirements(gceLabelTopologyZoneID, corev1.NodeSelectorOpIn, nil, requirements)
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
