package jsonmerge

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMarshalPreservesUnknownFieldsWithoutResurrectingKnownValues(t *testing.T) {
	type child struct {
		Known *string `json:"known,omitempty"`
		Flag  bool    `json:"flag,omitempty"`
	}
	type record struct {
		Name  string `json:"name"`
		Child *child `json:"child,omitempty"`
	}
	type body struct {
		Child *child   `json:"child,omitempty"`
		Items []record `json:"items,omitempty"`
		Count int      `json:"count,omitempty"`
	}
	original := json.RawMessage(`{"child":{"known":"old","flag":true,"futureFlag":false,"futureZero":0,"futureNull":null},"items":[{"name":"first","child":{"known":"old","future":"one"}},{"name":"second","child":{"future":"two"}}],"count":7,"futureRoot":{"value":9007199254740993}}`)
	current := body{Child: &child{}, Items: []record{{Name: "second", Child: &child{}}, {Name: "first", Child: &child{}}}}
	actual, err := MarshalPreservingUnknown(original, current)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(actual, &value); err != nil {
		t.Fatal(err)
	}
	if _, ok := value["count"]; ok {
		t.Fatalf("known zero restored: %s", actual)
	}
	var nested map[string]any
	if err := json.Unmarshal(value["child"], &nested); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"futureFlag": false, "futureZero": float64(0), "futureNull": nil}
	if !reflect.DeepEqual(nested, want) {
		t.Fatalf("nested fields = %#v, want %#v", nested, want)
	}
	var items []map[string]any
	if err := json.Unmarshal(value["items"], &items); err != nil {
		t.Fatal(err)
	}
	if items[0]["child"].(map[string]any)["future"] != "two" || items[1]["child"].(map[string]any)["future"] != "one" {
		t.Fatalf("array identity lost: %s", actual)
	}
	if string(value["futureRoot"]) != `{"value":9007199254740993}` {
		t.Fatalf("number precision changed: %s", actual)
	}
	current.Child = nil
	actual, err = MarshalPreservingUnknown(original, current)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(actual, &value); err != nil {
		t.Fatal(err)
	}
	// Decode into a fresh map because json.Unmarshal retains existing keys.
	fresh := map[string]json.RawMessage{}
	if err := json.Unmarshal(actual, &fresh); err != nil {
		t.Fatal(err)
	}
	if _, ok := fresh["child"]; ok {
		t.Fatalf("deleted object restored: %s", actual)
	}
}
