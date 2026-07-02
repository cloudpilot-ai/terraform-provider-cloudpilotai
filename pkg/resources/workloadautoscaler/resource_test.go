package workloadautoscaler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/gkeaccess"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type fakePostWriteStateHydratorClient struct {
	waConfig               *api.WAConfiguration
	recommendationPolicies []api.RecommendationPolicyResource
	autoscalingPolicies    []api.AutoscalingPolicyResource
}

func (f *fakePostWriteStateHydratorClient) GetWAConfiguration(string) (*api.WAConfiguration, error) {
	return f.waConfig, nil
}

func (f *fakePostWriteStateHydratorClient) ListRecommendationPolicies(string) ([]api.RecommendationPolicyResource, error) {
	return f.recommendationPolicies, nil
}

func (f *fakePostWriteStateHydratorClient) ListAutoscalingPolicies(string) ([]api.AutoscalingPolicyResource, error) {
	return f.autoscalingPolicies, nil
}

type fakeWorkloadAutoscalerClient struct {
	cloudpilotaiclient.Interface
	summary           *api.ClusterCostsSummary
	nodeClasses       api.RebalanceNodeClassList
	deletedAPs        []string
	deletedRPs        []string
	updatedWAConfigs  []*api.WAConfiguration
	deleteAPErrs      map[string]error
	deleteRPErrs      map[string]error
	deleteRPSequences map[string][]error
}

func (f *fakeWorkloadAutoscalerClient) GetCluster(string) (*api.ClusterCostsSummary, error) {
	if f.summary != nil {
		return f.summary, nil
	}
	return &api.ClusterCostsSummary{}, nil
}

func (f *fakeWorkloadAutoscalerClient) ListNodeClasses(string) (api.RebalanceNodeClassList, error) {
	return f.nodeClasses, nil
}

func (f *fakeWorkloadAutoscalerClient) DeleteAutoscalingPolicy(_ string, name string) error {
	if err := f.deleteAPErrs[name]; err != nil {
		return err
	}
	f.deletedAPs = append(f.deletedAPs, name)
	return nil
}

func (f *fakeWorkloadAutoscalerClient) DeleteRecommendationPolicy(_ string, name string) error {
	if seq := f.deleteRPSequences[name]; len(seq) > 0 {
		err := seq[0]
		f.deleteRPSequences[name] = seq[1:]
		if err != nil {
			return err
		}
	}
	if err := f.deleteRPErrs[name]; err != nil {
		return err
	}
	f.deletedRPs = append(f.deletedRPs, name)
	return nil
}

func (f *fakeWorkloadAutoscalerClient) UpdateWAConfiguration(_ string, cfg *api.WAConfiguration) error {
	f.updatedWAConfigs = append(f.updatedWAConfigs, cfg)
	return nil
}

func autoscalingPolicyWithServerDefaults(name, recommendationPolicyName string) api.AutoscalingPolicyResource {
	removeLimit := true
	autoHeadroom := "2"

	return api.AutoscalingPolicyResource{
		Name:   name,
		Enable: true,
		Spec: api.AutoscalingPolicySpec{
			RecommendationPolicyName:   recommendationPolicyName,
			Priority:                   0,
			DisableRuntimeOptimization: false,
			UpdateResources:            []string{"cpu", "memory"},
			DriftThresholds: map[string]string{
				"cpu":    "5%",
				"memory": "5%",
			},
			OnPolicyRemoval: "off",
			TargetRefs: []api.TypedObjectReference{{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			}},
			UpdateSchedule: []api.UpdateScheduleItem{{
				Name: "default",
				Mode: "off",
			}},
			LimitPolicies: map[string]api.ResourceLimitPolicy{
				"cpu": {
					RemoveLimit: &removeLimit,
				},
				"memory": {
					AutoHeadroom: &autoHeadroom,
				},
			},
			InPlaceFallback: &api.InPlaceFallback{
				ReasonPolicies: map[string]string{},
			},
		},
	}
}

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

func TestTopLevelOptionalConfigFieldsHaveNoSchemaDefaults(t *testing.T) {
	s := Schema(context.Background())

	for _, name := range []string{
		"storage_class",
		"enable_node_agent",
		"enable_new_workloads_proactive_update",
		"limiter_quota_per_window",
		"limiter_burst",
		"limiter_window_seconds",
		"enable_preempted_pod_gc",
		"preempted_pod_gc_ttl",
		"enable_initial_optimization_data_window_check",
	} {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Fatalf("schema missing %s", name)
		}
		switch typed := attr.(type) {
		case schema.StringAttribute:
			if typed.StringDefaultValue() != nil {
				t.Fatalf("%s should not have a schema default", name)
			}
			if typed.IsComputed() {
				t.Fatalf("%s should not be computed", name)
			}
		case schema.BoolAttribute:
			if typed.BoolDefaultValue() != nil {
				t.Fatalf("%s should not have a schema default", name)
			}
			if typed.IsComputed() {
				t.Fatalf("%s should not be computed", name)
			}
		case schema.Int64Attribute:
			if typed.Int64DefaultValue() != nil {
				t.Fatalf("%s should not have a schema default", name)
			}
			if typed.IsComputed() {
				t.Fatalf("%s should not be computed", name)
			}
		default:
			t.Fatalf("%s has unexpected type %T", name, attr)
		}
	}
}

func TestHydratePostWriteStateRefreshesBackendManagedFields(t *testing.T) {
	ctx := context.Background()
	quota := 7
	proactive := true

	data := WorkloadAutoscalerModel{
		ClusterID:                         types.StringValue("cluster-1"),
		Kubeconfig:                        types.StringValue("/tmp/kubeconfig"),
		StorageClass:                      types.StringValue(""),
		EnableNodeAgent:                   types.BoolValue(true),
		EnableNewWorkloadsProactiveUpdate: types.BoolUnknown(),
		LimiterQuotaPerWindow:             types.Int64Unknown(),
		RecommendationPolicies: customfield.NewObjectListMust(ctx, []api.RecommendationPolicyModel{{
			Name:                types.StringValue("java"),
			HistoryWindowCPU:    types.StringValue("24h"),
			HistoryWindowMemory: types.StringValue("24h"),
			EvaluationPeriod:    types.StringValue("1h"),
		}}),
		AutoscalingPolicies: customfield.NewObjectListMust(ctx, []api.AutoscalingPolicyModel{{
			Name:                     types.StringValue("default-ap"),
			RecommendationPolicyName: types.StringValue("java"),
		}}),
	}

	err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{
		waConfig: &api.WAConfiguration{
			EnableNewWorkloadsProactiveUpdate: &proactive,
			LimiterQuotaPerWindow:             &quota,
		},
		recommendationPolicies: []api.RecommendationPolicyResource{{
			Name: "java",
			Spec: api.RecommendationPolicySpec{
				StrategyType: "percentile",
				HistoryWindowDuration: api.WindowDuration{
					CPU:    "24h",
					Memory: "24h",
				},
				EvaluationPeriod: "1h",
			},
		}},
		autoscalingPolicies: []api.AutoscalingPolicyResource{{
			Name:   "default-ap",
			Enable: true,
			Spec: api.AutoscalingPolicySpec{
				RecommendationPolicyName: "java",
				OnPolicyRemoval:          "off",
			},
		}},
	}, "cluster-1", &data)
	if err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	if data.Kubeconfig.ValueString() != "/tmp/kubeconfig" {
		t.Fatalf("Kubeconfig = %q, want /tmp/kubeconfig", data.Kubeconfig.ValueString())
	}
	if data.EnableNewWorkloadsProactiveUpdate.IsUnknown() || !data.EnableNewWorkloadsProactiveUpdate.ValueBool() {
		t.Fatalf("EnableNewWorkloadsProactiveUpdate = %#v, want known true", data.EnableNewWorkloadsProactiveUpdate)
	}
	if data.LimiterQuotaPerWindow.IsUnknown() || data.LimiterQuotaPerWindow.ValueInt64() != 7 {
		t.Fatalf("LimiterQuotaPerWindow = %#v, want known 7", data.LimiterQuotaPerWindow)
	}
}

func TestMergeAutoscalingPoliciesFromAPIForReadPreservesOmittedDefaults(t *testing.T) {
	ctx := context.Background()

	data := WorkloadAutoscalerModel{
		AutoscalingPolicies: customfield.NewObjectListMust(ctx, []api.AutoscalingPolicyModel{{
			Name:                     types.StringValue("readonly"),
			Enable:                   types.BoolValue(true),
			RecommendationPolicyName: types.StringValue("cost-savings"),
			Priority:                 types.Int64Value(0),
			TargetRefs: customfield.NewObjectListMust(ctx, []api.TargetRefModel{{
				APIVersion: types.StringValue("apps/v1"),
				Kind:       types.StringValue("Deployment"),
			}}),
			UpdateSchedules: customfield.NewObjectListMust(ctx, []api.UpdateScheduleModel{{
				Name: types.StringValue("default"),
				Mode: types.StringValue("off"),
			}}),
		}}),
	}

	err := mergeAutoscalingPoliciesFromAPI(ctx, &data, []api.AutoscalingPolicyResource{
		autoscalingPolicyWithServerDefaults("readonly", "cost-savings"),
	}, false)
	if err != nil {
		t.Fatalf("mergeAutoscalingPoliciesFromAPI() error = %v", err)
	}

	policies, diags := data.AutoscalingPolicies.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("AutoscalingPolicies diagnostics = %v", diags)
	}
	if len(policies) != 1 {
		t.Fatalf("AutoscalingPolicies length = %d, want 1", len(policies))
	}

	got := policies[0]
	if !got.DisableRuntimeOptimization.IsNull() {
		t.Fatalf("DisableRuntimeOptimization should remain null for omitted config, got %#v", got.DisableRuntimeOptimization)
	}
	if got.UpdateResources != nil {
		t.Fatalf("UpdateResources should remain nil for omitted config, got %#v", got.UpdateResources)
	}
	if !got.DriftThresholdCPU.IsNull() {
		t.Fatalf("DriftThresholdCPU should remain null for omitted config, got %#v", got.DriftThresholdCPU)
	}
	if !got.DriftThresholdMemory.IsNull() {
		t.Fatalf("DriftThresholdMemory should remain null for omitted config, got %#v", got.DriftThresholdMemory)
	}
	if !got.OnPolicyRemoval.IsNull() {
		t.Fatalf("OnPolicyRemoval should remain null for omitted config, got %#v", got.OnPolicyRemoval)
	}
	if !got.LimitPolicies.IsNull() {
		t.Fatalf("LimitPolicies should remain null for omitted config, got %#v", got.LimitPolicies)
	}
	if !got.InPlaceFallbackReasonPolicies.IsNull() {
		t.Fatalf("InPlaceFallbackReasonPolicies should remain null for omitted config, got %#v", got.InPlaceFallbackReasonPolicies)
	}
}

func TestMergeAutoscalingPoliciesFromAPIForImportKeepsRemoteDefaults(t *testing.T) {
	ctx := context.Background()
	data := WorkloadAutoscalerModel{}

	err := mergeAutoscalingPoliciesFromAPI(ctx, &data, []api.AutoscalingPolicyResource{
		autoscalingPolicyWithServerDefaults("readonly", "cost-savings"),
	}, true)
	if err != nil {
		t.Fatalf("mergeAutoscalingPoliciesFromAPI() error = %v", err)
	}

	policies, diags := data.AutoscalingPolicies.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("AutoscalingPolicies diagnostics = %v", diags)
	}
	if len(policies) != 1 {
		t.Fatalf("AutoscalingPolicies length = %d, want 1", len(policies))
	}

	got := policies[0]
	if got.OnPolicyRemoval.ValueString() != "off" {
		t.Fatalf("OnPolicyRemoval = %#v, want off", got.OnPolicyRemoval)
	}
	if got.UpdateResources == nil || len(*got.UpdateResources) != 2 {
		t.Fatalf("UpdateResources = %#v, want cpu and memory", got.UpdateResources)
	}
	if got.DriftThresholdCPU.ValueString() != "5%" {
		t.Fatalf("DriftThresholdCPU = %#v, want 5%%", got.DriftThresholdCPU)
	}
	if got.LimitPolicies.IsNull() {
		t.Fatalf("LimitPolicies should be populated during import")
	}
}

func TestPreserveAutoscalingPolicyStateRepresentationKeepsEmptyReasonPolicyMap(t *testing.T) {
	ctx := context.Background()
	remote := api.AutoscalingPolicyModel{
		Name:                          types.StringValue("default-ap"),
		InPlaceFallbackReasonPolicies: customfield.NullMap[types.String](ctx),
	}
	state := api.AutoscalingPolicyModel{
		Name:                          types.StringValue("default-ap"),
		InPlaceFallbackReasonPolicies: customfield.NewMapMust[types.String](ctx, map[string]types.String{}),
	}

	got := preserveAutoscalingPolicyStateRepresentation(ctx, remote, state)
	if got.InPlaceFallbackReasonPolicies.IsNull() {
		t.Fatalf("InPlaceFallbackReasonPolicies should preserve an explicit empty map from state")
	}
	values, diags := got.InPlaceFallbackReasonPolicies.Value(ctx)
	if diags.HasError() {
		t.Fatalf("InPlaceFallbackReasonPolicies diagnostics = %v", diags)
	}
	if len(values) != 0 {
		t.Fatalf("expected empty reason policy map, got %#v", values)
	}
}

func TestShouldSkipBackendInstallCheckForStateBackfill(t *testing.T) {
	data := WorkloadAutoscalerModel{
		Kubeconfig:      types.StringValue("/tmp/kubeconfig"),
		StorageClass:    types.StringValue(""),
		EnableNodeAgent: types.BoolValue(true),
	}
	state := WorkloadAutoscalerModel{
		Kubeconfig:      types.StringValue(""),
		StorageClass:    types.StringValue(""),
		EnableNodeAgent: types.BoolValue(true),
	}

	if !shouldSkipBackendInstallCheckForStateBackfill(data, state) {
		t.Fatalf("expected kubeconfig-only state backfill to skip backend install check")
	}
}

func TestFillMissingParametersDoesNotPersistGeneratedKubeconfig(t *testing.T) {
	ctx := context.Background()
	client := &fakeWorkloadAutoscalerClient{
		summary: &api.ClusterCostsSummary{
			ClusterInfo: api.ClusterInfo{
				ClusterName:   "demo-gke",
				Region:        "us-central1",
				CloudProvider: api.CloudProviderGCP,
			},
		},
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

	data := WorkloadAutoscalerModel{
		ClusterID:  types.StringValue("cluster-1"),
		Kubeconfig: types.StringValue(""),
	}

	kubeconfigPath, err := (&WorkloadAutoscaler{client: client}).fillMissingParameters(ctx, &data, false)
	if err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}
	if filepath.Base(kubeconfigPath) != "test-project_us-central1_demo-gke_kubeconfig" {
		t.Fatalf("kubeconfigPath = %q, want inferred GKE kubeconfig", kubeconfigPath)
	}
	if data.Kubeconfig.ValueString() != "" {
		t.Fatalf("Kubeconfig = %q, want omitted/default value preserved", data.Kubeconfig.ValueString())
	}
}

func TestFillMissingParametersPersistsGeneratedKubeconfigForImport(t *testing.T) {
	ctx := context.Background()
	client := &fakeWorkloadAutoscalerClient{
		summary: &api.ClusterCostsSummary{
			ClusterInfo: api.ClusterInfo{
				ClusterName:   "demo-gke",
				Region:        "us-central1",
				CloudProvider: api.CloudProviderGCP,
			},
		},
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

	originalUpdate := gkeaccess.RunGcloudUpdateKubeconfig
	defer func() {
		gkeaccess.RunGcloudUpdateKubeconfig = originalUpdate
	}()
	gkeaccess.RunGcloudUpdateKubeconfig = func(_ context.Context, _, _, _, kubeconfigPath string) error {
		return os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600)
	}

	data := WorkloadAutoscalerModel{
		ClusterID:  types.StringValue("cluster-1"),
		Kubeconfig: types.StringValue(""),
	}

	kubeconfigPath, err := (&WorkloadAutoscaler{client: client}).fillMissingParameters(ctx, &data, true)
	if err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}
	if data.Kubeconfig.ValueString() != kubeconfigPath {
		t.Fatalf("Kubeconfig = %q, want persisted %q", data.Kubeconfig.ValueString(), kubeconfigPath)
	}
}

func TestWorkloadAutoscalerDeleteInfersGKEKubeconfig(t *testing.T) {
	ctx := context.Background()
	client := &fakeWorkloadAutoscalerClient{
		summary: &api.ClusterCostsSummary{
			ClusterInfo: api.ClusterInfo{
				ClusterName:   "demo-gke",
				Region:        "us-central1",
				CloudProvider: api.CloudProviderGCP,
			},
		},
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

	originalUninstall := uninstallWorkloadAutoscaler
	defer func() {
		uninstallWorkloadAutoscaler = originalUninstall
	}()

	var gotKubeconfig string
	uninstallWorkloadAutoscaler = func(_ context.Context, _ cloudpilotaiclient.Interface, _ string, kubeconfigPath string) error {
		gotKubeconfig = kubeconfigPath
		return nil
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	diags := state.Set(ctx, &WorkloadAutoscalerModel{
		ClusterID:  types.StringValue("cluster-1"),
		Kubeconfig: types.StringNull(),
		RecommendationPolicies: customfield.NewObjectListMust(ctx, []api.RecommendationPolicyModel{{
			Name:                types.StringValue("balanced"),
			HistoryWindowCPU:    types.StringValue("24h"),
			HistoryWindowMemory: types.StringValue("24h"),
			EvaluationPeriod:    types.StringValue("1h"),
		}}),
		AutoscalingPolicies: customfield.NewObjectListMust(ctx, []api.AutoscalingPolicyModel{{
			Name:                     types.StringValue("cloudpilot"),
			RecommendationPolicyName: types.StringValue("balanced"),
		}}),
	})
	if diags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", diags)
	}

	resp := &resource.DeleteResponse{}
	(&WorkloadAutoscaler{client: client}).Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics = %v", resp.Diagnostics)
	}

	if filepath.Base(gotKubeconfig) != "test-project_us-central1_demo-gke_kubeconfig" {
		t.Fatalf("uninstall kubeconfig = %q, want inferred GKE kubeconfig", gotKubeconfig)
	}
	if len(client.deletedAPs) != 1 || client.deletedAPs[0] != "cloudpilot" {
		t.Fatalf("deleted autoscaling policies = %#v, want [cloudpilot]", client.deletedAPs)
	}
	if len(client.deletedRPs) != 1 || client.deletedRPs[0] != "balanced" {
		t.Fatalf("deleted recommendation policies = %#v, want [balanced]", client.deletedRPs)
	}
	if len(client.updatedWAConfigs) != 1 || client.updatedWAConfigs[0] == nil || client.updatedWAConfigs[0].WorkloadAutoscalerInstalled == nil || *client.updatedWAConfigs[0].WorkloadAutoscalerInstalled {
		t.Fatalf("updated WA configs = %#v, want installed=false", client.updatedWAConfigs)
	}
}

func TestWorkloadAutoscalerDeleteRetriesRecommendationPolicyStillInUse(t *testing.T) {
	ctx := context.Background()
	client := &fakeWorkloadAutoscalerClient{
		summary: &api.ClusterCostsSummary{
			ClusterInfo: api.ClusterInfo{
				CloudProvider: api.CloudProviderGCP,
				ClusterName:   "demo-gke",
				Region:        "us-central1",
			},
		},
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
		deleteRPSequences: map[string][]error{
			"balanced": {fmt.Errorf("RecommendationPolicy in use"), nil},
		},
	}

	originalInterval := recommendationPolicyDeleteRetryInterval
	originalTimeout := recommendationPolicyDeleteRetryTimeout
	defer func() {
		recommendationPolicyDeleteRetryInterval = originalInterval
		recommendationPolicyDeleteRetryTimeout = originalTimeout
	}()
	recommendationPolicyDeleteRetryInterval = time.Millisecond
	recommendationPolicyDeleteRetryTimeout = 50 * time.Millisecond

	originalUpdate := gkeaccess.RunGcloudUpdateKubeconfig
	defer func() {
		gkeaccess.RunGcloudUpdateKubeconfig = originalUpdate
	}()
	gkeaccess.RunGcloudUpdateKubeconfig = func(_ context.Context, _, _, _, kubeconfigPath string) error {
		return os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600)
	}

	originalUninstall := uninstallWorkloadAutoscaler
	defer func() {
		uninstallWorkloadAutoscaler = originalUninstall
	}()
	uninstallWorkloadAutoscaler = func(_ context.Context, _ cloudpilotaiclient.Interface, _ string, _ string) error {
		return nil
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	diags := state.Set(ctx, &WorkloadAutoscalerModel{
		ClusterID:  types.StringValue("cluster-1"),
		Kubeconfig: types.StringNull(),
		RecommendationPolicies: customfield.NewObjectListMust(ctx, []api.RecommendationPolicyModel{{
			Name:                types.StringValue("balanced"),
			HistoryWindowCPU:    types.StringValue("24h"),
			HistoryWindowMemory: types.StringValue("24h"),
			EvaluationPeriod:    types.StringValue("1h"),
		}}),
		AutoscalingPolicies: customfield.NewObjectListMust(ctx, []api.AutoscalingPolicyModel{{
			Name:                     types.StringValue("cloudpilot"),
			RecommendationPolicyName: types.StringValue("balanced"),
		}}),
	})
	if diags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", diags)
	}

	resp := &resource.DeleteResponse{}
	(&WorkloadAutoscaler{client: client}).Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics should not contain errors: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 0 {
		t.Fatalf("Delete() diagnostics should not contain warnings: %v", resp.Diagnostics)
	}
	if len(client.deletedAPs) != 1 || client.deletedAPs[0] != "cloudpilot" {
		t.Fatalf("deleted autoscaling policies = %#v, want [cloudpilot]", client.deletedAPs)
	}
	if len(client.deletedRPs) != 1 || client.deletedRPs[0] != "balanced" {
		t.Fatalf("deleted recommendation policies = %#v, want [balanced]", client.deletedRPs)
	}
}

func TestWorkloadAutoscalerDeleteWarnsAndContinuesWhenRemotePolicyDeletionFails(t *testing.T) {
	ctx := context.Background()
	client := &fakeWorkloadAutoscalerClient{
		summary: &api.ClusterCostsSummary{
			ClusterInfo: api.ClusterInfo{
				CloudProvider: api.CloudProviderGCP,
				ClusterName:   "demo-gke",
				Region:        "us-central1",
			},
		},
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
		deleteAPErrs: map[string]error{"cloudpilot": fmt.Errorf("remote ap delete failed")},
		deleteRPErrs: map[string]error{"balanced": fmt.Errorf("remote rp delete failed")},
	}

	originalUpdate := gkeaccess.RunGcloudUpdateKubeconfig
	defer func() {
		gkeaccess.RunGcloudUpdateKubeconfig = originalUpdate
	}()
	gkeaccess.RunGcloudUpdateKubeconfig = func(_ context.Context, _, _, _, kubeconfigPath string) error {
		return os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\n"), 0o600)
	}

	originalUninstall := uninstallWorkloadAutoscaler
	defer func() {
		uninstallWorkloadAutoscaler = originalUninstall
	}()
	uninstallWorkloadAutoscaler = func(_ context.Context, _ cloudpilotaiclient.Interface, _ string, _ string) error {
		return nil
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	diags := state.Set(ctx, &WorkloadAutoscalerModel{
		ClusterID:  types.StringValue("cluster-1"),
		Kubeconfig: types.StringNull(),
		RecommendationPolicies: customfield.NewObjectListMust(ctx, []api.RecommendationPolicyModel{{
			Name:                types.StringValue("balanced"),
			HistoryWindowCPU:    types.StringValue("24h"),
			HistoryWindowMemory: types.StringValue("24h"),
			EvaluationPeriod:    types.StringValue("1h"),
		}}),
		AutoscalingPolicies: customfield.NewObjectListMust(ctx, []api.AutoscalingPolicyModel{{
			Name:                     types.StringValue("cloudpilot"),
			RecommendationPolicyName: types.StringValue("balanced"),
		}}),
	})
	if diags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", diags)
	}

	resp := &resource.DeleteResponse{}
	(&WorkloadAutoscaler{client: client}).Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics should not contain errors: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() < 2 {
		t.Fatalf("expected remote policy delete warnings, got %d", resp.Diagnostics.WarningsCount())
	}
}
