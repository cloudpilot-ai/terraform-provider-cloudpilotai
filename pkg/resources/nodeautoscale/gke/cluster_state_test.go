package gke

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/gkeaccess"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeleteClusterWarnsWhenRemoteClusterRecordDeletionFails(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{
		deleteClusterErr: fmt.Errorf("remote delete failed"),
	}
	cluster := &Cluster{client: client}
	stubGKEDeleteHooks(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "gke-kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	diags := state.Set(ctx, &ClusterModel{
		ClusterID:           types.StringValue("cluster-1"),
		ClusterName:         types.StringValue("demo-gke"),
		Region:              types.StringValue("us-central1"),
		ProjectID:           types.StringValue("cloudpilot-ai-dev"),
		ClusterUID:          types.StringValue("kube-system-uid-123"),
		Kubeconfig:          types.StringValue(kubeconfigPath),
		SkipRestore:         types.BoolValue(false),
		RestoreNodeNumber:   types.Int64Value(0),
		RestoreDesiredSizes: types.MapNull(types.Int64Type),
	})
	if diags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", diags)
	}

	resp := &resource.DeleteResponse{
		State: tfsdk.State{Schema: Schema(ctx)},
	}
	cluster.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics should not contain errors: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() == 0 {
		t.Fatal("expected delete warning when remote cluster record deletion fails")
	}
}

func TestDeleteClusterDoesNotCallRemoteNodeCleanup(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{
		deleteNodePoolErrs: map[string]error{
			"cloudpilot-general": fmt.Errorf("remote delete failed"),
		},
		deleteNodeClassErrs: map[string]error{
			"cloudpilot": fmt.Errorf("remote nodeclass delete failed"),
		},
	}
	cluster := &Cluster{client: client}
	stubGKEDeleteHooks(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "gke-kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	diags := state.Set(ctx, &ClusterModel{
		ClusterID:           types.StringValue("cluster-1"),
		ClusterName:         types.StringValue("demo-gke"),
		Region:              types.StringValue("us-central1"),
		ProjectID:           types.StringValue("test-project"),
		ClusterUID:          types.StringValue("kube-system-uid-123"),
		Kubeconfig:          types.StringValue(kubeconfigPath),
		RestoreDesiredSizes: types.MapNull(types.Int64Type),
		NodeClasses: customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{{
			Name: types.StringValue("cloudpilot"),
		}}),
		NodePools: customfield.NewObjectListMust(ctx, []api.GCENodePoolModel{
			testNodePoolModel("cloudpilot-general", "cloudpilot", nil, "0s"),
		}),
	})
	if diags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", diags)
	}

	resp := &resource.DeleteResponse{
		State: tfsdk.State{Schema: Schema(ctx)},
	}
	cluster.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics should not contain errors: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 0 {
		t.Fatalf("Delete() warnings = %d, want 0", resp.Diagnostics.WarningsCount())
	}
	if len(client.deletedNodePools) != 0 {
		t.Fatalf("deleted nodepools = %#v, want none", client.deletedNodePools)
	}
	if len(client.deletedNodeClasses) != 0 {
		t.Fatalf("deleted nodeclasses = %#v, want none", client.deletedNodeClasses)
	}
}

func TestDeleteClusterRestoresBeforeFullUninstallWhenConfigured(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{}
	cluster := &Cluster{client: client}
	stubGKEDeleteHooks(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "gke-kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	callOrder := make([]string, 0, 2)
	uninstallCloudpilotAIAgentComponent = func(_ context.Context, _ cloudpilotaiclient.Interface, _, _, _, _, _ string, _ map[string]string) error {
		callOrder = append(callOrder, "uninstall")
		return nil
	}
	var gotEnv map[string]string
	restoreCloudpilotAIRebalanceComponent = func(_ context.Context, _ cloudpilotaiclient.Interface, _, _, _ string, restoreEnv map[string]string, _ map[string]string) error {
		callOrder = append(callOrder, "restore")
		gotEnv = restoreEnv
		return nil
	}

	data := ClusterModel{
		ClusterName:         types.StringValue("demo-gke"),
		Region:              types.StringValue("us-central1"),
		ProjectID:           types.StringValue("cloudpilot-ai-dev"),
		ClusterUID:          types.StringValue("kube-system-uid-123"),
		Kubeconfig:          types.StringValue(kubeconfigPath),
		SkipRestore:         types.BoolValue(false),
		RestoreNodeNumber:   types.Int64Value(2),
		RestoreDesiredSizes: types.MapNull(types.Int64Type),
	}

	if err := cluster.deleteCluster(ctx, &data, "cluster-1", kubeconfigPath); err != nil {
		t.Fatalf("deleteCluster() error = %v", err)
	}
	if len(callOrder) != 2 || callOrder[0] != "restore" || callOrder[1] != "uninstall" {
		t.Fatalf("call order = %#v, want [restore uninstall]", callOrder)
	}
	if gotEnv["RESTORE_DESIRED_SIZE"] != "2" {
		t.Fatalf("RESTORE_DESIRED_SIZE = %q, want 2", gotEnv["RESTORE_DESIRED_SIZE"])
	}
}

func TestDeleteClusterUsesConfiguredPerPoolRestoreSizes(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{
		summary: &api.ClusterCostsSummary{
			ClusterInfo: api.ClusterInfo{
				ClusterName: "demo-gke",
				Region:      "us-central1",
			},
		},
	}
	cluster := &Cluster{client: client}
	stubGKEDeleteHooks(t)

	originalUpdate := gkeaccess.RunGcloudUpdateKubeconfig
	defer func() {
		gkeaccess.RunGcloudUpdateKubeconfig = originalUpdate
	}()
	gkeaccess.RunGcloudUpdateKubeconfig = func(_ context.Context, clusterName, region, projectID, kubeconfigPath string) error {
		if clusterName != "demo-gke" || region != "us-central1" || projectID != "test-project" {
			t.Fatalf("unexpected kubeconfig discovery inputs: %s %s %s", clusterName, region, projectID)
		}
		return os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600)
	}

	uninstallCloudpilotAIAgentComponent = func(_ context.Context, _ cloudpilotaiclient.Interface, _, _, _, _, _ string, _ map[string]string) error {
		return nil
	}

	var gotKubeconfig string
	var gotRestoreEnv map[string]string
	restoreCloudpilotAIRebalanceComponent = func(_ context.Context, _ cloudpilotaiclient.Interface, _, _, kubeconfigPath string, restoreEnv map[string]string, _ map[string]string) error {
		gotKubeconfig = kubeconfigPath
		gotRestoreEnv = restoreEnv
		return nil
	}

	data := ClusterModel{
		ClusterID:         types.StringValue("cluster-1"),
		ClusterName:       types.StringValue("demo-gke"),
		Region:            types.StringValue("us-central1"),
		ProjectID:         types.StringNull(),
		ClusterUID:        types.StringValue("kube-system-uid-123"),
		Kubeconfig:        types.StringNull(),
		RestoreNodeNumber: types.Int64Value(2),
		RestoreDesiredSizes: types.MapValueMust(
			types.Int64Type,
			map[string]attr.Value{
				"general-pool": types.Int64Value(3),
			},
		),
		NodeClasses: customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{{
			Name: types.StringValue("cloudpilot"),
			NetworkConfig: customfield.NewObjectMust(ctx, &api.GCENetworkConfigModel{
				Subnetwork: types.StringValue("projects/test-project/regions/us-central1/subnetworks/default"),
			}),
		}}),
	}

	kubeconfigPath, err := cluster.fillMissingParameters(ctx, &data, false)
	if err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}
	if err := cluster.deleteCluster(ctx, &data, "cluster-1", kubeconfigPath); err != nil {
		t.Fatalf("deleteCluster() error = %v", err)
	}

	if filepath.Base(gotKubeconfig) != "test-project_us-central1_demo-gke_kubeconfig" {
		t.Fatalf("restore kubeconfig = %q, want inferred GKE kubeconfig", gotKubeconfig)
	}
	if gotRestoreEnv["RESTORE_DESIRED_SIZE"] != "2" {
		t.Fatalf("RESTORE_DESIRED_SIZE = %q, want 2", gotRestoreEnv["RESTORE_DESIRED_SIZE"])
	}
	if gotRestoreEnv["RESTORE_DESIRED_SIZE_GENERAL_POOL"] != "3" {
		t.Fatalf("RESTORE_DESIRED_SIZE_GENERAL_POOL = %q, want 3", gotRestoreEnv["RESTORE_DESIRED_SIZE_GENERAL_POOL"])
	}
}

func TestFillMissingParametersInfersProjectFromGcloudAndClusterUIDFromKubeconfig(t *testing.T) {
	ctx := context.Background()
	cluster := &Cluster{client: &fakeClusterClient{}}

	originalCurrentProject := gkeaccess.RunGcloudCurrentProject
	defer func() {
		gkeaccess.RunGcloudCurrentProject = originalCurrentProject
	}()
	gkeaccess.RunGcloudCurrentProject = func(context.Context) (string, error) {
		return "cloudpilot-ai-dev", nil
	}

	originalUpdate := gkeaccess.RunGcloudUpdateKubeconfig
	defer func() {
		gkeaccess.RunGcloudUpdateKubeconfig = originalUpdate
	}()
	gkeaccess.RunGcloudUpdateKubeconfig = func(_ context.Context, clusterName, region, projectID, kubeconfigPath string) error {
		if clusterName != "demo-gke" || region != "us-central1" || projectID != "cloudpilot-ai-dev" {
			t.Fatalf("unexpected kubeconfig discovery inputs: %s %s %s", clusterName, region, projectID)
		}
		return os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600)
	}

	originalClusterUID := gkeaccess.RunKubectlGetClusterUID
	defer func() {
		gkeaccess.RunKubectlGetClusterUID = originalClusterUID
	}()
	gkeaccess.RunKubectlGetClusterUID = func(_ context.Context, kubeconfigPath string) (string, error) {
		if filepath.Base(kubeconfigPath) != "cloudpilot-ai-dev_us-central1_demo-gke_kubeconfig" {
			t.Fatalf("unexpected kubeconfig path: %s", kubeconfigPath)
		}
		return "kube-system-uid-123", nil
	}

	data := ClusterModel{
		ClusterName:         types.StringValue("demo-gke"),
		Region:              types.StringValue("us-central1"),
		ProjectID:           types.StringNull(),
		ClusterUID:          types.StringNull(),
		Kubeconfig:          types.StringNull(),
		SkipRestore:         types.BoolValue(false),
		RestoreNodeNumber:   types.Int64Value(0),
		RestoreDesiredSizes: types.MapNull(types.Int64Type),
	}

	effectiveKubeconfig, err := cluster.fillMissingParameters(ctx, &data, false)
	if err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}

	if data.ProjectID != types.StringValue("cloudpilot-ai-dev") {
		t.Fatalf("project_id = %#v, want cloudpilot-ai-dev", data.ProjectID)
	}
	if data.ClusterUID != types.StringValue("kube-system-uid-123") {
		t.Fatalf("cluster_uid = %#v, want kube-system-uid-123", data.ClusterUID)
	}
	if filepath.Base(effectiveKubeconfig) != "cloudpilot-ai-dev_us-central1_demo-gke_kubeconfig" {
		t.Fatalf("effective kubeconfig = %q, want inferred path", effectiveKubeconfig)
	}
	if !data.Kubeconfig.IsNull() {
		t.Fatalf("kubeconfig state = %#v, want null", data.Kubeconfig)
	}
}

func TestFillMissingParametersUsesClusterLocationForZonalKubeconfig(t *testing.T) {
	ctx := context.Background()
	cluster := &Cluster{client: &fakeClusterClient{}}

	originalCurrentProject := gkeaccess.RunGcloudCurrentProject
	defer func() {
		gkeaccess.RunGcloudCurrentProject = originalCurrentProject
	}()
	gkeaccess.RunGcloudCurrentProject = func(context.Context) (string, error) {
		return "cloudpilot-ai-dev", nil
	}

	originalUpdate := gkeaccess.RunGcloudUpdateKubeconfig
	defer func() {
		gkeaccess.RunGcloudUpdateKubeconfig = originalUpdate
	}()
	gkeaccess.RunGcloudUpdateKubeconfig = func(_ context.Context, clusterName, location, projectID, kubeconfigPath string) error {
		if clusterName != "demo-gke" || location != "us-central1-f" || projectID != "cloudpilot-ai-dev" {
			t.Fatalf("unexpected kubeconfig discovery inputs: %s %s %s", clusterName, location, projectID)
		}
		return os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600)
	}

	originalClusterUID := gkeaccess.RunKubectlGetClusterUID
	defer func() {
		gkeaccess.RunKubectlGetClusterUID = originalClusterUID
	}()
	gkeaccess.RunKubectlGetClusterUID = func(_ context.Context, kubeconfigPath string) (string, error) {
		if filepath.Base(kubeconfigPath) != "cloudpilot-ai-dev_us-central1-f_demo-gke_kubeconfig" {
			t.Fatalf("unexpected kubeconfig path: %s", kubeconfigPath)
		}
		return "kube-system-uid-123", nil
	}

	data := ClusterModel{
		ClusterName:         types.StringValue("demo-gke"),
		Region:              types.StringValue("us-central1"),
		ClusterLocation:     types.StringValue("us-central1-f"),
		ProjectID:           types.StringNull(),
		ClusterUID:          types.StringNull(),
		Kubeconfig:          types.StringNull(),
		SkipRestore:         types.BoolValue(false),
		RestoreNodeNumber:   types.Int64Value(0),
		RestoreDesiredSizes: types.MapNull(types.Int64Type),
	}

	effectiveKubeconfig, err := cluster.fillMissingParameters(ctx, &data, false)
	if err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}

	if filepath.Base(effectiveKubeconfig) != "cloudpilot-ai-dev_us-central1-f_demo-gke_kubeconfig" {
		t.Fatalf("effective kubeconfig = %q, want zonal inferred path", effectiveKubeconfig)
	}
	if !data.Kubeconfig.IsNull() {
		t.Fatalf("kubeconfig state = %#v, want null", data.Kubeconfig)
	}
}

func TestFillMissingParametersKeepsProjectIDUnmanagedWithConfiguredKubeconfig(t *testing.T) {
	ctx := context.Background()
	cluster := &Cluster{client: &fakeClusterClient{}}
	t.Chdir(t.TempDir())
	kubeconfigPath := "./gke-kubeconfig"
	if err := os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	originalCurrentProject := gkeaccess.RunGcloudCurrentProject
	originalUpdate := gkeaccess.RunGcloudUpdateKubeconfig
	originalClusterUID := gkeaccess.RunKubectlGetClusterUID
	defer func() {
		gkeaccess.RunGcloudCurrentProject = originalCurrentProject
		gkeaccess.RunGcloudUpdateKubeconfig = originalUpdate
		gkeaccess.RunKubectlGetClusterUID = originalClusterUID
	}()
	gkeaccess.RunGcloudCurrentProject = func(context.Context) (string, error) {
		t.Fatal("current project should not be read when kubeconfig is already configured")
		return "", nil
	}
	gkeaccess.RunGcloudUpdateKubeconfig = func(context.Context, string, string, string, string) error {
		t.Fatal("kubeconfig should not be generated when a valid kubeconfig is already configured")
		return nil
	}
	gkeaccess.RunKubectlGetClusterUID = func(context.Context, string) (string, error) {
		t.Fatal("cluster_uid should not be read when already configured")
		return "", nil
	}

	data := ClusterModel{
		ClusterName:         types.StringValue("demo-gke"),
		Region:              types.StringValue("us-central1"),
		ProjectID:           types.StringUnknown(),
		ClusterUID:          types.StringValue("kube-system-uid-123"),
		Kubeconfig:          types.StringValue(kubeconfigPath),
		SkipRestore:         types.BoolValue(false),
		RestoreNodeNumber:   types.Int64Value(0),
		RestoreDesiredSizes: types.MapNull(types.Int64Type),
	}

	effectiveKubeconfig, err := cluster.fillMissingParameters(ctx, &data, false)
	if err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}
	if !data.ProjectID.IsNull() {
		t.Fatalf("project_id = %#v, want null unmanaged value", data.ProjectID)
	}
	if data.Kubeconfig.ValueString() != kubeconfigPath {
		t.Fatalf("kubeconfig = %#v, want configured kubeconfig", data.Kubeconfig)
	}
	if !filepath.IsAbs(effectiveKubeconfig) || filepath.Base(effectiveKubeconfig) != filepath.Base(kubeconfigPath) {
		t.Fatalf("effective kubeconfig = %q, want absolute runtime path for %q", effectiveKubeconfig, kubeconfigPath)
	}
}

func TestFillMissingParametersCreatesMissingConfiguredKubeconfigAtExactPath(t *testing.T) {
	ctx := context.Background()
	cluster := &Cluster{client: &fakeClusterClient{}}
	t.Chdir(t.TempDir())
	configuredPath := filepath.Join("nested", "gke-kubeconfig")
	if err := os.MkdirAll(filepath.Dir(configuredPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	originalUpdate := gkeaccess.RunGcloudUpdateKubeconfig
	originalClusterUID := gkeaccess.RunKubectlGetClusterUID
	defer func() {
		gkeaccess.RunGcloudUpdateKubeconfig = originalUpdate
		gkeaccess.RunKubectlGetClusterUID = originalClusterUID
	}()
	var writtenPath string
	gkeaccess.RunGcloudUpdateKubeconfig = func(_ context.Context, clusterName, location, projectID, kubeconfigPath string) error {
		if clusterName != "demo-gke" || location != "us-central1-a" || projectID != "target-project" {
			t.Fatalf("kubeconfig inputs = %s/%s/%s", clusterName, location, projectID)
		}
		writtenPath = kubeconfigPath
		return os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600)
	}
	gkeaccess.RunKubectlGetClusterUID = func(context.Context, string) (string, error) {
		return "kube-system-uid-123", nil
	}

	data := ClusterModel{
		ClusterName:     types.StringValue("demo-gke"),
		Region:          types.StringValue("us-central1"),
		ClusterLocation: types.StringValue("us-central1-a"),
		ProjectID:       types.StringValue("target-project"),
		ClusterUID:      types.StringNull(),
		Kubeconfig:      types.StringValue(configuredPath),
	}
	effectivePath, err := cluster.fillMissingParameters(ctx, &data, false)
	if err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}
	wantPath, err := filepath.Abs(configuredPath)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	if effectivePath != wantPath || writtenPath != wantPath {
		t.Fatalf("effective path = %q, written path = %q, want %q", effectivePath, writtenPath, wantPath)
	}
	if data.Kubeconfig != types.StringValue(configuredPath) {
		t.Fatalf("kubeconfig state = %#v, want exact configured value %q", data.Kubeconfig, configuredPath)
	}
}

func TestValidateClusterIdentityRequiresProjectIDAndClusterUIDForManagedWrites(t *testing.T) {
	err := validateClusterIdentity(&ClusterModel{})
	if err == nil {
		t.Fatal("expected missing identity error")
	}
}

func TestReadHydratesGKERemoteStateWithoutAWSDiscovery(t *testing.T) {
	ctx := context.Background()

	repairDisabled := false
	client := &fakeClusterClient{
		summary: &api.ClusterCostsSummary{
			ClusterInfo: api.ClusterInfo{
				ClusterName: "demo-gke",
				Region:      "us-central1",
			},
		},
		clusterSetting: &api.ClusterSetting{
			EnableNodeRepair: &repairDisabled,
		},
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	diags := state.Set(ctx, &ClusterModel{
		ClusterID:           types.StringValue("cluster-1"),
		RestoreDesiredSizes: types.MapNull(types.Int64Type),
		ClusterSetting:      customfield.NewObjectMust(ctx, &ClusterSettingModel{EnableNodeRepair: types.BoolValue(true)}),
	})
	if diags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", diags)
	}

	resp := &resource.ReadResponse{
		State: tfsdk.State{Schema: Schema(ctx)},
	}
	(&Cluster{client: client}).Read(ctx, resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics = %v", resp.Diagnostics)
	}
	if client.getClusterCalls != 1 {
		t.Fatalf("GetCluster calls = %d, want 1", client.getClusterCalls)
	}

	var got ClusterModel
	getDiags := resp.State.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.State.Get() diagnostics = %v", getDiags)
	}

	clusterSetting, clusterSettingDiags := got.ClusterSetting.Value(ctx)
	if clusterSettingDiags.HasError() {
		t.Fatalf("cluster_setting diagnostics = %v", clusterSettingDiags)
	}
	if clusterSetting == nil || clusterSetting.EnableNodeRepair != types.BoolValue(false) {
		t.Fatalf("cluster setting = %#v, want enable_node_repair=false", clusterSetting)
	}
	if got.ClusterName != types.StringValue("demo-gke") {
		t.Fatalf("cluster_name = %#v, want demo-gke", got.ClusterName)
	}
	if got.Region != types.StringValue("us-central1") {
		t.Fatalf("region = %#v, want us-central1", got.Region)
	}

}

func TestReadClusterManagementStateToleratesMissingNodeAutoscalerListsOnImport(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{
		listNodeClassesErr: cloudpilotaiclient.ErrNotFound,
		listNodePoolsErr:   cloudpilotaiclient.ErrNotFound,
	}
	data := ClusterModel{
		NodeClasses: customfield.NullObjectList[api.GCENodeClassModel](ctx),
		NodePools:   customfield.NullObjectList[api.GCENodePoolModel](ctx),
	}

	if err := (&Cluster{client: client}).readClusterManagementState(ctx, &data, "cluster-1", true); err != nil {
		t.Fatalf("readClusterManagementState() error = %v", err)
	}
	if !data.NodeClasses.IsNull() {
		t.Fatalf("nodeclasses = %#v, want null when remote list is absent", data.NodeClasses)
	}
	if !data.NodePools.IsNull() {
		t.Fatalf("nodepools = %#v, want null when remote list is absent", data.NodePools)
	}
}

func TestReadClusterManagementStateImportsScheduledRebalances(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{scheduledRebalances: api.ScheduledRebalancePolicyList{
		ScheduledRebalancePolicies: []api.ScheduledRebalancePolicy{{
			Name: "nightly", Cron: "0 2 * * *", Timezone: "UTC", Enabled: true,
		}},
	}}
	data := ClusterModel{
		NodeClasses:         customfield.NullObjectList[api.GCENodeClassModel](ctx),
		NodePools:           customfield.NullObjectList[api.GCENodePoolModel](ctx),
		ScheduledRebalances: customfield.NullObjectList[api.ScheduledRebalancePolicyModel](ctx),
	}

	if err := (&Cluster{client: client}).readClusterManagementState(ctx, &data, "cluster-1", true); err != nil {
		t.Fatalf("readClusterManagementState() error = %v", err)
	}
	models, diags := data.ScheduledRebalances.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("scheduled_rebalances diagnostics = %v", diags)
	}
	if len(models) != 1 || models[0].Name.ValueString() != "nightly" || models[0].Cron.ValueString() != "0 2 * * *" {
		t.Fatalf("scheduled_rebalances = %#v, want imported nightly policy", models)
	}
}

func TestRefreshNodeAutoscalerStateDropsMissingRemoteObjects(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{}
	nodeClasses := customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{testNodeClassModel(ctx, "deleted-class")})
	nodePools := customfield.NewObjectListMust(ctx, []api.GCENodePoolModel{testNodePoolModel("deleted-pool", "deleted-class", nil, "0s")})

	refreshedNodeClasses, err := refreshNodeClassesState(ctx, client, "cluster-1", nodeClasses)
	if err != nil {
		t.Fatalf("refreshNodeClassesState() error = %v", err)
	}
	refreshedNodePools, err := refreshNodePoolsState(ctx, client, "cluster-1", nodePools)
	if err != nil {
		t.Fatalf("refreshNodePoolsState() error = %v", err)
	}
	classModels, classDiags := refreshedNodeClasses.AsStructSliceT(ctx)
	poolModels, poolDiags := refreshedNodePools.AsStructSliceT(ctx)
	if classDiags.HasError() || poolDiags.HasError() {
		t.Fatalf("refresh diagnostics: nodeclasses=%v nodepools=%v", classDiags, poolDiags)
	}
	if len(classModels) != 0 || len(poolModels) != 0 {
		t.Fatalf("refreshed state kept deleted objects: nodeclasses=%#v nodepools=%#v", classModels, poolModels)
	}
}

func TestRefreshNodeClassStateDropsMissingManagedLocalSSDConfiguration(t *testing.T) {
	ctx := context.Background()
	stateNodeClass := testNodeClassModel(ctx, "default")
	stateNodeClass.EphemeralStorageLocalSSD = customfield.NewObjectMust(ctx, &api.GCEEphemeralStorageLocalSSDModel{
		Count: types.Int32Value(4),
	})
	current := customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{stateNodeClass})
	client := &fakeClusterClient{nodeClasses: api.RebalanceNodeClassList{
		GCENodeClasses: []api.GCENodeClass{testRemoteNodeClass("default")},
	}}

	refreshed, err := refreshNodeClassesState(ctx, client, "cluster-1", current)
	if err != nil {
		t.Fatalf("refreshNodeClassesState() error = %v", err)
	}
	models, diags := refreshed.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("refreshed diagnostics = %v", diags)
	}
	if len(models) != 1 || !models[0].EphemeralStorageLocalSSD.IsNull() {
		t.Fatalf("refreshed Local SSD state = %#v, want remote null", models)
	}
}

func TestPostWriteNodeClassHydrationPreservesPendingLocalSSDConfiguration(t *testing.T) {
	ctx := context.Background()
	stateNodeClass := testNodeClassModel(ctx, "default")
	stateNodeClass.EphemeralStorageLocalSSD = customfield.NewObjectMust(ctx, &api.GCEEphemeralStorageLocalSSDModel{
		Count: types.Int32Value(4),
	})
	current := customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{stateNodeClass})
	client := &fakeClusterClient{nodeClasses: api.RebalanceNodeClassList{
		GCENodeClasses: []api.GCENodeClass{testRemoteNodeClass("default")},
	}}

	hydrated, err := hydrateNodeClassesPostWrite(ctx, client, "cluster-1", current)
	if err != nil {
		t.Fatalf("hydrateNodeClassesPostWrite() error = %v", err)
	}
	models, diags := hydrated.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("hydrated diagnostics = %v", diags)
	}
	localSSD, localSSDDiags := models[0].EphemeralStorageLocalSSD.Value(ctx)
	if localSSDDiags.HasError() || localSSD == nil || localSSD.Count.ValueInt32() != 4 {
		t.Fatalf("post-write Local SSD state = %#v, diagnostics = %v; want pending count preserved", localSSD, localSSDDiags)
	}
}

func TestPostWriteNodeClassHydrationNormalizesOmittedLocalSSDSwitchToFalse(t *testing.T) {
	ctx := context.Background()
	stateNodeClass := testNodeClassModel(ctx, "default")
	stateNodeClass.EnableLocalSSDEphemeralStorage = types.BoolValue(false)
	current := customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{stateNodeClass})
	client := &fakeClusterClient{nodeClasses: api.RebalanceNodeClassList{
		GCENodeClasses: []api.GCENodeClass{testRemoteNodeClass("default")},
	}}

	hydrated, err := hydrateNodeClassesPostWrite(ctx, client, "cluster-1", current)
	if err != nil {
		t.Fatalf("hydrateNodeClassesPostWrite() error = %v", err)
	}
	models, diags := hydrated.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("hydrated diagnostics = %v", diags)
	}
	if len(models) != 1 || models[0].EnableLocalSSDEphemeralStorage != types.BoolValue(false) {
		t.Fatalf("hydrated Local SSD switch = %#v, want false", models)
	}
}

func TestReadHydratesRemoteNodeAutoscalerState(t *testing.T) {
	ctx := context.Background()

	emptyFamilies := []types.String{}
	stateNodeClass := testNodeClassModel(ctx, "default")
	stateNodeClass.EnableImageAccelerator = types.BoolValue(true)
	stateNodeClass.OriginNodeClassJSON = types.StringValue(`{"kind":"GCENodeClass"}`)
	stateNodePool := testNodePoolModel("general", "default", &emptyFamilies, "60m")
	stateNodePool.EnableImageAccelerator = types.BoolValue(true)
	stateNodePool.OriginNodePoolJSON = types.StringValue(`{"kind":"NodePool"}`)

	client := &fakeClusterClient{
		summary: &api.ClusterCostsSummary{
			ClusterInfo: api.ClusterInfo{
				ClusterName: "demo-gke",
				Region:      "us-central1",
			},
		},
		rebalanceConfig: &api.RebalanceConfig{
			Enable:                   true,
			LastComponentsActiveTime: metav1.NewTime(time.Now()),
		},
		nodeClasses: api.RebalanceNodeClassList{
			GCENodeClasses: []api.GCENodeClass{
				testRemoteNodeClass("default"),
			},
		},
		nodePools: api.RebalanceNodePoolList{
			GCENodePools: []api.GCENodePool{
				testRemoteNodePool(t, ctx, "general", "default", nil, "1h"),
			},
		},
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	diags := state.Set(ctx, &ClusterModel{
		ClusterID:           types.StringValue("cluster-1"),
		EnableRebalance:     types.BoolValue(false),
		RestoreDesiredSizes: types.MapNull(types.Int64Type),
		NodeClasses: customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{
			stateNodeClass,
		}),
		NodePools: customfield.NewObjectListMust(ctx, []api.GCENodePoolModel{
			stateNodePool,
		}),
	})
	if diags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", diags)
	}

	resp := &resource.ReadResponse{
		State: tfsdk.State{Schema: Schema(ctx)},
	}
	(&Cluster{client: client}).Read(ctx, resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics = %v", resp.Diagnostics)
	}

	var got ClusterModel
	getDiags := resp.State.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.State.Get() diagnostics = %v", getDiags)
	}
	if got.EnableRebalance != types.BoolValue(true) {
		t.Fatalf("enable_rebalance = %#v, want true", got.EnableRebalance)
	}

	nodeClasses, nodeClassDiags := got.NodeClasses.AsStructSliceT(ctx)
	if nodeClassDiags.HasError() {
		t.Fatalf("nodeclasses diagnostics = %v", nodeClassDiags)
	}
	if len(nodeClasses) != 1 || nodeClasses[0].OriginNodeClassJSON != types.StringValue(`{"kind":"GCENodeClass"}`) {
		t.Fatalf("nodeclasses = %#v, want origin_nodeclass_json preserved", nodeClasses)
	}
	if nodeClasses[0].EnableImageAccelerator != types.BoolValue(true) {
		t.Fatalf("nodeclass enable_image_accelerator = %#v, want true", nodeClasses[0].EnableImageAccelerator)
	}

	nodePools, nodePoolDiags := got.NodePools.AsStructSliceT(ctx)
	if nodePoolDiags.HasError() {
		t.Fatalf("nodepools diagnostics = %v", nodePoolDiags)
	}
	if len(nodePools) != 1 {
		t.Fatalf("nodepools length = %d, want 1", len(nodePools))
	}
	if nodePools[0].EnableImageAccelerator != types.BoolValue(true) {
		t.Fatalf("nodepool enable_image_accelerator = %#v, want true", nodePools[0].EnableImageAccelerator)
	}
	if nodePools[0].NodeDisruptionDelay != types.StringValue("60m") {
		t.Fatalf("node_disruption_delay = %#v, want preserved 60m", nodePools[0].NodeDisruptionDelay)
	}
	if nodePools[0].InstanceFamily == nil || len(*nodePools[0].InstanceFamily) != 0 {
		t.Fatalf("instance_family = %#v, want preserved explicit empty list", nodePools[0].InstanceFamily)
	}
}

func TestMergeClusterSettingFromAPIPreservesNullEmptyCommands(t *testing.T) {
	setting := &ClusterSettingModel{
		PreRunCommand:  types.StringNull(),
		PostRunCommand: types.StringNull(),
	}
	empty := ""

	mergeClusterSettingFromAPI(setting, &api.ClusterSetting{
		PreRunCommand:  &empty,
		PostRunCommand: &empty,
	})

	if !setting.PreRunCommand.IsNull() {
		t.Fatalf("pre_run_command = %#v, want null", setting.PreRunCommand)
	}
	if !setting.PostRunCommand.IsNull() {
		t.Fatalf("post_run_command = %#v, want null", setting.PostRunCommand)
	}
}

func TestClusterSettingObjectPreservingStatePreservesNullEmptyCommands(t *testing.T) {
	ctx := context.Background()
	empty := ""
	current := customfield.NewObjectMust(ctx, &ClusterSettingModel{
		EnableNodeRepair:  types.BoolValue(true),
		EnableDiskMonitor: types.BoolValue(true),
		Discount:          types.Float64Value(0.1),
		PreRunCommand:     types.StringNull(),
		PostRunCommand:    types.StringNull(),
	})

	got, err := clusterSettingObjectPreservingState(ctx, current, &api.ClusterSetting{
		PreRunCommand:  &empty,
		PostRunCommand: &empty,
	})
	if err != nil {
		t.Fatalf("clusterSettingObjectPreservingState() error = %v", err)
	}
	value, diags := got.Value(ctx)
	if diags.HasError() {
		t.Fatalf("cluster_setting diagnostics = %v", diags)
	}
	if value == nil {
		t.Fatal("cluster_setting value is nil")
	}
	if !value.PreRunCommand.IsNull() {
		t.Fatalf("pre_run_command = %#v, want null", value.PreRunCommand)
	}
	if !value.PostRunCommand.IsNull() {
		t.Fatalf("post_run_command = %#v, want null", value.PostRunCommand)
	}
}
