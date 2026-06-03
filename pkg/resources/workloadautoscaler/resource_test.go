package workloadautoscaler

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestWorkloadAutoscalerModelToWAConfigurationIncludesAdvancedFields(t *testing.T) {
	model := WorkloadAutoscalerModel{
		EnableNewWorkloadsProactiveUpdate:        types.BoolValue(true),
		LimiterQuotaPerWindow:                    types.Int64Value(7),
		LimiterBurst:                             types.Int64Value(11),
		LimiterWindowSeconds:                     types.Int64Value(31),
		EnablePreemptedPodGC:                     types.BoolValue(false),
		PreemptedPodGCTTL:                        types.StringValue("45m"),
		EnableInitialOptimizationDataWindowCheck: types.BoolValue(false),
	}

	got := model.ToWAConfiguration()
	if got.EnableNewWorkloadsProactiveUpdate == nil || !*got.EnableNewWorkloadsProactiveUpdate {
		t.Fatalf("EnableNewWorkloadsProactiveUpdate = %#v", got.EnableNewWorkloadsProactiveUpdate)
	}
	if got.LimiterQuotaPerWindow == nil || *got.LimiterQuotaPerWindow != 7 {
		t.Fatalf("LimiterQuotaPerWindow = %#v", got.LimiterQuotaPerWindow)
	}
	if got.EnablePreemptedPodGC == nil || *got.EnablePreemptedPodGC {
		t.Fatalf("EnablePreemptedPodGC = %#v", got.EnablePreemptedPodGC)
	}
}
