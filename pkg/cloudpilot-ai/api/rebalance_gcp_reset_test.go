package api

import (
	"context"
	"encoding/json"
	"testing"

	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestGCELocalSSDResetPayloadAndUnknownPreservation(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		model customfield.NestedObject[GCEEphemeralStorageLocalSSDModel]
		reset bool
	}{
		{name: "null", model: customfield.NullObject[GCEEphemeralStorageLocalSSDModel](ctx)},
		{name: "unknown object", model: customfield.UnknownObject[GCEEphemeralStorageLocalSSDModel](ctx)},
		{name: "unknown children", model: customfield.NewObjectMust(ctx, &GCEEphemeralStorageLocalSSDModel{Count: types.Int32Unknown(), AutoCount: types.BoolUnknown()})},
		{name: "empty", model: customfield.NewObjectMust(ctx, &GCEEphemeralStorageLocalSSDModel{}), reset: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var current GCENodeClass
			if err := json.Unmarshal([]byte(`{"name":"example","enableLocalSSDEphemeralStorage":true,"nodeClassSpec":{"imageSelectorTerms":[{"family":"ContainerOptimizedOS","channel":"cluster"}],"ephemeralStorageLocalSSD":{"count":4,"future":"keep"}}}`), &current); err != nil {
				t.Fatal(err)
			}
			model := GCENodeClassModel{Name: types.StringValue("example"), EphemeralStorageLocalSSD: tc.model}
			got, err := model.ToGCENodeClass(ctx, current)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				Spec struct {
					LocalSSD map[string]any `json:"ephemeralStorageLocalSSD"`
				} `json:"nodeClassSpec"`
			}
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatal(err)
			}
			_, hasCount := payload.Spec.LocalSSD["count"]
			if hasCount == tc.reset || payload.Spec.LocalSSD["future"] != "keep" {
				t.Fatalf("reset=%v, payload=%s", tc.reset, encoded)
			}
			if current.NodeClassSpec.EphemeralStorageLocalSSD.Count == nil || *current.NodeClassSpec.EphemeralStorageLocalSSD.Count != 4 {
				t.Fatal("reset mutated the remote base")
			}
		})
	}
}

func TestGCEDiskPolicyEmptyResetSurvivesSerialization(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		policy customfield.NestedObject[VolumeSizePolicyModel]
		reset  bool
	}{
		{name: "null", policy: customfield.NullObject[VolumeSizePolicyModel](ctx)},
		{name: "unknown", policy: customfield.UnknownObject[VolumeSizePolicyModel](ctx)},
		{name: "unknown child", policy: customfield.NewObjectMust(ctx, &VolumeSizePolicyModel{MaxSizeGiB: types.Int64Unknown()})},
		{name: "empty", policy: customfield.NewObjectMust(ctx, &VolumeSizePolicyModel{}), reset: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var current GCENodeClass
			if err := json.Unmarshal([]byte(`{"name":"example","nodeClassSpec":{"imageSelectorTerms":[{"family":"ContainerOptimizedOS","channel":"cluster"}],"disks":[{"boot":true,"sizeGiB":80,"category":"pd-ssd","sizePolicy":{"perVCPUGiB":2,"maxSizeGiB":512},"future":"keep"}]}}`), &current); err != nil {
				t.Fatal(err)
			}
			model := GCENodeClassModel{Name: types.StringValue("example"), Disks: customfield.NewObjectListMust(ctx, []GCEDiskModel{{SizePolicy: tc.policy}})}
			got, err := model.ToGCENodeClass(ctx, current)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				Spec struct {
					Disks []map[string]any `json:"disks"`
				} `json:"nodeClassSpec"`
			}
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatal(err)
			}
			_, hasPolicy := payload.Spec.Disks[0]["sizePolicy"]
			if hasPolicy == tc.reset || payload.Spec.Disks[0]["future"] != "keep" || payload.Spec.Disks[0]["sizeGiB"] != float64(80) {
				t.Fatalf("reset=%v, payload=%s", tc.reset, encoded)
			}
		})
	}
}

func TestGCEJobAndCooldownResetPayload(t *testing.T) {
	ctx := context.Background()
	var current GCENodePool
	if err := json.Unmarshal([]byte(`{"name":"example","jobNodePoolOptimization":{"minActiveNodes":2,"provisioningDelay":"30s"},"nodePoolSpec":{"template":{"spec":{"nodeClassRef":{"name":"example"}}},"disruption":{"disruptionCooldown":"5m"}}}`), &current); err != nil {
		t.Fatal(err)
	}
	model := GCENodePoolModel{Name: types.StringValue("example"), DisruptionCooldown: types.StringValue(""), JobNodePoolOptimization: customfield.NewObjectMust(ctx, &JobNodePoolOptimizationModel{})}
	got, err := model.ToGCENodePool(ctx, current)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Job  map[string]any `json:"jobNodePoolOptimization"`
		Spec struct {
			Disruption map[string]any `json:"disruption"`
		} `json:"nodePoolSpec"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Job == nil || len(payload.Job) != 0 {
		t.Fatalf("Job reset must be sent as an explicit empty object: %s", encoded)
	}
	if _, exists := payload.Spec.Disruption["disruptionCooldown"]; exists {
		t.Fatalf("cooldown reset resurrected its previous value: %s", encoded)
	}
	if current.JobNodePoolOptimization.MinActiveNodes == nil || *current.JobNodePoolOptimization.MinActiveNodes != 2 || current.NodePoolSpec.Disruption.DisruptionCooldown == nil {
		t.Fatal("reset mutated the remote base")
	}
}
