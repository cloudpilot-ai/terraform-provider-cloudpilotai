package v1

import (
	"encoding/json"
	"testing"

	statuspkg "github.com/cloudpilot-ai/lib/pkg/gcp/awslabs/operatorpkg/status"
)

func TestNodePoolStatusNodesOmitEmpty(t *testing.T) {
	data, err := json.Marshal(NodePool{
		Status: NodePoolStatus{
			Conditions: []statuspkg.Condition{{Type: "Ready"}},
		},
	})
	if err != nil {
		t.Fatalf("marshal nodepool without nodes: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal nodepool without nodes: %v", err)
	}

	statusValue, ok := payload["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected status object, got %#v", payload["status"])
	}

	if _, exists := statusValue["nodes"]; exists {
		t.Fatalf("expected nodes to be omitted when nil, got %#v", statusValue["nodes"])
	}
}

func TestNodePoolStatusNodesPresentWhenSet(t *testing.T) {
	nodes := int64(0)
	data, err := json.Marshal(NodePool{
		Status: NodePoolStatus{
			Nodes: &nodes,
		},
	})
	if err != nil {
		t.Fatalf("marshal nodepool with nodes: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal nodepool with nodes: %v", err)
	}

	statusValue, ok := payload["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected status object, got %#v", payload["status"])
	}

	value, exists := statusValue["nodes"]
	if !exists {
		t.Fatal("expected nodes to be present when explicitly set")
	}
	if got, ok := value.(float64); !ok || got != 0 {
		t.Fatalf("expected nodes=0, got %#v", value)
	}
}
