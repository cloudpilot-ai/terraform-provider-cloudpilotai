package helper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestApplyPoliciesPreservesNewServerFieldsThroughHTTP(t *testing.T) {
	ctx := context.Background()
	inputs := map[string]string{
		"/api/v1/workloadautoscaler/clusters/test/recommendationpolicies/java": `{"name":"java","spec":{"strategyType":"percentile","historyWindowDuration":{"cpu":"24h","memory":"24h"},"evaluationPeriod":"1m","jvm":{"maxHeapXmx":"4Gi","futureProtection":false}}}`,
		"/api/v1/workloadautoscaler/clusters/test/autoscalingpolicies/app":     `{"name":"app","enable":true,"spec":{"recommendationPolicyName":"steady","resourceStartupBoost":{"enabled":true,"recommendationPolicyName":"startup","updateResources":["memory"],"futureProtection":0}}}`,
	}
	posted := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{"data": json.RawMessage(inputs[r.URL.Path])})
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		posted[r.URL.Path] = payload
		w.Write([]byte(`{"data":null}`))
	}))
	defer server.Close()
	client := cloudpilotaiclient.NewCloudPilotClient(server.URL, "example-key")
	rp := customfield.NewObjectListMust(ctx, []api.RecommendationPolicyModel{{Name: types.StringValue("java"), HistoryWindowCPU: types.StringValue("24h"), HistoryWindowMemory: types.StringValue("24h"), EvaluationPeriod: types.StringValue("1m"), BufferCPU: types.StringValue("20%")}})
	if _, err := ApplyRecommendationPolicies(ctx, client, "test", rp); err != nil {
		t.Fatal(err)
	}
	ap := customfield.NewObjectListMust(ctx, []api.AutoscalingPolicyModel{{Name: types.StringValue("app"), RecommendationPolicyName: types.StringValue("steady"), StartupBoostEnabled: types.BoolValue(true)}})
	if _, err := ApplyAutoscalingPolicies(ctx, client, "test", ap); err != nil {
		t.Fatal(err)
	}
	for path, payload := range posted {
		spec := payload["spec"].(map[string]any)
		if path[len(path)-4:] == "java" {
			jvm := spec["jvm"].(map[string]any)
			if jvm["maxHeapXmx"] != "4Gi" || jvm["futureProtection"] != false {
				t.Fatalf("RP protection lost: %#v", payload)
			}
		} else {
			boost := spec["resourceStartupBoost"].(map[string]any)
			if boost["recommendationPolicyName"] != "startup" || boost["futureProtection"] != float64(0) {
				t.Fatalf("startup protection lost: %#v", payload)
			}
		}
	}
	if len(posted) != 2 {
		t.Fatalf("posts=%d,want 2", len(posted))
	}
}
