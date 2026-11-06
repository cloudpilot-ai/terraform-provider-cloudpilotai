package eks

import (
	"context"
	"testing"

	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPreserveNodeClassStateRepresentationEBSQuantity(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured string
		remote     string
		want       string
	}{
		{name: "binary equivalent", configured: "1024Gi", remote: "1Ti", want: "1024Gi"},
		{name: "decimal equivalent", configured: "1000G", remote: "1T", want: "1000G"},
		{name: "changed size", configured: "1024Gi", remote: "2Ti", want: "2Ti"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			quantity := resource.MustParse(tc.remote)
			remoteClass := &api.EC2NodeClass{
				Name: "example",
				NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{
					BlockDeviceMappings: []*awsproviderv1.BlockDeviceMapping{{
						EBS: &awsproviderv1.BlockDevice{
							VolumeSize:       &quantity,
							VolumeSizePolicy: &awsproviderv1.VolumeSizePolicy{PerVCPUGiB: 8, MaxSizeGiB: 4096},
						},
					}},
				},
			}
			remote, err := remoteClass.ToEC2NodeClassModel(ctx)
			if err != nil {
				t.Fatal(err)
			}
			policy := customfield.NewObjectMust(ctx, &api.VolumeSizePolicyModel{
				PerVCPUGiB: types.Int64Value(8), MaxSizeGiB: types.Int64Value(4096),
			})
			state := api.EC2NodeClassModel{
				Name: types.StringValue("example"),
				BlockDeviceMappings: customfield.NewObjectListMust(ctx, []api.BlockDeviceMappingModel{{
					EBS: customfield.NewObjectMust(ctx, &api.BlockDeviceModel{
						VolumeSize:       types.StringValue(tc.configured),
						VolumeSizePolicy: policy,
					}),
				}}),
			}
			got, err := preserveNodeClassStateRepresentation(ctx, *remote, state)
			if err != nil {
				t.Fatal(err)
			}
			mappings, diags := got.BlockDeviceMappings.AsStructSliceT(ctx)
			if diags.HasError() || len(mappings) != 1 {
				t.Fatalf("block device mappings = %v, diagnostics = %v", mappings, diags)
			}
			ebs, diags := mappings[0].EBS.Value(ctx)
			if diags.HasError() || ebs == nil {
				t.Fatalf("ebs = %v, diagnostics = %v", ebs, diags)
			}
			if ebs.VolumeSize.ValueString() != tc.want {
				t.Fatalf("volume size = %q, want %q", ebs.VolumeSize.ValueString(), tc.want)
			}
			if !ebs.VolumeSizePolicy.Equal(policy) {
				t.Fatal("volume size policy changed")
			}
		})
	}
}
