package api

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	awscorev1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter/apis/v1"
	gcpproviderv1alpha1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter-provider-gcp/apis/v1alpha1"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
	corev1 "k8s.io/api/core/v1"
)

func TestKnownEmptyObjectDistinguishesNullAndUnknown(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		value customfield.NestedObject[JobNodePoolOptimizationModel]
		empty bool
	}{
		{name: "null", value: customfield.NullObject[JobNodePoolOptimizationModel](ctx)},
		{name: "unknown", value: customfield.UnknownObject[JobNodePoolOptimizationModel](ctx)},
		{name: "empty", value: customfield.NewObjectMust(ctx, &JobNodePoolOptimizationModel{}), empty: true},
		{name: "explicit null", value: customfield.NewObjectMust(ctx, &JobNodePoolOptimizationModel{MinActiveNodes: types.Int32Null(), ProvisioningDelay: types.StringNull()}), empty: true},
		{name: "one unknown", value: customfield.NewObjectMust(ctx, &JobNodePoolOptimizationModel{MinActiveNodes: types.Int32Unknown()})},
		{name: "partial", value: customfield.NewObjectMust(ctx, &JobNodePoolOptimizationModel{MinActiveNodes: types.Int32Value(2)})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsKnownEmptyObject(tc.value); got != tc.empty {
				t.Fatalf("empty = %v, want %v", got, tc.empty)
			}
		})
	}
}

func TestEmptyNodePoliciesResetExistingValues(t *testing.T) {
	ctx := context.Background()
	minimum, delay := int32(3), "30s"
	job, err := jobOptimizationFromModel(ctx, customfield.NewObjectMust(ctx, &JobNodePoolOptimizationModel{}), &JobNodePoolOptimization{MinActiveNodes: &minimum, ProvisioningDelay: &delay})
	if err != nil || job == nil || job.MinActiveNodes != nil || job.ProvisioningDelay != nil {
		t.Fatalf("Job reset = %#v, error = %v", job, err)
	}
	awsPolicy, err := awsVolumePolicyFromModel(ctx, customfield.NewObjectMust(ctx, &VolumeSizePolicyModel{}), &awsproviderv1.VolumeSizePolicy{PerVCPUGiB: 4, MaxSizeGiB: 512})
	if err != nil || awsPolicy != nil {
		t.Fatalf("AWS disk reset = %#v, error = %v", awsPolicy, err)
	}
	gcePolicy, err := gceVolumePolicyFromModel(ctx, customfield.NewObjectMust(ctx, &VolumeSizePolicyModel{}), &gcpproviderv1alpha1.VolumeSizePolicy{PerVCPUGiB: 4, MaxSizeGiB: 512})
	if err != nil || gcePolicy != nil {
		t.Fatalf("GCE disk reset = %#v, error = %v", gcePolicy, err)
	}
	nri, err := nriSwapFromModel(ctx, customfield.NewObjectMust(ctx, &NRISwapPolicyModel{}), &NRISwapPolicy{Enabled: true, MemoryLimitMultiplier: 2})
	if err != nil || nri == nil || nri.Enabled || nri.MemoryLimitMultiplier != 0 {
		t.Fatalf("NRI reset = %#v, error = %v", nri, err)
	}
	memory, err := (MemoryExtensionModel{}).ToAPI(ctx, &MemoryExtensionPolicy{Enabled: true, ColdMemory: ColdMemoryPolicy{MaxGiB: ptrInt32(32)}})
	if err != nil || memory == nil || !reflect.DeepEqual(*memory, MemoryExtensionPolicy{}) {
		t.Fatalf("Memory reset = %#v, error = %v", memory, err)
	}
}

func TestEC2EmptyEBSResetPayloadPreservesHiddenFields(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		ebs   customfield.NestedObject[BlockDeviceModel]
		reset bool
	}{
		{name: "null", ebs: customfield.NullObject[BlockDeviceModel](ctx)},
		{name: "unknown", ebs: customfield.UnknownObject[BlockDeviceModel](ctx)},
		{name: "unknown child", ebs: customfield.NewObjectMust(ctx, &BlockDeviceModel{Encrypted: types.BoolUnknown()})},
		{name: "empty", ebs: customfield.NewObjectMust(ctx, &BlockDeviceModel{}), reset: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var current EC2NodeClass
			if err := json.Unmarshal([]byte(`{"name":"example","nodeClassSpec":{"blockDeviceMappings":[{"deviceName":"/dev/xvda","ebs":{"encrypted":true,"volumeType":"gp3","volumeSize":"80Gi","volumeSizePolicy":{"maxSizeGiB":512},"iops":3000,"snapshotID":"snap-existing","future":"keep"}}]}}`), &current); err != nil {
				t.Fatal(err)
			}
			model := EC2NodeClassModel{Name: types.StringValue("example"), BlockDeviceMappings: customfield.NewObjectListMust(ctx, []BlockDeviceMappingModel{{DeviceName: types.StringValue("/dev/xvda"), EBS: tc.ebs}})}
			got, err := model.ToEC2NodeClass(ctx, "example", current, nil)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				Spec struct {
					Mappings []struct {
						EBS map[string]any `json:"ebs"`
					} `json:"blockDeviceMappings"`
				} `json:"nodeClassSpec"`
			}
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatal(err)
			}
			ebs := payload.Spec.Mappings[0].EBS
			for _, field := range []string{"encrypted", "volumeType", "volumeSize", "volumeSizePolicy"} {
				if _, exists := ebs[field]; exists == tc.reset {
					t.Fatalf("reset=%v, field %s, payload=%s", tc.reset, field, encoded)
				}
			}
			if ebs["iops"] != float64(3000) || ebs["future"] != "keep" || ebs["snapshotID"] != "snap-existing" {
				t.Fatalf("hidden EBS fields lost: %s", encoded)
			}
			if current.NodeClassSpec.BlockDeviceMappings[0].EBS.VolumeSizePolicy == nil {
				t.Fatal("reset mutated the remote input")
			}
		})
	}
}

func TestEC2EmptyRequirementsRemoveOnlyMatchingSelector(t *testing.T) {
	for _, key := range []string{"family", "arch", "zone", "missing"} {
		t.Run(key, func(t *testing.T) {
			requirements := []awscorev1.NodeSelectorRequirementWithMinValues{}
			want := []awscorev1.NodeSelectorRequirementWithMinValues{}
			for _, existing := range []string{"family", "arch", "zone"} {
				item := awscorev1.NodeSelectorRequirementWithMinValues{NodeSelectorRequirement: corev1.NodeSelectorRequirement{Key: existing, Operator: corev1.NodeSelectorOpIn, Values: []string{existing + "-value"}}}
				requirements = append(requirements, item)
				if existing != key {
					want = append(want, item)
				}
			}
			got := updateRequirements(key, corev1.NodeSelectorOpIn, []string{}, requirements)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("requirements = %#v, want %#v", got, want)
			}
		})
	}
}

func TestNodePolicyEmptyStringsClearOnlyManagedFields(t *testing.T) {
	ctx := context.Background()
	minimum, delay, mode, reclaim := int32(3), "30s", "Auto", "Conservative"
	job, err := jobOptimizationFromModel(ctx, customfield.NewObjectMust(ctx, &JobNodePoolOptimizationModel{ProvisioningDelay: types.StringValue("")}), &JobNodePoolOptimization{MinActiveNodes: &minimum, ProvisioningDelay: &delay})
	if err != nil || job == nil || job.MinActiveNodes == nil || *job.MinActiveNodes != 3 || job.ProvisioningDelay != nil {
		t.Fatalf("Job delay clear = %#v, error = %v", job, err)
	}
	model := MemoryExtensionModel{ReclaimPolicy: types.StringValue(""), ColdMemory: customfield.NewObjectMust(ctx, &ColdMemoryModel{SizingMode: types.StringValue("")})}
	memory, err := model.ToAPI(ctx, &MemoryExtensionPolicy{Enabled: true, ReclaimPolicy: &reclaim, ColdMemory: ColdMemoryPolicy{SizingMode: &mode, MaxGiB: ptrInt32(32)}})
	if err != nil || memory == nil || !memory.Enabled || memory.ReclaimPolicy != nil || memory.ColdMemory.SizingMode != nil || memory.ColdMemory.MaxGiB == nil || *memory.ColdMemory.MaxGiB != 32 {
		t.Fatalf("Memory string clear = %#v, error = %v", memory, err)
	}
	model.ColdMemory = customfield.NewObjectMust(ctx, &ColdMemoryModel{})
	model.CachedMemory = customfield.NewObjectMust(ctx, &CachedMemoryModel{})
	memory, err = model.ToAPI(ctx, &MemoryExtensionPolicy{Enabled: true, ColdMemory: ColdMemoryPolicy{MaxGiB: ptrInt32(32)}, CachedMemory: CachedMemoryPolicy{MaxMemoryPercent: ptrInt32(30)}})
	if err != nil || memory == nil || !memory.Enabled || memory.ColdMemory != (ColdMemoryPolicy{}) || memory.CachedMemory != (CachedMemoryPolicy{}) {
		t.Fatalf("Memory nested reset = %#v, error = %v", memory, err)
	}
}

func TestEC2EBSStringClearPayloadPreservesNullAndUnknownSiblings(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		model     BlockDeviceModel
		clearSize bool
		clearType bool
	}{
		{name: "size clear null siblings", model: BlockDeviceModel{VolumeSize: types.StringValue("")}, clearSize: true},
		{name: "type clear null siblings", model: BlockDeviceModel{VolumeType: types.StringValue("")}, clearType: true},
		{name: "size clear unknown siblings", model: BlockDeviceModel{VolumeSize: types.StringValue(""), VolumeType: types.StringUnknown(), Encrypted: types.BoolUnknown()}, clearSize: true},
		{name: "type clear unknown siblings", model: BlockDeviceModel{VolumeType: types.StringValue(""), VolumeSize: types.StringUnknown(), Encrypted: types.BoolUnknown()}, clearType: true},
		{name: "both clear", model: BlockDeviceModel{VolumeSize: types.StringValue(""), VolumeType: types.StringValue("")}, clearSize: true, clearType: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var current EC2NodeClass
			if err := json.Unmarshal([]byte(`{"name":"example","nodeClassSpec":{"blockDeviceMappings":[{"deviceName":"/dev/xvda","ebs":{"encrypted":true,"volumeType":"gp3","volumeSize":"80Gi","iops":3000,"snapshotID":"snap-existing","future":"keep"}}]}}`), &current); err != nil {
				t.Fatal(err)
			}
			model := EC2NodeClassModel{Name: types.StringValue("example"), BlockDeviceMappings: customfield.NewObjectListMust(ctx, []BlockDeviceMappingModel{{DeviceName: types.StringValue("/dev/xvda"), EBS: customfield.NewObjectMust(ctx, &tc.model)}})}
			got, err := model.ToEC2NodeClass(ctx, "example", current, nil)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				Spec struct {
					Mappings []struct {
						EBS map[string]any `json:"ebs"`
					} `json:"blockDeviceMappings"`
				} `json:"nodeClassSpec"`
			}
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatal(err)
			}
			ebs := payload.Spec.Mappings[0].EBS
			_, hasSize := ebs["volumeSize"]
			_, hasType := ebs["volumeType"]
			if hasSize == tc.clearSize || hasType == tc.clearType || ebs["encrypted"] != true || ebs["iops"] != float64(3000) || ebs["future"] != "keep" || ebs["snapshotID"] != "snap-existing" {
				t.Fatalf("string clear payload = %s", encoded)
			}
			if current.NodeClassSpec.BlockDeviceMappings[0].EBS.VolumeSize == nil || current.NodeClassSpec.BlockDeviceMappings[0].EBS.VolumeType == nil {
				t.Fatal("string clear mutated the remote input")
			}
		})
	}
}

func TestEC2EBSSizeClearRetainsDynamicPolicyValidation(t *testing.T) {
	ctx := context.Background()
	var current EC2NodeClass
	if err := json.Unmarshal([]byte(`{"name":"example","nodeClassSpec":{"blockDeviceMappings":[{"deviceName":"/dev/xvda","ebs":{"volumeType":"gp3","volumeSize":"80Gi","snapshotID":"snap-existing","volumeSizePolicy":{"maxSizeGiB":512}}}]}}`), &current); err != nil {
		t.Fatal(err)
	}
	model := EC2NodeClassModel{Name: types.StringValue("example"), BlockDeviceMappings: customfield.NewObjectListMust(ctx, []BlockDeviceMappingModel{{EBS: customfield.NewObjectMust(ctx, &BlockDeviceModel{VolumeSize: types.StringValue("")})}})}
	if _, err := model.ToEC2NodeClass(ctx, "example", current, nil); err == nil || !strings.Contains(err.Error(), "volume_size is required") {
		t.Fatalf("cleared base size with a retained dynamic policy error = %v", err)
	}
}

func TestEC2EBSClearWithoutSnapshotIsRejected(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		model BlockDeviceModel
	}{
		{name: "empty EBS"},
		{name: "empty size", model: BlockDeviceModel{VolumeSize: types.StringValue("")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var current EC2NodeClass
			if err := json.Unmarshal([]byte(`{"name":"example","nodeClassSpec":{"blockDeviceMappings":[{"deviceName":"/dev/xvda","ebs":{"volumeType":"gp3","volumeSize":"80Gi"}}]}}`), &current); err != nil {
				t.Fatal(err)
			}
			model := EC2NodeClassModel{Name: types.StringValue("example"), BlockDeviceMappings: customfield.NewObjectListMust(ctx, []BlockDeviceMappingModel{{EBS: customfield.NewObjectMust(ctx, &tc.model)}})}
			if _, err := model.ToEC2NodeClass(ctx, "example", current, nil); err == nil {
				t.Fatal("EBS reset without snapshot or size must be rejected before the API call")
			}
		})
	}
}
