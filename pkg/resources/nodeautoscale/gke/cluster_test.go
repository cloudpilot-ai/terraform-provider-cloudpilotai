package gke

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/gkeaccess"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type fakeClusterClient struct {
	cloudpilotaiclient.Interface
	summary             *api.ClusterCostsSummary
	getClusterErr       error
	rebalanceConfig     *api.RebalanceConfig
	getRebalanceErr     error
	clusterSetting      *api.ClusterSetting
	nodeClasses         api.RebalanceNodeClassList
	nodePools           api.RebalanceNodePoolList
	appliedNodeClasses  []api.RebalanceNodeClass
	appliedNodePools    []api.RebalanceNodePool
	rebalanceUpdates    []*api.RebalanceConfig
	deletedNodeClasses  []string
	deletedNodePools    []string
	operations          []string
	deletedClusters     []string
	getClusterCalls     int
	updateRebalanceErr  error
	listNodeClassesErr  error
	listNodePoolsErr    error
	deleteClusterErr    error
	deleteNodePoolErrs  map[string]error
	deleteNodeClassErrs map[string]error
}

type fakeAgentInstallClusterClient struct {
	cloudpilotaiclient.Interface
	getClusterCalls int
}

func (f *fakeAgentInstallClusterClient) GetCluster(string) (*api.ClusterCostsSummary, error) {
	f.getClusterCalls++
	if f.getClusterCalls == 1 {
		return nil, cloudpilotaiclient.ErrNotFound
	}
	return &api.ClusterCostsSummary{}, nil
}

func (f *fakeClusterClient) GetCluster(string) (*api.ClusterCostsSummary, error) {
	f.getClusterCalls++
	if f.getClusterErr != nil {
		return nil, f.getClusterErr
	}
	if f.summary != nil {
		return f.summary, nil
	}
	return &api.ClusterCostsSummary{}, nil
}

func (f *fakeClusterClient) GetClusterSetting(string) (*api.ClusterSetting, error) {
	if f.clusterSetting != nil {
		return f.clusterSetting, nil
	}
	return &api.ClusterSetting{}, nil
}

func (f *fakeClusterClient) GetRebalanceConfiguration(string) (*api.RebalanceConfig, error) {
	if f.getRebalanceErr != nil {
		return nil, f.getRebalanceErr
	}
	if f.rebalanceConfig != nil {
		return f.rebalanceConfig, nil
	}
	return &api.RebalanceConfig{}, nil
}

func (f *fakeClusterClient) ListNodeClasses(string) (api.RebalanceNodeClassList, error) {
	if f.listNodeClassesErr != nil {
		return api.RebalanceNodeClassList{}, f.listNodeClassesErr
	}
	return f.nodeClasses, nil
}

func (f *fakeClusterClient) ListNodePools(string) (api.RebalanceNodePoolList, error) {
	if f.listNodePoolsErr != nil {
		return api.RebalanceNodePoolList{}, f.listNodePoolsErr
	}
	return f.nodePools, nil
}

func (f *fakeClusterClient) ApplyNodeClass(_ string, nodeClass api.RebalanceNodeClass) error {
	if nodeClass.GCENodeClass != nil {
		f.operations = append(f.operations, fmt.Sprintf("apply-nodeclass:%s", nodeClass.GCENodeClass.Name))
	}
	f.appliedNodeClasses = append(f.appliedNodeClasses, nodeClass)
	return nil
}

func (f *fakeClusterClient) ApplyNodePool(_ string, nodePool api.RebalanceNodePool) error {
	if nodePool.GCENodePool != nil {
		f.operations = append(f.operations, fmt.Sprintf("apply-nodepool:%s", nodePool.GCENodePool.Name))
	}
	f.appliedNodePools = append(f.appliedNodePools, nodePool)
	return nil
}

func (f *fakeClusterClient) DeleteNodeClass(_ string, nodeClassName string) error {
	if err := f.deleteNodeClassErrs[nodeClassName]; err != nil {
		return err
	}
	for _, nodePool := range f.nodePools.GCENodePools {
		if nodePool.NodePoolSpec == nil || nodePool.NodePoolSpec.Template.Spec.NodeClassRef == nil {
			continue
		}
		if nodePool.NodePoolSpec.Template.Spec.NodeClassRef.Name == nodeClassName {
			return fmt.Errorf("gcenodeclass %s is referenced by nodepools [%s], please delete the nodepools first", nodeClassName, nodePool.Name)
		}
	}
	f.operations = append(f.operations, fmt.Sprintf("delete-nodeclass:%s", nodeClassName))
	f.deletedNodeClasses = append(f.deletedNodeClasses, nodeClassName)
	return nil
}

func (f *fakeClusterClient) DeleteNodePool(_ string, nodePoolName string) error {
	if err := f.deleteNodePoolErrs[nodePoolName]; err != nil {
		return err
	}
	f.operations = append(f.operations, fmt.Sprintf("delete-nodepool:%s", nodePoolName))
	f.deletedNodePools = append(f.deletedNodePools, nodePoolName)
	f.nodePools.GCENodePools = lo.Reject(f.nodePools.GCENodePools, func(item api.GCENodePool, _ int) bool {
		return item.Name == nodePoolName
	})
	return nil
}

func (f *fakeClusterClient) UpdateRebalanceConfiguration(_ string, config *api.RebalanceConfig) error {
	if f.updateRebalanceErr != nil {
		return f.updateRebalanceErr
	}
	if config != nil {
		f.operations = append(f.operations, fmt.Sprintf("rebalance:%t", config.Enable))
	}
	f.rebalanceUpdates = append(f.rebalanceUpdates, config)
	return nil
}

func (f *fakeClusterClient) DeleteCluster(clusterID string) error {
	if f.deleteClusterErr != nil {
		return f.deleteClusterErr
	}
	f.deletedClusters = append(f.deletedClusters, clusterID)
	return nil
}

func stubGKEDeleteHooks(t *testing.T) {
	t.Helper()

	originalUninstall := uninstallCloudpilotAIAgentComponent
	originalRestore := restoreCloudpilotAIRebalanceComponent
	originalWAUninstall := uninstallWorkloadAutoscaler
	originalDeleteNamespace := deleteCloudpilotNamespace
	t.Cleanup(func() {
		uninstallCloudpilotAIAgentComponent = originalUninstall
		restoreCloudpilotAIRebalanceComponent = originalRestore
		uninstallWorkloadAutoscaler = originalWAUninstall
		deleteCloudpilotNamespace = originalDeleteNamespace
	})

	uninstallCloudpilotAIAgentComponent = func(context.Context, cloudpilotaiclient.Interface, string, string, string, string, string, map[string]string) error {
		return nil
	}
	restoreCloudpilotAIRebalanceComponent = func(context.Context, cloudpilotaiclient.Interface, string, string, string, map[string]string, map[string]string) error {
		return nil
	}
	uninstallWorkloadAutoscaler = func(context.Context, cloudpilotaiclient.Interface, string, string) error {
		return nil
	}
	deleteCloudpilotNamespace = func(context.Context, string, map[string]string) error {
		return nil
	}
}

func TestSchemaExposesGKEIdentityAndClusterFields(t *testing.T) {
	s := Schema(context.Background())

	for _, name := range []string{
		"cluster_name",
		"region",
		"project_id",
		"cluster_uid",
		"kubeconfig",
		"cluster_setting",
		"only_install_agent",
		"enable_rebalance",
		"nodeclasses",
		"nodepools",
	} {
		if _, ok := s.Attributes[name]; !ok {
			t.Fatalf("gke schema missing %s", name)
		}
	}

	for _, name := range []string{
		"aws_profile",
		"aws_assume_role",
		"recommendation_policies",
		"autoscaling_policies",
		"workloads",
		"workload_templates",
	} {
		if _, ok := s.Attributes[name]; ok {
			t.Fatalf("gke schema should not expose %s", name)
		}
	}

	projectIDAttr, ok := s.Attributes["project_id"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("project_id attribute has unexpected type %T", s.Attributes["project_id"])
	}
	if !projectIDAttr.IsOptional() || !projectIDAttr.IsComputed() {
		t.Fatalf("project_id should be optional+computed for import discovery")
	}

	clusterUIDAttr, ok := s.Attributes["cluster_uid"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("cluster_uid attribute has unexpected type %T", s.Attributes["cluster_uid"])
	}
	if !clusterUIDAttr.IsOptional() || !clusterUIDAttr.IsComputed() {
		t.Fatalf("cluster_uid should be optional+computed for import discovery")
	}

	nodeClassesAttr, ok := s.Attributes["nodeclasses"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("nodeclasses attribute has unexpected type %T", s.Attributes["nodeclasses"])
	}
	if _, ok := nodeClassesAttr.NestedObject.Attributes["enable_image_accelerator"]; !ok {
		t.Fatal("nodeclasses schema missing enable_image_accelerator")
	}

	nodePoolsAttr, ok := s.Attributes["nodepools"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("nodepools attribute has unexpected type %T", s.Attributes["nodepools"])
	}
	if _, ok := nodePoolsAttr.NestedObject.Attributes["enable_image_accelerator"]; !ok {
		t.Fatal("nodepools schema missing enable_image_accelerator")
	}
}

func TestResolveClusterUIDPrefersConfiguredIDThenStateThenGeneratedGCPID(t *testing.T) {
	got := resolveClusterUID(
		types.StringValue("configured-uid"),
		types.StringValue("state-uid"),
		types.StringValue("test-gke"),
		types.StringValue("us-central1"),
		types.StringValue("gke-cluster-uid-123"),
	)
	if got != "configured-uid" {
		t.Fatalf("got cluster UID %q, want configured cluster UID", got)
	}

	got = resolveClusterUID(
		types.StringNull(),
		types.StringValue("state-uid"),
		types.StringValue("test-gke"),
		types.StringValue("us-central1"),
		types.StringValue("gke-cluster-uid-123"),
	)
	if got != "state-uid" {
		t.Fatalf("got cluster UID %q, want state cluster UID", got)
	}

	got = resolveClusterUID(
		types.StringUnknown(),
		types.StringNull(),
		types.StringValue("test-gke"),
		types.StringValue("us-central1"),
		types.StringValue("gke-cluster-uid-123"),
	)
	if got != "0afb8050-e986-517e-8310-e83e668a659e" {
		t.Fatalf("got generated cluster UID %q, want GCP-generated UID", got)
	}
}

func TestEnsureAgentInstalledPassesDisableWorkloadUploading(t *testing.T) {
	ctx := context.Background()
	kubeconfigPath := filepath.Join(t.TempDir(), "gke-kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	originalInstall := installCloudpilotAIAgentComponent
	defer func() {
		installCloudpilotAIAgentComponent = originalInstall
	}()

	var gotDisableWorkloadUploading bool
	installCloudpilotAIAgentComponent = func(
		_ context.Context,
		_ cloudpilotaiclient.Interface,
		provider, clusterName, kubeconfig string,
		disableWorkloadUploading bool,
		_ map[string]string,
	) error {
		if provider != api.CloudProviderGCP {
			t.Fatalf("provider = %q, want %q", provider, api.CloudProviderGCP)
		}
		if clusterName != "demo-gke" {
			t.Fatalf("clusterName = %q, want demo-gke", clusterName)
		}
		if kubeconfig != kubeconfigPath {
			t.Fatalf("kubeconfig = %q, want %q", kubeconfig, kubeconfigPath)
		}
		gotDisableWorkloadUploading = disableWorkloadUploading
		return nil
	}

	data := ClusterModel{
		ClusterID:                types.StringValue("cluster-1"),
		ClusterName:              types.StringValue("demo-gke"),
		Region:                   types.StringValue("us-central1"),
		ClusterUID:               types.StringValue("kube-system-uid-123"),
		Kubeconfig:               types.StringValue(kubeconfigPath),
		DisableWorkloadUploading: types.BoolValue(true),
	}

	cluster := &Cluster{client: &fakeAgentInstallClusterClient{}}
	if err := cluster.ensureAgentInstalled(ctx, &data, "cluster-1"); err != nil {
		t.Fatalf("ensureAgentInstalled() error = %v", err)
	}
	if !gotDisableWorkloadUploading {
		t.Fatal("disableWorkloadUploading = false, want true")
	}
}

func TestSyncConfigurationAppliesRebalanceNodeClassAndNodePool(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{
		rebalanceConfig: &api.RebalanceConfig{},
	}
	cluster := &Cluster{client: client}

	data := ClusterModel{
		ClusterID:       types.StringValue("cluster-1"),
		EnableRebalance: types.BoolValue(true),
		NodeClasses: customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{
			func() api.GCENodeClassModel {
				model := testNodeClassModel(ctx, "default")
				model.EnableImageAccelerator = types.BoolValue(true)
				return model
			}(),
		}),
		NodePools: customfield.NewObjectListMust(ctx, []api.GCENodePoolModel{
			func() api.GCENodePoolModel {
				model := testNodePoolModel("general", "default", nil, "0s")
				model.EnableImageAccelerator = types.BoolValue(true)
				return model
			}(),
		}),
	}

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", nil, nil); err != nil {
		t.Fatalf("syncConfiguration() error = %v", err)
	}
	if len(client.rebalanceUpdates) != 1 || client.rebalanceUpdates[0] == nil || !client.rebalanceUpdates[0].Enable {
		t.Fatalf("rebalance updates = %#v, want one enable call", client.rebalanceUpdates)
	}
	if len(client.appliedNodeClasses) != 1 {
		t.Fatalf("ApplyNodeClass calls = %d, want 1", len(client.appliedNodeClasses))
	}
	if client.appliedNodeClasses[0].GCENodeClass == nil || client.appliedNodeClasses[0].GCENodeClass.Name != "default" {
		t.Fatalf("applied nodeclass = %#v, want GCE nodeclass default", client.appliedNodeClasses[0].GCENodeClass)
	}
	if !client.appliedNodeClasses[0].GCENodeClass.EnableImageAccelerator {
		t.Fatalf("applied nodeclass enable_image_accelerator = false, want true")
	}
	if len(client.appliedNodePools) != 1 {
		t.Fatalf("ApplyNodePool calls = %d, want 1", len(client.appliedNodePools))
	}
	if client.appliedNodePools[0].GCENodePool == nil || client.appliedNodePools[0].GCENodePool.Name != "general" {
		t.Fatalf("applied nodepool = %#v, want GCE nodepool general", client.appliedNodePools[0].GCENodePool)
	}
	if !client.appliedNodePools[0].GCENodePool.EnableImageAccelerator {
		t.Fatalf("applied nodepool enable_image_accelerator = false, want true")
	}
	wantOperations := []string{
		"apply-nodeclass:default",
		"apply-nodepool:general",
		"rebalance:true",
	}
	if fmt.Sprint(client.operations) != fmt.Sprint(wantOperations) {
		t.Fatalf("operations = %#v, want %#v", client.operations, wantOperations)
	}
}

func TestSyncConfigurationDeletesStaleNodePoolsBeforeNodeClasses(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{
		nodeClasses: api.RebalanceNodeClassList{
			GCENodeClasses: []api.GCENodeClass{
				testRemoteNodeClass("old-class"),
			},
		},
		nodePools: api.RebalanceNodePoolList{
			GCENodePools: []api.GCENodePool{
				testRemoteNodePool(t, ctx, "old-pool", "old-class", nil, "0s"),
			},
		},
	}
	cluster := &Cluster{client: client}
	data := ClusterModel{
		NodeClasses: customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{}),
		NodePools:   customfield.NewObjectListMust(ctx, []api.GCENodePoolModel{}),
	}
	previousNCNames := map[string]struct{}{"old-class": {}}
	previousNPNames := map[string]struct{}{"old-pool": {}}

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", previousNCNames, previousNPNames); err != nil {
		t.Fatalf("syncConfiguration() error = %v", err)
	}

	wantOperations := []string{
		"delete-nodepool:old-pool",
		"delete-nodeclass:old-class",
	}
	if fmt.Sprint(client.operations) != fmt.Sprint(wantOperations) {
		t.Fatalf("operations = %#v, want %#v", client.operations, wantOperations)
	}
}

func TestSyncConfigurationSkipsAgentOnlyRebalanceDisable(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{}
	cluster := &Cluster{client: client}
	data := ClusterModel{
		OnlyInstallAgent: types.BoolValue(true),
		EnableRebalance:  types.BoolValue(false),
		NodeClasses:      customfield.NullObjectList[api.GCENodeClassModel](ctx),
		NodePools:        customfield.NullObjectList[api.GCENodePoolModel](ctx),
	}

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", nil, nil); err != nil {
		t.Fatalf("syncConfiguration() error = %v", err)
	}
	if len(client.rebalanceUpdates) != 0 {
		t.Fatalf("rebalance updates = %#v, want none", client.rebalanceUpdates)
	}
}

func TestSyncConfigurationDisablesExplicitRebalanceWithoutNodeAutoscalerConfig(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{
		rebalanceConfig: &api.RebalanceConfig{
			Enable: true,
		},
	}
	cluster := &Cluster{client: client}
	data := ClusterModel{
		EnableRebalance: types.BoolValue(false),
		NodeClasses:     customfield.NullObjectList[api.GCENodeClassModel](ctx),
		NodePools:       customfield.NullObjectList[api.GCENodePoolModel](ctx),
	}

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", nil, nil); err != nil {
		t.Fatalf("syncConfiguration() error = %v", err)
	}
	if len(client.rebalanceUpdates) != 1 || client.rebalanceUpdates[0] == nil || client.rebalanceUpdates[0].Enable {
		t.Fatalf("rebalance updates = %#v, want one disable call", client.rebalanceUpdates)
	}
	wantOperations := []string{"rebalance:false"}
	if fmt.Sprint(client.operations) != fmt.Sprint(wantOperations) {
		t.Fatalf("operations = %#v, want %#v", client.operations, wantOperations)
	}
}

func TestSyncConfigurationTreatsMissingRebalanceDisableAsNoop(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{
		getRebalanceErr: cloudpilotaiclient.ErrNotFound,
	}
	cluster := &Cluster{client: client}
	data := ClusterModel{
		EnableRebalance: types.BoolValue(false),
		NodeClasses:     customfield.NullObjectList[api.GCENodeClassModel](ctx),
		NodePools:       customfield.NullObjectList[api.GCENodePoolModel](ctx),
	}

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", nil, nil); err != nil {
		t.Fatalf("syncConfiguration() error = %v", err)
	}
	if len(client.rebalanceUpdates) != 0 {
		t.Fatalf("rebalance updates = %#v, want none", client.rebalanceUpdates)
	}
	if len(client.operations) != 0 {
		t.Fatalf("operations = %#v, want none", client.operations)
	}
}

func TestDeleteClusterWithoutRestoreUninstallsWAAndDeletesNamespace(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{}
	cluster := &Cluster{client: client}
	stubGKEDeleteHooks(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "gke-kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	calls := make([]string, 0, 2)
	uninstallWorkloadAutoscaler = func(_ context.Context, _ cloudpilotaiclient.Interface, clusterUID, kubeconfigPath string) error {
		calls = append(calls, fmt.Sprintf("wa:%s:%s", clusterUID, kubeconfigPath))
		return nil
	}
	deleteCloudpilotNamespace = func(_ context.Context, kubeconfigPath string, _ map[string]string) error {
		calls = append(calls, fmt.Sprintf("namespace:%s", kubeconfigPath))
		return nil
	}
	uninstallCloudpilotAIAgentComponent = func(context.Context, cloudpilotaiclient.Interface, string, string, string, string, string, map[string]string) error {
		t.Fatal("full uninstall should not run when GKE nodes are not restored")
		return nil
	}

	data := ClusterModel{
		ClusterName:         types.StringValue("demo-gke"),
		Region:              types.StringValue("us-central1"),
		ClusterUID:          types.StringValue("kube-system-uid-123"),
		Kubeconfig:          types.StringValue(kubeconfigPath),
		SkipRestore:         types.BoolValue(false),
		RestoreNodeNumber:   types.Int64Value(0),
		RestoreDesiredSizes: types.MapNull(types.Int64Type),
	}

	if err := cluster.deleteCluster(ctx, &data, "cluster-1"); err != nil {
		t.Fatalf("deleteCluster() error = %v", err)
	}
	if len(client.rebalanceUpdates) != 1 || client.rebalanceUpdates[0] == nil || client.rebalanceUpdates[0].Enable {
		t.Fatalf("rebalance updates = %#v, want one disable call", client.rebalanceUpdates)
	}
	wantCalls := []string{
		fmt.Sprintf("wa:cluster-1:%s", kubeconfigPath),
		fmt.Sprintf("namespace:%s", kubeconfigPath),
	}
	if len(calls) != len(wantCalls) || calls[0] != wantCalls[0] || calls[1] != wantCalls[1] {
		t.Fatalf("cleanup calls = %#v, want %#v", calls, wantCalls)
	}
	if len(client.deletedClusters) != 1 || client.deletedClusters[0] != "cluster-1" {
		t.Fatalf("DeleteCluster calls = %#v, want [cluster-1]", client.deletedClusters)
	}
}

func TestDeleteClusterSkipsRestoreByDefaultAndPreservesTrackedResources(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{}
	cluster := &Cluster{client: client}
	stubGKEDeleteHooks(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "gke-kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	restoreCalled := false
	restoreCloudpilotAIRebalanceComponent = func(_ context.Context, _ cloudpilotaiclient.Interface, _, _, _ string, _ map[string]string, _ map[string]string) error {
		restoreCalled = true
		return nil
	}
	fullUninstallCalled := false
	uninstallCloudpilotAIAgentComponent = func(context.Context, cloudpilotaiclient.Interface, string, string, string, string, string, map[string]string) error {
		fullUninstallCalled = true
		return nil
	}

	data := ClusterModel{
		ClusterID:           types.StringValue("cluster-1"),
		ClusterName:         types.StringValue("demo-gke"),
		Region:              types.StringValue("us-central1"),
		ProjectID:           types.StringValue("test-project"),
		ClusterUID:          types.StringValue("kube-system-uid-123"),
		Kubeconfig:          types.StringValue(kubeconfigPath),
		RestoreDesiredSizes: types.MapNull(types.Int64Type),
		NodeClasses: customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{
			testNodeClassModel(ctx, "default"),
		}),
		NodePools: customfield.NewObjectListMust(ctx, []api.GCENodePoolModel{
			testNodePoolModel("general", "default", nil, "0s"),
		}),
	}

	if err := cluster.deleteCluster(ctx, &data, "cluster-1"); err != nil {
		t.Fatalf("deleteCluster() error = %v", err)
	}
	if restoreCalled {
		t.Fatal("restore should be skipped when no restore configuration is set")
	}
	if fullUninstallCalled {
		t.Fatal("full uninstall should be skipped when no restore configuration is set")
	}
	if len(client.rebalanceUpdates) != 1 || client.rebalanceUpdates[0] == nil || client.rebalanceUpdates[0].Enable {
		t.Fatalf("rebalance updates = %#v, want one disable call", client.rebalanceUpdates)
	}
	if len(client.deletedNodePools) != 0 {
		t.Fatalf("deleted nodepools = %#v, want none", client.deletedNodePools)
	}
	if len(client.deletedNodeClasses) != 0 {
		t.Fatalf("deleted nodeclasses = %#v, want none", client.deletedNodeClasses)
	}
}

func TestDeleteClusterInfersKubeconfigFromNodeClasses(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{
		nodeClasses: api.RebalanceNodeClassList{
			GCENodeClasses: []api.GCENodeClass{{
				Name: "cloudpilot",
				NodeClassSpec: &api.GCENodeClassSpec{
					NetworkConfig: &api.GCENetworkConfig{
						Subnetwork: "projects/test-project/regions/us-central1/subnetworks/default",
					},
				},
			}},
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

	var gotWAKubeconfig string
	var gotNamespaceKubeconfig string
	uninstallWorkloadAutoscaler = func(_ context.Context, _ cloudpilotaiclient.Interface, _, kubeconfig string) error {
		gotWAKubeconfig = kubeconfig
		return nil
	}
	deleteCloudpilotNamespace = func(_ context.Context, kubeconfig string, _ map[string]string) error {
		gotNamespaceKubeconfig = kubeconfig
		return nil
	}

	data := ClusterModel{
		ClusterName:         types.StringValue("demo-gke"),
		Region:              types.StringValue("us-central1"),
		ClusterUID:          types.StringValue("kube-system-uid-123"),
		Kubeconfig:          types.StringNull(),
		SkipRestore:         types.BoolValue(false),
		RestoreNodeNumber:   types.Int64Value(0),
		RestoreDesiredSizes: types.MapNull(types.Int64Type),
	}

	if err := cluster.deleteCluster(ctx, &data, "cluster-1"); err != nil {
		t.Fatalf("deleteCluster() error = %v", err)
	}

	wantSuffix := filepath.Clean("test-project_us-central1_demo-gke_kubeconfig")
	if filepath.Base(gotWAKubeconfig) != wantSuffix {
		t.Fatalf("workload autoscaler uninstall kubeconfig = %q, want basename %q", gotWAKubeconfig, wantSuffix)
	}
	if filepath.Base(gotNamespaceKubeconfig) != wantSuffix {
		t.Fatalf("namespace cleanup kubeconfig = %q, want basename %q", gotNamespaceKubeconfig, wantSuffix)
	}
	if data.ProjectID != types.StringValue("test-project") {
		t.Fatalf("project_id = %#v, want inferred test-project", data.ProjectID)
	}
}

func TestDeleteClusterPreservesManagedRemoteNodePoolsAndNodeClasses(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{
		nodePools: api.RebalanceNodePoolList{
			GCENodePools: []api.GCENodePool{
				testManagedRemoteNodePool(t, ctx, "cloudpilot-general", "cloudpilot"),
				testManagedRemoteNodePool(t, ctx, "cloudpilot-gpu", "cloudpilot"),
			},
		},
	}
	cluster := &Cluster{client: client}
	stubGKEDeleteHooks(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "gke-kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	data := ClusterModel{
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
	}

	if err := cluster.deleteCluster(ctx, &data, "cluster-1"); err != nil {
		t.Fatalf("deleteCluster() error = %v", err)
	}
	if len(client.deletedNodePools) != 0 {
		t.Fatalf("deleted nodepools = %#v, want none", client.deletedNodePools)
	}
	if len(client.deletedNodeClasses) != 0 {
		t.Fatalf("deleted nodeclasses = %#v, want none", client.deletedNodeClasses)
	}
}

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

	if err := cluster.deleteCluster(ctx, &data, "cluster-1"); err != nil {
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

	if err := cluster.deleteCluster(ctx, &data, "cluster-1"); err != nil {
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

	if err := cluster.fillMissingParameters(ctx, &data); err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}

	if data.ProjectID != types.StringValue("cloudpilot-ai-dev") {
		t.Fatalf("project_id = %#v, want cloudpilot-ai-dev", data.ProjectID)
	}
	if data.ClusterUID != types.StringValue("kube-system-uid-123") {
		t.Fatalf("cluster_uid = %#v, want kube-system-uid-123", data.ClusterUID)
	}
	if filepath.Base(data.Kubeconfig.ValueString()) != "cloudpilot-ai-dev_us-central1_demo-gke_kubeconfig" {
		t.Fatalf("kubeconfig = %q, want inferred path", data.Kubeconfig.ValueString())
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

	if err := cluster.fillMissingParameters(ctx, &data); err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}

	if filepath.Base(data.Kubeconfig.ValueString()) != "cloudpilot-ai-dev_us-central1-f_demo-gke_kubeconfig" {
		t.Fatalf("kubeconfig = %q, want zonal inferred path", data.Kubeconfig.ValueString())
	}
}

func TestFillMissingParametersKeepsProjectIDUnmanagedWithConfiguredKubeconfig(t *testing.T) {
	ctx := context.Background()
	cluster := &Cluster{client: &fakeClusterClient{}}
	kubeconfigPath := filepath.Join(t.TempDir(), "gke-kubeconfig")
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

	if err := cluster.fillMissingParameters(ctx, &data); err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}
	if !data.ProjectID.IsNull() {
		t.Fatalf("project_id = %#v, want null unmanaged value", data.ProjectID)
	}
	if data.Kubeconfig.ValueString() != kubeconfigPath && filepath.Base(data.Kubeconfig.ValueString()) != filepath.Base(kubeconfigPath) {
		t.Fatalf("kubeconfig = %#v, want configured kubeconfig", data.Kubeconfig)
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

func TestClusterSettingObjectPreservingStateLeavesUnmanagedRemoteValuesNull(t *testing.T) {
	ctx := context.Background()
	enableNodeRepair := false
	enableDiskMonitor := false
	discount := 0.2
	preRunCommand := "remote-pre"
	postRunCommand := "remote-post"
	current := customfield.NewObjectMust(ctx, &ClusterSettingModel{
		EnableNodeRepair:  types.BoolNull(),
		EnableDiskMonitor: types.BoolValue(true),
		Discount:          types.Float64Null(),
		PreRunCommand:     types.StringNull(),
		PostRunCommand:    types.StringValue("configured-post"),
	})

	got, err := clusterSettingObjectPreservingState(ctx, current, &api.ClusterSetting{
		EnableNodeRepair:  &enableNodeRepair,
		EnableDiskMonitor: &enableDiskMonitor,
		Discount:          &discount,
		PreRunCommand:     &preRunCommand,
		PostRunCommand:    &postRunCommand,
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
	if !value.EnableNodeRepair.IsNull() {
		t.Fatalf("enable_node_repair = %#v, want null", value.EnableNodeRepair)
	}
	if value.EnableDiskMonitor != types.BoolValue(false) {
		t.Fatalf("enable_disk_monitor = %#v, want managed remote false", value.EnableDiskMonitor)
	}
	if !value.Discount.IsNull() {
		t.Fatalf("discount = %#v, want null", value.Discount)
	}
	if !value.PreRunCommand.IsNull() {
		t.Fatalf("pre_run_command = %#v, want null", value.PreRunCommand)
	}
	if value.PostRunCommand != types.StringValue("remote-post") {
		t.Fatalf("post_run_command = %#v, want managed remote value", value.PostRunCommand)
	}
}

func TestPreserveNodePoolStateRepresentationDropsUnmanagedRemoteLabels(t *testing.T) {
	ctx := context.Background()
	state := api.GCENodePoolModel{
		Name:   types.StringValue("cloudpilot-general"),
		Labels: customfield.NewMapMust[types.String](ctx, map[string]types.String{"codex-test": types.StringValue("update-1")}),
	}
	remote := api.GCENodePoolModel{
		Name: types.StringValue("cloudpilot-general"),
		Labels: customfield.NewMapMust[types.String](ctx, map[string]types.String{
			"codex-test":                 types.StringValue("update-1"),
			"node.cloudpilot.ai/managed": types.StringValue("true"),
		}),
	}

	got := preserveNodePoolStateRepresentation(ctx, remote, state)
	values, diags := got.Labels.Value(ctx)
	if diags.HasError() {
		t.Fatalf("labels diagnostics = %v", diags)
	}
	if len(values) != 1 {
		t.Fatalf("labels = %#v, want only the user-managed label", values)
	}
	if _, ok := values["node.cloudpilot.ai/managed"]; ok {
		t.Fatalf("labels should not keep unmanaged remote default: %#v", values)
	}
}

func TestPreserveNodeClassStateRepresentationLeavesUnmanagedNetworkConfigFieldsNull(t *testing.T) {
	ctx := context.Background()
	state := api.GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		NetworkConfig: customfield.NewObjectMust(ctx, &api.GCENetworkConfigModel{
			EnablePrivateNodes:          types.BoolNull(),
			Subnetwork:                  types.StringValue("projects/test/regions/us-central1/subnetworks/default"),
			AdditionalNetworkInterfaces: customfield.NullObjectList[api.GCEAdditionalNetworkInterfaceModel](ctx),
		}),
	}
	remote := api.GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		NetworkConfig: customfield.NewObjectMust(ctx, &api.GCENetworkConfigModel{
			EnablePrivateNodes:          types.BoolValue(false),
			Subnetwork:                  types.StringValue("projects/test/regions/us-central1/subnetworks/default"),
			AdditionalNetworkInterfaces: customfield.NullObjectList[api.GCEAdditionalNetworkInterfaceModel](ctx),
		}),
	}

	got := preserveNodeClassStateRepresentation(ctx, remote, state)
	networkConfig, diags := got.NetworkConfig.Value(ctx)
	if diags.HasError() {
		t.Fatalf("network_config diagnostics = %v", diags)
	}
	if networkConfig == nil {
		t.Fatal("network_config is nil")
	}
	if !networkConfig.EnablePrivateNodes.IsNull() {
		t.Fatalf("enable_private_nodes = %#v, want null for unmanaged field", networkConfig.EnablePrivateNodes)
	}
	if networkConfig.Subnetwork.ValueString() != "projects/test/regions/us-central1/subnetworks/default" {
		t.Fatalf("subnetwork = %#v, want configured subnetwork", networkConfig.Subnetwork)
	}
}

func TestPreserveNodeClassStateRepresentationKeepsManagedNetworkConfigFields(t *testing.T) {
	ctx := context.Background()
	state := api.GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		NetworkConfig: customfield.NewObjectMust(ctx, &api.GCENetworkConfigModel{
			EnablePrivateNodes:          types.BoolValue(false),
			Subnetwork:                  types.StringValue("projects/test/regions/us-central1/subnetworks/default"),
			AdditionalNetworkInterfaces: customfield.NullObjectList[api.GCEAdditionalNetworkInterfaceModel](ctx),
		}),
	}
	remote := api.GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		NetworkConfig: customfield.NewObjectMust(ctx, &api.GCENetworkConfigModel{
			EnablePrivateNodes:          types.BoolValue(true),
			Subnetwork:                  types.StringValue("projects/test/regions/us-central1/subnetworks/updated"),
			AdditionalNetworkInterfaces: customfield.NullObjectList[api.GCEAdditionalNetworkInterfaceModel](ctx),
		}),
	}

	got := preserveNodeClassStateRepresentation(ctx, remote, state)
	networkConfig, diags := got.NetworkConfig.Value(ctx)
	if diags.HasError() {
		t.Fatalf("network_config diagnostics = %v", diags)
	}
	if networkConfig == nil {
		t.Fatal("network_config is nil")
	}
	if !networkConfig.EnablePrivateNodes.ValueBool() {
		t.Fatalf("enable_private_nodes = %#v, want remote managed value true", networkConfig.EnablePrivateNodes)
	}
	if networkConfig.Subnetwork.ValueString() != "projects/test/regions/us-central1/subnetworks/updated" {
		t.Fatalf("subnetwork = %#v, want remote managed value", networkConfig.Subnetwork)
	}
}

func TestPreserveNodeClassStateRepresentationLeavesUnmanagedNestedFieldsNull(t *testing.T) {
	ctx := context.Background()
	state := api.GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		Disks: customfield.NewObjectListMust(ctx, []api.GCEDiskModel{{
			SizeGiB:  types.Int64Null(),
			Category: types.StringValue("pd-balanced"),
			Boot:     types.BoolNull(),
		}}),
		ImageSelectorTerms: customfield.NewObjectListMust(ctx, []api.GCEImageSelectorTermModel{{
			ID:      types.StringNull(),
			Family:  types.StringValue("ContainerOptimizedOS"),
			Channel: types.StringNull(),
			Version: types.StringValue("1.2.3.4"),
		}}),
		KubeletConfiguration: customfield.NewObjectMust(ctx, &api.GCEKubeletConfigurationModel{
			KubeReserved: customfield.NewMapMust[types.String](ctx, map[string]types.String{
				"cpu": types.StringValue("100m"),
			}),
			SystemReserved: customfield.NullMap[types.String](ctx),
			EvictionHard:   customfield.NewMapMust[types.String](ctx, map[string]types.String{}),
			EvictionSoft:   customfield.NullMap[types.String](ctx),
		}),
		NetworkConfig: customfield.NewObjectMust(ctx, &api.GCENetworkConfigModel{
			AdditionalNetworkInterfaces: customfield.NewObjectListMust(ctx, []api.GCEAdditionalNetworkInterfaceModel{{
				Network:    types.StringNull(),
				Subnetwork: types.StringValue("projects/test/regions/us-central1/subnetworks/secondary"),
			}}),
		}),
	}
	remote := api.GCENodeClassModel{
		Name: types.StringValue("cloudpilot"),
		Disks: customfield.NewObjectListMust(ctx, []api.GCEDiskModel{{
			SizeGiB:  types.Int64Value(100),
			Category: types.StringValue("pd-ssd"),
			Boot:     types.BoolValue(false),
		}}),
		ImageSelectorTerms: customfield.NewObjectListMust(ctx, []api.GCEImageSelectorTermModel{{
			ID:      types.StringValue("projects/test/global/images/cos"),
			Family:  types.StringValue("ContainerOptimizedOS"),
			Channel: types.StringValue("regular"),
			Version: types.StringValue("2.3.4.5"),
		}}),
		KubeletConfiguration: customfield.NewObjectMust(ctx, &api.GCEKubeletConfigurationModel{
			KubeReserved: customfield.NewMapMust[types.String](ctx, map[string]types.String{
				"cpu":    types.StringValue("200m"),
				"memory": types.StringValue("512Mi"),
			}),
			SystemReserved: customfield.NewMapMust[types.String](ctx, map[string]types.String{
				"cpu": types.StringValue("100m"),
			}),
			EvictionHard: customfield.NewMapMust[types.String](ctx, map[string]types.String{
				"memory.available": types.StringValue("100Mi"),
			}),
			EvictionSoft: customfield.NewMapMust[types.String](ctx, map[string]types.String{
				"memory.available": types.StringValue("200Mi"),
			}),
		}),
		NetworkConfig: customfield.NewObjectMust(ctx, &api.GCENetworkConfigModel{
			AdditionalNetworkInterfaces: customfield.NewObjectListMust(ctx, []api.GCEAdditionalNetworkInterfaceModel{{
				Network:    types.StringValue("projects/test/global/networks/default"),
				Subnetwork: types.StringValue("projects/test/regions/us-central1/subnetworks/updated"),
			}}),
		}),
	}

	got := preserveNodeClassStateRepresentation(ctx, remote, state)
	disks, diskDiags := got.Disks.AsStructSliceT(ctx)
	if diskDiags.HasError() {
		t.Fatalf("disks diagnostics = %v", diskDiags)
	}
	if len(disks) != 1 {
		t.Fatalf("disks length = %d, want 1", len(disks))
	}
	if !disks[0].SizeGiB.IsNull() {
		t.Fatalf("disks[0].size_gib = %#v, want null", disks[0].SizeGiB)
	}
	if disks[0].Category != types.StringValue("pd-ssd") {
		t.Fatalf("disks[0].category = %#v, want remote managed value", disks[0].Category)
	}
	if !disks[0].Boot.IsNull() {
		t.Fatalf("disks[0].boot = %#v, want null", disks[0].Boot)
	}

	terms, termDiags := got.ImageSelectorTerms.AsStructSliceT(ctx)
	if termDiags.HasError() {
		t.Fatalf("image_selector_terms diagnostics = %v", termDiags)
	}
	if !terms[0].ID.IsNull() {
		t.Fatalf("image_selector_terms[0].id = %#v, want null", terms[0].ID)
	}
	if terms[0].Family != types.StringValue("ContainerOptimizedOS") {
		t.Fatalf("image_selector_terms[0].family = %#v, want managed remote value", terms[0].Family)
	}
	if !terms[0].Channel.IsNull() {
		t.Fatalf("image_selector_terms[0].channel = %#v, want null", terms[0].Channel)
	}
	if terms[0].Version != types.StringValue("2.3.4.5") {
		t.Fatalf("image_selector_terms[0].version = %#v, want managed remote value", terms[0].Version)
	}

	kubeletConfig, kubeletDiags := got.KubeletConfiguration.Value(ctx)
	if kubeletDiags.HasError() {
		t.Fatalf("kubelet_configuration diagnostics = %v", kubeletDiags)
	}
	kubeReserved, kubeReservedDiags := kubeletConfig.KubeReserved.Value(ctx)
	if kubeReservedDiags.HasError() {
		t.Fatalf("kube_reserved diagnostics = %v", kubeReservedDiags)
	}
	if len(kubeReserved) != 1 || kubeReserved["cpu"] != types.StringValue("200m") {
		t.Fatalf("kube_reserved = %#v, want only managed cpu from remote", kubeReserved)
	}
	if !kubeletConfig.SystemReserved.IsNull() {
		t.Fatalf("system_reserved = %#v, want null", kubeletConfig.SystemReserved)
	}
	evictionHard, evictionHardDiags := kubeletConfig.EvictionHard.Value(ctx)
	if evictionHardDiags.HasError() {
		t.Fatalf("eviction_hard diagnostics = %v", evictionHardDiags)
	}
	if len(evictionHard) != 0 {
		t.Fatalf("eviction_hard = %#v, want explicit empty map", evictionHard)
	}
	if !kubeletConfig.EvictionSoft.IsNull() {
		t.Fatalf("eviction_soft = %#v, want null", kubeletConfig.EvictionSoft)
	}

	networkConfig, networkConfigDiags := got.NetworkConfig.Value(ctx)
	if networkConfigDiags.HasError() {
		t.Fatalf("network_config diagnostics = %v", networkConfigDiags)
	}
	nics, nicDiags := networkConfig.AdditionalNetworkInterfaces.AsStructSliceT(ctx)
	if nicDiags.HasError() {
		t.Fatalf("additional_network_interfaces diagnostics = %v", nicDiags)
	}
	if !nics[0].Network.IsNull() {
		t.Fatalf("additional_network_interfaces[0].network = %#v, want null", nics[0].Network)
	}
	if nics[0].Subnetwork != types.StringValue("projects/test/regions/us-central1/subnetworks/updated") {
		t.Fatalf("additional_network_interfaces[0].subnetwork = %#v, want managed remote value", nics[0].Subnetwork)
	}
}

func TestPreserveNodePoolStateRepresentationLeavesUnmanagedTaintValueNull(t *testing.T) {
	ctx := context.Background()
	state := api.GCENodePoolModel{
		Name: types.StringValue("cloudpilot-general"),
		Taints: customfield.NewObjectListMust(ctx, []api.TaintModel{{
			Key:    types.StringValue("dedicated"),
			Value:  types.StringNull(),
			Effect: types.StringValue("NoSchedule"),
		}}),
	}
	remote := api.GCENodePoolModel{
		Name: types.StringValue("cloudpilot-general"),
		Taints: customfield.NewObjectListMust(ctx, []api.TaintModel{{
			Key:    types.StringValue("dedicated"),
			Value:  types.StringValue("remote-default"),
			Effect: types.StringValue("NoSchedule"),
		}}),
	}

	got := preserveNodePoolStateRepresentation(ctx, remote, state)
	taints, diags := got.Taints.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("taints diagnostics = %v", diags)
	}
	if len(taints) != 1 {
		t.Fatalf("taints length = %d, want 1", len(taints))
	}
	if !taints[0].Value.IsNull() {
		t.Fatalf("taints[0].value = %#v, want null", taints[0].Value)
	}
}

func testNodeClassModel(ctx context.Context, name string) api.GCENodeClassModel {
	return api.GCENodeClassModel{
		Name: types.StringValue(name),
		ImageSelectorTerms: customfield.NewObjectListMust(ctx, []api.GCEImageSelectorTermModel{
			{
				ID: types.StringValue("projects/demo/global/images/container-optimized-os"),
			},
		}),
	}
}

func testNodePoolModel(name, nodeClass string, instanceFamily *[]types.String, disruptionDelay string) api.GCENodePoolModel {
	return api.GCENodePoolModel{
		Name:                types.StringValue(name),
		NodeClass:           types.StringValue(nodeClass),
		InstanceFamily:      instanceFamily,
		NodeDisruptionDelay: types.StringValue(disruptionDelay),
	}
}

func testRemoteNodeClass(name string) api.GCENodeClass {
	return api.GCENodeClass{
		Name:                   name,
		EnableImageAccelerator: true,
		NodeClassSpec: &api.GCENodeClassSpec{
			ImageSelectorTerms: []api.GCEImageSelectorTerm{
				{
					ID: "projects/demo/global/images/container-optimized-os",
				},
			},
		},
	}
}

func testRemoteNodePool(t *testing.T, ctx context.Context, name, nodeClass string, instanceFamily *[]types.String, disruptionDelay string) api.GCENodePool {
	t.Helper()

	model := testNodePoolModel(name, nodeClass, instanceFamily, disruptionDelay)
	model.EnableImageAccelerator = types.BoolValue(true)
	nodePool, err := model.ToGCENodePool(ctx, api.GCENodePool{Name: name})
	if err != nil {
		t.Fatalf("ToGCENodePool() error = %v", err)
	}
	return *nodePool
}

func testManagedRemoteNodePool(t *testing.T, ctx context.Context, name, nodeClass string) api.GCENodePool {
	t.Helper()

	nodePool := testRemoteNodePool(t, ctx, name, nodeClass, nil, "60m")
	if nodePool.NodePoolSpec == nil {
		t.Fatalf("testRemoteNodePool() returned nil NodePoolSpec")
	}
	if nodePool.NodePoolSpec.Template.ObjectMeta.Labels == nil {
		nodePool.NodePoolSpec.Template.ObjectMeta.Labels = map[string]string{}
	}
	nodePool.NodePoolSpec.Template.ObjectMeta.Labels["node.cloudpilot.ai/managed"] = "true"
	return nodePool
}
