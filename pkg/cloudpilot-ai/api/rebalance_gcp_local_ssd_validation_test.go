package api

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestGCELocalSSDEmptyObjectRequiresEnabledFeature(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name       string
		remoteJSON string
		enabled    types.Bool
		wantError  string
	}{
		{
			name: "disabled response omits switch", remoteJSON: `{"name":"example","nodeClassSpec":{}}`,
			enabled: types.BoolNull(), wantError: "set enable_local_ssd_ephemeral_storage to true explicitly",
		},
		{
			name: "disabled response has false switch", remoteJSON: `{"name":"example","enableLocalSSDEphemeralStorage":false,"nodeClassSpec":{}}`,
			enabled: types.BoolNull(), wantError: "set enable_local_ssd_ephemeral_storage to true explicitly",
		},
		{
			name: "explicit false while disabled", remoteJSON: `{"name":"example","nodeClassSpec":{}}`,
			enabled: types.BoolValue(false), wantError: "cannot be configured when enable_local_ssd_ephemeral_storage is false",
		},
		{
			name: "explicit false while enabled", remoteJSON: `{"name":"example","enableLocalSSDEphemeralStorage":true,"nodeClassSpec":{"ephemeralStorageLocalSSD":{"count":4}}}`,
			enabled: types.BoolValue(false), wantError: "cannot be configured when enable_local_ssd_ephemeral_storage is false",
		},
		{
			name: "enabled fixed count resets", remoteJSON: `{"name":"example","enableLocalSSDEphemeralStorage":true,"nodeClassSpec":{"ephemeralStorageLocalSSD":{"count":4}}}`,
			enabled: types.BoolNull(),
		},
		{
			name: "enabled automatic count resets", remoteJSON: `{"name":"example","enableLocalSSDEphemeralStorage":true,"nodeClassSpec":{"ephemeralStorageLocalSSD":{"autoCount":true}}}`,
			enabled: types.BoolValue(true),
		},
		{
			name: "explicit true enables default configuration", remoteJSON: `{"name":"example","nodeClassSpec":{}}`,
			enabled: types.BoolValue(true),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var current GCENodeClass
			if err := json.Unmarshal([]byte(tc.remoteJSON), &current); err != nil {
				t.Fatal(err)
			}
			model := GCENodeClassModel{
				Name: types.StringValue("example"), EnableLocalSSDEphemeralStorage: tc.enabled,
				EphemeralStorageLocalSSD: customfield.NewObjectMust(ctx, &GCEEphemeralStorageLocalSSDModel{}),
			}
			got, err := model.ToGCENodeClass(ctx, current)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("ToGCENodeClass() error = %v, want %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			ssd := got.NodeClassSpec.EphemeralStorageLocalSSD
			if !got.localSSDEphemeralStorageEnabled() || ssd == nil || ssd.Count != nil || ssd.AutoCount {
				t.Fatalf("empty reset = %#v, want enabled default configuration", got)
			}
		})
	}
}

func TestGCELocalSSDUnknownValuesDeferEnabledValidation(t *testing.T) {
	ctx := context.Background()
	values := []struct {
		name  string
		model customfield.NestedObject[GCEEphemeralStorageLocalSSDModel]
	}{
		{name: "null object", model: customfield.NullObject[GCEEphemeralStorageLocalSSDModel](ctx)},
		{name: "unknown object", model: customfield.UnknownObject[GCEEphemeralStorageLocalSSDModel](ctx)},
		{name: "unknown children", model: customfield.NewObjectMust(ctx, &GCEEphemeralStorageLocalSSDModel{Count: types.Int32Unknown(), AutoCount: types.BoolUnknown()})},
		{name: "unknown count", model: customfield.NewObjectMust(ctx, &GCEEphemeralStorageLocalSSDModel{Count: types.Int32Unknown()})},
		{name: "unknown automatic mode", model: customfield.NewObjectMust(ctx, &GCEEphemeralStorageLocalSSDModel{AutoCount: types.BoolUnknown()})},
	}
	for _, remoteJSON := range []string{
		`{"name":"example","nodeClassSpec":{}}`,
		`{"name":"example","enableLocalSSDEphemeralStorage":false,"nodeClassSpec":{}}`,
		`{"name":"example","enableLocalSSDEphemeralStorage":true,"nodeClassSpec":{"ephemeralStorageLocalSSD":{"count":4}}}`,
		`{"name":"example","enableLocalSSDEphemeralStorage":true,"nodeClassSpec":{"ephemeralStorageLocalSSD":{"autoCount":true}}}`,
	} {
		for _, value := range values {
			t.Run(remoteJSON+"/"+value.name, func(t *testing.T) {
				var current GCENodeClass
				if err := json.Unmarshal([]byte(remoteJSON), &current); err != nil {
					t.Fatal(err)
				}
				model := GCENodeClassModel{Name: types.StringValue("example"), EphemeralStorageLocalSSD: value.model}
				got, err := model.ToGCENodeClass(ctx, current)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got.NodeClassSpec.EphemeralStorageLocalSSD, current.NodeClassSpec.EphemeralStorageLocalSSD) ||
					!reflect.DeepEqual(got.EnableLocalSSDEphemeralStorage, current.EnableLocalSSDEphemeralStorage) {
					t.Fatalf("unmanaged Local SSD values changed the remote configuration: %#v", got)
				}
			})
		}
	}
}

func TestGCELocalSSDModeTransitionsWithExplicitEnable(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		current GCEEphemeralStorageLocalSSD
		model   GCEEphemeralStorageLocalSSDModel
		want    GCEEphemeralStorageLocalSSD
	}{
		{
			name: "fixed to automatic", current: GCEEphemeralStorageLocalSSD{Count: ptrInt32(4)},
			model: GCEEphemeralStorageLocalSSDModel{AutoCount: types.BoolValue(true)}, want: GCEEphemeralStorageLocalSSD{AutoCount: true},
		},
		{
			name: "automatic to fixed", current: GCEEphemeralStorageLocalSSD{AutoCount: true},
			model: GCEEphemeralStorageLocalSSDModel{Count: types.Int32Value(2)}, want: GCEEphemeralStorageLocalSSD{Count: ptrInt32(2)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := GCENodeClass{
				Name: "example", EnableLocalSSDEphemeralStorage: ptrBool(true),
				NodeClassSpec: &GCENodeClassSpec{EphemeralStorageLocalSSD: &tc.current},
			}
			model := GCENodeClassModel{
				Name: types.StringValue("example"), EnableLocalSSDEphemeralStorage: types.BoolValue(true),
				EphemeralStorageLocalSSD: customfield.NewObjectMust(ctx, &tc.model),
			}
			got, err := model.ToGCENodeClass(ctx, current)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.NodeClassSpec.EphemeralStorageLocalSSD, &tc.want) {
				t.Fatalf("Local SSD configuration = %#v, want %#v", got.NodeClassSpec.EphemeralStorageLocalSSD, tc.want)
			}
		})
	}
}
