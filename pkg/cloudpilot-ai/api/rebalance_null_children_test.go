package api

import (
	"context"
	"testing"

	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	gcpproviderv1alpha1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter-provider-gcp/apis/v1alpha1"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUnmanagedVolumePolicyChildrenPreservePolicy(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		model VolumeSizePolicyModel
	}{
		{name: "unknown maximum", model: VolumeSizePolicyModel{MaxSizeGiB: types.Int64Unknown()}},
		{name: "unknown", model: VolumeSizePolicyModel{PerVCPUGiB: types.Int64Unknown(), MaxSizeGiB: types.Int64Unknown()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := customfield.NewObjectMust(ctx, &tc.model)
			awsPolicy, err := awsVolumePolicyFromModel(ctx, model, nil)
			if err != nil || awsPolicy != nil {
				t.Fatalf("unmanaged AWS policy = %#v, error = %v", awsPolicy, err)
			}
			gcePolicy, err := gceVolumePolicyFromModel(ctx, model, nil)
			if err != nil || gcePolicy != nil {
				t.Fatalf("unmanaged GCE policy = %#v, error = %v", gcePolicy, err)
			}
			awsPolicy, err = awsVolumePolicyFromModel(ctx, model, &awsproviderv1.VolumeSizePolicy{PerVCPUGiB: 4, MaxSizeGiB: 512})
			if err != nil || awsPolicy == nil || awsPolicy.PerVCPUGiB != 4 || awsPolicy.MaxSizeGiB != 512 {
				t.Fatalf("existing AWS policy lost: %#v, error = %v", awsPolicy, err)
			}
			gcePolicy, err = gceVolumePolicyFromModel(ctx, model, &gcpproviderv1alpha1.VolumeSizePolicy{PerVCPUGiB: 4, MaxSizeGiB: 512})
			if err != nil || gcePolicy == nil || gcePolicy.PerVCPUGiB != 4 || gcePolicy.MaxSizeGiB != 512 {
				t.Fatalf("existing GCE policy lost: %#v, error = %v", gcePolicy, err)
			}
		})
	}
	model := customfield.NewObjectMust(ctx, &VolumeSizePolicyModel{PerVCPUGiB: types.Int64Value(4)})
	if _, err := awsVolumePolicyFromModel(ctx, model, nil); err == nil {
		t.Fatal("new AWS policy without max_size_gib accepted")
	}
	if _, err := gceVolumePolicyFromModel(ctx, model, nil); err == nil {
		t.Fatal("new GCE policy without max_size_gib accepted")
	}
}

func TestUnmanagedNRISwapChildrenPreservePolicy(t *testing.T) {
	ctx := context.Background()
	for _, value := range []NRISwapPolicyModel{{Enabled: types.BoolUnknown(), MemoryLimitMultiplier: types.Float64Unknown()}, {Enabled: types.BoolUnknown()}} {
		model := customfield.NewObjectMust(ctx, &value)
		got, err := nriSwapFromModel(ctx, model, nil)
		if err != nil || got != nil {
			t.Fatalf("unmanaged NRI swap policy = %#v, error = %v", got, err)
		}
		current := &NRISwapPolicy{Enabled: true, MemoryLimitMultiplier: 2}
		got, err = nriSwapFromModel(ctx, model, current)
		if err != nil || got == nil || !got.Enabled || got.MemoryLimitMultiplier != 2 {
			t.Fatalf("existing NRI swap policy lost: %#v, error = %v", got, err)
		}
	}
	model := customfield.NewObjectMust(ctx, &NRISwapPolicyModel{Enabled: types.BoolValue(true)})
	if _, err := nriSwapFromModel(ctx, model, nil); err == nil {
		t.Fatal("new enabled NRI swap policy without multiplier accepted")
	}
}

func TestUnmanagedMemoryExtensionChildrenPreservePolicy(t *testing.T) {
	ctx := context.Background()
	for _, model := range []MemoryExtensionModel{
		{Enabled: types.BoolUnknown(), ReclaimPolicy: types.StringUnknown(), ColdMemory: customfield.UnknownObject[ColdMemoryModel](ctx), CachedMemory: customfield.UnknownObject[CachedMemoryModel](ctx)},
		{ColdMemory: customfield.NewObjectMust(ctx, &ColdMemoryModel{SizingMode: types.StringUnknown(), MemoryRatioPercent: types.Int32Unknown(), MinGiB: types.Int32Unknown(), MaxGiB: types.Int32Unknown(), MaxDiskPercent: types.Int32Unknown()}), CachedMemory: customfield.NewObjectMust(ctx, &CachedMemoryModel{MaxMemoryPercent: types.Int32Unknown()})},
	} {
		got, err := model.ToAPI(ctx, nil)
		if err != nil || got != nil {
			t.Fatalf("unmanaged Memory Extension policy = %#v, error = %v", got, err)
		}
		current := &MemoryExtensionPolicy{Enabled: true, ColdMemory: ColdMemoryPolicy{MaxGiB: ptrInt32(32)}}
		got, err = model.ToAPI(ctx, current)
		if err != nil || got == nil || !got.Enabled || got.ColdMemory.MaxGiB == nil || *got.ColdMemory.MaxGiB != 32 {
			t.Fatalf("existing Memory Extension policy lost: %#v, error = %v", got, err)
		}
	}
}
