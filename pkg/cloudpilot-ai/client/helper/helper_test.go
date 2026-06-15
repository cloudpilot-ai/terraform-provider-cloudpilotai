package helper

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type fakeRebalanceConfigurationClient struct {
	config        *api.RebalanceConfig
	updatedConfig *api.RebalanceConfig
}

func (f *fakeRebalanceConfigurationClient) GetRebalanceConfiguration(string) (*api.RebalanceConfig, error) {
	return f.config, nil
}

func (f *fakeRebalanceConfigurationClient) UpdateRebalanceConfiguration(_ string, config *api.RebalanceConfig) error {
	copy := *config
	f.updatedConfig = &copy
	return nil
}

func TestSyncRebalanceConfigurationFirstTerraformEnableUsesNodeMode(t *testing.T) {
	client := &fakeRebalanceConfigurationClient{
		config: &api.RebalanceConfig{
			Enable:                      false,
			UploadConfig:                true,
			EnableDiversityInstanceType: true,
			RebalanceType:               "",
		},
	}

	if err := SyncRebalanceConfiguration(context.Background(), client, "cluster-1", true); err != nil {
		t.Fatalf("SyncRebalanceConfiguration() error = %v", err)
	}
	if client.updatedConfig == nil {
		t.Fatal("expected rebalance configuration update")
	}
	if !client.updatedConfig.UploadConfig {
		t.Fatalf("UploadConfig = %v, want true", client.updatedConfig.UploadConfig)
	}
	if !client.updatedConfig.EnableDiversityInstanceType {
		t.Fatalf("EnableDiversityInstanceType = %v, want true", client.updatedConfig.EnableDiversityInstanceType)
	}
	if client.updatedConfig.RebalanceType != api.RebalanceTypeNode {
		t.Fatalf("RebalanceType = %q, want %q", client.updatedConfig.RebalanceType, api.RebalanceTypeNode)
	}
}

func TestSyncRebalanceConfigurationPreservesDiversityWhenDisablingRebalance(t *testing.T) {
	client := &fakeRebalanceConfigurationClient{
		config: &api.RebalanceConfig{
			Enable:                      true,
			UploadConfig:                false,
			EnableDiversityInstanceType: true,
			RebalanceType:               api.RebalanceTypeAll,
		},
	}

	if err := SyncRebalanceConfiguration(context.Background(), client, "cluster-1", false); err != nil {
		t.Fatalf("SyncRebalanceConfiguration() error = %v", err)
	}
	if client.updatedConfig == nil {
		t.Fatal("expected rebalance configuration update")
	}
	if !client.updatedConfig.EnableDiversityInstanceType {
		t.Fatalf("EnableDiversityInstanceType = %v, want true", client.updatedConfig.EnableDiversityInstanceType)
	}
}

func TestSyncRebalanceConfigurationSkipsUpdateWhenAlreadyEnabled(t *testing.T) {
	client := &fakeRebalanceConfigurationClient{
		config: &api.RebalanceConfig{
			Enable:        true,
			UploadConfig:  false,
			RebalanceType: api.RebalanceTypeAll,
		},
	}

	if err := SyncRebalanceConfiguration(context.Background(), client, "cluster-1", true); err != nil {
		t.Fatalf("SyncRebalanceConfiguration() error = %v", err)
	}
	if client.updatedConfig != nil {
		t.Fatalf("expected no rebalance configuration update, got %#v", client.updatedConfig)
	}
}

func TestSyncRebalanceConfigurationSkipsUpdateWhileStillDisabled(t *testing.T) {
	client := &fakeRebalanceConfigurationClient{
		config: &api.RebalanceConfig{
			Enable:        false,
			UploadConfig:  false,
			RebalanceType: api.RebalanceTypeAll,
		},
	}

	if err := SyncRebalanceConfiguration(context.Background(), client, "cluster-1", false); err != nil {
		t.Fatalf("SyncRebalanceConfiguration() error = %v", err)
	}
	if client.updatedConfig != nil {
		t.Fatalf("expected no rebalance configuration update, got %#v", client.updatedConfig)
	}
}

func TestBuildShellEnvIncludesKubeconfigAndAssumeRoleCredentials(t *testing.T) {
	got := buildShellEnv(
		"/tmp/kubeconfig",
		map[string]string{"CUSTOM_NODE_ROLE": "node-role"},
		map[string]string{
			"AWS_ACCESS_KEY_ID":     "AKIA_TEST",
			"AWS_SECRET_ACCESS_KEY": "secret",
			"AWS_SESSION_TOKEN":     "token",
		},
	)

	if got["KUBECONFIG"] != "/tmp/kubeconfig" {
		t.Fatalf("KUBECONFIG = %q, want /tmp/kubeconfig", got["KUBECONFIG"])
	}
	if got["CUSTOM_NODE_ROLE"] != "node-role" {
		t.Fatalf("CUSTOM_NODE_ROLE = %q, want node-role", got["CUSTOM_NODE_ROLE"])
	}
	if got["AWS_ACCESS_KEY_ID"] != "AKIA_TEST" {
		t.Fatalf("AWS_ACCESS_KEY_ID = %q, want AKIA_TEST", got["AWS_ACCESS_KEY_ID"])
	}
	if got["AWS_SECRET_ACCESS_KEY"] != "secret" {
		t.Fatalf("AWS_SECRET_ACCESS_KEY = %q, want secret", got["AWS_SECRET_ACCESS_KEY"])
	}
	if got["AWS_SESSION_TOKEN"] != "token" {
		t.Fatalf("AWS_SESSION_TOKEN = %q, want token", got["AWS_SESSION_TOKEN"])
	}
}

type fakeWorkloadConfigurationClient struct {
	workloadConfig   *api.ClusterWorkloadSpec
	updatedWorkloads []api.Workload
}

func (f *fakeWorkloadConfigurationClient) GetWorkloadRebalanceConfiguration(string) (*api.ClusterWorkloadSpec, error) {
	return f.workloadConfig, nil
}

func (f *fakeWorkloadConfigurationClient) UpdateWorkloadRebalanceConfiguration(_ string, workload api.Workload) error {
	f.updatedWorkloads = append(f.updatedWorkloads, workload)
	return nil
}

type fakeAgentScriptClient struct {
	agentSH         string
	getAgentSHCalls int
}

func (f *fakeAgentScriptClient) GetAgentSH(bool) (string, error) {
	f.getAgentSHCalls++
	return f.agentSH, nil
}

func TestSyncWorkloadConfigurationPreservesRemoteFieldsWhenOmitted(t *testing.T) {
	client := &fakeWorkloadConfigurationClient{
		workloadConfig: &api.ClusterWorkloadSpec{
			Workloads: []api.Workload{{
				Name:               "api",
				Type:               "Deployment",
				Namespace:          "default",
				Replicas:           3,
				RebalanceAble:      true,
				SpotFriendly:       true,
				MinNonSpotReplicas: 2,
			}},
		},
	}

	workloads := customfield.NewObjectListMust(context.Background(), []api.WorkloadModel{{
		Name:               types.StringValue("api"),
		Type:               types.StringValue("Deployment"),
		Namespace:          types.StringValue("default"),
		RebalanceAble:      types.BoolNull(),
		SpotFriendly:       types.BoolNull(),
		MinNonSpotReplicas: types.Int64Null(),
	}})

	if err := SyncWorkloadConfiguration(context.Background(), client, "cluster-1", workloads, customfield.NullObjectList[api.WorkloadTemplateModel](context.Background())); err != nil {
		t.Fatalf("SyncWorkloadConfiguration() error = %v", err)
	}
	if len(client.updatedWorkloads) != 1 {
		t.Fatalf("updated workloads = %d, want 1", len(client.updatedWorkloads))
	}
	got := client.updatedWorkloads[0]
	if !got.RebalanceAble {
		t.Fatalf("RebalanceAble = %v, want true", got.RebalanceAble)
	}
	if !got.SpotFriendly {
		t.Fatalf("SpotFriendly = %v, want true", got.SpotFriendly)
	}
	if got.MinNonSpotReplicas != 2 {
		t.Fatalf("MinNonSpotReplicas = %d, want 2", got.MinNonSpotReplicas)
	}
}

func TestInstallCloudpilotAIAgentComponentDoesNotNormalizeServerScript(t *testing.T) {
	client := &fakeAgentScriptClient{
		agentSH: strings.Join([]string{
			`export AWS_PROFILE=manual-profile`,
			`aws sts get-caller-identity --profile manual-profile`,
		}, "\n"),
	}
	var executedSH string
	var executedEnv map[string]string

	err := installCloudpilotAIAgentComponent(
		context.Background(),
		client,
		"/tmp/kubeconfig",
		true,
		map[string]string{"AWS_PROFILE": "terraform-profile"},
		func(_ context.Context, sh string, env map[string]string) error {
			executedSH = sh
			executedEnv = env
			return nil
		},
	)
	if err != nil {
		t.Fatalf("installCloudpilotAIAgentComponent() error = %v", err)
	}
	if executedSH != client.agentSH {
		t.Fatalf("executed shell = %q, want original agent script %q", executedSH, client.agentSH)
	}
	if !strings.Contains(executedSH, "manual-profile") {
		t.Fatalf("executed shell should preserve manual profile override: %q", executedSH)
	}
	if !strings.Contains(executedSH, "--profile manual-profile") {
		t.Fatalf("executed shell should preserve aws profile flag: %q", executedSH)
	}
	if executedEnv["AWS_PROFILE"] != "terraform-profile" {
		t.Fatalf("AWS_PROFILE = %q, want terraform-profile", executedEnv["AWS_PROFILE"])
	}
	if executedEnv["KUBECONFIG"] != "/tmp/kubeconfig" {
		t.Fatalf("KUBECONFIG = %q, want /tmp/kubeconfig", executedEnv["KUBECONFIG"])
	}
	if client.getAgentSHCalls != 1 {
		t.Fatalf("GetAgentSH calls = %d, want 1", client.getAgentSHCalls)
	}
}

type fakeClusterUpgradeClient struct {
	summary           *api.ClusterCostsSummary
	upgradeSH         string
	getClusterCalls   int
	getUpgradeSHCalls int
}

type fakeRebalanceScriptClient struct {
	rebalanceSH         string
	getRebalanceSHCalls int
}

func (f *fakeRebalanceScriptClient) GetRebalanceSH(string) (string, error) {
	f.getRebalanceSHCalls++
	return f.rebalanceSH, nil
}

func (f *fakeClusterUpgradeClient) GetCluster(string) (*api.ClusterCostsSummary, error) {
	f.getClusterCalls++
	return f.summary, nil
}

func (f *fakeClusterUpgradeClient) GetClusterUpgradeSH(string) (string, error) {
	f.getUpgradeSHCalls++
	return f.upgradeSH, nil
}

func TestUpgradeCloudpilotAIComponentsIfNeededSkipsWhenClusterDoesNotNeedUpgrade(t *testing.T) {
	client := &fakeClusterUpgradeClient{
		summary: &api.ClusterCostsSummary{NeedUpgrade: false},
	}
	executed := false

	upgraded, err := upgradeCloudpilotAIComponentsIfNeeded(
		context.Background(),
		client,
		"cluster-1",
		"/tmp/kubeconfig",
		"",
		map[string]string{},
		func(context.Context, string, map[string]string) error {
			executed = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("upgradeCloudpilotAIComponentsIfNeeded() error = %v", err)
	}
	if upgraded {
		t.Fatalf("upgradeCloudpilotAIComponentsIfNeeded() upgraded = true, want false")
	}
	if executed {
		t.Fatalf("upgrade script should not execute when cluster does not need upgrade")
	}
	if client.getUpgradeSHCalls != 0 {
		t.Fatalf("GetClusterUpgradeSH calls = %d, want 0", client.getUpgradeSHCalls)
	}
}

func TestUpgradeCloudpilotAIComponentsIfNeededRunsUpgradeScriptWithExpectedEnv(t *testing.T) {
	client := &fakeClusterUpgradeClient{
		summary:   &api.ClusterCostsSummary{NeedUpgrade: true},
		upgradeSH: "echo upgrade",
	}
	var executedSH string
	var executedEnv map[string]string

	upgraded, err := upgradeCloudpilotAIComponentsIfNeeded(
		context.Background(),
		client,
		"cluster-1",
		"/tmp/kubeconfig",
		"custom-node-role",
		map[string]string{
			"AWS_ACCESS_KEY_ID":     "AKIA_TEST",
			"AWS_SECRET_ACCESS_KEY": "secret",
			"AWS_SESSION_TOKEN":     "token",
		},
		func(_ context.Context, sh string, env map[string]string) error {
			executedSH = sh
			executedEnv = env
			return nil
		},
	)
	if err != nil {
		t.Fatalf("upgradeCloudpilotAIComponentsIfNeeded() error = %v", err)
	}
	if !upgraded {
		t.Fatalf("upgradeCloudpilotAIComponentsIfNeeded() upgraded = false, want true")
	}
	if executedSH != "echo upgrade" {
		t.Fatalf("executed shell = %q, want %q", executedSH, "echo upgrade")
	}
	if executedEnv["KUBECONFIG"] != "/tmp/kubeconfig" {
		t.Fatalf("KUBECONFIG = %q, want /tmp/kubeconfig", executedEnv["KUBECONFIG"])
	}
	if executedEnv["CUSTOM_NODE_ROLE"] != "custom-node-role" {
		t.Fatalf("CUSTOM_NODE_ROLE = %q, want custom-node-role", executedEnv["CUSTOM_NODE_ROLE"])
	}
	if executedEnv["AWS_ACCESS_KEY_ID"] != "AKIA_TEST" {
		t.Fatalf("AWS_ACCESS_KEY_ID = %q, want AKIA_TEST", executedEnv["AWS_ACCESS_KEY_ID"])
	}
	if executedEnv["AWS_SECRET_ACCESS_KEY"] != "secret" {
		t.Fatalf("AWS_SECRET_ACCESS_KEY = %q, want secret", executedEnv["AWS_SECRET_ACCESS_KEY"])
	}
	if executedEnv["AWS_SESSION_TOKEN"] != "token" {
		t.Fatalf("AWS_SESSION_TOKEN = %q, want token", executedEnv["AWS_SESSION_TOKEN"])
	}
	if client.getUpgradeSHCalls != 1 {
		t.Fatalf("GetClusterUpgradeSH calls = %d, want 1", client.getUpgradeSHCalls)
	}
}

func TestInstallCloudpilotAIRebalanceComponentStripsTerraformManagedOverridesFromServerScript(t *testing.T) {
	client := &fakeRebalanceScriptClient{
		rebalanceSH: strings.Join([]string{
			`export AWS_PROFILE=manual-profile`,
			`CUSTOM_NODE_ROLE=manual-role kubectl get nodes`,
			`aws sts get-caller-identity --profile manual-profile`,
		}, "\n"),
	}
	var executedSH string
	var executedEnv map[string]string

	err := installCloudpilotAIRebalanceComponent(
		context.Background(),
		client,
		"cluster-1",
		"/tmp/kubeconfig",
		"terraform-role",
		map[string]string{"AWS_PROFILE": "terraform-profile"},
		func(_ context.Context, sh string, env map[string]string) error {
			executedSH = sh
			executedEnv = env
			return nil
		},
	)
	if err != nil {
		t.Fatalf("installCloudpilotAIRebalanceComponent() error = %v", err)
	}
	if strings.Contains(executedSH, "manual-profile") {
		t.Fatalf("executed shell still contains manual profile override: %q", executedSH)
	}
	if strings.Contains(executedSH, "manual-role") {
		t.Fatalf("executed shell still contains manual role override: %q", executedSH)
	}
	if strings.Contains(executedSH, "--profile") {
		t.Fatalf("executed shell still contains aws profile flag: %q", executedSH)
	}
	if !strings.Contains(executedSH, "aws sts get-caller-identity") {
		t.Fatalf("executed shell lost aws command body: %q", executedSH)
	}
	if executedEnv["AWS_PROFILE"] != "terraform-profile" {
		t.Fatalf("AWS_PROFILE = %q, want terraform-profile", executedEnv["AWS_PROFILE"])
	}
	if executedEnv["CUSTOM_NODE_ROLE"] != "terraform-role" {
		t.Fatalf("CUSTOM_NODE_ROLE = %q, want terraform-role", executedEnv["CUSTOM_NODE_ROLE"])
	}
	if client.getRebalanceSHCalls != 1 {
		t.Fatalf("GetRebalanceSH calls = %d, want 1", client.getRebalanceSHCalls)
	}
}

func TestUpgradeCloudpilotAIComponentsIfNeededStripsAssumeRoleCredentialOverridesFromServerScript(t *testing.T) {
	client := &fakeClusterUpgradeClient{
		summary: &api.ClusterCostsSummary{NeedUpgrade: true},
		upgradeSH: strings.Join([]string{
			`unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN AWS_ROLE_ARN KEEP_ME`,
			`export AWS_ACCESS_KEY_ID=manual`,
			`export AWS_SECRET_ACCESS_KEY=manual-secret`,
			`export AWS_SESSION_TOKEN=manual-token`,
			`AWS_ROLE_ARN=arn:aws:iam::123456789012:role/manual aws sts get-caller-identity`,
		}, "\n"),
	}
	var executedSH string
	var executedEnv map[string]string

	upgraded, err := upgradeCloudpilotAIComponentsIfNeeded(
		context.Background(),
		client,
		"cluster-1",
		"/tmp/kubeconfig",
		"terraform-role",
		map[string]string{
			"AWS_ACCESS_KEY_ID":     "AKIA_TEST",
			"AWS_SECRET_ACCESS_KEY": "secret",
			"AWS_SESSION_TOKEN":     "token",
		},
		func(_ context.Context, sh string, env map[string]string) error {
			executedSH = sh
			executedEnv = env
			return nil
		},
	)
	if err != nil {
		t.Fatalf("upgradeCloudpilotAIComponentsIfNeeded() error = %v", err)
	}
	if !upgraded {
		t.Fatalf("upgradeCloudpilotAIComponentsIfNeeded() upgraded = false, want true")
	}
	if strings.Contains(executedSH, "AWS_ACCESS_KEY_ID=manual") {
		t.Fatalf("executed shell still contains manual access key override: %q", executedSH)
	}
	if strings.Contains(executedSH, "AWS_SECRET_ACCESS_KEY=manual-secret") {
		t.Fatalf("executed shell still contains manual secret override: %q", executedSH)
	}
	if strings.Contains(executedSH, "AWS_SESSION_TOKEN=manual-token") {
		t.Fatalf("executed shell still contains manual session token override: %q", executedSH)
	}
	if strings.Contains(executedSH, "unset AWS_ACCESS_KEY_ID") || strings.Contains(executedSH, "unset AWS_SECRET_ACCESS_KEY") || strings.Contains(executedSH, "unset AWS_SESSION_TOKEN") || strings.Contains(executedSH, "unset AWS_ROLE_ARN") {
		t.Fatalf("executed shell still contains managed unset names: %q", executedSH)
	}
	if !strings.Contains(executedSH, "unset KEEP_ME") {
		t.Fatalf("executed shell should preserve unrelated unset names: %q", executedSH)
	}
	if !strings.Contains(executedSH, "aws sts get-caller-identity") {
		t.Fatalf("executed shell lost aws command body: %q", executedSH)
	}
	if executedEnv["AWS_ACCESS_KEY_ID"] != "AKIA_TEST" {
		t.Fatalf("AWS_ACCESS_KEY_ID = %q, want AKIA_TEST", executedEnv["AWS_ACCESS_KEY_ID"])
	}
	if executedEnv["AWS_SECRET_ACCESS_KEY"] != "secret" {
		t.Fatalf("AWS_SECRET_ACCESS_KEY = %q, want secret", executedEnv["AWS_SECRET_ACCESS_KEY"])
	}
	if executedEnv["AWS_SESSION_TOKEN"] != "token" {
		t.Fatalf("AWS_SESSION_TOKEN = %q, want token", executedEnv["AWS_SESSION_TOKEN"])
	}
	if executedEnv["CUSTOM_NODE_ROLE"] != "terraform-role" {
		t.Fatalf("CUSTOM_NODE_ROLE = %q, want terraform-role", executedEnv["CUSTOM_NODE_ROLE"])
	}
}

func TestDeleteOptimizedNodesSHDrainsManagedNodesBeforeDeletingNodeClaims(t *testing.T) {
	drain := "kubectl drain $node --ignore-daemonsets --delete-emptydir-data --force"
	deleteNodeClaim := "kubectl delete nodeclaim --all"

	if !strings.Contains(DeleteOptimizedNodesSH, drain) {
		t.Fatalf("DeleteOptimizedNodesSH missing drain command: %q", DeleteOptimizedNodesSH)
	}

	drainIdx := strings.Index(DeleteOptimizedNodesSH, drain)
	deleteIdx := strings.Index(DeleteOptimizedNodesSH, deleteNodeClaim)
	if deleteIdx == -1 {
		t.Fatalf("DeleteOptimizedNodesSH missing nodeclaim deletion command: %q", DeleteOptimizedNodesSH)
	}
	if drainIdx > deleteIdx {
		t.Fatalf("drain command must run before deleting nodeclaims: %q", DeleteOptimizedNodesSH)
	}
}
