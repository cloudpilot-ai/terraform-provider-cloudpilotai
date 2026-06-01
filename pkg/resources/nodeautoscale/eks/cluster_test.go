package eks

import "testing"

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
