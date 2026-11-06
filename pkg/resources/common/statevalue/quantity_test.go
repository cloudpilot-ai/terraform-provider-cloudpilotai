package statevalue

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPreserveQuantity(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured types.String
		remote     types.String
		want       types.String
	}{
		{"binary units", types.StringValue("1024Mi"), types.StringValue("1Gi"), types.StringValue("1024Mi")},
		{"decimal units", types.StringValue("1000M"), types.StringValue("1G"), types.StringValue("1000M")},
		{"real drift", types.StringValue("1000Mi"), types.StringValue("1Gi"), types.StringValue("1Gi")},
		{"invalid quantity", types.StringValue("invalid"), types.StringValue("1Gi"), types.StringValue("1Gi")},
		{"saturated binary quantity", types.StringValue("8Ei"), types.StringValue("9223372036854775807"), types.StringValue("9223372036854775807")},
		{"percentages unchanged", types.StringValue("25.0%"), types.StringValue("25%"), types.StringValue("25%")},
		{"different types", types.StringValue("25%"), types.StringValue("25Gi"), types.StringValue("25Gi")},
		{"null configured", types.StringNull(), types.StringValue("1Gi"), types.StringValue("1Gi")},
		{"unknown configured", types.StringUnknown(), types.StringValue("1Gi"), types.StringValue("1Gi")},
		{"null remote", types.StringValue("1Gi"), types.StringNull(), types.StringNull()},
		{"unknown remote", types.StringValue("1Gi"), types.StringUnknown(), types.StringUnknown()},
		{"explicit clear", types.StringValue(""), types.StringValue(""), types.StringValue("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := PreserveQuantity(tc.configured, tc.remote); !got.Equal(tc.want) {
				t.Fatalf("quantity representation = %v, want %v", got, tc.want)
			}
		})
	}
}
