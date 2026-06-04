package api

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	awscorev1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter/apis/v1"
	"github.com/hashicorp/terraform-plugin-framework/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func TestEC2NodeClassToModelLeavesExtraAllocationsNullWhenKubeReservedKeysMissing(t *testing.T) {
	model, err := (&EC2NodeClass{
		Name: "cloudpilot",
		NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{
			Kubelet: &awsproviderv1.KubeletConfiguration{
				KubeReserved: map[string]string{
					"ephemeral-storage": "1Gi",
				},
			},
		},
	}).ToEC2NodeClassModel(context.Background())
	if err != nil {
		t.Fatalf("ToEC2NodeClassModel() error = %v", err)
	}

	if !model.ExtraCPUAllocationMCore.IsNull() {
		t.Fatalf("ExtraCPUAllocationMCore should be null when kubeReserved.cpu is missing")
	}

	if !model.ExtraMemoryAllocationMib.IsNull() {
		t.Fatalf("ExtraMemoryAllocationMib should be null when kubeReserved.memory is missing")
	}
}

func TestEC2NodeClassToModelLeavesBlockDeviceMappingsNullWhenUnset(t *testing.T) {
	model, err := (&EC2NodeClass{
		Name:          "cloudpilot",
		NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{},
	}).ToEC2NodeClassModel(context.Background())
	if err != nil {
		t.Fatalf("ToEC2NodeClassModel() error = %v", err)
	}

	if !model.BlockDeviceMappings.IsNull() {
		t.Fatalf("BlockDeviceMappings should be null when block device mappings are absent")
	}
}

func TestEC2NodePoolToModelLeavesInstanceMinimumsNullWhenRequirementsMissing(t *testing.T) {
	model, err := (&EC2NodePool{
		Name:         "cloudpilot-general",
		NodePoolSpec: DefaultGeneralEC2NodePoolSpec(),
	}).ToEC2NodePoolModel()
	if err != nil {
		t.Fatalf("ToEC2NodePoolModel() error = %v", err)
	}

	if !model.InstanceCPUMIN.IsNull() {
		t.Fatalf("InstanceCPUMIN should be null when instance-cpu Gt requirement is missing")
	}

	if !model.InstanceMemoryMIN.IsNull() {
		t.Fatalf("InstanceMemoryMIN should be null when instance-memory Gt requirement is missing")
	}
}

func TestEC2NodePoolToModelLeavesLabelsAndTaintsNullWhenUnset(t *testing.T) {
	model, err := (&EC2NodePool{
		Name: "cloudpilot-general",
		NodePoolSpec: &awscorev1.NodePoolSpec{
			Template: awscorev1.NodeClaimTemplate{
				ObjectMeta: awscorev1.ObjectMeta{},
				Spec:       awscorev1.NodeClaimTemplateSpec{},
			},
		},
	}).ToEC2NodePoolModel()
	if err != nil {
		t.Fatalf("ToEC2NodePoolModel() error = %v", err)
	}

	if !model.Labels.IsNull() {
		t.Fatalf("Labels should be null when labels are absent")
	}
	if !model.Taints.IsNull() {
		t.Fatalf("Taints should be null when taints are absent")
	}
}

func TestDefaultEC2NodeClassSpecUsesServerAlignedAmiAlias(t *testing.T) {
	spec := DefaultEC2NodeClassSpec("test-cluster")
	if spec == nil {
		t.Fatal("DefaultEC2NodeClassSpec() returned nil")
	}
	if len(spec.AMISelectorTerms) != 1 {
		t.Fatalf("AMISelectorTerms length = %d, want 1", len(spec.AMISelectorTerms))
	}
	if got := spec.AMISelectorTerms[0].Alias; got != "al2023@v20260423" {
		t.Fatalf("AMISelectorTerms[0].Alias = %q, want %q", got, "al2023@v20260423")
	}
}

func TestEC2NodeClassModelAppliesFrontendFields(t *testing.T) {
	ctx := context.Background()
	model := EC2NodeClassModel{
		Name:     types.StringValue("cloudpilot"),
		AmiAlias: types.StringValue("al2023@latest"),
		UserData: types.StringValue("#!/bin/bash\necho hello"),
		BlockDeviceMappings: customfield.NewObjectListMust(ctx, []BlockDeviceMappingModel{{
			DeviceName: types.StringValue("/dev/xvda"),
			RootVolume: types.BoolValue(true),
			EBS: customfield.NewObjectMust(ctx, &BlockDeviceModel{
				VolumeSize: types.StringValue("80Gi"),
				VolumeType: types.StringValue("gp3"),
				Encrypted:  types.BoolValue(true),
			}),
		}}),
	}

	got, err := model.ToEC2NodeClass(ctx, "test-cluster", EC2NodeClass{}, nil)
	if err != nil {
		t.Fatalf("ToEC2NodeClass() error = %v", err)
	}
	if len(got.NodeClassSpec.AMISelectorTerms) != 1 || got.NodeClassSpec.AMISelectorTerms[0].Alias != "al2023@latest" {
		t.Fatalf("AMISelectorTerms = %#v", got.NodeClassSpec.AMISelectorTerms)
	}
	if got.NodeClassSpec.UserData == nil || *got.NodeClassSpec.UserData != "#!/bin/bash\necho hello" {
		t.Fatalf("UserData = %#v", got.NodeClassSpec.UserData)
	}
	if len(got.NodeClassSpec.BlockDeviceMappings) != 1 {
		t.Fatalf("BlockDeviceMappings length = %d", len(got.NodeClassSpec.BlockDeviceMappings))
	}
	ebs := got.NodeClassSpec.BlockDeviceMappings[0].EBS
	if ebs == nil || ebs.VolumeSize == nil || ebs.VolumeSize.String() != "80Gi" {
		t.Fatalf("EBS volume size = %#v", ebs)
	}
	if ebs.VolumeType == nil || *ebs.VolumeType != "gp3" {
		t.Fatalf("EBS volume type = %#v", ebs)
	}
	if ebs.Encrypted == nil || !*ebs.Encrypted {
		t.Fatalf("EBS encrypted = %#v", ebs)
	}
}

func TestEC2NodeClassToModelReadsFrontendFields(t *testing.T) {
	ctx := context.Background()
	userData := "echo existing"
	deviceName := "/dev/xvda"
	volumeSize := resource.MustParse("64Gi")
	volumeType := "gp3"
	nodeClass := EC2NodeClass{
		Name: "cloudpilot",
		NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{
			AMISelectorTerms: []awsproviderv1.AMISelectorTerm{{Alias: "al2@latest"}},
			UserData:         &userData,
			BlockDeviceMappings: []*awsproviderv1.BlockDeviceMapping{{
				DeviceName: &deviceName,
				RootVolume: true,
				EBS: &awsproviderv1.BlockDevice{
					Encrypted:  aws.Bool(true),
					VolumeSize: &volumeSize,
					VolumeType: &volumeType,
				},
			}},
		},
	}

	model, err := nodeClass.ToEC2NodeClassModel(ctx)
	if err != nil {
		t.Fatalf("ToEC2NodeClassModel() error = %v", err)
	}
	if model.AmiAlias.ValueString() != "al2@latest" {
		t.Fatalf("AmiAlias = %q", model.AmiAlias.ValueString())
	}
	if model.UserData.ValueString() != userData {
		t.Fatalf("UserData = %q", model.UserData.ValueString())
	}
	mappings, diags := model.BlockDeviceMappings.AsStructSliceT(ctx)
	if diags.HasError() {
		t.Fatalf("BlockDeviceMappings diagnostics = %v", diags)
	}
	if len(mappings) != 1 || mappings[0].DeviceName.ValueString() != "/dev/xvda" {
		t.Fatalf("BlockDeviceMappings = %#v", mappings)
	}
	ebs, diags := mappings[0].EBS.Value(ctx)
	if diags.HasError() {
		t.Fatalf("BlockDeviceMappings EBS diagnostics = %v", diags)
	}
	if ebs == nil || !ebs.Encrypted.ValueBool() {
		t.Fatalf("BlockDeviceMappings EBS encrypted = %#v", ebs)
	}
}

func TestEC2NodeClassModelPreservesHiddenBlockDeviceFields(t *testing.T) {
	ctx := context.Background()
	deviceName := "/dev/xvda"
	volumeSize := resource.MustParse("80Gi")
	volumeType := "gp3"
	existing := EC2NodeClass{
		Name: "cloudpilot",
		NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{
			BlockDeviceMappings: []*awsproviderv1.BlockDeviceMapping{{
				DeviceName: &deviceName,
				EBS: &awsproviderv1.BlockDevice{
					Encrypted:           aws.Bool(true),
					VolumeSize:          &volumeSize,
					VolumeType:          &volumeType,
					IOPS:                aws.Int64(3000),
					Throughput:          aws.Int64(125),
					DeleteOnTermination: aws.Bool(true),
				},
			}},
		},
	}
	model := EC2NodeClassModel{
		Name: types.StringValue("cloudpilot"),
		BlockDeviceMappings: customfield.NewObjectListMust(ctx, []BlockDeviceMappingModel{{
			DeviceName: types.StringValue("/dev/xvda"),
			EBS: customfield.NewObjectMust(ctx, &BlockDeviceModel{
				VolumeSize: types.StringValue("80Gi"),
				VolumeType: types.StringValue("gp3"),
				Encrypted:  types.BoolValue(true),
			}),
		}}),
	}

	got, err := model.ToEC2NodeClass(ctx, "test-cluster", existing, nil)
	if err != nil {
		t.Fatalf("ToEC2NodeClass() error = %v", err)
	}
	ebs := got.NodeClassSpec.BlockDeviceMappings[0].EBS
	if ebs == nil || ebs.IOPS == nil || *ebs.IOPS != 3000 {
		t.Fatalf("EBS IOPS = %#v", ebs)
	}
	if ebs.Throughput == nil || *ebs.Throughput != 125 {
		t.Fatalf("EBS throughput = %#v", ebs)
	}
	if ebs.DeleteOnTermination == nil || !*ebs.DeleteOnTermination {
		t.Fatalf("EBS delete_on_termination = %#v", ebs)
	}
}

func TestEC2NodeClassModelDoesNotPreserveExposedEBSFieldsWhenOmitted(t *testing.T) {
	ctx := context.Background()
	deviceName := "/dev/xvda"
	existingVolumeSize := resource.MustParse("40Gi")
	existingVolumeType := "gp3"
	modelVolumeSize := "80Gi"
	existing := EC2NodeClass{
		Name: "cloudpilot",
		NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{
			BlockDeviceMappings: []*awsproviderv1.BlockDeviceMapping{{
				DeviceName: &deviceName,
				EBS: &awsproviderv1.BlockDevice{
					Encrypted:           aws.Bool(true),
					VolumeSize:          &existingVolumeSize,
					VolumeType:          &existingVolumeType,
					IOPS:                aws.Int64(3000),
					Throughput:          aws.Int64(125),
					DeleteOnTermination: aws.Bool(true),
				},
			}},
		},
	}
	model := EC2NodeClassModel{
		Name: types.StringValue("cloudpilot"),
		BlockDeviceMappings: customfield.NewObjectListMust(ctx, []BlockDeviceMappingModel{{
			DeviceName: types.StringValue("/dev/xvda"),
			EBS: customfield.NewObjectMust(ctx, &BlockDeviceModel{
				VolumeSize: types.StringValue(modelVolumeSize),
			}),
		}}),
	}

	got, err := model.ToEC2NodeClass(ctx, "test-cluster", existing, nil)
	if err != nil {
		t.Fatalf("ToEC2NodeClass() error = %v", err)
	}
	ebs := got.NodeClassSpec.BlockDeviceMappings[0].EBS
	if ebs == nil {
		t.Fatal("EBS should not be nil")
	}
	if ebs.VolumeSize == nil || ebs.VolumeSize.String() != modelVolumeSize {
		t.Fatalf("EBS volume size = %#v", ebs.VolumeSize)
	}
	if ebs.Encrypted != nil {
		t.Fatalf("EBS encrypted should remain nil when omitted from config, got %#v", ebs.Encrypted)
	}
	if ebs.VolumeType != nil {
		t.Fatalf("EBS volume type should remain nil when omitted from config, got %#v", ebs.VolumeType)
	}
	if ebs.IOPS == nil || *ebs.IOPS != 3000 {
		t.Fatalf("EBS IOPS = %#v", ebs)
	}
	if ebs.Throughput == nil || *ebs.Throughput != 125 {
		t.Fatalf("EBS throughput = %#v", ebs)
	}
	if ebs.DeleteOnTermination == nil || !*ebs.DeleteOnTermination {
		t.Fatalf("EBS delete_on_termination = %#v", ebs)
	}
}

func TestEC2NodePoolModelAppliesLabelsAndTaints(t *testing.T) {
	ctx := context.Background()
	model := EC2NodePoolModel{
		Name: types.StringValue("cloudpilot-general"),
		Labels: customfield.NewMapMust[types.String](ctx, map[string]types.String{
			"team": types.StringValue("platform"),
		}),
		Taints: customfield.NewObjectListMust(ctx, []TaintModel{{
			Key:    types.StringValue("dedicated"),
			Value:  types.StringValue("wa"),
			Effect: types.StringValue("NoSchedule"),
		}}),
	}

	got, err := model.ToEC2NodePool(ctx, EC2NodePool{}, nil)
	if err != nil {
		t.Fatalf("ToEC2NodePool() error = %v", err)
	}
	if got.NodePoolSpec.Template.ObjectMeta.Labels["team"] != "platform" {
		t.Fatalf("labels = %#v", got.NodePoolSpec.Template.ObjectMeta.Labels)
	}
	taints := got.NodePoolSpec.Template.Spec.Taints
	if len(taints) != 1 || taints[0].Key != "dedicated" || string(taints[0].Effect) != "NoSchedule" {
		t.Fatalf("taints = %#v", taints)
	}
}

func TestEC2NodePoolModelReplacesExistingLabels(t *testing.T) {
	ctx := context.Background()
	model := EC2NodePoolModel{
		Name: types.StringValue("cloudpilot-general"),
		Labels: customfield.NewMapMust[types.String](ctx, map[string]types.String{
			"team": types.StringValue("platform"),
		}),
	}
	existing := EC2NodePool{
		Name:         "cloudpilot-general",
		NodePoolSpec: DefaultGeneralEC2NodePoolSpec(),
	}
	existing.NodePoolSpec.Template.ObjectMeta.Labels = map[string]string{
		"team":   "old",
		"delete": "me",
	}

	got, err := model.ToEC2NodePool(ctx, existing, nil)
	if err != nil {
		t.Fatalf("ToEC2NodePool() error = %v", err)
	}
	if got.NodePoolSpec.Template.ObjectMeta.Labels["team"] != "platform" {
		t.Fatalf("team label = %q", got.NodePoolSpec.Template.ObjectMeta.Labels["team"])
	}
	if _, ok := got.NodePoolSpec.Template.ObjectMeta.Labels["delete"]; ok {
		t.Fatalf("stale label was preserved: %#v", got.NodePoolSpec.Template.ObjectMeta.Labels)
	}
}

func TestEC2NodePoolDisruptionLimitCreatesFirstBudget(t *testing.T) {
	model := EC2NodePoolModel{
		Name:                types.StringValue("cloudpilot-general"),
		NodeDisruptionLimit: types.StringValue("2"),
	}

	got, err := model.ToEC2NodePool(context.Background(), EC2NodePool{}, nil)
	if err != nil {
		t.Fatalf("ToEC2NodePool() error = %v", err)
	}
	if len(got.NodePoolSpec.Disruption.Budgets) != 1 {
		t.Fatalf("budgets length = %d", len(got.NodePoolSpec.Disruption.Budgets))
	}
	if got.NodePoolSpec.Disruption.Budgets[0].Nodes != "2" {
		t.Fatalf("budget nodes = %q", got.NodePoolSpec.Disruption.Budgets[0].Nodes)
	}
}

func TestEC2NodePoolToModelReadsLabelsAndTaints(t *testing.T) {
	nodePool := EC2NodePool{
		Name:         "cloudpilot-general",
		NodePoolSpec: DefaultGeneralEC2NodePoolSpec(),
	}
	nodePool.NodePoolSpec.Template.ObjectMeta.Labels = map[string]string{"team": "platform"}
	nodePool.NodePoolSpec.Template.Spec.Taints = []corev1.Taint{{
		Key:    "dedicated",
		Value:  "wa",
		Effect: corev1.TaintEffectNoSchedule,
	}}

	model, err := nodePool.ToEC2NodePoolModel()
	if err != nil {
		t.Fatalf("ToEC2NodePoolModel() error = %v", err)
	}
	labels, diags := model.Labels.Value(context.Background())
	if diags.HasError() {
		t.Fatalf("labels diagnostics = %v", diags)
	}
	if labels["team"].ValueString() != "platform" {
		t.Fatalf("labels = %#v", labels)
	}
	taints, diags := model.Taints.AsStructSliceT(context.Background())
	if diags.HasError() {
		t.Fatalf("taints diagnostics = %v", diags)
	}
	if len(taints) != 1 || taints[0].Key.ValueString() != "dedicated" {
		t.Fatalf("taints = %#v", taints)
	}
}
