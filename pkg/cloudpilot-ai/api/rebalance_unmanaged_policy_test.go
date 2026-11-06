package api

import (
	"context"
	"testing"

	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestJobOptimizationNullChildrenPreserveRemoteValues(t *testing.T) {
	ctx := context.Background()
	minimum, delay := int32(3), "30s"
	current := &JobNodePoolOptimization{MinActiveNodes: &minimum, ProvisioningDelay: &delay}
	for _, tc := range []struct {
		name        string
		model       JobNodePoolOptimizationModel
		wantMinimum int32
		wantDelay   string
	}{
		{name: "unknown", model: JobNodePoolOptimizationModel{MinActiveNodes: types.Int32Unknown(), ProvisioningDelay: types.StringUnknown()}, wantMinimum: 3, wantDelay: "30s"},
		{name: "minimum only", model: JobNodePoolOptimizationModel{MinActiveNodes: types.Int32Value(5)}, wantMinimum: 5, wantDelay: "30s"},
		{name: "delay only", model: JobNodePoolOptimizationModel{ProvisioningDelay: types.StringValue("1m")}, wantMinimum: 3, wantDelay: "1m"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := jobOptimizationFromModel(ctx, customfield.NewObjectMust(ctx, &tc.model), current)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || got.MinActiveNodes == nil || *got.MinActiveNodes != tc.wantMinimum || got.ProvisioningDelay == nil || *got.ProvisioningDelay != tc.wantDelay {
				t.Fatalf("Job policy = %#v, want minimum %d and delay %q", got, tc.wantMinimum, tc.wantDelay)
			}
			if *current.MinActiveNodes != 3 || *current.ProvisioningDelay != "30s" {
				t.Fatal("updating Job policy mutated the remote input")
			}
		})
	}
}

func TestEmptyJobOptimizationCreatesResetRequest(t *testing.T) {
	ctx := context.Background()
	got, err := jobOptimizationFromModel(ctx, customfield.NewObjectMust(ctx, &JobNodePoolOptimizationModel{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.MinActiveNodes != nil || got.ProvisioningDelay != nil {
		t.Fatalf("empty Job policy = %#v, want an explicit reset object", got)
	}
}

func TestEmptyLocalSSDModelCreatesResetConfiguration(t *testing.T) {
	spec := &GCENodeClassSpec{}
	applyGCELocalSSDModel(spec, GCEEphemeralStorageLocalSSDModel{})
	if spec.EphemeralStorageLocalSSD == nil || spec.EphemeralStorageLocalSSD.Count != nil || spec.EphemeralStorageLocalSSD.AutoCount {
		t.Fatalf("empty Local SSD policy = %#v, want empty reset configuration", spec.EphemeralStorageLocalSSD)
	}
}

func TestUnmanagedLocalSSDBlockPreservesDisabledConfiguration(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		current GCENodeClass
		model   GCEEphemeralStorageLocalSSDModel
	}{
		{name: "unknown missing", model: GCEEphemeralStorageLocalSSDModel{Count: types.Int32Unknown(), AutoCount: types.BoolUnknown()}},
		{name: "unknown disabled", current: GCENodeClass{EnableLocalSSDEphemeralStorage: ptrBool(false), NodeClassSpec: &GCENodeClassSpec{}}, model: GCEEphemeralStorageLocalSSDModel{Count: types.Int32Unknown(), AutoCount: types.BoolUnknown()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := GCENodeClassModel{
				Name: types.StringValue("example"), EphemeralStorageLocalSSD: customfield.NewObjectMust(ctx, &tc.model),
				ImageSelectorTerms: customfield.NewObjectListMust(ctx, []GCEImageSelectorTermModel{{Family: types.StringValue("ContainerOptimizedOS"), Channel: types.StringValue("cluster")}}),
			}
			got, err := model.ToGCENodeClass(ctx, tc.current)
			if err != nil {
				t.Fatalf("unmanaged Local SSD block rejected: %v", err)
			}
			if got.NodeClassSpec.EphemeralStorageLocalSSD != nil || got.EnableLocalSSDEphemeralStorage != tc.current.EnableLocalSSDEphemeralStorage {
				t.Fatalf("unmanaged Local SSD block changed the configuration: %#v", got)
			}
		})
	}
}

func TestVolumePolicyUpdatePreservesUnmanagedEBSFields(t *testing.T) {
	ctx := context.Background()
	encrypted, volumeType := true, "gp3"
	size := resource.MustParse("100Gi")
	current := &awsproviderv1.BlockDevice{
		Encrypted: &encrypted, VolumeType: &volumeType, VolumeSize: &size,
		VolumeSizePolicy: &awsproviderv1.VolumeSizePolicy{PerVCPUGiB: 4, MaxSizeGiB: 256},
	}
	for _, tc := range []struct {
		name  string
		model BlockDeviceModel
	}{
		{name: "null children"},
		{name: "unknown children", model: BlockDeviceModel{Encrypted: types.BoolUnknown(), VolumeType: types.StringUnknown(), VolumeSize: types.StringUnknown()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.model.VolumeSizePolicy = customfield.NewObjectMust(ctx, &VolumeSizePolicyModel{MaxSizeGiB: types.Int64Value(512)})
			got, err := blockDeviceModelToAWS(ctx, current, tc.model)
			if err != nil {
				t.Fatal(err)
			}
			if got.Encrypted == nil || !*got.Encrypted || got.VolumeType == nil || *got.VolumeType != "gp3" || got.VolumeSize == nil || got.VolumeSize.Cmp(size) != 0 {
				t.Fatalf("unmanaged EBS fields lost: %#v", got)
			}
			if got.VolumeSizePolicy == nil || got.VolumeSizePolicy.PerVCPUGiB != 4 || got.VolumeSizePolicy.MaxSizeGiB != 512 {
				t.Fatalf("updated volume policy = %#v", got.VolumeSizePolicy)
			}
			if current.VolumeSizePolicy.MaxSizeGiB != 256 {
				t.Fatal("volume policy update mutated the remote input")
			}
			if err := validateEC2VolumeSizePolicies([]*awsproviderv1.BlockDeviceMapping{{EBS: got}}); err != nil {
				t.Fatalf("partial volume policy update rejected: %v", err)
			}
		})
	}
}
