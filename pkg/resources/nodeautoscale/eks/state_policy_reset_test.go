package eks

import (
	"context"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestEC2PolicyResetStatePreservesSuccessAndExposesDrift(t *testing.T) {
	ctx := context.Background()
	job := customfield.NewObjectMust(ctx, &api.JobNodePoolOptimizationModel{})
	if got := preserveJobOptimization(ctx, customfield.NullObject[api.JobNodePoolOptimizationModel](ctx), job); !got.Equal(job) {
		t.Fatal("successful Job reset lost empty state")
	}
	jobDrift := customfield.NewObjectMust(ctx, &api.JobNodePoolOptimizationModel{MinActiveNodes: types.Int32Value(3)})
	if got := preserveJobOptimization(ctx, jobDrift, job); !got.Equal(jobDrift) {
		t.Fatal("Job reset masked remote drift")
	}
	disk := customfield.NewObjectMust(ctx, &api.VolumeSizePolicyModel{})
	if got := preserveVolumeSizePolicy(ctx, customfield.NullObject[api.VolumeSizePolicyModel](ctx), disk); !got.Equal(disk) {
		t.Fatal("successful disk reset lost empty state")
	}
	diskDrift := customfield.NewObjectMust(ctx, &api.VolumeSizePolicyModel{MaxSizeGiB: types.Int64Value(512)})
	if got := preserveVolumeSizePolicy(ctx, diskDrift, disk); !got.Equal(diskDrift) {
		t.Fatal("disk reset masked remote drift")
	}
	ebs := customfield.NewObjectMust(ctx, &api.BlockDeviceModel{})
	if got := preserveEBS(ctx, customfield.NullObject[api.BlockDeviceModel](ctx), ebs); !got.Equal(ebs) {
		t.Fatal("successful EBS reset lost empty state")
	}
	ebsDrift := customfield.NewObjectMust(ctx, &api.BlockDeviceModel{Encrypted: types.BoolValue(true)})
	if got := preserveEBS(ctx, ebsDrift, ebs); !got.Equal(ebsDrift) {
		t.Fatal("EBS reset masked remote drift")
	}
}

func TestEC2EmptyDisableStateRequiresOptionalSettingsReset(t *testing.T) {
	ctx := context.Background()
	memory := customfield.NewObjectMust(ctx, &api.MemoryExtensionModel{})
	nri := customfield.NewObjectMust(ctx, &api.NRISwapPolicyModel{})
	for _, tc := range []struct {
		name    string
		enabled bool
		stale   bool
		success bool
	}{
		{name: "reset disabled", success: true},
		{name: "enabled drift", enabled: true},
		{name: "disabled stale settings", stale: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remoteMemory := api.MemoryExtensionModel{Enabled: types.BoolValue(tc.enabled)}
			remoteNRI := api.NRISwapPolicyModel{Enabled: types.BoolValue(tc.enabled), MemoryLimitMultiplier: types.Float64Value(0)}
			if tc.stale {
				remoteMemory.ColdMemory = customfield.NewObjectMust(ctx, &api.ColdMemoryModel{MaxGiB: types.Int32Value(32)})
				remoteNRI.MemoryLimitMultiplier = types.Float64Value(2)
			}
			memoryObject := customfield.NewObjectMust(ctx, &remoteMemory)
			nriObject := customfield.NewObjectMust(ctx, &remoteNRI)
			wantMemory, wantNRI := memoryObject, nriObject
			if tc.success {
				wantMemory, wantNRI = memory, nri
			}
			if got := preserveMemoryExtension(ctx, memoryObject, memory); !got.Equal(wantMemory) {
				t.Fatalf("Memory reset state = %s, want %s", got.String(), wantMemory.String())
			}
			if got := preserveNRISwapPolicy(ctx, nriObject, nri); !got.Equal(wantNRI) {
				t.Fatalf("NRI reset state = %s, want %s", got.String(), wantNRI.String())
			}
		})
	}
}

func TestEC2NestedMemoryResetPreservesServerDefaultsAndExposesDrift(t *testing.T) {
	ctx := context.Background()
	cold := customfield.NewObjectMust(ctx, &api.ColdMemoryModel{})
	cached := customfield.NewObjectMust(ctx, &api.CachedMemoryModel{})
	defaults := customfield.NewObjectMust(ctx, &api.ColdMemoryModel{SizingMode: types.StringValue("Auto"), MemoryRatioPercent: types.Int32Value(30), MinGiB: types.Int32Value(2), MaxGiB: types.Int32Value(16), MaxDiskPercent: types.Int32Value(20)})
	if got := preserveColdMemory(ctx, defaults, cold); !got.Equal(cold) {
		t.Fatal("cold memory defaults did not retain reset state")
	}
	drift := customfield.NewObjectMust(ctx, &api.ColdMemoryModel{MaxGiB: types.Int32Value(32)})
	if got := preserveColdMemory(ctx, drift, cold); !got.Equal(drift) {
		t.Fatal("cold memory reset masked drift")
	}
	cacheDefault := customfield.NewObjectMust(ctx, &api.CachedMemoryModel{MaxMemoryPercent: types.Int32Value(20)})
	if got := preserveCachedMemory(ctx, cacheDefault, cached); !got.Equal(cached) {
		t.Fatal("cached memory defaults did not retain reset state")
	}
	cacheDrift := customfield.NewObjectMust(ctx, &api.CachedMemoryModel{MaxMemoryPercent: types.Int32Value(30)})
	if got := preserveCachedMemory(ctx, cacheDrift, cached); !got.Equal(cacheDrift) {
		t.Fatal("cached memory reset masked drift")
	}
}

func TestEC2ClearStringsStateRetainsSuccessfulReset(t *testing.T) {
	ctx := context.Background()
	clear := types.StringValue("")
	if got := preserveClearedDuration(clear, types.StringNull()); !got.Equal(clear) {
		t.Fatal("cleared cooldown lost configured string")
	}
	if got := preserveClearedDuration(clear, types.StringValue("5m")); got.ValueString() != "5m" {
		t.Fatal("cleared cooldown masked drift")
	}
	job := customfield.NewObjectMust(ctx, &api.JobNodePoolOptimizationModel{ProvisioningDelay: clear})
	if got := preserveJobOptimization(ctx, customfield.NullObject[api.JobNodePoolOptimizationModel](ctx), job); !got.Equal(job) {
		t.Fatal("cleared final Job field lost configured string")
	}
	memory := api.MemoryExtensionModel{Enabled: types.BoolValue(true), ReclaimPolicy: clear, ColdMemory: customfield.NewObjectMust(ctx, &api.ColdMemoryModel{SizingMode: clear})}
	remote := api.MemoryExtensionModel{Enabled: types.BoolValue(true), ReclaimPolicy: types.StringValue("Conservative"), ColdMemory: customfield.NewObjectMust(ctx, &api.ColdMemoryModel{SizingMode: types.StringValue("Auto")})}
	state, raw := customfield.NewObjectMust(ctx, &memory), customfield.NewObjectMust(ctx, &remote)
	if got := preserveMemoryExtension(ctx, raw, state); !got.Equal(state) {
		t.Fatalf("defaulted Memory clear changed state: %s", got.String())
	}
}

func TestEC2EBSStringClearStateRetainsNullReadbackAndExposesDrift(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name       string
		size       bool
		volumeType bool
	}{
		{name: "size", size: true},
		{name: "type", volumeType: true},
		{name: "both", size: true, volumeType: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configured := api.BlockDeviceModel{}
			remote := api.BlockDeviceModel{}
			if tc.size {
				configured.VolumeSize = types.StringValue("")
			}
			if tc.volumeType {
				configured.VolumeType = types.StringValue("")
			}
			state := customfield.NewObjectMust(ctx, &configured)
			if got := preserveEBS(ctx, customfield.NewObjectMust(ctx, &remote), state); !got.Equal(state) {
				t.Fatalf("successful string clear lost configuration: %s", got.String())
			}
			if tc.size {
				remote.VolumeSize = types.StringValue("80Gi")
			}
			if tc.volumeType {
				remote.VolumeType = types.StringValue("gp3")
			}
			drift := customfield.NewObjectMust(ctx, &remote)
			if got := preserveEBS(ctx, drift, state); !got.Equal(drift) {
				t.Fatalf("string clear masked drift: %s", got.String())
			}
		})
	}
}

func TestEC2EmptyBlockMappingsRetainResetStateAndExposeDrift(t *testing.T) {
	ctx := context.Background()
	state := customfield.NewObjectListMust(ctx, []api.BlockDeviceMappingModel{})
	for _, remote := range []customfield.NestedObjectList[api.BlockDeviceMappingModel]{
		customfield.NullObjectList[api.BlockDeviceMappingModel](ctx),
		customfield.NewObjectListMust(ctx, []api.BlockDeviceMappingModel{}),
	} {
		if got := preserveBlockDeviceMappings(ctx, remote, state); !got.Equal(state) {
			t.Fatal("successful empty mapping reset lost its list representation")
		}
	}
	drift := customfield.NewObjectListMust(ctx, []api.BlockDeviceMappingModel{{DeviceName: types.StringValue("/dev/xvda")}})
	if got := preserveBlockDeviceMappings(ctx, drift, state); !got.Equal(drift) {
		t.Fatal("empty mapping reset masked remote drift")
	}
	unknown := customfield.UnknownObjectList[api.BlockDeviceMappingModel](ctx)
	if got := preserveBlockDeviceMappings(ctx, unknown, state); !got.IsUnknown() {
		t.Fatal("unknown mapping readback must remain unknown")
	}
}
