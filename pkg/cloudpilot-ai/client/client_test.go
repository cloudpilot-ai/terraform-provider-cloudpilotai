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

func TestClientSendsTerraformClientHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-CloudPilot-Client"); got != "terraform-provider-cloudpilotai" {
			t.Fatalf("X-CloudPilot-Client = %q", got)
		}
		if got := r.Header.Get("X-API-KEY"); got != "test-key" {
			t.Fatalf("X-API-KEY = %q", got)
		}
		_ = json.NewEncoder(w).Encode(api.ResponseBody{
			Data: map[string]any{
				"enableNodeRepair": false,
			},
		})
	}))
	defer server.Close()

	c := client.NewCloudPilotClient(server.URL, "test-key")
	if _, err := c.GetClusterSetting("cluster-1"); err != nil {
		t.Fatalf("GetClusterSetting() error = %v", err)
	}
}

func TestClientGetRebalanceConfigurationSpoofsBrowserUserAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rebalance/clusters/cluster-1/configuration" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("User-Agent"); got == "" || got == "Go-http-client/1.1" {
			t.Fatalf("User-Agent = %q", got)
		}
		_ = json.NewEncoder(w).Encode(api.ResponseBody{
			Data: map[string]any{
				"enable":       true,
				"uploadConfig": true,
			},
		})
	}))
	defer server.Close()

	c := client.NewCloudPilotClient(server.URL, "test-key")
	if _, err := c.GetRebalanceConfiguration("cluster-1"); err != nil {
		t.Fatalf("GetRebalanceConfiguration() error = %v", err)
	}
}

func TestClientGetAgentSHPassesProviderAndClusterName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/sh" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("provider"); got != "gcp" {
			t.Fatalf("provider = %q", got)
		}
		if got := r.URL.Query().Get("cluster_name"); got != "test-gke" {
			t.Fatalf("cluster_name = %q", got)
		}
		if got := r.URL.Query().Get("disable_workload_uploading"); got != "true" {
			t.Fatalf("disable_workload_uploading = %q", got)
		}
		_ = json.NewEncoder(w).Encode(api.ResponseBody{Data: "echo ok"})
	}))
	defer server.Close()

	c := client.NewCloudPilotClient(server.URL, "test-key")
	if _, err := c.GetAgentSH("gcp", "test-gke", true); err != nil {
		t.Fatalf("GetAgentSH() error = %v", err)
	}
}

func TestClientGetRebalanceSHPassesProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rebalance/clusters/cluster-1/sh" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("provider"); got != "gcp" {
			t.Fatalf("provider = %q", got)
		}
		_ = json.NewEncoder(w).Encode(api.ResponseBody{Data: "echo rebalance"})
	}))
	defer server.Close()

	c := client.NewCloudPilotClient(server.URL, "test-key")
	if _, err := c.GetRebalanceSH("cluster-1", "gcp"); err != nil {
		t.Fatalf("GetRebalanceSH() error = %v", err)
	}
}

func TestClientGetClusterUpgradeSHPassesProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/clusters/cluster-1/upgrade/sh" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("provider"); got != "gcp" {
			t.Fatalf("provider = %q", got)
		}
		_ = json.NewEncoder(w).Encode(api.ResponseBody{Data: "echo upgrade"})
	}))
	defer server.Close()

	c := client.NewCloudPilotClient(server.URL, "test-key")
	if _, err := c.GetClusterUpgradeSH("cluster-1", "gcp"); err != nil {
		t.Fatalf("GetClusterUpgradeSH() error = %v", err)
	}
}

func TestClientGetAgentSHRequiresProviderAndClusterName(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		t.Fatalf("unexpected request: %s", r.URL.String())
	}))
	defer server.Close()

	c := client.NewCloudPilotClient(server.URL, "test-key")
	if _, err := c.GetAgentSH("", "test-gke", true); err == nil {
		t.Fatal("expected error for missing provider")
	}
	if _, err := c.GetAgentSH("gcp", "", true); err == nil {
		t.Fatal("expected error for missing cluster_name")
	}
	if serverCalled {
		t.Fatal("server should not be called when validation fails")
	}
}

func TestClientGetRebalanceSHRequiresProvider(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		t.Fatalf("unexpected request: %s", r.URL.String())
	}))
	defer server.Close()

	c := client.NewCloudPilotClient(server.URL, "test-key")
	if _, err := c.GetRebalanceSH("cluster-1", ""); err == nil {
		t.Fatal("expected error for missing provider")
	}
	if serverCalled {
		t.Fatal("server should not be called when validation fails")
	}
}

func TestClientGetClusterUpgradeSHRequiresProvider(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		t.Fatalf("unexpected request: %s", r.URL.String())
	}))
	defer server.Close()

	c := client.NewCloudPilotClient(server.URL, "test-key")
	if _, err := c.GetClusterUpgradeSH("cluster-1", ""); err == nil {
		t.Fatal("expected error for missing provider")
	}
	if serverCalled {
		t.Fatal("server should not be called when validation fails")
	}
}
