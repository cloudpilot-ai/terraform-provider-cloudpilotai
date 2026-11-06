package jsonmerge

import (
	"encoding/json"
	"testing"
)

type positionalDisk struct {
	Size int  `json:"size"`
	Boot bool `json:"boot"`
}
type diskContainer struct {
	Disks []positionalDisk `json:"disks"`
}

func TestScopedPositionalArraysRetainSurvivingEntriesOnAppendAndRemove(t *testing.T) {
	cases := []struct {
		name     string
		original string
		current  diskContainer
		count    int
	}{
		{"append", `{"disks":[{"size":50,"boot":true,"futureDisk":"boot"}]}`, diskContainer{Disks: []positionalDisk{{Size: 60, Boot: true}, {Size: 100}}}, 2},
		{"remove", `{"disks":[{"size":50,"boot":true,"futureDisk":"boot"},{"size":100,"boot":false,"futureDisk":"removed"}]}`, diskContainer{Disks: []positionalDisk{{Size: 60, Boot: true}}}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := MarshalPreservingUnknown(json.RawMessage(tc.original), tc.current, WithPositionalArrayElements(positionalDisk{}))
			if err != nil {
				t.Fatal(err)
			}
			disks := decodeObject(t, encoded)["disks"].([]any)
			if len(disks) != tc.count {
				t.Fatalf("disk count=%d,want %d: %s", len(disks), tc.count, encoded)
			}
			first := disks[0].(map[string]any)
			if first["futureDisk"] != "boot" || first["size"] != float64(60) {
				t.Fatalf("surviving disk was reset: %s", encoded)
			}
			if len(disks) > 1 {
				if _, ok := disks[1].(map[string]any)["futureDisk"]; ok {
					t.Fatalf("new disk inherited old unknown fields: %s", encoded)
				}
			}
		})
	}
	// The opt-in applies to this element type only; other anonymous arrays retain
	// the conservative behavior when their membership changes.
	encoded, err := MarshalPreservingUnknown(json.RawMessage(cases[0].original), cases[0].current)
	if err != nil {
		t.Fatal(err)
	}
	disks := decodeObject(t, encoded)["disks"].([]any)
	if _, ok := disks[0].(map[string]any)["futureDisk"]; ok {
		t.Fatalf("unscoped anonymous array inherited unknown fields: %s", encoded)
	}
}

type identifiedTarget struct {
	APIVersion    string    `json:"apiVersion"`
	Kind          string    `json:"kind"`
	UID           string    `json:"uid,omitempty"`
	LabelSelector *selector `json:"labelSelector,omitempty"`
}
type targetContainer struct {
	Targets []identifiedTarget `json:"targets"`
}

func TestTargetIdentitiesIncludeSelectorsAndRetainDuplicateOccurrences(t *testing.T) {
	raw := json.RawMessage(`{"targets":[{"apiVersion":"apps/v1","kind":"Deployment","uid":"a","labelSelector":{"matchLabels":{"app":"a"}},"futureTarget":"first-a"},{"apiVersion":"apps/v1","kind":"Deployment","uid":"b","labelSelector":{"matchLabels":{"app":"b"}},"futureTarget":"b"},{"apiVersion":"apps/v1","kind":"Deployment","uid":"a","labelSelector":{"matchLabels":{"app":"a"}},"futureTarget":"second-a"}]}`)
	var current targetContainer
	if err := json.Unmarshal(raw, &current); err != nil {
		t.Fatal(err)
	}
	current.Targets = []identifiedTarget{current.Targets[1], current.Targets[0], current.Targets[2]}
	encoded, err := MarshalPreservingUnknown(raw, current)
	if err != nil {
		t.Fatal(err)
	}
	targets := decodeObject(t, encoded)["targets"].([]any)
	for i, want := range []string{"b", "first-a", "second-a"} {
		if targets[i].(map[string]any)["futureTarget"] != want {
			t.Fatalf("target %d did not retain its own unknown fields: %s", i, encoded)
		}
	}
	// Removing A must leave B's fields attached to B, and never recreate A.
	current.Targets = current.Targets[:1]
	encoded, err = MarshalPreservingUnknown(raw, current)
	if err != nil {
		t.Fatal(err)
	}
	targets = decodeObject(t, encoded)["targets"].([]any)
	if len(targets) != 1 || targets[0].(map[string]any)["futureTarget"] != "b" {
		t.Fatalf("target removal was not preserved: %s", encoded)
	}
	current.Targets[0].UID = "replacement"
	encoded, err = MarshalPreservingUnknown(raw, current)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := decodeObject(t, encoded)["targets"].([]any)[0].(map[string]any)["futureTarget"]; ok {
		t.Fatalf("replacement target inherited old unknown fields: %s", encoded)
	}
}

func decodeObject(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
