package eks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	schemaplanmodifier "github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	cloudpilotaiclient "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
	awsauth "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/utils/aws"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
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

func TestRunUpgradeActionAndRefreshClusterSummaryRefreshesAfterUpgrade(t *testing.T) {
	calls := make([]string, 0, 2)
	data := &ClusterModel{}
	client := &fakeClusterSummaryClient{
		summary: &api.ClusterCostsSummary{
			AgentVersion:           "v1.18.6",
			OnboardManifestVersion: "v1.18.7",
			NeedUpgrade:            true,
		},
		onGetCluster: func() {
			calls = append(calls, "refresh")
		},
	}

	applyClusterSummaryStatus(data, client.summary)

	upgradeAction := func() error {
		calls = append(calls, "upgrade")
		client.summary = &api.ClusterCostsSummary{
			AgentVersion:           "v1.18.7",
			OnboardManifestVersion: "v1.18.7",
			NeedUpgrade:            false,
		}
		return nil
	}

	if err := runUpgradeActionAndRefreshClusterSummary(data, client, "cluster-1", upgradeAction); err != nil {
		t.Fatalf("runUpgradeActionAndRefreshClusterSummary() error = %v", err)
	}

	if client.getClusterCalls != 1 {
		t.Fatalf("GetCluster calls = %d, want 1", client.getClusterCalls)
	}
	if len(calls) != 2 || calls[0] != "upgrade" || calls[1] != "refresh" {
		t.Fatalf("call order = %v, want [upgrade refresh]", calls)
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

func TestModifyPlanRefreshesUpgradeStatusFromRemoteSummary(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterSummaryClient{
		summary: &api.ClusterCostsSummary{
			AgentVersion:           "v1.18.6",
			OnboardManifestVersion: "v1.18.7",
			NeedUpgrade:            true,
		},
	}

	plan := tfsdk.Plan{Schema: Schema(ctx)}
	planDiags := plan.Set(ctx, &ClusterModel{
		ClusterID:              types.StringUnknown(),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringNull(),
		OnboardManifestVersion: types.StringNull(),
		NeedUpgrade:            types.BoolNull(),
	})
	if planDiags.HasError() {
		t.Fatalf("plan.Set() diagnostics = %v", planDiags)
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	stateDiags := state.Set(ctx, &ClusterModel{
		ClusterID:              types.StringValue("server-imported-id"),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringValue("123456789012"),
		OnboardManifestVersion: types.StringValue("old-manifest"),
		NeedUpgrade:            types.BoolValue(false),
	})
	if stateDiags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", stateDiags)
	}

	resp := &resource.ModifyPlanResponse{
		Plan: tfsdk.Plan{Schema: Schema(ctx)},
	}
	(&Cluster{client: client}).ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:  plan,
		State: state,
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan() diagnostics = %v", resp.Diagnostics)
	}
	if client.getClusterCalls != 1 {
		t.Fatalf("GetCluster calls = %d, want 1", client.getClusterCalls)
	}

	var got ClusterModel
	getDiags := resp.Plan.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.Plan.Get() diagnostics = %v", getDiags)
	}
	if got.OnboardManifestVersion != types.StringValue("v1.18.7") {
		t.Fatalf("OnboardManifestVersion = %#v, want v1.18.7", got.OnboardManifestVersion)
	}
	if got.NeedUpgrade != types.BoolValue(true) {
		t.Fatalf("NeedUpgrade = %#v, want true", got.NeedUpgrade)
	}
}

func TestModifyPlanLeavesUpgradeStatusUnknownWhenUpgradeWillRun(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterSummaryClient{
		summary: &api.ClusterCostsSummary{
			AgentVersion:           "v1.20.0",
			OnboardManifestVersion: "v1.20.1",
			NeedUpgrade:            true,
		},
	}

	plan := tfsdk.Plan{Schema: Schema(ctx)}
	planDiags := plan.Set(ctx, &ClusterModel{
		ClusterID:              types.StringUnknown(),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringNull(),
		EnableUpgrade:          types.BoolValue(true),
		AgentVersion:           types.StringValue("v1.20.0"),
		OnboardManifestVersion: types.StringValue("v1.20.1"),
		NeedUpgrade:            types.BoolValue(true),
	})
	if planDiags.HasError() {
		t.Fatalf("plan.Set() diagnostics = %v", planDiags)
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	stateDiags := state.Set(ctx, &ClusterModel{
		ClusterID:              types.StringValue("server-imported-id"),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringValue("123456789012"),
		EnableUpgrade:          types.BoolValue(true),
		AgentVersion:           types.StringValue("v1.20.0"),
		OnboardManifestVersion: types.StringValue("v1.20.1"),
		NeedUpgrade:            types.BoolValue(true),
	})
	if stateDiags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", stateDiags)
	}

	resp := &resource.ModifyPlanResponse{
		Plan: tfsdk.Plan{Schema: Schema(ctx)},
	}
	(&Cluster{client: client}).ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:  plan,
		State: state,
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan() diagnostics = %v", resp.Diagnostics)
	}

	var got ClusterModel
	getDiags := resp.Plan.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.Plan.Get() diagnostics = %v", getDiags)
	}
	if !got.AgentVersion.IsUnknown() {
		t.Fatalf("AgentVersion = %#v, want unknown", got.AgentVersion)
	}
	if !got.OnboardManifestVersion.IsUnknown() {
		t.Fatalf("OnboardManifestVersion = %#v, want unknown", got.OnboardManifestVersion)
	}
	if !got.NeedUpgrade.IsUnknown() {
		t.Fatalf("NeedUpgrade = %#v, want unknown", got.NeedUpgrade)
	}
}

func TestModifyPlanSkipsRefreshWhenClusterNotFound(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterSummaryClient{
		err: cloudpilotaiclient.ErrNotFound,
	}

	plan := tfsdk.Plan{Schema: Schema(ctx)}
	planDiags := plan.Set(ctx, &ClusterModel{
		ClusterID:              types.StringUnknown(),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringNull(),
		AgentVersion:           types.StringValue("planned-agent"),
		OnboardManifestVersion: types.StringValue("planned-manifest"),
		NeedUpgrade:            types.BoolValue(true),
	})
	if planDiags.HasError() {
		t.Fatalf("plan.Set() diagnostics = %v", planDiags)
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	stateDiags := state.Set(ctx, &ClusterModel{
		ClusterID:              types.StringValue("server-imported-id"),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringValue("123456789012"),
		AgentVersion:           types.StringValue("state-agent"),
		OnboardManifestVersion: types.StringValue("state-manifest"),
		NeedUpgrade:            types.BoolValue(false),
	})
	if stateDiags.HasError() {
		t.Fatalf("state.Set() diagnostics = %v", stateDiags)
	}

	resp := &resource.ModifyPlanResponse{
		Plan: plan,
	}
	(&Cluster{client: client}).ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:  plan,
		State: state,
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan() diagnostics = %v", resp.Diagnostics)
	}
	if client.getClusterCalls != 1 {
		t.Fatalf("GetCluster calls = %d, want 1", client.getClusterCalls)
	}

	var got ClusterModel
	getDiags := resp.Plan.Get(ctx, &got)
	if getDiags.HasError() {
		t.Fatalf("resp.Plan.Get() diagnostics = %v", getDiags)
	}
	if got.AgentVersion != types.StringValue("planned-agent") {
		t.Fatalf("AgentVersion = %#v, want planned-agent", got.AgentVersion)
	}
	if got.OnboardManifestVersion != types.StringValue("planned-manifest") {
		t.Fatalf("OnboardManifestVersion = %#v, want planned-manifest", got.OnboardManifestVersion)
	}
	if got.NeedUpgrade != types.BoolValue(true) {
		t.Fatalf("NeedUpgrade = %#v, want true", got.NeedUpgrade)
	}
}

func TestReadRefreshesUpgradeStatusFromRemoteSummary(t *testing.T) {
	ctx := context.Background()
	client := &fakeReadClusterClient{
		summary: &api.ClusterCostsSummary{
			AgentVersion:           "v1.18.6",
			OnboardManifestVersion: "v1.18.7",
			NeedUpgrade:            true,
		},
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	diags := state.Set(ctx, &ClusterModel{
		ClusterID:              types.StringValue("cluster-1"),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringValue("123456789012"),
		Kubeconfig:             types.StringValue("/tmp/kubeconfig"),
		AgentVersion:           types.StringValue("old-agent"),
		OnboardManifestVersion: types.StringValue("old-manifest"),
		NeedUpgrade:            types.BoolValue(false),
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
	if got.AgentVersion != types.StringValue("v1.18.6") {
		t.Fatalf("AgentVersion = %#v, want v1.18.6", got.AgentVersion)
	}
	if got.OnboardManifestVersion != types.StringValue("v1.18.7") {
		t.Fatalf("OnboardManifestVersion = %#v, want v1.18.7", got.OnboardManifestVersion)
	}
	if got.NeedUpgrade != types.BoolValue(true) {
		t.Fatalf("NeedUpgrade = %#v, want true", got.NeedUpgrade)
	}
}

func TestReadKeepsStateWhenClusterNotFound(t *testing.T) {
	ctx := context.Background()
	client := &fakeReadClusterClient{
		getClusterErr: cloudpilotaiclient.ErrNotFound,
	}

	state := tfsdk.State{Schema: Schema(ctx)}
	diags := state.Set(ctx, &ClusterModel{
		ClusterID:              types.StringValue("cluster-1"),
		ClusterName:            types.StringValue("demo"),
		Region:                 types.StringValue("us-east-2"),
		AccountID:              types.StringValue("123456789012"),
		Kubeconfig:             types.StringValue("/tmp/kubeconfig"),
		AgentVersion:           types.StringValue("state-agent"),
		OnboardManifestVersion: types.StringValue("state-manifest"),
		NeedUpgrade:            types.BoolValue(true),
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
	if got.ClusterID != types.StringValue("cluster-1") {
		t.Fatalf("ClusterID = %#v, want cluster-1", got.ClusterID)
	}
	if got.AgentVersion != types.StringValue("state-agent") {
		t.Fatalf("AgentVersion = %#v, want state-agent", got.AgentVersion)
	}
	if got.OnboardManifestVersion != types.StringValue("state-manifest") {
		t.Fatalf("OnboardManifestVersion = %#v, want state-manifest", got.OnboardManifestVersion)
	}
	if got.NeedUpgrade != types.BoolValue(true) {
		t.Fatalf("NeedUpgrade = %#v, want true", got.NeedUpgrade)
	}
}

func TestUpgradeStatusSchemaModifiersPreservePriorState(t *testing.T) {
	s := Schema(context.Background())

	needUpgradeAttr, ok := s.Attributes["need_upgrade"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("need_upgrade attribute has unexpected type %T", s.Attributes["need_upgrade"])
	}
	boolModifiers := needUpgradeAttr.BoolPlanModifiers()
	if len(boolModifiers) == 0 {
		t.Fatalf("need_upgrade should have a bool plan modifier")
	}

	boolResp := &schemaplanmodifier.BoolResponse{
		PlanValue: types.BoolUnknown(),
	}
	boolModifiers[0].PlanModifyBool(context.Background(), schemaplanmodifier.BoolRequest{
		State: tfsdk.State{
			Raw: tftypes.NewValue(
				tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"need_upgrade": tftypes.Bool,
					},
				},
				map[string]tftypes.Value{
					"need_upgrade": tftypes.NewValue(tftypes.Bool, true),
				},
			),
		},
		ConfigValue: types.BoolNull(),
		StateValue:  types.BoolValue(true),
		PlanValue:   types.BoolUnknown(),
	}, boolResp)
	if boolResp.PlanValue != types.BoolValue(true) {
		t.Fatalf("need_upgrade PlanValue = %#v, want prior state true", boolResp.PlanValue)
	}

	stringTests := []struct {
		name  string
		value string
	}{
		{name: "agent_version", value: "v1.2.3"},
		{name: "onboard_manifest_version", value: "manifest-2026-06-15"},
	}

	for _, tt := range stringTests {
		t.Run(tt.name, func(t *testing.T) {
			attr, ok := s.Attributes[tt.name].(schema.StringAttribute)
			if !ok {
				t.Fatalf("%s attribute has unexpected type %T", tt.name, s.Attributes[tt.name])
			}
			modifiers := attr.StringPlanModifiers()
			if len(modifiers) == 0 {
				t.Fatalf("%s should have a string plan modifier", tt.name)
			}

			resp := &schemaplanmodifier.StringResponse{
				PlanValue: types.StringUnknown(),
			}
			modifiers[0].PlanModifyString(context.Background(), schemaplanmodifier.StringRequest{
				State: tfsdk.State{
					Raw: tftypes.NewValue(
						tftypes.Object{
							AttributeTypes: map[string]tftypes.Type{
								tt.name: tftypes.String,
							},
						},
						map[string]tftypes.Value{
							tt.name: tftypes.NewValue(tftypes.String, tt.value),
						},
					),
				},
				ConfigValue: types.StringNull(),
				StateValue:  types.StringValue(tt.value),
				PlanValue:   types.StringUnknown(),
			}, resp)

			if resp.PlanValue != types.StringValue(tt.value) {
				t.Fatalf("%s PlanValue = %#v, want prior state %q", tt.name, resp.PlanValue, tt.value)
			}
		})
	}
}

func TestUseStateForUnknownInt64PreservesNullState(t *testing.T) {
	resp := &schemaplanmodifier.Int64Response{
		PlanValue: types.Int64Unknown(),
	}

	useStateForUnknownInt64().PlanModifyInt64(context.Background(), schemaplanmodifier.Int64Request{
		State: tfsdk.State{
			Raw: tftypes.NewValue(
				tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"attr": tftypes.Number,
					},
				},
				map[string]tftypes.Value{
					"attr": tftypes.NewValue(tftypes.Number, nil),
				},
			),
		},
		StateValue:  types.Int64Null(),
		PlanValue:   types.Int64Unknown(),
		ConfigValue: types.Int64Null(),
	}, resp)

	if !resp.PlanValue.IsNull() {
		t.Fatalf("plan value should remain null, got %v", resp.PlanValue)
	}
}

func TestUseStateForUnknownStringPreservesNullState(t *testing.T) {
	resp := &schemaplanmodifier.StringResponse{
		PlanValue: types.StringUnknown(),
	}

	useStateForUnknownString().PlanModifyString(context.Background(), schemaplanmodifier.StringRequest{
		State: tfsdk.State{
			Raw: tftypes.NewValue(
				tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"attr": tftypes.String,
					},
				},
				map[string]tftypes.Value{
					"attr": tftypes.NewValue(tftypes.String, nil),
				},
			),
		},
		StateValue:  types.StringNull(),
		PlanValue:   types.StringUnknown(),
		ConfigValue: types.StringNull(),
	}, resp)

	if !resp.PlanValue.IsNull() {
		t.Fatalf("plan value should remain null, got %v", resp.PlanValue)
	}
}

func TestOperationalStringAttributesDoNotPreserveNullState(t *testing.T) {
	kubeconfigAttr, ok := Schema(context.Background()).Attributes["kubeconfig"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("kubeconfig attribute has unexpected type %T", Schema(context.Background()).Attributes["kubeconfig"])
	}
	if !kubeconfigAttr.IsOptional() || kubeconfigAttr.IsComputed() {
		t.Fatalf("kubeconfig must be optional-only, got optional=%v computed=%v", kubeconfigAttr.IsOptional(), kubeconfigAttr.IsComputed())
	}
	if len(kubeconfigAttr.StringPlanModifiers()) != 0 {
		t.Fatal("kubeconfig must not preserve an execution-local path from prior state")
	}

	tests := []string{"account_id"}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			attr, ok := Schema(context.Background()).Attributes[name].(schema.StringAttribute)
			if !ok {
				t.Fatalf("%s attribute has unexpected type %T", name, Schema(context.Background()).Attributes[name])
			}
			modifiers := attr.StringPlanModifiers()
			if len(modifiers) == 0 {
				t.Fatalf("%s should define a string plan modifier", name)
			}

			resp := &schemaplanmodifier.StringResponse{
				PlanValue: types.StringUnknown(),
			}
			modifiers[0].PlanModifyString(context.Background(), schemaplanmodifier.StringRequest{
				State: tfsdk.State{
					Raw: tftypes.NewValue(
						tftypes.Object{
							AttributeTypes: map[string]tftypes.Type{
								name: tftypes.String,
							},
						},
						map[string]tftypes.Value{
							name: tftypes.NewValue(tftypes.String, nil),
						},
					),
				},
				StateValue:  types.StringNull(),
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringNull(),
			}, resp)

			if !resp.PlanValue.IsUnknown() {
				t.Fatalf("%s plan value should stay unknown so apply can backfill it, got %v", name, resp.PlanValue)
			}
		})
	}
}

func TestNodeClassSchemaIncludesFrontendFields(t *testing.T) {
	s := Schema(context.Background())
	nodeClassesAttr := s.Attributes["nodeclasses"].(schema.ListNestedAttribute)
	attrs := nodeClassesAttr.NestedObject.Attributes
	for _, name := range []string{"ami_alias", "user_data", "block_device_mappings"} {
		if _, ok := attrs[name]; !ok {
			t.Fatalf("nodeclasses schema missing %s", name)
		}
	}
}

func TestNodeClassBlockDeviceMappingsSchemaMatchesFrontendSurface(t *testing.T) {
	s := Schema(context.Background())
	nodeClassesAttr := s.Attributes["nodeclasses"].(schema.ListNestedAttribute)
	blockDeviceMappingsAttr, ok := nodeClassesAttr.NestedObject.Attributes["block_device_mappings"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("block_device_mappings attribute has unexpected type %T", nodeClassesAttr.NestedObject.Attributes["block_device_mappings"])
	}
	ebsAttr, ok := blockDeviceMappingsAttr.NestedObject.Attributes["ebs"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("ebs attribute has unexpected type %T", blockDeviceMappingsAttr.NestedObject.Attributes["ebs"])
	}
	for _, name := range []string{"encrypted", "volume_size", "volume_type"} {
		if _, ok := ebsAttr.Attributes[name]; !ok {
			t.Fatalf("ebs schema missing %s", name)
		}
	}
	for _, name := range []string{"delete_on_termination", "iops", "kms_key_id", "snapshot_id", "throughput"} {
		if _, ok := ebsAttr.Attributes[name]; ok {
			t.Fatalf("ebs schema should not expose %s", name)
		}
	}
}

func TestNodeClassFrontendStringFieldsHaveNoDefault(t *testing.T) {
	s := Schema(context.Background())
	nodeClassesAttr := s.Attributes["nodeclasses"].(schema.ListNestedAttribute)
	attrs := nodeClassesAttr.NestedObject.Attributes

	amiAlias, ok := attrs["ami_alias"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("ami_alias attribute has unexpected type %T", attrs["ami_alias"])
	}
	if amiAlias.StringDefaultValue() != nil {
		t.Fatalf("ami_alias should not have a schema default")
	}

	userData, ok := attrs["user_data"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("user_data attribute has unexpected type %T", attrs["user_data"])
	}
	if userData.StringDefaultValue() != nil {
		t.Fatalf("user_data should not have a schema default")
	}
}

func TestPreserveNodeClassStateRepresentationKeepsSystemDiskConvenience(t *testing.T) {
	ctx := context.Background()
	remote := api.EC2NodeClassModel{
		Name: types.StringValue("cloudpilot"),
		BlockDeviceMappings: customfield.NewObjectListMust(ctx, []api.BlockDeviceMappingModel{{
			DeviceName: types.StringValue("/dev/xvda"),
			EBS: customfield.NewObjectMust(ctx, &api.BlockDeviceModel{
				VolumeSize: types.StringValue("64Gi"),
			}),
		}}),
	}
	state := api.EC2NodeClassModel{
		Name:              types.StringValue("cloudpilot"),
		SystemDiskSizeGib: types.Int64Value(20),
	}

	got, err := preserveNodeClassStateRepresentation(ctx, remote, state)
	if err != nil {
		t.Fatalf("preserveNodeClassStateRepresentation() error = %v", err)
	}
	if got.SystemDiskSizeGib.ValueInt64() != 64 {
		t.Fatalf("SystemDiskSizeGib = %d, want 64", got.SystemDiskSizeGib.ValueInt64())
	}
	if !got.BlockDeviceMappings.IsNull() {
		t.Fatalf("BlockDeviceMappings should stay null for system_disk_size_gib representation")
	}
}

func TestPreserveNodeClassStateRepresentationLeavesBlockDeviceMappingsNullWhenOmitted(t *testing.T) {
	ctx := context.Background()
	remote := api.EC2NodeClassModel{
		Name: types.StringValue("cloudpilot"),
		BlockDeviceMappings: customfield.NewObjectListMust(ctx, []api.BlockDeviceMappingModel{{
			DeviceName: types.StringValue("/dev/xvda"),
			EBS: customfield.NewObjectMust(ctx, &api.BlockDeviceModel{
				VolumeSize: types.StringValue("64Gi"),
			}),
		}}),
	}
	state := api.EC2NodeClassModel{
		Name: types.StringValue("cloudpilot"),
	}

	got, err := preserveNodeClassStateRepresentation(ctx, remote, state)
	if err != nil {
		t.Fatalf("preserveNodeClassStateRepresentation() error = %v", err)
	}
	if !got.BlockDeviceMappings.IsNull() {
		t.Fatalf("BlockDeviceMappings should remain null when block_device_mappings is omitted from state")
	}
}

func TestNodePoolSchemaIncludesLabelsAndTaints(t *testing.T) {
	s := Schema(context.Background())
	nodePoolsAttr := s.Attributes["nodepools"].(schema.ListNestedAttribute)
	attrs := nodePoolsAttr.NestedObject.Attributes
	for _, name := range []string{"labels", "taints"} {
		if _, ok := attrs[name]; !ok {
			t.Fatalf("nodepools schema missing %s", name)
		}
	}
}

func TestPreserveNodePoolStateRepresentationLeavesLabelsAndTaintsNullWhenOmitted(t *testing.T) {
	ctx := context.Background()
	remote := api.EC2NodePoolModel{
		Name: types.StringValue("cloudpilot-general"),
		Labels: customfield.NewMapMust[types.String](ctx, map[string]types.String{
			"team": types.StringValue("platform"),
		}),
		Taints: customfield.NewObjectListMust(ctx, []api.TaintModel{{
			Key:    types.StringValue("dedicated"),
			Value:  types.StringValue("wa"),
			Effect: types.StringValue("NoSchedule"),
		}}),
	}
	state := api.EC2NodePoolModel{
		Name: types.StringValue("cloudpilot-general"),
	}

	got := preserveNodePoolStateRepresentation(ctx, remote, state)
	if !got.Labels.IsNull() {
		t.Fatalf("Labels should remain null when labels are omitted from state")
	}
	if !got.Taints.IsNull() {
		t.Fatalf("Taints should remain null when taints are omitted from state")
	}
}

func TestPreserveNodePoolStateRepresentationKeepsEmptyTaintsList(t *testing.T) {
	ctx := context.Background()
	remote := api.EC2NodePoolModel{
		Name:   types.StringValue("cloudpilot-general"),
		Taints: customfield.NullObjectList[api.TaintModel](ctx),
	}
	state := api.EC2NodePoolModel{
		Name:   types.StringValue("cloudpilot-general"),
		Taints: customfield.NewObjectListMust(ctx, []api.TaintModel{}),
	}

	got := preserveNodePoolStateRepresentation(ctx, remote, state)
	if got.Taints.IsNull() {
		t.Fatalf("Taints should preserve an explicit empty list from state")
	}
	taints, diags := got.Taints.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("Taints diagnostics = %v", diags)
	}
	if len(taints) != 0 {
		t.Fatalf("expected empty taints list, got %#v", taints)
	}
}

func TestClusterSettingModelToAPIOnlySendsConfiguredValues(t *testing.T) {
	model := ClusterSettingModel{
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
}

func TestClusterSettingObjectFromAPI(t *testing.T) {
	ctx := context.Background()
	enableRepair := true
	enableDisk := false
	discount := 0.1
	pre := "echo pre"
	post := "echo post"

	got := clusterSettingObjectFromAPI(ctx, &api.ClusterSetting{
		EnableNodeRepair:  &enableRepair,
		EnableDiskMonitor: &enableDisk,
		Discount:          &discount,
		PreRunCommand:     &pre,
		PostRunCommand:    &post,
	})

	value, diags := got.Value(ctx)
	if diags.HasError() {
		t.Fatalf("cluster setting diagnostics = %v", diags)
	}
	if value == nil {
		t.Fatalf("cluster setting should not be nil")
	}
	if !value.EnableNodeRepair.ValueBool() {
		t.Fatalf("EnableNodeRepair should be true")
	}
	if value.PreRunCommand.ValueString() != "echo pre" {
		t.Fatalf("PreRunCommand = %q", value.PreRunCommand.ValueString())
	}
}

func TestClusterSettingSchemaHasExpectedFields(t *testing.T) {
	s := Schema(context.Background())
	clusterSettingAttr, ok := s.Attributes["cluster_setting"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("cluster_setting attribute has unexpected type %T", s.Attributes["cluster_setting"])
	}
	for _, name := range []string{
		"enable_node_repair",
		"enable_disk_monitor",
		"discount",
		"pre_run_command",
		"post_run_command",
	} {
		if _, ok := clusterSettingAttr.Attributes[name]; !ok {
			t.Fatalf("cluster_setting schema missing %s", name)
		}
	}
	if _, ok := clusterSettingAttr.Attributes["maintenance_enabled"]; ok {
		t.Fatalf("cluster_setting schema should not expose maintenance_enabled")
	}
}

func TestClusterSchemaDoesNotExposeEnableDiversityInstanceType(t *testing.T) {
	s := Schema(context.Background())
	if _, ok := s.Attributes["enable_diversity_instance_type"]; ok {
		t.Fatalf("schema should not expose enable_diversity_instance_type")
	}
}

func TestClusterSchemaDoesNotExposeEnableUploadConfig(t *testing.T) {
	s := Schema(context.Background())
	if _, ok := s.Attributes["enable_upload_config"]; ok {
		t.Fatalf("schema should not expose enable_upload_config")
	}
}

func TestHydratePostWriteStateRefreshesConfiguredClusterSetting(t *testing.T) {
	ctx := context.Background()
	enableNodeRepair := true
	discount := 0.15
	data := ClusterModel{
		ClusterSetting: customfield.NewObjectMust(ctx, &ClusterSettingModel{
			EnableNodeRepair:  types.BoolUnknown(),
			EnableDiskMonitor: types.BoolUnknown(),
			Discount:          types.Float64Value(discount),
			PreRunCommand:     types.StringUnknown(),
			PostRunCommand:    types.StringUnknown(),
		}),
	}

	err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{
		clusterSetting: &api.ClusterSetting{
			EnableNodeRepair: &enableNodeRepair,
			Discount:         &discount,
		},
	}, "cluster-1", &data)
	if err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	value, diags := data.ClusterSetting.Value(ctx)
	if diags.HasError() {
		t.Fatalf("cluster setting diagnostics = %v", diags)
	}
	if value == nil {
		t.Fatal("cluster setting should not be nil")
	}
	if value.EnableNodeRepair.IsUnknown() || !value.EnableNodeRepair.ValueBool() {
		t.Fatalf("EnableNodeRepair = %#v, want known true", value.EnableNodeRepair)
	}
	if value.EnableDiskMonitor.IsUnknown() {
		t.Fatalf("EnableDiskMonitor should not remain unknown")
	}
	if value.PreRunCommand.IsUnknown() || value.PostRunCommand.IsUnknown() {
		t.Fatalf("cluster setting string siblings should not remain unknown")
	}
}

func TestSyncClusterSettingSendsEmptyPreAndPostCommands(t *testing.T) {
	ctx := context.Background()
	client := &fakeClusterSettingUpdateClient{}
	data := ClusterModel{
		ClusterSetting: customfield.NewObjectMust(ctx, &ClusterSettingModel{
			PreRunCommand:  types.StringValue(""),
			PostRunCommand: types.StringValue(""),
		}),
	}

	if err := (&Cluster{client: client}).syncClusterSetting(ctx, &data, "cluster-1"); err != nil {
		t.Fatalf("syncClusterSetting() error = %v", err)
	}

	if client.setting == nil {
		t.Fatalf("UpdateClusterSetting was not called")
	}
	if client.setting.PreRunCommand == nil || *client.setting.PreRunCommand != "" {
		t.Fatalf("PreRunCommand = %#v, want empty string pointer", client.setting.PreRunCommand)
	}
	if client.setting.PostRunCommand == nil || *client.setting.PostRunCommand != "" {
		t.Fatalf("PostRunCommand = %#v, want empty string pointer", client.setting.PostRunCommand)
	}
}

func TestHydratePostWriteStateRefreshesNodeClassesFromServer(t *testing.T) {
	ctx := context.Background()
	userData := "echo existing"
	data := ClusterModel{
		NodeClasses: customfield.NewObjectListMust(ctx, []api.EC2NodeClassModel{{
			Name:         types.StringValue("cloudpilot"),
			TemplateName: types.StringValue("default"),
			AmiAlias:     types.StringUnknown(),
			UserData:     types.StringUnknown(),
		}}),
	}

	err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{
		nodeClasses: api.RebalanceNodeClassList{
			EC2NodeClasses: []api.EC2NodeClass{{
				Name: "cloudpilot",
				NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{
					AMISelectorTerms: []awsproviderv1.AMISelectorTerm{{Alias: "al2023@latest"}},
					UserData:         &userData,
				},
			}},
		},
	}, "cluster-1", &data)
	if err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	nodeClasses, diags := data.NodeClasses.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("nodeclasses diagnostics = %v", diags)
	}
	if len(nodeClasses) != 1 {
		t.Fatalf("got %d nodeclasses, want 1", len(nodeClasses))
	}
	if nodeClasses[0].AmiAlias.IsUnknown() || nodeClasses[0].AmiAlias.ValueString() != "al2023@latest" {
		t.Fatalf("AmiAlias = %#v, want known al2023@latest", nodeClasses[0].AmiAlias)
	}
	if nodeClasses[0].UserData.IsUnknown() || nodeClasses[0].UserData.ValueString() != userData {
		t.Fatalf("UserData = %#v, want known %q", nodeClasses[0].UserData, userData)
	}
	if nodeClasses[0].TemplateName.ValueString() != "default" {
		t.Fatalf("TemplateName = %q, want default", nodeClasses[0].TemplateName.ValueString())
	}
}

func TestHydratePostWriteStateLeavesMissingNodeClassStringsNull(t *testing.T) {
	ctx := context.Background()
	data := ClusterModel{
		NodeClasses: customfield.NewObjectListMust(ctx, []api.EC2NodeClassModel{{
			Name:     types.StringValue("cloudpilot"),
			AmiAlias: types.StringUnknown(),
			UserData: types.StringUnknown(),
		}}),
	}

	err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{
		nodeClasses: api.RebalanceNodeClassList{
			EC2NodeClasses: []api.EC2NodeClass{{
				Name:          "cloudpilot",
				NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{},
			}},
		},
	}, "cluster-1", &data)
	if err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	nodeClasses, diags := data.NodeClasses.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("nodeclasses diagnostics = %v", diags)
	}
	if !nodeClasses[0].AmiAlias.IsNull() {
		t.Fatalf("AmiAlias should be null after hydration, got %#v", nodeClasses[0].AmiAlias)
	}
	if !nodeClasses[0].UserData.IsNull() {
		t.Fatalf("UserData should be null after hydration, got %#v", nodeClasses[0].UserData)
	}
}

func TestHydratePostWriteStateNormalizesUnknownNodeClassTemplateStrings(t *testing.T) {
	ctx := context.Background()
	data := ClusterModel{
		NodeClassTemplates: customfield.NewObjectListMust(ctx, []api.EC2NodeClassTemplateModel{{
			TemplateName: types.StringValue("default"),
			AmiAlias:     types.StringUnknown(),
			UserData:     types.StringUnknown(),
		}}),
	}

	if err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{}, "cluster-1", &data); err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	templates, diags := data.NodeClassTemplates.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("nodeclass templates diagnostics = %v", diags)
	}
	if len(templates) != 1 {
		t.Fatalf("got %d nodeclass templates, want 1", len(templates))
	}
	if !templates[0].AmiAlias.IsNull() {
		t.Fatalf("AmiAlias should be null after hydration, got %#v", templates[0].AmiAlias)
	}
	if !templates[0].UserData.IsNull() {
		t.Fatalf("UserData should be null after hydration, got %#v", templates[0].UserData)
	}
}

func TestHydratePostWriteStateRefreshesNodePoolsFromServer(t *testing.T) {
	ctx := context.Background()
	data := ClusterModel{
		NodePools: customfield.NewObjectListMust(ctx, []api.EC2NodePoolModel{{
			Name:              types.StringValue("cloudpilot-general"),
			TemplateName:      types.StringValue("default"),
			InstanceCPUMIN:    types.Int64Unknown(),
			InstanceMemoryMIN: types.Int64Unknown(),
		}}),
	}

	err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{
		nodePools: api.RebalanceNodePoolList{
			EC2NodePools: []api.EC2NodePool{{
				Name:         "cloudpilot-general",
				NodePoolSpec: api.DefaultGeneralEC2NodePoolSpec(),
			}},
		},
	}, "cluster-1", &data)
	if err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	nodePools, diags := data.NodePools.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("nodepools diagnostics = %v", diags)
	}
	if len(nodePools) != 1 {
		t.Fatalf("got %d nodepools, want 1", len(nodePools))
	}
	if nodePools[0].InstanceCPUMIN.IsUnknown() {
		t.Fatalf("InstanceCPUMIN should not remain unknown")
	}
	if !nodePools[0].InstanceCPUMIN.IsNull() {
		t.Fatalf("InstanceCPUMIN should be null when server omits the minimum filter, got %#v", nodePools[0].InstanceCPUMIN)
	}
	if nodePools[0].InstanceMemoryMIN.IsUnknown() {
		t.Fatalf("InstanceMemoryMIN should not remain unknown")
	}
	if !nodePools[0].InstanceMemoryMIN.IsNull() {
		t.Fatalf("InstanceMemoryMIN should be null when server omits the minimum filter, got %#v", nodePools[0].InstanceMemoryMIN)
	}
	if nodePools[0].TemplateName.ValueString() != "default" {
		t.Fatalf("TemplateName = %q, want default", nodePools[0].TemplateName.ValueString())
	}
}

func TestHydratePostWriteStateNormalizesUnknownNodePoolTemplateMinimums(t *testing.T) {
	ctx := context.Background()
	data := ClusterModel{
		NodePoolTemplates: customfield.NewObjectListMust(ctx, []api.EC2NodePoolTemplateModel{{
			TemplateName:      types.StringValue("default"),
			InstanceCPUMIN:    types.Int64Unknown(),
			InstanceMemoryMIN: types.Int64Unknown(),
		}}),
	}

	if err := hydratePostWriteState(ctx, &fakePostWriteStateHydratorClient{}, "cluster-1", &data); err != nil {
		t.Fatalf("hydratePostWriteState() error = %v", err)
	}

	templates, diags := data.NodePoolTemplates.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("nodepool templates diagnostics = %v", diags)
	}
	if len(templates) != 1 {
		t.Fatalf("got %d nodepool templates, want 1", len(templates))
	}
	if !templates[0].InstanceCPUMIN.IsNull() {
		t.Fatalf("InstanceCPUMIN should be null after hydration, got %#v", templates[0].InstanceCPUMIN)
	}
	if !templates[0].InstanceMemoryMIN.IsNull() {
		t.Fatalf("InstanceMemoryMIN should be null after hydration, got %#v", templates[0].InstanceMemoryMIN)
	}
}
