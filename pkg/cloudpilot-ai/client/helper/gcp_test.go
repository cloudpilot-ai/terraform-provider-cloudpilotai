package helper

import (
	"context"
	"testing"

	gcpcorev1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter/apis/v1"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type fakeGCPNodeClassConfigurationClient struct {
	listResponse api.RebalanceNodeClassList
	applied      []string
	deleted      []string
}

func (f *fakeGCPNodeClassConfigurationClient) ListNodeClasses(string) (api.RebalanceNodeClassList, error) {
	return f.listResponse, nil
}

func (f *fakeGCPNodeClassConfigurationClient) ApplyNodeClass(_ string, rebalanceNodeClass api.RebalanceNodeClass) error {
	f.applied = append(f.applied, rebalanceNodeClass.GCENodeClass.Name)
	return nil
}

func (f *fakeGCPNodeClassConfigurationClient) DeleteNodeClass(_ string, nodeClassName string) error {
	f.deleted = append(f.deleted, nodeClassName)
	return nil
}

type fakeGCPNodePoolConfigurationClient struct {
	listResponse api.RebalanceNodePoolList
	applied      []string
	deleted      []string
}

func (f *fakeGCPNodePoolConfigurationClient) ListNodePools(string) (api.RebalanceNodePoolList, error) {
	return f.listResponse, nil
}

func (f *fakeGCPNodePoolConfigurationClient) ApplyNodePool(_ string, rebalanceNodePool api.RebalanceNodePool) error {
	f.applied = append(f.applied, rebalanceNodePool.GCENodePool.Name)
	return nil
}

func (f *fakeGCPNodePoolConfigurationClient) DeleteNodePool(_ string, nodePoolName string) error {
	f.deleted = append(f.deleted, nodePoolName)
	return nil
}

func TestSyncGCENodeClassConfigurationDeletesOnlyPreviouslyTrackedNames(t *testing.T) {
	ctx := context.Background()
	client := &fakeGCPNodeClassConfigurationClient{
		listResponse: api.RebalanceNodeClassList{
			GCENodeClasses: []api.GCENodeClass{
				{Name: "keep-remote", NodeClassSpec: &api.GCENodeClassSpec{ServiceAccount: "remote@test-project.iam.gserviceaccount.com"}},
				{Name: "terraform-old", NodeClassSpec: &api.GCENodeClassSpec{ServiceAccount: "old@test-project.iam.gserviceaccount.com"}},
			},
		},
	}
	desired := customfield.NewObjectListMust(ctx, []api.GCENodeClassModel{{
		Name:           types.StringValue("keep-remote"),
		ServiceAccount: types.StringValue("updated@test-project.iam.gserviceaccount.com"),
	}})
	previousStateNames := map[string]struct{}{
		"terraform-old": {},
		"keep-remote":   {},
	}

	if err := SyncGCENodeClassConfiguration(ctx, client, "cluster-1", desired, previousStateNames); err != nil {
		t.Fatalf("SyncGCENodeClassConfiguration() error = %v", err)
	}
	if len(client.applied) != 1 || client.applied[0] != "keep-remote" {
		t.Fatalf("applied = %#v", client.applied)
	}
	if len(client.deleted) != 1 || client.deleted[0] != "terraform-old" {
		t.Fatalf("deleted = %#v", client.deleted)
	}
}

func TestSyncGCENodePoolConfigurationDeletesOnlyPreviouslyTrackedNames(t *testing.T) {
	ctx := context.Background()
	client := &fakeGCPNodePoolConfigurationClient{
		listResponse: api.RebalanceNodePoolList{
			GCENodePools: []api.GCENodePool{
				{
					Name:   "keep-remote",
					Enable: true,
					NodePoolSpec: &gcpcorev1.NodePoolSpec{
						Template: gcpcorev1.NodeClaimTemplate{
							Spec: gcpcorev1.NodeClaimTemplateSpec{
								NodeClassRef: &gcpcorev1.NodeClassReference{
									Group: "karpenter.k8s.gcp",
									Kind:  "GCENodeClass",
									Name:  "cloudpilot",
								},
							},
						},
					},
				},
				{
					Name:   "terraform-old",
					Enable: true,
					NodePoolSpec: &gcpcorev1.NodePoolSpec{
						Template: gcpcorev1.NodeClaimTemplate{
							Spec: gcpcorev1.NodeClaimTemplateSpec{
								NodeClassRef: &gcpcorev1.NodeClassReference{
									Group: "karpenter.k8s.gcp",
									Kind:  "GCENodeClass",
									Name:  "cloudpilot",
								},
							},
						},
					},
				},
			},
		},
	}
	desired := customfield.NewObjectListMust(ctx, []api.GCENodePoolModel{{
		Name:      types.StringValue("keep-remote"),
		Enable:    types.BoolValue(true),
		NodeClass: types.StringValue("cloudpilot"),
	}})
	previousStateNames := map[string]struct{}{
		"terraform-old": {},
		"keep-remote":   {},
	}

	if err := SyncGCENodePoolConfiguration(ctx, client, "cluster-1", desired, previousStateNames); err != nil {
		t.Fatalf("SyncGCENodePoolConfiguration() error = %v", err)
	}
	if len(client.applied) != 1 || client.applied[0] != "keep-remote" {
		t.Fatalf("applied = %#v", client.applied)
	}
	if len(client.deleted) != 1 || client.deleted[0] != "terraform-old" {
		t.Fatalf("deleted = %#v", client.deleted)
	}
}
