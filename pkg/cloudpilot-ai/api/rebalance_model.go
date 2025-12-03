package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	awscorev1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter/apis/v1"
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

func (w *WorkloadModel) ToWorkload(workloadTemplate *WorkloadTemplateModel, replicas int) *Workload {
	workload := Workload{
		Name:      w.Name.ValueString(),
		Type:      w.Type.ValueString(),
		Namespace: w.Namespace.ValueString(),
		Replicas:  replicas,
	}

	if workloadTemplate != nil {
		workload.RebalanceAble = workloadTemplate.RebalanceAble.ValueBool()
		workload.SpotFriendly = workloadTemplate.SpotFriendly.ValueBool()
		workload.MinNonSpotReplicas = int(workloadTemplate.MinNonSpotReplicas.ValueInt64())
	} else {
		workload.RebalanceAble = w.RebalanceAble.ValueBool()
		workload.SpotFriendly = w.SpotFriendly.ValueBool()
		workload.MinNonSpotReplicas = int(w.MinNonSpotReplicas.ValueInt64())
	}

	return applyTemplate(applyTemplate(&workload, workloadTemplate), workloadTemplate)
}

func applyTemplate(workload *Workload, workloadTemplate *WorkloadTemplateModel) *Workload {
	if workloadTemplate == nil {
		return workload
	}

	if workloadTemplate.RebalanceAble.IsNull() || workloadTemplate.RebalanceAble.IsUnknown() {
		workload.RebalanceAble = workloadTemplate.RebalanceAble.ValueBool()
	}

	if workloadTemplate.SpotFriendly.IsNull() || workloadTemplate.SpotFriendly.IsUnknown() {
		workload.SpotFriendly = workloadTemplate.SpotFriendly.ValueBool()
	}

	if workloadTemplate.MinNonSpotReplicas.IsNull() || workloadTemplate.MinNonSpotReplicas.IsUnknown() {
		workload.MinNonSpotReplicas = int(workloadTemplate.MinNonSpotReplicas.ValueInt64())
	}

	return workload
}

type EC2NodeClassTemplateModel struct {
	TemplateName types.String `tfsdk:"template_name"`

	InstanceTags             customfield.Map[types.String] `tfsdk:"instance_tags"`
	SystemDiskSizeGib        types.Int64                   `tfsdk:"system_disk_size_gib"`
	ExtraCPUAllocationMCore  types.Int64                   `tfsdk:"extra_cpu_allocation_mcore"`
	ExtraMemoryAllocationMib types.Int64                   `tfsdk:"extra_memory_allocation_mib"`
}

type EC2NodeClassModel struct {
	Name types.String `tfsdk:"name"`

	TemplateName types.String `tfsdk:"template_name"`

	InstanceTags             customfield.Map[types.String] `tfsdk:"instance_tags"`
	SystemDiskSizeGib        types.Int64                   `tfsdk:"system_disk_size_gib"`
	ExtraCPUAllocationMCore  types.Int64                   `tfsdk:"extra_cpu_allocation_mcore"`
	ExtraMemoryAllocationMib types.Int64                   `tfsdk:"extra_memory_allocation_mib"`

	// TODO: When the origin_nodeclass_json is configured, the other configuration items are invalid.
	OriginNodeClassJSON types.String `tfsdk:"origin_nodeclass_json"`
}

func (e *EC2NodeClassModel) ToEC2NodeClassTemplateModel() *EC2NodeClassTemplateModel {
	return &EC2NodeClassTemplateModel{
		TemplateName: e.TemplateName,

		InstanceTags:             e.InstanceTags,
		SystemDiskSizeGib:        e.SystemDiskSizeGib,
		ExtraCPUAllocationMCore:  e.ExtraCPUAllocationMCore,
		ExtraMemoryAllocationMib: e.ExtraMemoryAllocationMib,
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

	if !ec2NodeClassTemplate.InstanceTags.IsNull() && !ec2NodeClassTemplate.InstanceTags.IsUnknown() {
		nodeclass.NodeClassSpec.Tags = make(map[string]string)
		instanceTags, diagnostics := ec2NodeClassTemplate.InstanceTags.Value(ctx)
		if diagnostics.HasError() {
			return nil, fmt.Errorf("failed to parse instance_tags: %v", diagnostics)
		}

		nodeclass.NodeClassSpec.Tags = lo.MapValues(instanceTags, func(v types.String, key string) string {
			return v.String()
		})
	}

	// TODO: Force add CloudPilot management label to prevent anomalies during automated processing
	{
		if nodeclass.NodeClassSpec.Tags == nil {
			nodeclass.NodeClassSpec.Tags = make(map[string]string)
		}

		nodeclass.NodeClassSpec.Tags[CloudPilotManagedNodeLabelKey] = "true"
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

	return nodeclass, nil
}

type EC2NodePoolTemplateModel struct {
	TemplateName types.String `tfsdk:"template_name"`

	Enable    types.Bool   `tfsdk:"enable"`
	NodeClass types.String `tfsdk:"nodeclass"`

	EnableGPU types.Bool `tfsdk:"enable_gpu"`

	ProvisionPriority   types.Int32     `tfsdk:"provision_priority"`
	InstanceFamily      *[]types.String `tfsdk:"instance_family"`
	InstanceArch        *[]types.String `tfsdk:"instance_arch"`
	CapacityType        *[]types.String `tfsdk:"capacity_type"`
	Zone                *[]types.String `tfsdk:"zone"`
	InstanceCPUMAX      types.Int64     `tfsdk:"instance_cpu_max"`
	InstanceCPUMIN      types.Int64     `tfsdk:"instance_cpu_min"`
	InstanceMemoryMAX   types.Int64     `tfsdk:"instance_memory_max"`
	InstanceMemoryMIN   types.Int64     `tfsdk:"instance_memory_min"`
	NodeDisruptionLimit types.String    `tfsdk:"node_disruption_limit"`
	NodeDisruptionDelay types.String    `tfsdk:"node_disruption_delay"`
}

type EC2NodePoolModel struct {
	Name types.String `tfsdk:"name"`

	TemplateName types.String `tfsdk:"template_name"`

	Enable    types.Bool   `tfsdk:"enable"`
	NodeClass types.String `tfsdk:"nodeclass"`

	EnableGPU types.Bool `tfsdk:"enable_gpu"`

	ProvisionPriority   types.Int32     `tfsdk:"provision_priority"`
	InstanceFamily      *[]types.String `tfsdk:"instance_family"`
	InstanceArch        *[]types.String `tfsdk:"instance_arch"`
	CapacityType        *[]types.String `tfsdk:"capacity_type"`
	Zone                *[]types.String `tfsdk:"zone"`
	InstanceCPUMAX      types.Int64     `tfsdk:"instance_cpu_max"`
	InstanceCPUMIN      types.Int64     `tfsdk:"instance_cpu_min"`
	InstanceMemoryMAX   types.Int64     `tfsdk:"instance_memory_max"`
	InstanceMemoryMIN   types.Int64     `tfsdk:"instance_memory_min"`
	NodeDisruptionLimit types.String    `tfsdk:"node_disruption_limit"`
	NodeDisruptionDelay types.String    `tfsdk:"node_disruption_delay"`

	// TODO: When the origin_nodepool_json is configured, the other configuration items are invalid.
	OriginNodePoolJSON types.String `tfsdk:"origin_nodepool_json"`
}

func (e *EC2NodePoolModel) ToEC2NodePoolTemplateModel() *EC2NodePoolTemplateModel {
	return &EC2NodePoolTemplateModel{
		TemplateName: e.TemplateName,

		Enable:    e.Enable,
		NodeClass: e.NodeClass,

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

	return applyEC2NodePoolTemplateModel(applyEC2NodePoolTemplateModel(&nodepool, ec2NodePoolTemplate),
		e.ToEC2NodePoolTemplateModel()), nil
}

func applyEC2NodePoolTemplateModel(nodepool *EC2NodePool, ec2NodePoolTemplate *EC2NodePoolTemplateModel) *EC2NodePool {
	if ec2NodePoolTemplate == nil {
		return nodepool
	}

	if nodepool.NodePoolSpec == nil {
		nodepool.NodePoolSpec = EnableGPUEC2NodePoolSpec(nil, ec2NodePoolTemplate.EnableGPU.ValueBool())
	}

	if !ec2NodePoolTemplate.Enable.IsNull() && !ec2NodePoolTemplate.Enable.IsUnknown() {
		nodepool.Enable = ec2NodePoolTemplate.Enable.ValueBool()
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
		if len(nodepool.NodePoolSpec.Disruption.Budgets) == 0 {
			nodepool.NodePoolSpec.Disruption.Budgets = []awscorev1.Budget{}
		}

		nodepool.NodePoolSpec.Disruption.Budgets[0].Nodes = ec2NodePoolTemplate.NodeDisruptionLimit.ValueString()
	}

	if !ec2NodePoolTemplate.NodeDisruptionDelay.IsNull() && !ec2NodePoolTemplate.NodeDisruptionDelay.IsUnknown() {
		nodepool.NodePoolSpec.Disruption.ConsolidateAfter = awscorev1.MustParseNillableDuration(ec2NodePoolTemplate.NodeDisruptionDelay.ValueString())
	}

	return nodepool
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
