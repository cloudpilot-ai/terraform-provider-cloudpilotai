package api

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	gcpproviderv1alpha1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter-provider-gcp/apis/v1alpha1"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEC2TypedNodeClassPreservesNewAndUnknownFields(t *testing.T) {
	ctx := context.Background()
	const payload = `{"name":"example","futureClass":{"enabled":true},"memoryExtension":{"enabled":true,"coldMemory":{"maxGiB":32,"futureCold":7}},"nodeClassSpec":{"role":"old","blockDeviceMappings":[{"deviceName":"/dev/xvda","ebs":{"volumeType":"gp3","volumeSize":"100Gi","volumeSizePolicy":{"perVCPUGiB":2,"maxSizeGiB":512},"futureEBS":{"value":9007199254740993}}}],"futureSpec":true}}`
	var current EC2NodeClass
	if err := json.Unmarshal([]byte(payload), &current); err != nil {
		t.Fatal(err)
	}
	model := EC2NodeClassModel{Name: types.StringValue("example"), Role: types.StringValue("updated")}
	out, err := model.ToEC2NodeClass(ctx, "example", current, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"futureClass":{"enabled":true}`, `"futureSpec":true`, `"futureEBS":{"value":9007199254740993}`, `"futureCold":7`, `"volumeSizePolicy":{"maxSizeGiB":512,"perVCPUGiB":2}`, `"role":"updated"`} {
		if !strings.Contains(string(encoded), fragment) {
			t.Fatalf("missing %s: %s", fragment, encoded)
		}
	}
	if current.NodeClassSpec.Role != "old" {
		t.Fatal("typed update mutated its input")
	}
}

func TestEC2OriginJSONPreservesNestedUnknownFields(t *testing.T) {
	ctx := context.Background()
	const classJSON = `{"name":"example","memoryExtension":{"enabled":false,"futureMemory":true},"nodeClassSpec":{"futureSpec":true,"blockDeviceMappings":[{"deviceName":"/dev/xvda","ebs":{"volumeSize":"80Gi","volumeSizePolicy":{"maxSizeGiB":512},"futureEBS":true}}]}}`
	class, err := (&EC2NodeClassModel{OriginNodeClassJSON: types.StringValue(classJSON)}).ToEC2NodeClass(ctx, "example", EC2NodeClass{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(class)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"futureMemory":true`, `"futureSpec":true`, `"futureEBS":true`, `"maxSizeGiB":512`} {
		if !strings.Contains(string(encoded), fragment) {
			t.Fatalf("missing %s: %s", fragment, encoded)
		}
	}
	const poolJSON = `{"name":"example","nriSwapPolicy":{"enabled":true,"memoryLimitMultiplier":2,"futureNRI":true},"jobNodePoolOptimization":{"minActiveNodes":2,"futureJob":true},"nodePoolSpec":{"disruption":{"consolidateAfter":"1m","disruptionCooldown":"10m","futureDisruption":true}}}`
	pool, err := (&EC2NodePoolModel{OriginNodePoolJSON: types.StringValue(poolJSON)}).ToEC2NodePool(ctx, EC2NodePool{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(pool)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"futureNRI":true`, `"futureJob":true`, `"futureDisruption":true`, `"disruptionCooldown":"10m"`} {
		if !strings.Contains(string(encoded), fragment) {
			t.Fatalf("missing %s: %s", fragment, encoded)
		}
	}
}

func TestEC2TypedNodePoolPreservesPoliciesAndSupportsExplicitDisable(t *testing.T) {
	ctx := context.Background()
	var current EC2NodePool
	err := json.Unmarshal([]byte(`{"name":"example","enable":true,"nriSwapPolicy":{"enabled":true,"memoryLimitMultiplier":2},"jobNodePoolOptimization":{"minActiveNodes":2,"provisioningDelay":"30s"},"nodePoolSpec":{"template":{"spec":{"nodeClassRef":{"name":"example","group":"karpenter.k8s.aws","kind":"EC2NodeClass"}}},"disruption":{"consolidateAfter":"1m","disruptionCooldown":"10m","futureDisruption":true}}}`), &current)
	if err != nil {
		t.Fatal(err)
	}
	model := EC2NodePoolModel{Name: types.StringValue("example"), DisruptionCooldown: types.StringUnknown()}
	out, err := model.ToEC2NodePool(ctx, current, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.NRISwapPolicy == nil || !out.NRISwapPolicy.Enabled || out.JobNodePoolOptimization == nil || out.NodePoolSpec.Disruption.DisruptionCooldown.Duration != 10*time.Minute {
		t.Fatalf("remote policies lost: %#v", out)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"futureDisruption":true`) {
		t.Fatalf("unknown disruption field lost: %s", encoded)
	}
	hydrated, err := out.ToEC2NodePoolModel()
	if err != nil {
		t.Fatal(err)
	}
	if hydrated.DisruptionCooldown.ValueString() != "10m0s" || hydrated.NRISwapPolicy.IsNull() || hydrated.JobNodePoolOptimization.IsNull() {
		t.Fatalf("policies not hydrated: %#v", hydrated)
	}
	model.DisruptionCooldown = types.StringValue("0s")
	model.NRISwapPolicy = customfield.NewObjectMust(ctx, &NRISwapPolicyModel{Enabled: types.BoolValue(false)})
	model.JobNodePoolOptimization = customfield.NewObjectMust(ctx, &JobNodePoolOptimizationModel{})
	out, err = model.ToEC2NodePool(ctx, current, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.NRISwapPolicy.Enabled || out.NodePoolSpec.Disruption.DisruptionCooldown.Duration != 0 || out.JobNodePoolOptimization == nil || out.JobNodePoolOptimization.MinActiveNodes != nil || out.JobNodePoolOptimization.ProvisioningDelay != nil {
		t.Fatalf("explicit disable or empty Job reset failed: %#v", out)
	}
	if current.NodePoolSpec.Disruption.DisruptionCooldown.Duration != 10*time.Minute {
		t.Fatal("typed update mutated its input")
	}
}

func TestGCETypedNodeClassPreservesNestedNewAndUnknownFields(t *testing.T) {
	ctx := context.Background()
	var current GCENodeClass
	const payload = `{"name":"example","enableLocalSSDEphemeralStorage":true,"futureClass":true,"nodeClassSpec":{"ephemeralStorageLocalSSD":{"autoCount":true,"futureSSD":3},"disks":[{"boot":true,"category":"pd-balanced","sizeGiB":100,"sizePolicy":{"perVCPUGiB":2,"maxSizeGiB":512},"futureDisk":{"enabled":true}}],"imageSelectorTerms":[{"family":"ContainerOptimizedOS","channel":"cluster"}]}}`
	if err := json.Unmarshal([]byte(payload), &current); err != nil {
		t.Fatal(err)
	}
	model := GCENodeClassModel{Name: types.StringValue("example"), EnableLocalSSDEphemeralStorage: types.BoolValue(true), Disks: customfield.NewObjectListMust(ctx, []GCEDiskModel{{SizeGiB: types.Int64Value(120)}})}
	out, err := model.ToGCENodeClass(ctx, current)
	if err != nil {
		t.Fatal(err)
	}
	if !out.NodeClassSpec.EphemeralStorageLocalSSD.AutoCount || out.NodeClassSpec.Disks[0].SizePolicy == nil {
		t.Fatalf("new fields lost: %#v", out.NodeClassSpec)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"futureClass":true`, `"futureSSD":3`, `"futureDisk":{"enabled":true}`, `"sizeGiB":120`} {
		if !strings.Contains(string(encoded), fragment) {
			t.Fatalf("missing %s: %s", fragment, encoded)
		}
	}
	if current.NodeClassSpec.Disks[0].SizeGiB != 100 {
		t.Fatal("typed update mutated its input")
	}
	hydrated, err := out.ToGCENodeClassModel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ssd, diags := hydrated.EphemeralStorageLocalSSD.Value(ctx)
	if diags.HasError() || !ssd.AutoCount.ValueBool() {
		t.Fatalf("auto_count not hydrated: %#v %v", ssd, diags)
	}
}

func TestGCETypedNodePoolPreservesCooldownAndUnknownFields(t *testing.T) {
	ctx := context.Background()
	var current GCENodePool
	if err := json.Unmarshal([]byte(`{"name":"example","futurePool":true,"jobNodePoolOptimization":{"minActiveNodes":2},"nodePoolSpec":{"template":{"spec":{"nodeClassRef":{"name":"example","group":"karpenter.k8s.gcp","kind":"GCENodeClass"}}},"disruption":{"consolidateAfter":"1m","disruptionCooldown":"10m","futureDisruption":true}}}`), &current); err != nil {
		t.Fatal(err)
	}
	model := GCENodePoolModel{Name: types.StringValue("example"), Enable: types.BoolValue(true)}
	out, err := model.ToGCENodePool(ctx, current)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"futurePool":true`, `"futureDisruption":true`, `"disruptionCooldown":"10m0s"`} {
		if !strings.Contains(string(encoded), fragment) {
			t.Fatalf("missing %s: %s", fragment, encoded)
		}
	}
	hydrated, err := out.ToGCENodePoolModel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if hydrated.DisruptionCooldown.ValueString() != "10m0s" || hydrated.JobNodePoolOptimization.IsNull() {
		t.Fatal("pool policy readback missing")
	}
}

func TestGCELocalSSDModeTransitions(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		count     *int32
		auto      bool
		model     GCEEphemeralStorageLocalSSDModel
		wantCount bool
		wantAuto  bool
		invalid   bool
	}{
		{name: "fixed to auto", count: ptrInt32(2), model: GCEEphemeralStorageLocalSSDModel{AutoCount: types.BoolValue(true)}, wantAuto: true},
		{name: "auto to fixed", auto: true, model: GCEEphemeralStorageLocalSSDModel{Count: types.Int32Value(2)}, wantCount: true},
		{name: "explicit conflict", model: GCEEphemeralStorageLocalSSDModel{Count: types.Int32Value(2), AutoCount: types.BoolValue(true)}, invalid: true},
		{name: "empty clears fixed", count: ptrInt32(2), model: GCEEphemeralStorageLocalSSDModel{}},
		{name: "empty clears auto", auto: true, model: GCEEphemeralStorageLocalSSDModel{}},
		{name: "unknown preserves fixed", count: ptrInt32(2), model: GCEEphemeralStorageLocalSSDModel{Count: types.Int32Unknown(), AutoCount: types.BoolUnknown()}, wantCount: true},
		{name: "unknown preserves auto", auto: true, model: GCEEphemeralStorageLocalSSDModel{Count: types.Int32Unknown(), AutoCount: types.BoolUnknown()}, wantAuto: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := GCENodeClass{Name: "example", EnableLocalSSDEphemeralStorage: ptrBool(true), NodeClassSpec: &GCENodeClassSpec{EphemeralStorageLocalSSD: &GCEEphemeralStorageLocalSSD{Count: tc.count, AutoCount: tc.auto}}}
			model := GCENodeClassModel{Name: types.StringValue("example"), EphemeralStorageLocalSSD: customfield.NewObjectMust(ctx, &tc.model)}
			out, err := model.ToGCENodeClass(ctx, current)
			if tc.invalid {
				if err == nil {
					t.Fatal("expected mutually exclusive count/auto_count rejection")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := out.NodeClassSpec.EphemeralStorageLocalSSD
			if (got.Count != nil) != tc.wantCount || got.AutoCount != tc.wantAuto {
				t.Fatalf("unexpected mode: %#v", got)
			}
		})
	}
}

func ptrInt32(v int32) *int32 { return &v }
func ptrBool(v bool) *bool    { return &v }

func TestVolumeSizePolicyLimitsAndOwnership(t *testing.T) {
	ctx := context.Background()
	model := func(per, max int64) customfield.NestedObject[VolumeSizePolicyModel] {
		return customfield.NewObjectMust(ctx, &VolumeSizePolicyModel{PerVCPUGiB: types.Int64Value(per), MaxSizeGiB: types.Int64Value(max)})
	}
	for _, tc := range []struct {
		name     string
		per, max int64
		gce      bool
		invalid  bool
	}{
		{name: "aws expanded gp3 limit", max: 65536},
		{name: "aws per CPU too large", per: 16385, max: 65536, invalid: true},
		{name: "aws max too large", max: 65537, invalid: true},
		{name: "negative per CPU", per: -1, max: 512, invalid: true},
		{name: "gce max", per: 65536, max: 65536, gce: true},
		{name: "gce below minimum", max: 9, gce: true, invalid: true},
		{name: "gce int32 overflow", per: math.MaxInt32 + 1, max: 512, gce: true, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.gce {
				_, err = gceVolumePolicyFromModel(ctx, model(tc.per, tc.max), nil)
			} else {
				_, err = awsVolumePolicyFromModel(ctx, model(tc.per, tc.max), nil)
			}
			if (err != nil) != tc.invalid {
				t.Fatalf("error=%v, expected invalid=%t", err, tc.invalid)
			}
		})
	}
	base := resource.MustParse("80Gi")
	policy := &awsproviderv1.VolumeSizePolicy{MaxSizeGiB: 512}
	mapping := &awsproviderv1.BlockDeviceMapping{EBS: &awsproviderv1.BlockDevice{VolumeSize: &base, VolumeSizePolicy: policy}}
	if err := validateEC2VolumeSizePolicies([]*awsproviderv1.BlockDeviceMapping{mapping}); err != nil {
		t.Fatalf("single unmarked root should be accepted: %v", err)
	}
	if err := validateEC2VolumeSizePolicies([]*awsproviderv1.BlockDeviceMapping{mapping, {EBS: &awsproviderv1.BlockDevice{}}}); err == nil {
		t.Fatal("ambiguous root policy accepted")
	}
	mapping.RootVolume = true
	standard := "standard"
	mapping.EBS.VolumeType = &standard
	mapping.EBS.VolumeSizePolicy = &awsproviderv1.VolumeSizePolicy{MaxSizeGiB: 2048}
	if err := validateEC2VolumeSizePolicies([]*awsproviderv1.BlockDeviceMapping{mapping}); err == nil {
		t.Fatal("standard-volume policy over 1024 GiB accepted")
	}
	if err := validateGCENodeClassDiskSizePolicy(&GCENodeClassSpec{Disks: []GCEDisk{{Boot: false, SizeGiB: 80, SizePolicy: &gcpproviderv1alpha1.VolumeSizePolicy{MaxSizeGiB: 512}}}}); err == nil {
		t.Fatal("nonboot GCE policy accepted")
	}
}

func TestNAOptionalPolicyValidationAndPreservation(t *testing.T) {
	ctx := context.Background()
	original := &awsproviderv1.VolumeSizePolicy{PerVCPUGiB: 4, MaxSizeGiB: 512}
	policy, err := awsVolumePolicyFromModel(ctx, customfield.NullObject[VolumeSizePolicyModel](ctx), original)
	if err != nil || policy != original {
		t.Fatal("omitted disk policy did not preserve remote value")
	}
	for _, ratio := range []float64{1, 10.001, 1.0001} {
		_, err := nriSwapFromModel(ctx, customfield.NewObjectMust(ctx, &NRISwapPolicyModel{Enabled: types.BoolValue(true), MemoryLimitMultiplier: types.Float64Value(ratio)}), nil)
		if err == nil {
			t.Fatalf("invalid NRI multiplier accepted: %v", ratio)
		}
	}
	_, err = jobOptimizationFromModel(ctx, customfield.NewObjectMust(ctx, &JobNodePoolOptimizationModel{MinActiveNodes: types.Int32Value(0)}), nil)
	if err == nil {
		t.Fatal("zero min_active_nodes accepted")
	}
	for _, value := range []string{"-1s", "0s", " 30s", "no-duration"} {
		_, err = jobOptimizationFromModel(ctx, customfield.NewObjectMust(ctx, &JobNodePoolOptimizationModel{ProvisioningDelay: types.StringValue(value)}), nil)
		if err == nil {
			t.Fatalf("invalid Job provisioning delay accepted: %q", value)
		}
	}
	old := &metav1.Duration{Duration: time.Minute}
	got, err := cooldownFromModel(types.StringNull(), old)
	if err != nil || got != old {
		t.Fatal("omitted cooldown did not preserve remote value")
	}
}

func TestMemoryExtensionPartialUpdatesPreserveRemoteFields(t *testing.T) {
	ctx := context.Background()
	maximum := int32(32)
	current := &MemoryExtensionPolicy{Enabled: true, ColdMemory: ColdMemoryPolicy{MaxGiB: &maximum}}
	model := MemoryExtensionModel{Enabled: types.BoolValue(true), ColdMemory: customfield.NewObjectMust(ctx, &ColdMemoryModel{MemoryRatioPercent: types.Int32Value(40)})}
	out, err := model.ToAPI(ctx, current)
	if err != nil {
		t.Fatal(err)
	}
	if *out.ColdMemory.MaxGiB != 32 || *out.ColdMemory.MemoryRatioPercent != 40 {
		t.Fatalf("partial policy dropped remote fields: %#v", out)
	}
	hydrated := memoryExtensionModelFromAPI(ctx, out)
	read, diags := hydrated.Value(ctx)
	if diags.HasError() || !read.Enabled.ValueBool() {
		t.Fatalf("memory extension hydration failed: %v", diags)
	}
	invalid := MemoryExtensionModel{Enabled: types.BoolValue(true), ColdMemory: customfield.NewObjectMust(ctx, &ColdMemoryModel{MinGiB: types.Int32Value(20), MaxGiB: types.Int32Value(10)})}
	if _, err := invalid.ToAPI(ctx, nil); err == nil {
		t.Fatal("inverted memory size bounds accepted")
	}
	disabled, err := (MemoryExtensionModel{Enabled: types.BoolValue(false)}).ToAPI(ctx, current)
	if err != nil || disabled.Enabled {
		t.Fatal("explicit Memory Extension disable ignored")
	}
}

func TestNodeClassOriginJSONPreservesFieldOmission(t *testing.T) {
	ctx := context.Background()
	const payload = `{"name":"example","nodeClassSpec":{"futureField":true}}`
	e, err := (&EC2NodeClassModel{OriginNodeClassJSON: types.StringValue(payload)}).ToEC2NodeClass(ctx, "example", EC2NodeClass{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	g, err := (&GCENodeClassModel{OriginNodeClassJSON: types.StringValue(payload)}).ToGCENodeClass(ctx, GCENodeClass{})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{e, g} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "enableImageAccelerator") {
			t.Fatalf("raw omission changed to false: %s", encoded)
		}
	}
}

func TestGCEDiskResizePreservesPositionalUnknownFields(t *testing.T) {
	ctx := context.Background()
	var current GCENodeClass
	if err := json.Unmarshal([]byte(`{"name":"example","nodeClassSpec":{"disks":[{"boot":true,"category":"pd-balanced","sizeGiB":80,"futureDisk":42}],"imageSelectorTerms":[{"family":"ContainerOptimizedOS","channel":"cluster"}]}}`), &current); err != nil {
		t.Fatal(err)
	}
	model := GCENodeClassModel{Name: types.StringValue("example"), Disks: customfield.NewObjectListMust(ctx, []GCEDiskModel{
		{SizeGiB: types.Int64Value(100), Boot: types.BoolValue(true)},
		{SizeGiB: types.Int64Value(200), Category: types.StringValue("pd-ssd"), Boot: types.BoolValue(false)},
	})}
	out, err := model.ToGCENodeClass(ctx, current)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Disks []map[string]json.RawMessage `json:"disks"`
	}
	if err := json.Unmarshal(payload["nodeClassSpec"], &spec); err != nil {
		t.Fatal(err)
	}
	if string(spec.Disks[0]["futureDisk"]) != "42" {
		t.Fatalf("existing positional disk lost unknown data: %s", encoded)
	}
	if _, ok := spec.Disks[1]["futureDisk"]; ok {
		t.Fatalf("new disk inherited another disk's unknown data: %s", encoded)
	}
}
