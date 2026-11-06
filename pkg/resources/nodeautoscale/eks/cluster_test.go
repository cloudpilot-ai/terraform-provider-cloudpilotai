package eks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	awsauth "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/utils/aws"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	schemaplanmodifier "github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type namedItem struct {
	name string
}

type fakePostWriteStateHydratorClient struct {
	clusterSetting *api.ClusterSetting
	nodeClasses    api.RebalanceNodeClassList
	nodePools      api.RebalanceNodePoolList
}

func (f *fakePostWriteStateHydratorClient) GetClusterSetting(string) (*api.ClusterSetting, error) {
	return f.clusterSetting, nil
}

func (f *fakePostWriteStateHydratorClient) ListNodeClasses(string) (api.RebalanceNodeClassList, error) {
	return f.nodeClasses, nil
}

func (f *fakePostWriteStateHydratorClient) ListNodePools(string) (api.RebalanceNodePoolList, error) {
	return f.nodePools, nil
}

func (f *fakePostWriteStateHydratorClient) ListScheduledRebalances(string) (api.ScheduledRebalancePolicyList, error) {
	return api.ScheduledRebalancePolicyList{}, nil
}

type fakeReadAuthClient struct {
	cloudpilotaiclient.Interface
	getClusterCalls int
}

func (f *fakeReadAuthClient) GetCluster(string) (*api.ClusterCostsSummary, error) {
	f.getClusterCalls++
	return nil, errors.New("client should not be reached before aws env validation")
}

type fakeClusterSummaryClient struct {
	cloudpilotaiclient.Interface
	summary         *api.ClusterCostsSummary
	summaries       []*api.ClusterCostsSummary
	err             error
	onGetCluster    func()
	getClusterCalls int
}

func (f *fakeClusterSummaryClient) GetCluster(string) (*api.ClusterCostsSummary, error) {
	f.getClusterCalls++
	if f.onGetCluster != nil {
		f.onGetCluster()
	}
	if f.err != nil {
		return nil, f.err
	}
	if len(f.summaries) > 0 {
		summary := f.summaries[0]
		f.summaries = f.summaries[1:]
		return summary, nil
	}
	return f.summary, nil
}

type fakeClusterSettingUpdateClient struct {
	cloudpilotaiclient.Interface
	setting *api.ClusterSetting
}

type fakeConfigurationPreflightClient struct {
	cloudpilotaiclient.Interface
	operations []string
	nodePools  api.RebalanceNodePoolList
}

func (f *fakeConfigurationPreflightClient) ListScheduledRebalances(string) (api.ScheduledRebalancePolicyList, error) {
	return api.ScheduledRebalancePolicyList{}, nil
}

func (f *fakeConfigurationPreflightClient) ListNodePools(string) (api.RebalanceNodePoolList, error) {
	return f.nodePools, nil
}

func (f *fakeConfigurationPreflightClient) ListNodeClasses(string) (api.RebalanceNodeClassList, error) {
	return api.RebalanceNodeClassList{}, nil
}

func (f *fakeConfigurationPreflightClient) ApplyNodeClass(string, api.RebalanceNodeClass) error {
	f.operations = append(f.operations, "apply-nodeclass")
	return nil
}

func (f *fakeConfigurationPreflightClient) ApplyNodePool(string, api.RebalanceNodePool) error {
	f.operations = append(f.operations, "apply-nodepool")
	return nil
}

func (f *fakeConfigurationPreflightClient) DeleteNodeClass(string, string) error {
	f.operations = append(f.operations, "delete-nodeclass")
	return nil
}

func (f *fakeConfigurationPreflightClient) DeleteNodePool(string, string) error {
	f.operations = append(f.operations, "delete-nodepool")
	f.nodePools = api.RebalanceNodePoolList{}
	return nil
}

func (f *fakeConfigurationPreflightClient) UpdateClusterSetting(string, *api.ClusterSetting) error {
	f.operations = append(f.operations, "update-cluster-setting")
	return nil
}

func (f *fakeConfigurationPreflightClient) GetRebalanceConfiguration(string) (*api.RebalanceConfig, error) {
	return &api.RebalanceConfig{Enable: true}, nil
}

func (f *fakeConfigurationPreflightClient) UpdateRebalanceConfiguration(string, *api.RebalanceConfig) error {
	f.operations = append(f.operations, "update-rebalance")
	return nil
}

type fakeDeleteClusterClient struct {
	cloudpilotaiclient.Interface
	rebalanceClusterID string
	deletedClusterID   string
}

func (f *fakeDeleteClusterClient) UpdateRebalanceConfiguration(clusterID string, _ *api.RebalanceConfig) error {
	f.rebalanceClusterID = clusterID
	return nil
}

func (f *fakeDeleteClusterClient) DeleteCluster(clusterID string) error {
	f.deletedClusterID = clusterID
	return nil
}

func (f *fakeClusterSettingUpdateClient) UpdateClusterSetting(_ string, setting *api.ClusterSetting) error {
	f.setting = setting
	return nil
}

func TestSyncConfigurationPrevalidatesNodePoolsBeforeWrites(t *testing.T) {
	ctx := context.Background()
	client := &fakeConfigurationPreflightClient{}
	cluster := &Cluster{client: client}
	data := ClusterModel{
		ClusterName:        types.StringValue("test-cluster"),
		EnableRebalance:    types.BoolValue(false),
		NodeClasses:        customfield.NewObjectListMust(ctx, []api.EC2NodeClassModel{{Name: types.StringValue("default")}}),
		NodeClassTemplates: customfield.NullObjectList[api.EC2NodeClassTemplateModel](ctx),
		NodePools: customfield.NewObjectListMust(ctx, []api.EC2NodePoolModel{{
			Name:      types.StringValue("invalid"),
			NodeClass: types.StringValue("default"),
			NodeDisruptionBudgets: customfield.NewObjectListMust(ctx, []api.DisruptionBudgetModel{{
				Nodes: types.StringValue("10%"), Schedule: types.StringValue("daily"), Duration: types.StringValue("1h"),
			}}),
		}}),
		NodePoolTemplates:   customfield.NullObjectList[api.EC2NodePoolTemplateModel](ctx),
		Workloads:           customfield.NullObjectList[api.WorkloadModel](ctx),
		WorkloadTemplates:   customfield.NullObjectList[api.WorkloadTemplateModel](ctx),
		ScheduledRebalances: customfield.NullObjectList[api.ScheduledRebalancePolicyModel](ctx),
	}

	if err := cluster.syncConfiguration(ctx, &data, "cluster-1", nil, nil, nil); err == nil {
		t.Fatal("syncConfiguration() error = nil, want invalid disruption schedule error")
	}
	if len(client.operations) != 0 {
		t.Fatalf("invalid nodepool configuration made writes: %#v", client.operations)
	}
}

type fakeReadClusterClient struct {
	cloudpilotaiclient.Interface
	summary                 *api.ClusterCostsSummary
	getClusterErr           error
	clusterSetting          *api.ClusterSetting
	rebalanceConfiguration  *api.RebalanceConfig
	workloadConfiguration   *api.ClusterWorkloadSpec
	nodeClasses             api.RebalanceNodeClassList
	nodePools               api.RebalanceNodePoolList
	getClusterCalls         int
	getClusterSettingCalls  int
	getRebalanceConfigCalls int
	getWorkloadConfigCalls  int
	listNodeClassesCalls    int
	listNodePoolsCalls      int
}

func (f *fakeReadClusterClient) GetCluster(string) (*api.ClusterCostsSummary, error) {
	f.getClusterCalls++
	if f.getClusterErr != nil {
		return nil, f.getClusterErr
	}
	return f.summary, nil
}

func (f *fakeReadClusterClient) GetClusterSetting(string) (*api.ClusterSetting, error) {
	f.getClusterSettingCalls++
	if f.clusterSetting != nil {
		return f.clusterSetting, nil
	}
	return &api.ClusterSetting{}, nil
}

func (f *fakeReadClusterClient) GetRebalanceConfiguration(string) (*api.RebalanceConfig, error) {
	f.getRebalanceConfigCalls++
	if f.rebalanceConfiguration != nil {
		return f.rebalanceConfiguration, nil
	}
	return &api.RebalanceConfig{}, nil
}

func (f *fakeReadClusterClient) GetWorkloadRebalanceConfiguration(string) (*api.ClusterWorkloadSpec, error) {
	f.getWorkloadConfigCalls++
	if f.workloadConfiguration != nil {
		return f.workloadConfiguration, nil
	}
	return &api.ClusterWorkloadSpec{}, nil
}

func (f *fakeReadClusterClient) ListNodeClasses(string) (api.RebalanceNodeClassList, error) {
	f.listNodeClassesCalls++
	return f.nodeClasses, nil
}

func (f *fakeReadClusterClient) ListNodePools(string) (api.RebalanceNodePoolList, error) {
	f.listNodePoolsCalls++
	return f.nodePools, nil
}

func TestSortedValuesByName(t *testing.T) {
	items := map[string]namedItem{
		"p2p":                {name: "p2p"},
		"cloudpilot-general": {name: "cloudpilot-general"},
		"cloudpilot-gpu":     {name: "cloudpilot-gpu"},
	}

	got := sortedValuesByName(items, func(item namedItem) string {
		return item.name
	})

	want := []string{"cloudpilot-general", "cloudpilot-gpu", "p2p"}
	if len(got) != len(want) {
		t.Fatalf("got %d items, want %d", len(got), len(want))
	}

	for i, item := range got {
		if item.name != want[i] {
			t.Fatalf("got order %v, want %v", []string{got[0].name, got[1].name, got[2].name}, want)
		}
	}
}

func TestOrderByStateNameDropsMissingRemoteObjects(t *testing.T) {
	state := []namedItem{{name: "existing"}, {name: "deleted"}}
	remote := map[string]namedItem{
		"existing":    {name: "existing"},
		"server-only": {name: "server-only"},
	}

	got := orderByStateName(state, remote, func(item namedItem) string { return item.name })
	if len(got) != 1 || got[0].name != "existing" {
		t.Fatalf("orderByStateName() = %#v, want only existing managed object", got)
	}
}

func TestUpdatePrefersStateClusterIDOverGeneratedID(t *testing.T) {
	got := resolveClusterUID(
		types.StringUnknown(),
		types.StringValue("server-imported-id"),
		types.StringValue("test-saving-20260601-144407"),
		types.StringValue("us-east-2"),
		types.StringValue("306107317780"),
	)

	if got != "server-imported-id" {
		t.Fatalf("got cluster ID %q, want imported state cluster ID", got)
	}
}

func TestUpdatePrefersConfiguredClusterIDOverStateAndGeneratedID(t *testing.T) {
	got := resolveClusterUID(
		types.StringValue("user-specified-id"),
		types.StringValue("server-imported-id"),
		types.StringValue("test-saving-20260601-144407"),
		types.StringValue("us-east-2"),
		types.StringValue("306107317780"),
	)

	if got != "user-specified-id" {
		t.Fatalf("got cluster ID %q, want user-specified cluster ID", got)
	}
}

func TestSchemaUsesUnifiedUpgradeFlag(t *testing.T) {
	s := Schema(context.Background())

	if _, ok := s.Attributes["enable_upgrade"]; !ok {
		t.Fatalf("eks schema missing enable_upgrade")
	}
	if _, ok := s.Attributes["enable_upgrade_agent"]; ok {
		t.Fatalf("eks schema should not expose enable_upgrade_agent")
	}
	if _, ok := s.Attributes["enable_upgrade_rebalance_component"]; ok {
		t.Fatalf("eks schema should not expose enable_upgrade_rebalance_component")
	}
}

func TestSchemaExposesAWSAssumeRoleNestedAttribute(t *testing.T) {
	attr, ok := Schema(context.Background()).Attributes["aws_assume_role"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("aws_assume_role attribute has unexpected type %T", Schema(context.Background()).Attributes["aws_assume_role"])
	}
	if !attr.IsOptional() {
		t.Fatalf("aws_assume_role should be optional")
	}

	roleARNAttr, ok := attr.Attributes["role_arn"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("role_arn attribute has unexpected type %T", attr.Attributes["role_arn"])
	}
	if !roleARNAttr.IsRequired() {
		t.Fatalf("role_arn should be required")
	}

	sessionAttr, ok := attr.Attributes["session_name"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("session_name attribute has unexpected type %T", attr.Attributes["session_name"])
	}
	if !sessionAttr.IsOptional() {
		t.Fatalf("session_name should be optional")
	}
}

func TestTemplateAttributesAreDeprecated(t *testing.T) {
	s := Schema(context.Background())

	for _, name := range []string{"workload_templates", "nodeclass_templates", "nodepool_templates"} {
		attr, ok := s.Attributes[name].(schema.ListNestedAttribute)
		if !ok {
			t.Fatalf("%s attribute has unexpected type %T", name, s.Attributes[name])
		}
		if attr.DeprecationMessage == "" {
			t.Fatalf("%s should be deprecated", name)
		}
	}
}

func TestTemplateNameFieldsAreDeprecated(t *testing.T) {
	s := Schema(context.Background())

	workloadsAttr := s.Attributes["workloads"].(schema.ListNestedAttribute)
	workloadTemplateName := workloadsAttr.NestedObject.Attributes["template_name"].(schema.StringAttribute)
	if workloadTemplateName.DeprecationMessage == "" {
		t.Fatalf("workloads.template_name should be deprecated")
	}

	nodeclassesAttr := s.Attributes["nodeclasses"].(schema.ListNestedAttribute)
	nodeclassTemplateName := nodeclassesAttr.NestedObject.Attributes["template_name"].(schema.StringAttribute)
	if nodeclassTemplateName.DeprecationMessage == "" {
		t.Fatalf("nodeclasses.template_name should be deprecated")
	}

	nodepoolsAttr := s.Attributes["nodepools"].(schema.ListNestedAttribute)
	nodepoolTemplateName := nodepoolsAttr.NestedObject.Attributes["template_name"].(schema.StringAttribute)
	if nodepoolTemplateName.DeprecationMessage == "" {
		t.Fatalf("nodepools.template_name should be deprecated")
	}
}

func TestNodeDisruptionLimitIsDeprecated(t *testing.T) {
	nodepools := Schema(context.Background()).Attributes["nodepools"].(schema.ListNestedAttribute)
	limit := nodepools.NestedObject.Attributes["node_disruption_limit"].(schema.StringAttribute)
	if limit.DeprecationMessage == "" {
		t.Fatalf("nodepools.node_disruption_limit should be deprecated")
	}
}

func TestExecutionAuthConfigFromClusterModelReadsAssumeRoleAndProfile(t *testing.T) {
	ctx := context.Background()
	data := ClusterModel{
		AWSProfile: types.StringValue("dev"),
		AWSAssumeRole: customfield.NewObjectMust(ctx, &AWSAssumeRoleModel{
			RoleARN:     types.StringValue("arn:aws:iam::123456789012:role/sts-admin"),
			SessionName: types.StringValue("terraform-user"),
		}),
	}

	got, err := executionAuthConfigFromModel(ctx, data)
	if err != nil {
		t.Fatalf("executionAuthConfigFromModel() error = %v", err)
	}
	if got.Profile != "dev" {
		t.Fatalf("Profile = %q, want dev", got.Profile)
	}
	if got.AssumeRoleARN != "arn:aws:iam::123456789012:role/sts-admin" {
		t.Fatalf("AssumeRoleARN = %q, want sts-admin role", got.AssumeRoleARN)
	}
	if got.AssumeRoleSessionName != "terraform-user" {
		t.Fatalf("AssumeRoleSessionName = %q, want terraform-user", got.AssumeRoleSessionName)
	}
}

func TestFillMissingParametersKeepsGeneratedKubeconfigOutOfState(t *testing.T) {
	ctx := context.Background()
	t.Chdir(t.TempDir())
	awsPath := filepath.Join(t.TempDir(), "aws")
	script := "#!/bin/sh\nif [ \"$1\" = 'sts' ]; then\n  printf '{\"Account\":\"123456789012\"}\\n'\n  exit 0\nfi\npath=''\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = '--kubeconfig' ]; then shift; path=\"$1\"; fi\n  shift\ndone\nprintf 'apiVersion: v1\\n' > \"$path\"\n"
	if err := os.WriteFile(awsPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PATH", filepath.Dir(awsPath))

	data := ClusterModel{
		ClusterName: types.StringValue("demo-eks"),
		Region:      types.StringValue("us-east-2"),
		AccountID:   types.StringNull(),
		Kubeconfig:  types.StringNull(),
	}
	kubeconfigPath, err := (&Cluster{}).fillMissingParameters(ctx, &data, awsauth.ExecutionAuthConfig{}, false)
	if err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}
	if filepath.Base(kubeconfigPath) != "us-east-2_demo-eks_kubeconfig" {
		t.Fatalf("runtime kubeconfig = %q, want generated EKS path", kubeconfigPath)
	}
	if !data.Kubeconfig.IsNull() {
		t.Fatalf("kubeconfig state = %#v, want null", data.Kubeconfig)
	}
	if data.AccountID.ValueString() != "123456789012" {
		t.Fatalf("account_id = %q, want discovered account", data.AccountID.ValueString())
	}
}

func TestFillMissingParametersPreservesExplicitRelativeKubeconfigInState(t *testing.T) {
	ctx := context.Background()
	t.Chdir(t.TempDir())
	configuredPath := "./explicit-kubeconfig"
	if err := os.WriteFile(configuredPath, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	data := ClusterModel{
		ClusterName: types.StringValue("demo-eks"),
		Region:      types.StringValue("us-east-2"),
		AccountID:   types.StringValue("123456789012"),
		Kubeconfig:  types.StringValue(configuredPath),
	}
	kubeconfigPath, err := (&Cluster{}).fillMissingParameters(ctx, &data, awsauth.ExecutionAuthConfig{}, false)
	if err != nil {
		t.Fatalf("fillMissingParameters() error = %v", err)
	}
	if data.Kubeconfig.ValueString() != configuredPath {
		t.Fatalf("kubeconfig state = %q, want exact configured value %q", data.Kubeconfig.ValueString(), configuredPath)
	}
	if !filepath.IsAbs(kubeconfigPath) || filepath.Base(kubeconfigPath) != filepath.Base(configuredPath) {
		t.Fatalf("runtime kubeconfig = %q, want absolute path for %q", kubeconfigPath, configuredPath)
	}
}

func TestReadPreparesAWSAuthEnvironmentWhenClusterIDExists(t *testing.T) {
	ctx := context.Background()
	awsPath := filepath.Join(t.TempDir(), "aws")
	if err := os.WriteFile(awsPath, []byte("#!/bin/sh\necho sentinel-assume-role >&2\nexit 42\n"), 0o755); err != nil {
		t.Fatalf("failed to write fake aws command: %v", err)
	}
	t.Setenv("PATH", filepath.Dir(awsPath))

	state := tfsdk.State{Schema: Schema(ctx)}
	diags := state.Set(ctx, &ClusterModel{
		ClusterID:   types.StringValue("cluster-1"),
		ClusterName: types.StringValue("demo"),
		Region:      types.StringValue("us-east-2"),
		AccountID:   types.StringValue("123456789012"),
		Kubeconfig:  types.StringValue("/tmp/kubeconfig"),
		AWSAssumeRole: customfield.NewObjectMust(ctx, &AWSAssumeRoleModel{
			RoleARN: types.StringValue("arn:aws:iam::123456789012:role/sts-admin"),
		}),
	})
	if diags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", diags)
	}

	client := &fakeReadAuthClient{}
	resp := &resource.ReadResponse{}
	(&Cluster{client: client}).Read(ctx, resource.ReadRequest{State: state}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics had no error, want aws auth environment error")
	}
	if resp.Diagnostics[0].Summary() != "failed to prepare aws auth environment" {
		t.Fatalf("Read() diagnostic title = %q, want failed to prepare aws auth environment", resp.Diagnostics[0].Summary())
	}
	if client.getClusterCalls != 0 {
		t.Fatalf("GetCluster calls = %d, want 0 before aws auth environment succeeds", client.getClusterCalls)
	}
}

func TestEnableRebalanceHasNoSchemaDefault(t *testing.T) {
	attr, ok := Schema(context.Background()).Attributes["enable_rebalance"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("enable_rebalance attribute has unexpected type %T", Schema(context.Background()).Attributes["enable_rebalance"])
	}
	if attr.BoolDefaultValue() != nil {
		t.Fatalf("enable_rebalance should not have a schema default")
	}
	if attr.IsComputed() {
		t.Fatalf("enable_rebalance should not be computed")
	}
}

func TestDeletePrefersConfiguredClusterIDOverGeneratedID(t *testing.T) {
	got := resolveDeleteClusterUID(
		types.StringValue("user-specified-id"),
		types.StringValue("test-saving-20260601-144407"),
		types.StringValue("us-east-2"),
		types.StringValue("306107317780"),
	)

	if got != "user-specified-id" {
		t.Fatalf("got cluster ID %q, want configured cluster ID during delete", got)
	}
}

func TestDeleteRegeneratesMissingLegacyKubeconfigInCurrentWorkingDirectory(t *testing.T) {
	ctx := context.Background()
	t.Chdir(t.TempDir())
	binDir := t.TempDir()
	awsPath := filepath.Join(binDir, "aws")
	awsScript := `#!/bin/sh
path=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = '--kubeconfig' ]; then shift; path="$1"; fi
  shift
done
printf 'apiVersion: v1\n' > "$path"
printf '%s' "$path" > "$CAPTURE_PATH"
`
	if err := os.WriteFile(awsPath, []byte(awsScript), 0o755); err != nil {
		t.Fatalf("WriteFile(aws) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "kubectl"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(kubectl) error = %v", err)
	}
	capturePath := filepath.Join(t.TempDir(), "generated-path")
	t.Setenv("CAPTURE_PATH", capturePath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	state := tfsdk.State{Schema: Schema(ctx)}
	diags := state.Set(ctx, &ClusterModel{
		ClusterID:                types.StringValue("cluster-1"),
		ClusterName:              types.StringValue("demo"),
		Region:                   types.StringValue("us-east-2"),
		AccountID:                types.StringValue("123456789012"),
		Kubeconfig:               types.StringValue("/old-runner/.terragrunt-cache/us-east-2_demo_kubeconfig"),
		SkipRestore:              types.BoolValue(true),
		RestoreNodeNumber:        types.Int64Value(0),
		EnableRebalance:          types.BoolNull(),
		DisableWorkloadUploading: types.BoolNull(),
	})
	if diags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", diags)
	}

	client := &fakeDeleteClusterClient{}
	resp := &resource.DeleteResponse{State: state}
	(&Cluster{client: client}).Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics = %v", resp.Diagnostics)
	}
	generatedPathBytes, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("ReadFile(capture) error = %v", err)
	}
	generatedPath := string(generatedPathBytes)
	if filepath.Base(generatedPath) != "us-east-2_demo_kubeconfig" || strings.Contains(generatedPath, ".terragrunt-cache") {
		t.Fatalf("generated path = %q, want current execution-local path", generatedPath)
	}
	if client.rebalanceClusterID != "cluster-1" || client.deletedClusterID != "cluster-1" {
		t.Fatalf("delete calls used %q/%q, want cluster-1", client.rebalanceClusterID, client.deletedClusterID)
	}
}

func TestWorkloadOptionalFieldsHaveNoDefault(t *testing.T) {
	workloadsAttr, ok := Schema(context.Background()).Attributes["workloads"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("workloads attribute has unexpected type %T", Schema(context.Background()).Attributes["workloads"])
	}

	rebalanceAttr, ok := workloadsAttr.NestedObject.Attributes["rebalance_able"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("rebalance_able attribute has unexpected type %T", workloadsAttr.NestedObject.Attributes["rebalance_able"])
	}
	if rebalanceAttr.BoolDefaultValue() != nil {
		t.Fatalf("rebalance_able should not have a schema default")
	}
	if rebalanceAttr.IsComputed() {
		t.Fatalf("rebalance_able should not be computed")
	}

	spotAttr, ok := workloadsAttr.NestedObject.Attributes["spot_friendly"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("spot_friendly attribute has unexpected type %T", workloadsAttr.NestedObject.Attributes["spot_friendly"])
	}
	if spotAttr.BoolDefaultValue() != nil {
		t.Fatalf("spot_friendly should not have a schema default")
	}
	if spotAttr.IsComputed() {
		t.Fatalf("spot_friendly should not be computed")
	}

	replicasAttr, ok := workloadsAttr.NestedObject.Attributes["min_non_spot_replicas"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("min_non_spot_replicas attribute has unexpected type %T", workloadsAttr.NestedObject.Attributes["min_non_spot_replicas"])
	}
	if replicasAttr.Int64DefaultValue() != nil {
		t.Fatalf("min_non_spot_replicas should not have a schema default")
	}
	if replicasAttr.IsComputed() {
		t.Fatalf("min_non_spot_replicas should not be computed")
	}
}

func TestNodeClassExtraAllocationAttributesHaveNoDefault(t *testing.T) {
	nodeClassesAttr, ok := Schema(context.Background()).Attributes["nodeclasses"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("nodeclasses attribute has unexpected type %T", Schema(context.Background()).Attributes["nodeclasses"])
	}

	cpuAttr, ok := nodeClassesAttr.NestedObject.Attributes["extra_cpu_allocation_mcore"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("extra_cpu_allocation_mcore attribute has unexpected type %T", nodeClassesAttr.NestedObject.Attributes["extra_cpu_allocation_mcore"])
	}
	if cpuAttr.Int64DefaultValue() != nil {
		t.Fatalf("extra_cpu_allocation_mcore should not have a schema default")
	}
	if len(cpuAttr.Int64PlanModifiers()) == 0 {
		t.Fatalf("extra_cpu_allocation_mcore should preserve null state in the plan")
	}

	memoryAttr, ok := nodeClassesAttr.NestedObject.Attributes["extra_memory_allocation_mib"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("extra_memory_allocation_mib attribute has unexpected type %T", nodeClassesAttr.NestedObject.Attributes["extra_memory_allocation_mib"])
	}
	if memoryAttr.Int64DefaultValue() != nil {
		t.Fatalf("extra_memory_allocation_mib should not have a schema default")
	}
	if len(memoryAttr.Int64PlanModifiers()) == 0 {
		t.Fatalf("extra_memory_allocation_mib should preserve null state in the plan")
	}
}

func TestNodePoolMinimumInstanceFilterAttributesHaveNoDefault(t *testing.T) {
	nodePoolsAttr, ok := Schema(context.Background()).Attributes["nodepools"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("nodepools attribute has unexpected type %T", Schema(context.Background()).Attributes["nodepools"])
	}

	cpuMinAttr, ok := nodePoolsAttr.NestedObject.Attributes["instance_cpu_min"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("instance_cpu_min attribute has unexpected type %T", nodePoolsAttr.NestedObject.Attributes["instance_cpu_min"])
	}
	if cpuMinAttr.Int64DefaultValue() != nil {
		t.Fatalf("instance_cpu_min should not have a schema default")
	}
	if len(cpuMinAttr.Int64PlanModifiers()) == 0 {
		t.Fatalf("instance_cpu_min should preserve null state in the plan")
	}

	memoryMinAttr, ok := nodePoolsAttr.NestedObject.Attributes["instance_memory_min"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("instance_memory_min attribute has unexpected type %T", nodePoolsAttr.NestedObject.Attributes["instance_memory_min"])
	}
	if memoryMinAttr.Int64DefaultValue() != nil {
		t.Fatalf("instance_memory_min should not have a schema default")
	}
	if len(memoryMinAttr.Int64PlanModifiers()) == 0 {
		t.Fatalf("instance_memory_min should preserve null state in the plan")
	}
}

func TestSchemaExposesUpgradeStatusReadOnlyFields(t *testing.T) {
	s := Schema(context.Background())

	agentVersionAttr, ok := s.Attributes["agent_version"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("agent_version attribute has unexpected type %T", s.Attributes["agent_version"])
	}
	if !agentVersionAttr.IsComputed() {
		t.Fatalf("agent_version should be computed")
	}
	if agentVersionAttr.IsOptional() {
		t.Fatalf("agent_version should not be optional")
	}

	onboardAttr, ok := s.Attributes["onboard_manifest_version"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("onboard_manifest_version attribute has unexpected type %T", s.Attributes["onboard_manifest_version"])
	}
	if !onboardAttr.IsComputed() {
		t.Fatalf("onboard_manifest_version should be computed")
	}
	if onboardAttr.IsOptional() {
		t.Fatalf("onboard_manifest_version should not be optional")
	}

	needUpgradeAttr, ok := s.Attributes["need_upgrade"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("need_upgrade attribute has unexpected type %T", s.Attributes["need_upgrade"])
	}
	if !needUpgradeAttr.IsComputed() {
		t.Fatalf("need_upgrade should be computed")
	}
	if needUpgradeAttr.IsOptional() {
		t.Fatalf("need_upgrade should not be optional")
	}
}

func TestNeedUpgradeUsesStateForUnknownBoolPlanModifier(t *testing.T) {
	attr, ok := Schema(context.Background()).Attributes["need_upgrade"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("need_upgrade attribute has unexpected type %T", Schema(context.Background()).Attributes["need_upgrade"])
	}
	if len(attr.BoolPlanModifiers()) == 0 {
		t.Fatalf("need_upgrade should have a bool plan modifier")
	}
}

func TestUseStateForUnknownBoolPreservesPriorState(t *testing.T) {
	resp := &schemaplanmodifier.BoolResponse{
		PlanValue: types.BoolUnknown(),
	}

	useStateForUnknownBool().PlanModifyBool(context.Background(), schemaplanmodifier.BoolRequest{
		State: tfsdk.State{
			Raw: tftypes.NewValue(
				tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"attr": tftypes.Bool,
					},
				},
				map[string]tftypes.Value{
					"attr": tftypes.NewValue(tftypes.Bool, true),
				},
			),
		},
		ConfigValue: types.BoolNull(),
		StateValue:  types.BoolValue(true),
		PlanValue:   types.BoolUnknown(),
	}, resp)

	if resp.PlanValue != types.BoolValue(true) {
		t.Fatalf("PlanValue = %#v, want prior state true", resp.PlanValue)
	}
}

func TestApplyClusterSummaryStatusPopulatesUpgradeFields(t *testing.T) {
	data := &ClusterModel{}

	applyClusterSummaryStatus(data, &api.ClusterCostsSummary{
		AgentVersion:           "v1.18.6",
		OnboardManifestVersion: "v1.18.7",
		NeedUpgrade:            true,
	})

	if data.AgentVersion != types.StringValue("v1.18.6") {
		t.Fatalf("AgentVersion = %#v, want v1.18.6", data.AgentVersion)
	}
	if data.OnboardManifestVersion != types.StringValue("v1.18.7") {
		t.Fatalf("OnboardManifestVersion = %#v, want v1.18.7", data.OnboardManifestVersion)
	}
	if data.NeedUpgrade != types.BoolValue(true) {
		t.Fatalf("NeedUpgrade = %#v, want true", data.NeedUpgrade)
	}
}

func TestRefreshClusterSummaryStatusOverwritesStaleUpgradeFields(t *testing.T) {
	data := &ClusterModel{}
	applyClusterSummaryStatus(data, &api.ClusterCostsSummary{
		AgentVersion:           "v1.18.6",
		OnboardManifestVersion: "v1.18.7",
		NeedUpgrade:            true,
	})

	client := &fakeClusterSummaryClient{
		summaries: []*api.ClusterCostsSummary{
			{
				AgentVersion:           "v1.18.7",
				OnboardManifestVersion: "v1.18.7",
				NeedUpgrade:            false,
			},
		},
	}

	if err := refreshClusterSummaryStatus(data, client, "cluster-1"); err != nil {
		t.Fatalf("refreshClusterSummaryStatus() error = %v", err)
	}
	if client.getClusterCalls != 1 {
		t.Fatalf("GetCluster calls = %d, want 1", client.getClusterCalls)
	}
	if data.AgentVersion != types.StringValue("v1.18.7") {
		t.Fatalf("AgentVersion = %#v, want v1.18.7", data.AgentVersion)
	}
	if data.OnboardManifestVersion != types.StringValue("v1.18.7") {
		t.Fatalf("OnboardManifestVersion = %#v, want v1.18.7", data.OnboardManifestVersion)
	}
	if data.NeedUpgrade != types.BoolValue(false) {
		t.Fatalf("NeedUpgrade = %#v, want false", data.NeedUpgrade)
	}
}

func (f *fakeConfigurationPreflightClient) ListNodePoolsContext(_ context.Context, clusterID string) (api.RebalanceNodePoolList, error) {
	return f.ListNodePools(clusterID)
}

func (f *fakeConfigurationPreflightClient) DeleteNodePoolContext(_ context.Context, clusterID, name string) error {
	return f.DeleteNodePool(clusterID, name)
}
