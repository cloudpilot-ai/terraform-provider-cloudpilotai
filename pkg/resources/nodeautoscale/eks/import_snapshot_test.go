package eks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestImportedClusterSettingOmitsEmptyCommands(t *testing.T) {
	ctx := context.Background()
	empty, command := "", "echo ready"
	enabled := true
	for _, test := range []struct {
		name              string
		remote            *api.ClusterSetting
		wantPre, wantPost types.String
		wantNull          bool
	}{
		{name: "empty commands only", remote: &api.ClusterSetting{PreRunCommand: &empty, PostRunCommand: &empty}, wantNull: true},
		{name: "empty commands with settings", remote: &api.ClusterSetting{EnableNodeRepair: &enabled, PreRunCommand: &empty, PostRunCommand: &empty}, wantPre: types.StringNull(), wantPost: types.StringNull()},
		{name: "nonempty commands", remote: &api.ClusterSetting{PreRunCommand: &command, PostRunCommand: &command}, wantPre: types.StringValue(command), wantPost: types.StringValue(command)},
		{name: "mixed commands", remote: &api.ClusterSetting{PreRunCommand: &empty, PostRunCommand: &command}, wantPre: types.StringNull(), wantPost: types.StringValue(command)},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := clusterSettingObjectFromAPI(ctx, test.remote)
			if got.IsNull() != test.wantNull {
				t.Fatalf("imported cluster setting null = %t, want %t", got.IsNull(), test.wantNull)
			}
			if test.wantNull {
				return
			}
			value, diags := got.Value(ctx)
			if diags.HasError() || value == nil {
				t.Fatalf("imported cluster setting = %#v, diagnostics = %v", value, diags)
			}
			if !value.PreRunCommand.Equal(test.wantPre) || !value.PostRunCommand.Equal(test.wantPost) {
				t.Fatalf("imported commands = (%s, %s), want (%s, %s)", value.PreRunCommand, value.PostRunCommand, test.wantPre, test.wantPost)
			}
		})
	}
}

func TestManagedEmptyCommandsStillClearAndRefreshAsEmpty(t *testing.T) {
	ctx := context.Background()
	current := customfield.NewObjectMust(ctx, &ClusterSettingModel{PreRunCommand: types.StringValue(""), PostRunCommand: types.StringValue("")})
	value, diags := current.Value(ctx)
	if diags.HasError() || value == nil {
		t.Fatalf("configured cluster setting = %#v, diagnostics = %v", value, diags)
	}
	payload := value.ToAPI()
	if payload.PreRunCommand == nil || *payload.PreRunCommand != "" || payload.PostRunCommand == nil || *payload.PostRunCommand != "" {
		t.Fatalf("explicit empty commands were not retained in update payload: %#v", payload)
	}
	got, err := clusterSettingObjectPreservingState(ctx, current, payload)
	if err != nil {
		t.Fatal(err)
	}
	refreshed, diags := got.Value(ctx)
	if diags.HasError() || refreshed == nil {
		t.Fatalf("refreshed cluster setting = %#v, diagnostics = %v", refreshed, diags)
	}
	if !refreshed.PreRunCommand.Equal(types.StringValue("")) || !refreshed.PostRunCommand.Equal(types.StringValue("")) {
		t.Fatalf("managed empty commands changed during refresh: %#v", refreshed)
	}
}

func TestNodePoolImportUsesFullBudgetsWhileManagedLegacyAliasStillRefreshes(t *testing.T) {
	ctx := context.Background()
	var pools api.RebalanceNodePoolList
	if err := json.Unmarshal([]byte(`{"ec2NodePools":[{"name":"general","nodePoolSpec":{"disruption":{"budgets":[{"nodes":"10%"},{"nodes":"2","schedule":"0 9 * * mon-fri","duration":"1h"}]}}}]}`), &pools); err != nil {
		t.Fatal(err)
	}
	cluster := &Cluster{client: &fakeReadClusterClient{nodePools: pools}}
	remote, err := pools.EC2NodePools[0].ToEC2NodePoolModel()
	if err != nil {
		t.Fatal(err)
	}
	if !remote.NodeDisruptionLimit.Equal(types.StringValue("10%")) {
		t.Fatalf("API conversion lost legacy alias: %s", remote.NodeDisruptionLimit)
	}
	data := ClusterModel{NodePools: customfield.NullObjectList[api.EC2NodePoolModel](ctx)}
	response := &resource.ReadResponse{}
	if !cluster.readNodePools(ctx, &data, "cluster-1", true, response) {
		t.Fatalf("import diagnostics: %v", response.Diagnostics)
	}
	models, diags := data.NodePools.AsStructSliceT(ctx)
	if diags.HasError() || len(models) != 1 {
		t.Fatalf("imported nodepools = %#v, diagnostics = %v", models, diags)
	}
	if !models[0].NodeDisruptionLimit.IsNull() || !models[0].NodeDisruptionBudgets.Equal(remote.NodeDisruptionBudgets) {
		t.Fatalf("import did not use canonical full budgets: alias=%s budgets=%s", models[0].NodeDisruptionLimit, models[0].NodeDisruptionBudgets)
	}
	data.NodePools = customfield.NewObjectListMust(ctx, []api.EC2NodePoolModel{{Name: types.StringValue("general"), NodeDisruptionLimit: types.StringValue("2")}})
	if !cluster.readNodePools(ctx, &data, "cluster-1", false, response) {
		t.Fatalf("refresh diagnostics: %v", response.Diagnostics)
	}
	models, diags = data.NodePools.AsStructSliceT(ctx)
	if diags.HasError() || len(models) != 1 {
		t.Fatalf("refreshed nodepools = %#v, diagnostics = %v", models, diags)
	}
	if !models[0].NodeDisruptionLimit.Equal(types.StringValue("10%")) || !models[0].NodeDisruptionBudgets.IsNull() {
		t.Fatalf("managed legacy alias behavior changed: alias=%s budgets=%s", models[0].NodeDisruptionLimit, models[0].NodeDisruptionBudgets)
	}
}
