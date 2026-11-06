package helper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestApplyAutoscalingPoliciesPostsOnlySelectedLimitAction(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name      string
		policy    api.LimitPolicyModel
		wantError bool
		wantPosts int
	}{
		{
			name: "replace multiplier with remove limit",
			policy: api.LimitPolicyModel{
				Resource: types.StringValue("cpu"), RemoveLimit: types.BoolValue(true), KeepLimit: types.BoolValue(false),
			},
			wantPosts: 1,
		},
		{
			name: "reject competing selected actions",
			policy: api.LimitPolicyModel{
				Resource: types.StringValue("cpu"), RemoveLimit: types.BoolValue(true), Multiplier: types.StringValue("3"),
			},
			wantError: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			posts := 0
			var postedAction map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/workloadautoscaler/clusters/cluster/autoscalingpolicies/app" {
					t.Errorf("unexpected request path: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					w.Write([]byte(`{"data":{"name":"app","enable":true,"spec":{"recommendationPolicyName":"steady","limitPolicies":{"cpu":{"multiplier":"2"}}}}}`))
					return
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected request method: %s", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				var payload struct {
					Spec struct {
						LimitPolicies map[string]map[string]any `json:"limitPolicies"`
					} `json:"spec"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				mu.Lock()
				posts++
				postedAction = payload.Spec.LimitPolicies["cpu"]
				mu.Unlock()
				w.Write([]byte(`{"data":null}`))
			}))
			defer server.Close()

			client := cloudpilotaiclient.NewCloudPilotClient(server.URL, "example-key")
			models := customfield.NewObjectListMust(ctx, []api.AutoscalingPolicyModel{{
				Name: types.StringValue("app"), RecommendationPolicyName: types.StringValue("steady"),
				LimitPolicies: customfield.NewObjectListMust(ctx, []api.LimitPolicyModel{test.policy}),
			}})
			_, err := ApplyAutoscalingPolicies(ctx, client, "cluster", models)
			if (err != nil) != test.wantError {
				t.Fatalf("ApplyAutoscalingPolicies() error = %v, wantError = %v", err, test.wantError)
			}
			mu.Lock()
			defer mu.Unlock()
			if posts != test.wantPosts {
				t.Fatalf("POST calls = %d, want %d", posts, test.wantPosts)
			}
			if posts > 0 && (len(postedAction) != 1 || postedAction["removeLimit"] != true) {
				t.Fatalf("posted limit action = %#v, want only removeLimit=true", postedAction)
			}
		})
	}
}
