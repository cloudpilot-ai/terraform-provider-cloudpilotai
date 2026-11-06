package gke

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/gkeaccess"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
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
	scheduledRebalances api.ScheduledRebalancePolicyList
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

func (f *fakeClusterClient) ListScheduledRebalances(string) (api.ScheduledRebalancePolicyList, error) {
	return f.scheduledRebalances, nil
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
		"workload_templates",
	} {
		if _, ok := s.Attributes[name]; ok {
			t.Fatalf("gke schema should not expose %s", name)
		}
	}

	if _, ok := s.Attributes["workloads"]; !ok {
		t.Fatal("gke schema missing workloads")
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

	kubeconfigAttr, ok := s.Attributes["kubeconfig"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("kubeconfig attribute has unexpected type %T", s.Attributes["kubeconfig"])
	}
	if !kubeconfigAttr.IsOptional() || kubeconfigAttr.IsComputed() {
		t.Fatalf("kubeconfig must be optional-only, got optional=%v computed=%v", kubeconfigAttr.IsOptional(), kubeconfigAttr.IsComputed())
	}
	if len(kubeconfigAttr.StringPlanModifiers()) != 0 {
		t.Fatal("kubeconfig must not preserve an execution-local path from prior state")
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

func TestNodeDisruptionLimitIsDeprecated(t *testing.T) {
	nodepools := Schema(context.Background()).Attributes["nodepools"].(schema.ListNestedAttribute)
	limit := nodepools.NestedObject.Attributes["node_disruption_limit"].(schema.StringAttribute)
	if limit.DeprecationMessage == "" {
		t.Fatalf("nodepools.node_disruption_limit should be deprecated")
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
	if err := cluster.ensureAgentInstalled(ctx, &data, "cluster-1", kubeconfigPath); err != nil {
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

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", nil, nil, nil); err != nil {
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

func TestSyncConfigurationPrevalidatesScheduledRebalancesBeforeWrites(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{}
	cluster := &Cluster{client: client}
	data := ClusterModel{
		NodeClasses: customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{testNodeClassModel(ctx, "default")}),
		NodePools:   customfield.NullObjectList[api.GCENodePoolModel](ctx),
		ScheduledRebalances: customfield.NewObjectListMust(ctx, []api.ScheduledRebalancePolicyModel{
			{Name: types.StringValue("valid"), Cron: types.StringValue("0 2 * * *")},
			{Name: types.StringValue("invalid-cron"), Cron: types.StringValue("61 2 * * *")},
		}),
	}

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", nil, nil, nil); err == nil {
		t.Fatal("syncConfiguration() error = nil, want invalid cron error")
	}
	if len(client.operations) != 0 {
		t.Fatalf("invalid scheduled rebalance configuration made writes: %#v", client.operations)
	}
}

func TestSyncConfigurationPrevalidatesNodePoolsBeforeWrites(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{rebalanceConfig: &api.RebalanceConfig{Enable: true}}
	cluster := &Cluster{client: client}
	data := ClusterModel{
		EnableRebalance: types.BoolValue(false),
		NodeClasses:     customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{testNodeClassModel(ctx, "default")}),
		NodePools: customfield.NewObjectListMust(ctx, []api.GCENodePoolModel{{
			Name:      types.StringValue("invalid"),
			NodeClass: types.StringValue("default"),
			NodeDisruptionBudgets: customfield.NewObjectListMust(ctx, []api.DisruptionBudgetModel{{
				Nodes: types.StringValue("10%"), Schedule: types.StringValue("daily"), Duration: types.StringValue("1h"),
			}}),
		}}),
		ScheduledRebalances: customfield.NullObjectList[api.ScheduledRebalancePolicyModel](ctx),
	}

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", nil, nil, nil); err == nil {
		t.Fatal("syncConfiguration() error = nil, want invalid disruption schedule error")
	}
	if len(client.operations) != 0 {
		t.Fatalf("invalid nodepool configuration made writes: %#v", client.operations)
	}
}

func TestSyncConfigurationPrevalidatesNodeClassesBeforeWrites(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterClient{rebalanceConfig: &api.RebalanceConfig{Enable: true}}
	cluster := &Cluster{client: client}
	invalidNodeClass := testNodeClassModel(ctx, "invalid")
	invalidNodeClass.EnableLocalSSDEphemeralStorage = types.BoolValue(false)
	invalidNodeClass.EphemeralStorageLocalSSD = customfield.NewObjectMust(ctx, &api.GCEEphemeralStorageLocalSSDModel{
		Count: types.Int32Value(2),
	})
	data := ClusterModel{
		EnableRebalance:     types.BoolValue(false),
		NodeClasses:         customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{testNodeClassModel(ctx, "valid"), invalidNodeClass}),
		NodePools:           customfield.NullObjectList[api.GCENodePoolModel](ctx),
		ScheduledRebalances: customfield.NullObjectList[api.ScheduledRebalancePolicyModel](ctx),
	}

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", nil, nil, nil); err == nil {
		t.Fatal("syncConfiguration() error = nil, want invalid nodeclass error")
	}
	if len(client.operations) != 0 {
		t.Fatalf("invalid nodeclass configuration made writes: %#v", client.operations)
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

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", previousNCNames, previousNPNames, nil); err != nil {
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

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", nil, nil, nil); err != nil {
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

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", nil, nil, nil); err != nil {
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

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", nil, nil, nil); err != nil {
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
		ClusterID:           types.StringValue("cluster-1"),
		ClusterName:         types.StringValue("demo-gke"),
		Region:              types.StringValue("us-central1"),
		ClusterUID:          types.StringValue("kube-system-uid-123"),
		Kubeconfig:          types.StringValue(kubeconfigPath),
		SkipRestore:         types.BoolValue(false),
		RestoreNodeNumber:   types.Int64Value(0),
		RestoreDesiredSizes: types.MapNull(types.Int64Type),
	}

	kubeconfigPath, err := cluster.fillMissingParameters(ctx, &data, false)
	if err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}
	if err := cluster.deleteCluster(ctx, &data, "cluster-1", kubeconfigPath); err != nil {
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

	kubeconfigPath, err := cluster.fillMissingParameters(ctx, &data, false)
	if err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}
	if err := cluster.deleteCluster(ctx, &data, "cluster-1", kubeconfigPath); err != nil {
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
		ClusterID:           types.StringValue("cluster-1"),
		ClusterName:         types.StringValue("demo-gke"),
		Region:              types.StringValue("us-central1"),
		ClusterUID:          types.StringValue("kube-system-uid-123"),
		Kubeconfig:          types.StringNull(),
		SkipRestore:         types.BoolValue(false),
		RestoreNodeNumber:   types.Int64Value(0),
		RestoreDesiredSizes: types.MapNull(types.Int64Type),
	}

	kubeconfigPath, err := cluster.fillMissingParameters(ctx, &data, false)
	if err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}
	if err := cluster.deleteCluster(ctx, &data, "cluster-1", kubeconfigPath); err != nil {
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

	if err := cluster.deleteCluster(ctx, &data, "cluster-1", kubeconfigPath); err != nil {
		t.Fatalf("deleteCluster() error = %v", err)
	}
	if len(client.deletedNodePools) != 0 {
		t.Fatalf("deleted nodepools = %#v, want none", client.deletedNodePools)
	}
	if len(client.deletedNodeClasses) != 0 {
		t.Fatalf("deleted nodeclasses = %#v, want none", client.deletedNodeClasses)
	}
}

func (f *fakeClusterClient) ListNodePoolsContext(_ context.Context, clusterID string) (api.RebalanceNodePoolList, error) {
	return f.ListNodePools(clusterID)
}

func (f *fakeClusterClient) DeleteNodePoolContext(_ context.Context, clusterID, name string) error {
	return f.DeleteNodePool(clusterID, name)
}
