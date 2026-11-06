package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/client"
)

func TestNodePoolRequestsCancelWhileServerHangs(t *testing.T) {
	operations := []struct {
		name string
		call func(context.Context, *client.Client) error
	}{
		{name: "list", call: func(ctx context.Context, c *client.Client) error {
			_, err := c.ListNodePoolsContext(ctx, "cluster-1")
			return err
		}},
		{name: "delete", call: func(ctx context.Context, c *client.Client) error {
			return c.DeleteNodePoolContext(ctx, "cluster-1", "pool-1")
		}},
	}
	for _, operation := range operations {
		for _, phase := range []string{"headers", "body"} {
			t.Run(operation.name+"/"+phase, func(t *testing.T) {
				started := make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if phase == "body" {
						_, _ = w.Write([]byte(`{"data":`))
						w.(http.Flusher).Flush()
					}
					close(started)
					<-r.Context().Done()
				}))
				defer server.Close()
				defer server.CloseClientConnections()
				c := client.NewCloudPilotClient(server.URL, "test-key")
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				result := make(chan error, 1)
				go func() { result <- operation.call(ctx, c) }()
				select {
				case <-started:
				case <-time.After(2 * time.Second):
					t.Fatal("HTTP request did not reach server")
				}
				cancel()
				select {
				case err := <-result:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("request error = %v, want cancellation", err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("request ignored cancellation")
				}
			})
		}
	}
}

func TestListNodePoolsContextHonorsDeadline(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	defer server.CloseClientConnections()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan error, 1)
	c := client.NewCloudPilotClient(server.URL, "test-key")
	go func() { _, err := c.ListNodePoolsContext(ctx, "cluster-1"); result <- err }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP request did not reach server")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("request error = %v, want deadline", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request ignored deadline")
	}
}

func TestNodePoolContextMethodsPreserveLegacyRequests(t *testing.T) {
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "test-key" || r.Header.Get("X-CloudPilot-Client") != "terraform-provider-cloudpilotai" {
			t.Error("authentication or client headers missing")
		}
		requests <- r.Method + " " + r.URL.Path
		_ = json.NewEncoder(w).Encode(api.ResponseBody{Data: api.RebalanceNodePoolList{EC2NodePools: []api.EC2NodePool{{Name: "pool-1"}}}})
	}))
	defer server.Close()
	c := client.NewCloudPilotClient(server.URL, "test-key")
	legacy, err := c.ListNodePools("cluster-1")
	if err != nil || len(legacy.EC2NodePools) != 1 {
		t.Fatalf("legacy list = %#v, %v", legacy, err)
	}
	contextual, err := c.ListNodePoolsContext(context.Background(), "cluster-1")
	if err != nil || len(contextual.EC2NodePools) != 1 {
		t.Fatalf("context list = %#v, %v", contextual, err)
	}
	if err := c.DeleteNodePool("cluster-1", "pool-1"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteNodePoolContext(context.Background(), "cluster-1", "pool-1"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"GET /api/v1/rebalance/clusters/cluster-1/nodepools",
		"GET /api/v1/rebalance/clusters/cluster-1/nodepools",
		"DELETE /api/v1/rebalance/clusters/cluster-1/nodepools/pool-1",
		"DELETE /api/v1/rebalance/clusters/cluster-1/nodepools/pool-1",
	} {
		if got := <-requests; got != want {
			t.Fatalf("request = %q, want %q", got, want)
		}
	}
}
