package clustersetting

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
)

func TestClusterSettingModelToAPIOnlySendsConfiguredValues(t *testing.T) {
	model := ClusterSettingModel{
		ClusterID:        types.StringValue("cluster-1"),
		EnableNodeRepair: types.BoolValue(true),
		Discount:         types.Float64Value(0.25),
	}

	got := model.ToAPI()
	if got.EnableNodeRepair == nil || !*got.EnableNodeRepair {
		t.Fatalf("EnableNodeRepair = %#v", got.EnableNodeRepair)
	}
	if got.Discount == nil || *got.Discount != 0.25 {
		t.Fatalf("Discount = %#v", got.Discount)
	}
	if got.EnableDiskMonitor != nil {
		t.Fatalf("EnableDiskMonitor should be omitted, got %#v", got.EnableDiskMonitor)
	}
	if got.MaintenanceEnabled != nil {
		t.Fatalf("MaintenanceEnabled should be omitted from /setting payload, got %#v", got.MaintenanceEnabled)
	}
}

func TestClusterSettingModelToMaintenanceStatus(t *testing.T) {
	model := ClusterSettingModel{
		ClusterID:          types.StringValue("cluster-1"),
		MaintenanceEnabled: types.BoolValue(true),
	}

	got := model.ToMaintenanceStatus()
	if got == nil || got.MaintenanceModeEnabled == nil || !*got.MaintenanceModeEnabled {
		t.Fatalf("MaintenanceModeEnabled = %#v", got)
	}
}

func TestClusterSettingModelToMaintenanceStatusOmitsUnsetValue(t *testing.T) {
	model := ClusterSettingModel{
		ClusterID: types.StringValue("cluster-1"),
	}

	if got := model.ToMaintenanceStatus(); got != nil {
		t.Fatalf("expected nil maintenance payload, got %#v", got)
	}
}

func TestClusterSettingModelFromAPI(t *testing.T) {
	enableRepair := true
	enableDisk := false
	maintenance := true
	discount := 0.1
	pre := "echo pre"
	post := "echo post"

	got := ClusterSettingModelFromAPI("cluster-1", &api.ClusterSetting{
		EnableNodeRepair:   &enableRepair,
		EnableDiskMonitor:  &enableDisk,
		MaintenanceEnabled: &maintenance,
		Discount:           &discount,
		PreRunCommand:      &pre,
		PostRunCommand:     &post,
	})

	if got.ClusterID.ValueString() != "cluster-1" {
		t.Fatalf("ClusterID = %s", got.ClusterID.ValueString())
	}
	if !got.EnableNodeRepair.ValueBool() {
		t.Fatalf("EnableNodeRepair should be true")
	}
	if got.PreRunCommand.ValueString() != "echo pre" {
		t.Fatalf("PreRunCommand = %q", got.PreRunCommand.ValueString())
	}
}

func TestClusterSettingSchemaHasExpectedFields(t *testing.T) {
	s := Schema(context.Background())
	for _, name := range []string{
		"cluster_id",
		"enable_node_repair",
		"enable_disk_monitor",
		"maintenance_enabled",
		"discount",
		"pre_run_command",
		"post_run_command",
	} {
		if _, ok := s.Attributes[name]; !ok {
			t.Fatalf("schema missing %s", name)
		}
	}
}
