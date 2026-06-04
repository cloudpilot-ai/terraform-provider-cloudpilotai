package workloadautoscaler

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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

func TestSchemaRemovesEnableUpgrade(t *testing.T) {
	s := Schema(context.Background())

	if _, ok := s.Attributes["enable_upgrade"]; ok {
		t.Fatalf("workload autoscaler schema should not expose enable_upgrade")
	}
	if _, ok := s.Attributes["enable_node_agent"]; !ok {
		t.Fatalf("workload autoscaler schema missing enable_node_agent")
	}

	if _, ok := s.Attributes["cluster_id"].(schema.StringAttribute); !ok {
		t.Fatalf("cluster_id attribute has unexpected type %T", s.Attributes["cluster_id"])
	}
}
