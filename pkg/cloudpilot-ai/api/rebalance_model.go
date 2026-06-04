package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	awscorev1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter/apis/v1"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

type WorkloadTemplateModel struct {
	TemplateName types.String `tfsdk:"template_name"`

	RebalanceAble      types.Bool  `tfsdk:"rebalance_able"`
	SpotFriendly       types.Bool  `tfsdk:"spot_friendly"`
	MinNonSpotReplicas types.Int64 `tfsdk:"min_non_spot_replicas"`
}

type WorkloadModel struct {
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	Namespace types.String `tfsdk:"namespace"`

	TemplateName types.String `tfsdk:"template_name"`

	RebalanceAble      types.Bool  `tfsdk:"rebalance_able"`
	SpotFriendly       types.Bool  `tfsdk:"spot_friendly"`
	MinNonSpotReplicas types.Int64 `tfsdk:"min_non_spot_replicas"`
}

func (w *WorkloadModel) ToWorkload(base *Workload, workloadTemplate *WorkloadTemplateModel, replicas int) *Workload {
	workload := Workload{}
	if base != nil {
		workload = *base
	}

	workload.Name = w.Name.ValueString()
	workload.Type = w.Type.ValueString()
	workload.Namespace = w.Namespace.ValueString()
	workload.Replicas = replicas

	if !w.RebalanceAble.IsNull() && !w.RebalanceAble.IsUnknown() {
		workload.RebalanceAble = w.RebalanceAble.ValueBool()
	}
	if !w.SpotFriendly.IsNull() && !w.SpotFriendly.IsUnknown() {
		workload.SpotFriendly = w.SpotFriendly.ValueBool()
	}
	if !w.MinNonSpotReplicas.IsNull() && !w.MinNonSpotReplicas.IsUnknown() {
		workload.MinNonSpotReplicas = int(w.MinNonSpotReplicas.ValueInt64())
	}

	return applyTemplate(&workload, workloadTemplate)
}

// applyTemplate overlays explicitly-set template fields onto the workload,
// leaving workload's own values as defaults for any unset template fields.
func applyTemplate(workload *Workload, workloadTemplate *WorkloadTemplateModel) *Workload {
	if workloadTemplate == nil {
		return workload
	}

	if !workloadTemplate.RebalanceAble.IsNull() && !workloadTemplate.RebalanceAble.IsUnknown() {
		workload.RebalanceAble = workloadTemplate.RebalanceAble.ValueBool()
	}

	if !workloadTemplate.SpotFriendly.IsNull() && !workloadTemplate.SpotFriendly.IsUnknown() {
		workload.SpotFriendly = workloadTemplate.SpotFriendly.ValueBool()
	}

	if !workloadTemplate.MinNonSpotReplicas.IsNull() && !workloadTemplate.MinNonSpotReplicas.IsUnknown() {
		workload.MinNonSpotReplicas = int(workloadTemplate.MinNonSpotReplicas.ValueInt64())
	}

	return workload
}

// SubnetSelectorTermModel mirrors subnet selector term semantics (tags and/or id; id is mutually exclusive with tags).
type SubnetSelectorTermModel struct {
	Tags customfield.Map[types.String] `tfsdk:"tags"`
	ID   types.String                  `tfsdk:"id"`
}

// SecurityGroupSelectorTermModel mirrors security group selector term semantics (exactly one of tags, id, or name per block).
type SecurityGroupSelectorTermModel struct {
	Tags customfield.Map[types.String] `tfsdk:"tags"`
	ID   types.String                  `tfsdk:"id"`
	Name types.String                  `tfsdk:"name"`
}

type BlockDeviceModel struct {
	Encrypted  types.Bool   `tfsdk:"encrypted"`
	VolumeSize types.String `tfsdk:"volume_size"`
	VolumeType types.String `tfsdk:"volume_type"`
}

type BlockDeviceMappingModel struct {
	DeviceName types.String                               `tfsdk:"device_name"`
	RootVolume types.Bool                                 `tfsdk:"root_volume"`
	EBS        customfield.NestedObject[BlockDeviceModel] `tfsdk:"ebs"`
}

type EC2NodeClassTemplateModel struct {
	TemplateName types.String `tfsdk:"template_name"`

	Role                   types.String `tfsdk:"role"`
	EnableImageAccelerator types.Bool   `tfsdk:"enable_image_accelerator"`
	AmiAlias               types.String `tfsdk:"ami_alias"`
	UserData               types.String `tfsdk:"user_data"`

	SubnetSelectorTerms        customfield.NestedObjectList[SubnetSelectorTermModel]        `tfsdk:"subnet_selector_terms"`
	SecurityGroupSelectorTerms customfield.NestedObjectList[SecurityGroupSelectorTermModel] `tfsdk:"security_group_selector_terms"`

	InstanceTags             customfield.Map[types.String]                         `tfsdk:"instance_tags"`
	SystemDiskSizeGib        types.Int64                                           `tfsdk:"system_disk_size_gib"`
	BlockDeviceMappings      customfield.NestedObjectList[BlockDeviceMappingModel] `tfsdk:"block_device_mappings"`
	ExtraCPUAllocationMCore  types.Int64                                           `tfsdk:"extra_cpu_allocation_mcore"`
	ExtraMemoryAllocationMib types.Int64                                           `tfsdk:"extra_memory_allocation_mib"`
}

type EC2NodeClassModel struct {
	Name types.String `tfsdk:"name"`

	TemplateName types.String `tfsdk:"template_name"`

	Role                   types.String `tfsdk:"role"`
	EnableImageAccelerator types.Bool   `tfsdk:"enable_image_accelerator"`
	AmiAlias               types.String `tfsdk:"ami_alias"`
	UserData               types.String `tfsdk:"user_data"`

	SubnetSelectorTerms        customfield.NestedObjectList[SubnetSelectorTermModel]        `tfsdk:"subnet_selector_terms"`
	SecurityGroupSelectorTerms customfield.NestedObjectList[SecurityGroupSelectorTermModel] `tfsdk:"security_group_selector_terms"`

	InstanceTags             customfield.Map[types.String]                         `tfsdk:"instance_tags"`
	SystemDiskSizeGib        types.Int64                                           `tfsdk:"system_disk_size_gib"`
	BlockDeviceMappings      customfield.NestedObjectList[BlockDeviceMappingModel] `tfsdk:"block_device_mappings"`
	ExtraCPUAllocationMCore  types.Int64                                           `tfsdk:"extra_cpu_allocation_mcore"`
	ExtraMemoryAllocationMib types.Int64                                           `tfsdk:"extra_memory_allocation_mib"`

	// TODO: When the origin_nodeclass_json is configured, the other configuration items are invalid.
	OriginNodeClassJSON types.String `tfsdk:"origin_nodeclass_json"`
}

func (e *EC2NodeClassModel) ToEC2NodeClassTemplateModel() *EC2NodeClassTemplateModel {
	return &EC2NodeClassTemplateModel{
		TemplateName: e.TemplateName,

		Role:                   e.Role,
		EnableImageAccelerator: e.EnableImageAccelerator,
		AmiAlias:               e.AmiAlias,
		UserData:               e.UserData,

		SubnetSelectorTerms:        e.SubnetSelectorTerms,
		SecurityGroupSelectorTerms: e.SecurityGroupSelectorTerms,
		InstanceTags:               e.InstanceTags,
		SystemDiskSizeGib:          e.SystemDiskSizeGib,
		BlockDeviceMappings:        e.BlockDeviceMappings,
		ExtraCPUAllocationMCore:    e.ExtraCPUAllocationMCore,
		ExtraMemoryAllocationMib:   e.ExtraMemoryAllocationMib,
	}
}

func (e *EC2NodeClassModel) ToEC2NodeClass(ctx context.Context, clusterName string, nodeclass EC2NodeClass, ec2NodeClassTemplate *EC2NodeClassTemplateModel) (*EC2NodeClass, error) {
	if !e.OriginNodeClassJSON.IsNull() && !e.OriginNodeClassJSON.IsUnknown() && e.OriginNodeClassJSON.ValueString() != "" {
		var ec2NodeClass EC2NodeClass
		if err := json.Unmarshal([]byte(e.OriginNodeClassJSON.ValueString()), &ec2NodeClass); err != nil {
			return nil, err
		}

		return &ec2NodeClass, nil
	}

	newNodeClass, err := applyEC2NodeClassTemplateModel(ctx, clusterName, &nodeclass, ec2NodeClassTemplate)
	if err != nil {
		return nil, err
	}

	return applyEC2NodeClassTemplateModel(ctx, clusterName, newNodeClass, e.ToEC2NodeClassTemplateModel())
}

func applyEC2NodeClassTemplateModel(ctx context.Context, clusterName string, nodeclass *EC2NodeClass, ec2NodeClassTemplate *EC2NodeClassTemplateModel) (*EC2NodeClass, error) {
	if nodeclass.NodeClassSpec == nil {
		nodeclass.NodeClassSpec = DefaultEC2NodeClassSpec(clusterName)
	}

	if ec2NodeClassTemplate == nil {
		return nodeclass, nil
	}

	if !ec2NodeClassTemplate.Role.IsNull() && !ec2NodeClassTemplate.Role.IsUnknown() && ec2NodeClassTemplate.Role.ValueString() != "" {
		nodeclass.NodeClassSpec.Role = ec2NodeClassTemplate.Role.ValueString()
	}

	if !ec2NodeClassTemplate.EnableImageAccelerator.IsNull() && !ec2NodeClassTemplate.EnableImageAccelerator.IsUnknown() {
		nodeclass.EnableImageAccelerator = ec2NodeClassTemplate.EnableImageAccelerator.ValueBool()
	}

	if !ec2NodeClassTemplate.SubnetSelectorTerms.IsNull() && !ec2NodeClassTemplate.SubnetSelectorTerms.IsUnknown() {
		slice, diagnostics := ec2NodeClassTemplate.SubnetSelectorTerms.AsStructSliceT(ctx)
		if diagnostics.HasError() {
			return nil, fmt.Errorf("subnet_selector_terms: %v", diagnostics)
		}
		if len(slice) == 0 {
			return nil, fmt.Errorf("subnet_selector_terms cannot be empty; omit the attribute to use defaults")
		}
		terms := make([]awsproviderv1.SubnetSelectorTerm, 0, len(slice))
		for i, m := range slice {
			t, err := subnetSelectorTermModelToAWS(ctx, m, i)
			if err != nil {
				return nil, err
			}
			terms = append(terms, t)
		}
		nodeclass.NodeClassSpec.SubnetSelectorTerms = terms
	}

	if !ec2NodeClassTemplate.SecurityGroupSelectorTerms.IsNull() && !ec2NodeClassTemplate.SecurityGroupSelectorTerms.IsUnknown() {
		slice, diagnostics := ec2NodeClassTemplate.SecurityGroupSelectorTerms.AsStructSliceT(ctx)
		if diagnostics.HasError() {
			return nil, fmt.Errorf("security_group_selector_terms: %v", diagnostics)
		}
		if len(slice) == 0 {
			return nil, fmt.Errorf("security_group_selector_terms cannot be empty; omit the attribute to use defaults")
		}
		terms := make([]awsproviderv1.SecurityGroupSelectorTerm, 0, len(slice))
		for i, m := range slice {
			t, err := securityGroupSelectorTermModelToAWS(ctx, m, i)
			if err != nil {
				return nil, err
			}
			terms = append(terms, t)
		}
		nodeclass.NodeClassSpec.SecurityGroupSelectorTerms = terms
	}

	if !ec2NodeClassTemplate.InstanceTags.IsNull() && !ec2NodeClassTemplate.InstanceTags.IsUnknown() {
		nodeclass.NodeClassSpec.Tags = make(map[string]string)
		instanceTags, diagnostics := ec2NodeClassTemplate.InstanceTags.Value(ctx)
		if diagnostics.HasError() {
			return nil, fmt.Errorf("failed to parse instance_tags: %v", diagnostics)
		}

		nodeclass.NodeClassSpec.Tags = lo.MapValues(instanceTags, func(v types.String, key string) string {
			return v.ValueString()
		})
	}

	if !ec2NodeClassTemplate.SystemDiskSizeGib.IsNull() && !ec2NodeClassTemplate.SystemDiskSizeGib.IsUnknown() {
		if len(nodeclass.NodeClassSpec.BlockDeviceMappings) == 0 {
			nodeclass.NodeClassSpec.BlockDeviceMappings = []*awsproviderv1.BlockDeviceMapping{
				{
					DeviceName: aws.String("/dev/xvda"),
					EBS: &awsproviderv1.BlockDevice{
						Encrypted:  aws.Bool(true),
						VolumeType: aws.String("gp3"),
					},
				},
			}
		}

		blockSize := resource.MustParse(fmt.Sprintf("%dGi", ec2NodeClassTemplate.SystemDiskSizeGib.ValueInt64()))
		nodeclass.NodeClassSpec.BlockDeviceMappings[0].EBS.VolumeSize = &blockSize
	}

	if !ec2NodeClassTemplate.ExtraCPUAllocationMCore.IsNull() && !ec2NodeClassTemplate.ExtraCPUAllocationMCore.IsUnknown() {
		if nodeclass.NodeClassSpec.Kubelet == nil {
			nodeclass.NodeClassSpec.Kubelet = &awsproviderv1.KubeletConfiguration{
				KubeReserved: make(map[string]string),
			}
		}

		if nodeclass.NodeClassSpec.Kubelet.KubeReserved == nil {
			nodeclass.NodeClassSpec.Kubelet.KubeReserved = make(map[string]string)
		}
		nodeclass.NodeClassSpec.Kubelet.KubeReserved["cpu"] = fmt.Sprintf("%dm", ec2NodeClassTemplate.ExtraCPUAllocationMCore.ValueInt64())
	}

	if !ec2NodeClassTemplate.ExtraMemoryAllocationMib.IsNull() && !ec2NodeClassTemplate.ExtraMemoryAllocationMib.IsUnknown() {
		if nodeclass.NodeClassSpec.Kubelet == nil {
			nodeclass.NodeClassSpec.Kubelet = &awsproviderv1.KubeletConfiguration{
				KubeReserved: make(map[string]string),
			}
		}

		if nodeclass.NodeClassSpec.Kubelet.KubeReserved == nil {
			nodeclass.NodeClassSpec.Kubelet.KubeReserved = make(map[string]string)
		}
		nodeclass.NodeClassSpec.Kubelet.KubeReserved["memory"] = fmt.Sprintf("%dMi", ec2NodeClassTemplate.ExtraMemoryAllocationMib.ValueInt64())
	}

	applyAmiAlias(nodeclass, ec2NodeClassTemplate.AmiAlias)
	applyUserData(nodeclass, ec2NodeClassTemplate.UserData)
	if err := applyBlockDeviceMappings(ctx, nodeclass, ec2NodeClassTemplate.BlockDeviceMappings); err != nil {
		return nil, err
	}

	return nodeclass, nil
}

func applyAmiAlias(nodeclass *EC2NodeClass, alias types.String) {
	if alias.IsNull() || alias.IsUnknown() {
		return
	}
	value := strings.TrimSpace(alias.ValueString())
	terms := nodeclass.NodeClassSpec.AMISelectorTerms[:0]
	for _, term := range nodeclass.NodeClassSpec.AMISelectorTerms {
		if term.Alias == "" {
			terms = append(terms, term)
		}
	}
	if value != "" {
		terms = append(terms, awsproviderv1.AMISelectorTerm{Alias: value})
	}
	nodeclass.NodeClassSpec.AMISelectorTerms = terms
}

func applyUserData(nodeclass *EC2NodeClass, userData types.String) {
	if userData.IsNull() || userData.IsUnknown() {
		return
	}
	value := userData.ValueString()
	if value == "" {
		nodeclass.NodeClassSpec.UserData = nil
		return
	}
	nodeclass.NodeClassSpec.UserData = &value
}

func applyBlockDeviceMappings(ctx context.Context, nodeclass *EC2NodeClass, mappings customfield.NestedObjectList[BlockDeviceMappingModel]) error {
	if mappings.IsNullOrUnknown() {
		return nil
	}
	models, diags := mappings.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("block_device_mappings: %v", diags)
	}
	out := make([]*awsproviderv1.BlockDeviceMapping, 0, len(models))
	for index, m := range models {
		mapping := cloneBlockDeviceMapping(selectExistingBlockDeviceMapping(nodeclass.NodeClassSpec.BlockDeviceMappings, index, m.DeviceName))
		if !m.DeviceName.IsNull() && !m.DeviceName.IsUnknown() && m.DeviceName.ValueString() != "" {
			v := m.DeviceName.ValueString()
			mapping.DeviceName = &v
		}
		if !m.RootVolume.IsNull() && !m.RootVolume.IsUnknown() {
			mapping.RootVolume = m.RootVolume.ValueBool()
		}
		if !m.EBS.IsNull() && !m.EBS.IsUnknown() {
			ebsModel, ebsDiags := m.EBS.Value(ctx)
			if ebsDiags.HasError() {
				return fmt.Errorf("block_device_mappings.ebs: %v", ebsDiags)
			}
			if ebsModel != nil {
				ebs, err := blockDeviceModelToAWS(mapping.EBS, *ebsModel)
				if err != nil {
					return err
				}
				mapping.EBS = ebs
			}
		}
		out = append(out, mapping)
	}
	nodeclass.NodeClassSpec.BlockDeviceMappings = out
	return nil
}

func blockDeviceModelToAWS(base *awsproviderv1.BlockDevice, m BlockDeviceModel) (*awsproviderv1.BlockDevice, error) {
	out := cloneHiddenBlockDeviceFields(base)
	if !m.Encrypted.IsNull() && !m.Encrypted.IsUnknown() {
		v := m.Encrypted.ValueBool()
		out.Encrypted = &v
	}
	if !m.VolumeSize.IsNull() && !m.VolumeSize.IsUnknown() && m.VolumeSize.ValueString() != "" {
		q, err := resource.ParseQuantity(m.VolumeSize.ValueString())
		if err != nil {
			return nil, fmt.Errorf("block_device_mappings.ebs.volume_size: %w", err)
		}
		out.VolumeSize = &q
	}
	if !m.VolumeType.IsNull() && !m.VolumeType.IsUnknown() && m.VolumeType.ValueString() != "" {
		v := m.VolumeType.ValueString()
		out.VolumeType = &v
	}
	return out, nil
}

func cloneHiddenBlockDeviceFields(in *awsproviderv1.BlockDevice) *awsproviderv1.BlockDevice {
	if in == nil {
		return &awsproviderv1.BlockDevice{}
	}
	return &awsproviderv1.BlockDevice{
		DeleteOnTermination: in.DeleteOnTermination,
		IOPS:                in.IOPS,
		KMSKeyID:            in.KMSKeyID,
		SnapshotID:          in.SnapshotID,
		Throughput:          in.Throughput,
	}
}

func selectExistingBlockDeviceMapping(existing []*awsproviderv1.BlockDeviceMapping, index int, deviceName types.String) *awsproviderv1.BlockDeviceMapping {
	if !deviceName.IsNull() && !deviceName.IsUnknown() && deviceName.ValueString() != "" {
		for _, mapping := range existing {
			if mapping != nil && mapping.DeviceName != nil && *mapping.DeviceName == deviceName.ValueString() {
				return mapping
			}
		}
	}
	if index < len(existing) {
		return existing[index]
	}
	return nil
}

func cloneBlockDeviceMapping(in *awsproviderv1.BlockDeviceMapping) *awsproviderv1.BlockDeviceMapping {
	if in == nil {
		return &awsproviderv1.BlockDeviceMapping{}
	}
	out := *in
	out.EBS = cloneBlockDevice(in.EBS)
	return &out
}

func cloneBlockDevice(in *awsproviderv1.BlockDevice) *awsproviderv1.BlockDevice {
	if in == nil {
		return &awsproviderv1.BlockDevice{}
	}
	out := *in
	return &out
}

type EC2NodePoolTemplateModel struct {
	TemplateName types.String `tfsdk:"template_name"`

	Enable                 types.Bool   `tfsdk:"enable"`
	NodeClass              types.String `tfsdk:"nodeclass"`
	EnableImageAccelerator types.Bool   `tfsdk:"enable_image_accelerator"`

	EnableGPU types.Bool `tfsdk:"enable_gpu"`

	ProvisionPriority   types.Int32                              `tfsdk:"provision_priority"`
	InstanceFamily      *[]types.String                          `tfsdk:"instance_family"`
	InstanceArch        *[]types.String                          `tfsdk:"instance_arch"`
	CapacityType        *[]types.String                          `tfsdk:"capacity_type"`
	Zone                *[]types.String                          `tfsdk:"zone"`
	InstanceCPUMAX      types.Int64                              `tfsdk:"instance_cpu_max"`
	InstanceCPUMIN      types.Int64                              `tfsdk:"instance_cpu_min"`
	InstanceMemoryMAX   types.Int64                              `tfsdk:"instance_memory_max"`
	InstanceMemoryMIN   types.Int64                              `tfsdk:"instance_memory_min"`
	NodeDisruptionLimit types.String                             `tfsdk:"node_disruption_limit"`
	NodeDisruptionDelay types.String                             `tfsdk:"node_disruption_delay"`
	Labels              customfield.Map[types.String]            `tfsdk:"labels"`
	Taints              customfield.NestedObjectList[TaintModel] `tfsdk:"taints"`
}

type TaintModel struct {
	Key    types.String `tfsdk:"key"`
	Value  types.String `tfsdk:"value"`
	Effect types.String `tfsdk:"effect"`
}

type EC2NodePoolModel struct {
	Name types.String `tfsdk:"name"`

	TemplateName types.String `tfsdk:"template_name"`

	Enable                 types.Bool   `tfsdk:"enable"`
	NodeClass              types.String `tfsdk:"nodeclass"`
	EnableImageAccelerator types.Bool   `tfsdk:"enable_image_accelerator"`

	EnableGPU types.Bool `tfsdk:"enable_gpu"`

	ProvisionPriority   types.Int32                              `tfsdk:"provision_priority"`
	InstanceFamily      *[]types.String                          `tfsdk:"instance_family"`
	InstanceArch        *[]types.String                          `tfsdk:"instance_arch"`
	CapacityType        *[]types.String                          `tfsdk:"capacity_type"`
	Zone                *[]types.String                          `tfsdk:"zone"`
	InstanceCPUMAX      types.Int64                              `tfsdk:"instance_cpu_max"`
	InstanceCPUMIN      types.Int64                              `tfsdk:"instance_cpu_min"`
	InstanceMemoryMAX   types.Int64                              `tfsdk:"instance_memory_max"`
	InstanceMemoryMIN   types.Int64                              `tfsdk:"instance_memory_min"`
	NodeDisruptionLimit types.String                             `tfsdk:"node_disruption_limit"`
	NodeDisruptionDelay types.String                             `tfsdk:"node_disruption_delay"`
	Labels              customfield.Map[types.String]            `tfsdk:"labels"`
	Taints              customfield.NestedObjectList[TaintModel] `tfsdk:"taints"`

	// TODO: When the origin_nodepool_json is configured, the other configuration items are invalid.
	OriginNodePoolJSON types.String `tfsdk:"origin_nodepool_json"`
}

func (e *EC2NodePoolModel) ToEC2NodePoolTemplateModel() *EC2NodePoolTemplateModel {
	return &EC2NodePoolTemplateModel{
		TemplateName: e.TemplateName,

		Enable:                 e.Enable,
		NodeClass:              e.NodeClass,
		EnableImageAccelerator: e.EnableImageAccelerator,

		EnableGPU: e.EnableGPU,

		ProvisionPriority:   e.ProvisionPriority,
		InstanceFamily:      e.InstanceFamily,
		InstanceArch:        e.InstanceArch,
		CapacityType:        e.CapacityType,
		Zone:                e.Zone,
		InstanceCPUMAX:      e.InstanceCPUMAX,
		InstanceCPUMIN:      e.InstanceCPUMIN,
		InstanceMemoryMAX:   e.InstanceMemoryMAX,
		InstanceMemoryMIN:   e.InstanceMemoryMIN,
		NodeDisruptionLimit: e.NodeDisruptionLimit,
		NodeDisruptionDelay: e.NodeDisruptionDelay,
		Labels:              e.Labels,
		Taints:              e.Taints,
	}
}

func (e *EC2NodePoolModel) ToEC2NodePool(ctx context.Context, nodepool EC2NodePool, ec2NodePoolTemplate *EC2NodePoolTemplateModel) (*EC2NodePool, error) {
	if !e.OriginNodePoolJSON.IsNull() && !e.OriginNodePoolJSON.IsUnknown() && e.OriginNodePoolJSON.ValueString() != "" {
		var ec2NodePool EC2NodePool
		if err := json.Unmarshal([]byte(e.OriginNodePoolJSON.ValueString()), &ec2NodePool); err != nil {
			return nil, err
		}

		return &ec2NodePool, nil
	}

	if nodepool.NodePoolSpec == nil {
		enableGPU := false
		if ec2NodePoolTemplate != nil {
			if !ec2NodePoolTemplate.EnableGPU.IsNull() && !ec2NodePoolTemplate.EnableGPU.IsUnknown() {
				enableGPU = ec2NodePoolTemplate.EnableGPU.ValueBool()
			}
		}

		if !e.EnableGPU.IsNull() && !e.EnableGPU.IsUnknown() {
			enableGPU = e.EnableGPU.ValueBool()
		}

		nodepool.NodePoolSpec = EnableGPUEC2NodePoolSpec(nil, enableGPU)
	}

	nodepoolPtr := &nodepool
	var err error
	if nodepoolPtr, err = applyEC2NodePoolTemplateModel(ctx, nodepoolPtr, ec2NodePoolTemplate); err != nil {
		return nil, err
	}
	return applyEC2NodePoolTemplateModel(ctx, nodepoolPtr, e.ToEC2NodePoolTemplateModel())
}

func applyEC2NodePoolTemplateModel(ctx context.Context, nodepool *EC2NodePool, ec2NodePoolTemplate *EC2NodePoolTemplateModel) (*EC2NodePool, error) {
	if ec2NodePoolTemplate == nil {
		return nodepool, nil
	}

	if nodepool.NodePoolSpec == nil {
		nodepool.NodePoolSpec = EnableGPUEC2NodePoolSpec(nil, ec2NodePoolTemplate.EnableGPU.ValueBool())
	}

	if !ec2NodePoolTemplate.Enable.IsNull() && !ec2NodePoolTemplate.Enable.IsUnknown() {
		nodepool.Enable = ec2NodePoolTemplate.Enable.ValueBool()
	}

	if !ec2NodePoolTemplate.EnableImageAccelerator.IsNull() && !ec2NodePoolTemplate.EnableImageAccelerator.IsUnknown() {
		nodepool.EnableImageAccelerator = ec2NodePoolTemplate.EnableImageAccelerator.ValueBool()
	}

	nodepool.NodePoolSpec.Template.Spec.NodeClassRef.Name = ec2NodePoolTemplate.NodeClass.ValueString()

	if !ec2NodePoolTemplate.ProvisionPriority.IsNull() && !ec2NodePoolTemplate.ProvisionPriority.IsUnknown() {
		provisionPriority := ec2NodePoolTemplate.ProvisionPriority.ValueInt32()
		nodepool.NodePoolSpec.Weight = &provisionPriority
	}

	if ec2NodePoolTemplate.InstanceFamily != nil {
		values := lo.Map(*ec2NodePoolTemplate.InstanceFamily, func(item types.String, _ int) string { return item.ValueString() })
		nodepool.NodePoolSpec.Template.Spec.Requirements = updateRequirements(awsproviderv1.LabelInstanceFamily, corev1.NodeSelectorOpIn, values, nodepool.NodePoolSpec.Template.Spec.Requirements)
	}

	if ec2NodePoolTemplate.InstanceArch != nil {
		values := lo.Map(*ec2NodePoolTemplate.InstanceArch, func(item types.String, _ int) string { return item.ValueString() })
		nodepool.NodePoolSpec.Template.Spec.Requirements = updateRequirements(corev1.LabelArchStable, corev1.NodeSelectorOpIn, values, nodepool.NodePoolSpec.Template.Spec.Requirements)
	}

	if ec2NodePoolTemplate.CapacityType != nil {
		values := lo.Map(*ec2NodePoolTemplate.CapacityType, func(item types.String, _ int) string { return item.ValueString() })
		nodepool.NodePoolSpec.Template.Spec.Requirements = updateRequirements(awscorev1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, values, nodepool.NodePoolSpec.Template.Spec.Requirements)
	}

	if ec2NodePoolTemplate.Zone != nil {
		values := lo.Map(*ec2NodePoolTemplate.Zone, func(item types.String, _ int) string { return item.ValueString() })
		nodepool.NodePoolSpec.Template.Spec.Requirements = updateRequirements(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, values, nodepool.NodePoolSpec.Template.Spec.Requirements)
	}

	if !ec2NodePoolTemplate.InstanceCPUMAX.IsNull() && !ec2NodePoolTemplate.InstanceCPUMAX.IsUnknown() {
		nodepool.NodePoolSpec.Template.Spec.Requirements = updateRequirements(awsproviderv1.LabelInstanceCPU, corev1.NodeSelectorOpLt, []string{strconv.FormatInt(ec2NodePoolTemplate.InstanceCPUMAX.ValueInt64(), 10)}, nodepool.NodePoolSpec.Template.Spec.Requirements)
	}

	if !ec2NodePoolTemplate.InstanceCPUMIN.IsNull() && !ec2NodePoolTemplate.InstanceCPUMIN.IsUnknown() {
		nodepool.NodePoolSpec.Template.Spec.Requirements = updateRequirements(awsproviderv1.LabelInstanceCPU, corev1.NodeSelectorOpGt, []string{strconv.FormatInt(ec2NodePoolTemplate.InstanceCPUMIN.ValueInt64(), 10)}, nodepool.NodePoolSpec.Template.Spec.Requirements)
	}

	if !ec2NodePoolTemplate.InstanceMemoryMAX.IsNull() && !ec2NodePoolTemplate.InstanceMemoryMAX.IsUnknown() {
		nodepool.NodePoolSpec.Template.Spec.Requirements = updateRequirements(awsproviderv1.LabelInstanceMemory, corev1.NodeSelectorOpLt, []string{strconv.FormatInt(ec2NodePoolTemplate.InstanceMemoryMAX.ValueInt64(), 10)}, nodepool.NodePoolSpec.Template.Spec.Requirements)
	}

	if !ec2NodePoolTemplate.InstanceMemoryMIN.IsNull() && !ec2NodePoolTemplate.InstanceMemoryMIN.IsUnknown() {
		nodepool.NodePoolSpec.Template.Spec.Requirements = updateRequirements(awsproviderv1.LabelInstanceMemory, corev1.NodeSelectorOpGt, []string{strconv.FormatInt(ec2NodePoolTemplate.InstanceMemoryMIN.ValueInt64(), 10)}, nodepool.NodePoolSpec.Template.Spec.Requirements)
	}

	if !ec2NodePoolTemplate.NodeDisruptionLimit.IsNull() && !ec2NodePoolTemplate.NodeDisruptionLimit.IsUnknown() {
		ensureFirstDisruptionBudget(nodepool).Nodes = ec2NodePoolTemplate.NodeDisruptionLimit.ValueString()
	}

	if !ec2NodePoolTemplate.NodeDisruptionDelay.IsNull() && !ec2NodePoolTemplate.NodeDisruptionDelay.IsUnknown() {
		nodepool.NodePoolSpec.Disruption.ConsolidateAfter = awscorev1.MustParseNillableDuration(ec2NodePoolTemplate.NodeDisruptionDelay.ValueString())
	}

	if err := applyNodePoolLabels(ctx, nodepool, ec2NodePoolTemplate.Labels); err != nil {
		return nodepool, err
	}
	if err := applyNodePoolTaints(ctx, nodepool, ec2NodePoolTemplate.Taints); err != nil {
		return nodepool, err
	}

	return nodepool, nil
}

func applyNodePoolLabels(ctx context.Context, nodepool *EC2NodePool, labels customfield.Map[types.String]) error {
	if labels.IsNull() || labels.IsUnknown() {
		return nil
	}
	values, diags := labels.Value(ctx)
	if diags.HasError() {
		return fmt.Errorf("labels: %v", diags)
	}
	nodepool.NodePoolSpec.Template.ObjectMeta.Labels = map[string]string{}
	for k, v := range values {
		nodepool.NodePoolSpec.Template.ObjectMeta.Labels[k] = v.ValueString()
	}
	return nil
}

func applyNodePoolTaints(ctx context.Context, nodepool *EC2NodePool, taints customfield.NestedObjectList[TaintModel]) error {
	if taints.IsNullOrUnknown() {
		return nil
	}
	values, diags := taints.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("taints: %v", diags)
	}
	out := make([]corev1.Taint, 0, len(values))
	for _, t := range values {
		out = append(out, corev1.Taint{
			Key:    t.Key.ValueString(),
			Value:  t.Value.ValueString(),
			Effect: corev1.TaintEffect(t.Effect.ValueString()),
		})
	}
	nodepool.NodePoolSpec.Template.Spec.Taints = out
	return nil
}

func ensureFirstDisruptionBudget(nodepool *EC2NodePool) *awscorev1.Budget {
	if len(nodepool.NodePoolSpec.Disruption.Budgets) == 0 {
		nodepool.NodePoolSpec.Disruption.Budgets = []awscorev1.Budget{{}}
	}
	return &nodepool.NodePoolSpec.Disruption.Budgets[0]
}

func updateRequirements(key string, operator corev1.NodeSelectorOperator, values []string, requirements []awscorev1.NodeSelectorRequirementWithMinValues) []awscorev1.NodeSelectorRequirementWithMinValues {
	_, index, found := lo.FindIndexOf(requirements, func(item awscorev1.NodeSelectorRequirementWithMinValues) bool {
		return item.Key == key && item.Operator == operator
	})

	if found {
		if len(values) == 0 {
			requirements = lo.Drop(requirements, index)
			return requirements
		}

		requirements[index].Values = values
		return requirements
	}

	if len(values) == 0 {
		return requirements
	}

	requirements = append(requirements, awscorev1.NodeSelectorRequirementWithMinValues{
		NodeSelectorRequirement: corev1.NodeSelectorRequirement{
			Key:      key,
			Operator: operator,
			Values:   values,
		},
	})

	return requirements
}

func subnetSelectorTermModelToAWS(ctx context.Context, m SubnetSelectorTermModel, index int) (awsproviderv1.SubnetSelectorTerm, error) {
	hasID := !m.ID.IsNull() && !m.ID.IsUnknown() && strings.TrimSpace(m.ID.ValueString()) != ""
	var tagMap map[string]types.String
	hasTags := false
	if !m.Tags.IsNull() && !m.Tags.IsUnknown() {
		var diagnostics diag.Diagnostics
		tagMap, diagnostics = m.Tags.Value(ctx)
		if diagnostics.HasError() {
			return awsproviderv1.SubnetSelectorTerm{}, fmt.Errorf("subnet_selector_terms[%d] tags: %v", index, diagnostics)
		}
		hasTags = len(tagMap) > 0
	}
	if !hasID && !hasTags {
		return awsproviderv1.SubnetSelectorTerm{}, fmt.Errorf("subnet_selector_terms[%d]: set either id or non-empty tags", index)
	}
	if hasID && hasTags {
		return awsproviderv1.SubnetSelectorTerm{}, fmt.Errorf("subnet_selector_terms[%d]: id and tags are mutually exclusive", index)
	}
	var term awsproviderv1.SubnetSelectorTerm
	if hasID {
		term.ID = strings.TrimSpace(m.ID.ValueString())
	}
	if hasTags {
		term.Tags = lo.MapValues(tagMap, func(v types.String, _ string) string { return v.ValueString() })
	}
	return term, nil
}

func securityGroupSelectorTermModelToAWS(ctx context.Context, m SecurityGroupSelectorTermModel, index int) (awsproviderv1.SecurityGroupSelectorTerm, error) {
	hasID := !m.ID.IsNull() && !m.ID.IsUnknown() && strings.TrimSpace(m.ID.ValueString()) != ""
	hasName := !m.Name.IsNull() && !m.Name.IsUnknown() && strings.TrimSpace(m.Name.ValueString()) != ""
	var tagMap map[string]types.String
	hasTags := false
	if !m.Tags.IsNull() && !m.Tags.IsUnknown() {
		var diagnostics diag.Diagnostics
		tagMap, diagnostics = m.Tags.Value(ctx)
		if diagnostics.HasError() {
			return awsproviderv1.SecurityGroupSelectorTerm{}, fmt.Errorf("security_group_selector_terms[%d] tags: %v", index, diagnostics)
		}
		hasTags = len(tagMap) > 0
	}
	n := 0
	if hasID {
		n++
	}
	if hasName {
		n++
	}
	if hasTags {
		n++
	}
	if n != 1 {
		return awsproviderv1.SecurityGroupSelectorTerm{}, fmt.Errorf("security_group_selector_terms[%d]: exactly one of id, name, or non-empty tags must be set", index)
	}
	var term awsproviderv1.SecurityGroupSelectorTerm
	if hasID {
		term.ID = strings.TrimSpace(m.ID.ValueString())
	}
	if hasName {
		term.Name = strings.TrimSpace(m.Name.ValueString())
	}
	if hasTags {
		term.Tags = lo.MapValues(tagMap, func(v types.String, _ string) string { return v.ValueString() })
	}
	return term, nil
}
