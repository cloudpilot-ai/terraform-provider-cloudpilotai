package eks

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

type namedItem struct {
	name string
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
