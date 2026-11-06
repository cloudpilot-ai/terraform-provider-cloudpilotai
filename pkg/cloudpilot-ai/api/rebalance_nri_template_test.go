package api

import (
	"context"
	"reflect"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestEC2NodePoolNRITemplateComposition(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		template NRISwapPolicyModel
		override *NRISwapPolicyModel
		want     NRISwapPolicy
		wantErr  bool
	}{
		{
			name:     "template enables and pool supplies multiplier",
			template: NRISwapPolicyModel{Enabled: types.BoolValue(true)},
			override: &NRISwapPolicyModel{MemoryLimitMultiplier: types.Float64Value(2)},
			want:     NRISwapPolicy{Enabled: true, MemoryLimitMultiplier: 2},
		},
		{
			name:     "template supplies multiplier and pool enables",
			template: NRISwapPolicyModel{MemoryLimitMultiplier: types.Float64Value(2)},
			override: &NRISwapPolicyModel{Enabled: types.BoolValue(true)},
			want:     NRISwapPolicy{Enabled: true, MemoryLimitMultiplier: 2},
		},
		{
			name:     "pool disables incomplete template",
			template: NRISwapPolicyModel{Enabled: types.BoolValue(true)},
			override: &NRISwapPolicyModel{Enabled: types.BoolValue(false)},
		},
		{
			name:     "pool resets incomplete template",
			template: NRISwapPolicyModel{Enabled: types.BoolValue(true)},
			override: &NRISwapPolicyModel{},
		},
		{
			name:     "pool replaces invalid template multiplier",
			template: NRISwapPolicyModel{Enabled: types.BoolValue(true), MemoryLimitMultiplier: types.Float64Value(20)},
			override: &NRISwapPolicyModel{MemoryLimitMultiplier: types.Float64Value(2)},
			want:     NRISwapPolicy{Enabled: true, MemoryLimitMultiplier: 2},
		},
		{
			name:     "invalid effective multiplier is rejected",
			template: NRISwapPolicyModel{Enabled: types.BoolValue(true), MemoryLimitMultiplier: types.Float64Value(2)},
			override: &NRISwapPolicyModel{MemoryLimitMultiplier: types.Float64Value(1)},
			wantErr:  true,
		},
		{
			name:     "missing effective multiplier is rejected",
			template: NRISwapPolicyModel{Enabled: types.BoolValue(true)},
			wantErr:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			template := EC2NodePoolTemplateModel{NRISwapPolicy: customfield.NewObjectMust(ctx, &tc.template)}
			model := EC2NodePoolModel{NRISwapPolicy: customfield.NullObject[NRISwapPolicyModel](ctx)}
			if tc.override != nil {
				model.NRISwapPolicy = customfield.NewObjectMust(ctx, tc.override)
			}
			got, err := model.ToEC2NodePool(ctx, EC2NodePool{}, &template)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ToEC2NodePool() error = %v, wantError = %v", err, tc.wantErr)
			}
			if !tc.wantErr && !reflect.DeepEqual(got.NRISwapPolicy, &tc.want) {
				t.Fatalf("effective NRI policy = %#v, want %#v", got.NRISwapPolicy, tc.want)
			}
		})
	}
}

func TestEC2NodePoolNRILayersPreserveRemotePolicy(t *testing.T) {
	ctx := context.Background()
	current := NRISwapPolicy{Enabled: true, MemoryLimitMultiplier: 3}
	unknown := customfield.UnknownObject[NRISwapPolicyModel](ctx)
	model := EC2NodePoolModel{NRISwapPolicy: customfield.NewObjectMust(ctx, &NRISwapPolicyModel{
		MemoryLimitMultiplier: types.Float64Value(2),
	})}
	got, err := model.ToEC2NodePool(ctx, EC2NodePool{NRISwapPolicy: &current}, &EC2NodePoolTemplateModel{NRISwapPolicy: unknown})
	if err != nil {
		t.Fatal(err)
	}
	if got.NRISwapPolicy == nil || !got.NRISwapPolicy.Enabled || got.NRISwapPolicy.MemoryLimitMultiplier != 2 {
		t.Fatalf("effective NRI policy = %#v", got.NRISwapPolicy)
	}
	if current.MemoryLimitMultiplier != 3 {
		t.Fatal("applying NRI layers mutated the remote policy")
	}
	for _, policy := range []*NRISwapPolicy{nil, {Enabled: true}} {
		model.NRISwapPolicy = customfield.NullObject[NRISwapPolicyModel](ctx)
		got, err := model.ToEC2NodePool(ctx, EC2NodePool{NRISwapPolicy: policy}, &EC2NodePoolTemplateModel{NRISwapPolicy: unknown})
		if err != nil || !reflect.DeepEqual(got.NRISwapPolicy, policy) {
			t.Fatalf("unmanaged NRI policy = %#v, error = %v", got, err)
		}
	}
}
