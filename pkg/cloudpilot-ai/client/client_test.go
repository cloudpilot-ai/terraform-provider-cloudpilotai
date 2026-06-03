package client_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
)

func TestClientGetClusterSetting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/clusters/cluster-1/setting" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(api.ResponseBody{
			Data: map[string]any{
				"enableNodeRepair":   true,
				"enableDiskMonitor":  false,
				"maintenanceEnabled": true,
				"discount":           0.12,
				"preRunCommand":      "echo pre",
				"postRunCommand":     "echo post",
			},
		})
	}))
	defer server.Close()

	c := client.NewCloudPilotClient(server.URL, "test-key")
	got, err := c.GetClusterSetting("cluster-1")
	if err != nil {
		t.Fatalf("GetClusterSetting() error = %v", err)
	}
	if got == nil || got.EnableNodeRepair == nil || !*got.EnableNodeRepair {
		t.Fatalf("EnableNodeRepair = %#v", got)
	}
	if got.PreRunCommand == nil || *got.PreRunCommand != "echo pre" {
		t.Fatalf("PreRunCommand = %#v", got.PreRunCommand)
	}
}

func TestClientUpdateClusterSetting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/clusters/cluster-1/setting" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var payload api.ClusterSetting
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Discount == nil || *payload.Discount != 0.15 {
			t.Fatalf("Discount = %#v", payload.Discount)
		}
		_ = json.NewEncoder(w).Encode(api.ResponseBody{Data: nil})
	}))
	defer server.Close()

	c := client.NewCloudPilotClient(server.URL, "test-key")
	discount := 0.15
	if err := c.UpdateClusterSetting("cluster-1", &api.ClusterSetting{Discount: &discount}); err != nil {
		t.Fatalf("UpdateClusterSetting() error = %v", err)
	}
}

func TestClientUpdateClusterMaintenanceStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/clusters/cluster-1/maintenance/status" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var payload api.ClusterMaintenanceStatus
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.MaintenanceModeEnabled == nil || !*payload.MaintenanceModeEnabled {
			t.Fatalf("MaintenanceModeEnabled = %#v", payload.MaintenanceModeEnabled)
		}
		_ = json.NewEncoder(w).Encode(api.ResponseBody{Data: nil})
	}))
	defer server.Close()

	c := client.NewCloudPilotClient(server.URL, "test-key")
	maintenanceEnabled := true
	if err := c.UpdateClusterMaintenanceStatus("cluster-1", &api.ClusterMaintenanceStatus{MaintenanceModeEnabled: &maintenanceEnabled}); err != nil {
		t.Fatalf("UpdateClusterMaintenanceStatus() error = %v", err)
	}
}
