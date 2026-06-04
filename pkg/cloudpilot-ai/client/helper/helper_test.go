package helper

import (
	"context"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
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

type fakeClusterUpgradeClient struct {
	summary           *api.ClusterCostsSummary
	upgradeSH         string
	getClusterCalls   int
	getUpgradeSHCalls int
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
		"",
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
		"aws-profile",
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
	if executedEnv["AWS_PROFILE"] != "aws-profile" {
		t.Fatalf("AWS_PROFILE = %q, want aws-profile", executedEnv["AWS_PROFILE"])
	}
	if client.getUpgradeSHCalls != 1 {
		t.Fatalf("GetClusterUpgradeSH calls = %d, want 1", client.getUpgradeSHCalls)
	}
}
