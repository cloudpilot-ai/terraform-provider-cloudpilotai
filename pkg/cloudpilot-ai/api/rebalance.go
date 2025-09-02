package api

import (
	"context"
	"fmt"
	"strconv"

	alibabacloudproviderv1alpha1 "github.com/cloudpilot-ai/lib/pkg/alibabacloud/karpenter-provider-alibabacloud/apis/v1alpha1"
	alibabacloudcorev1 "github.com/cloudpilot-ai/lib/pkg/alibabacloud/karpenter/apis/v1"
	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	awscorev1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter/apis/v1"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

const (
	MilliCoreToCore = 1000.0
	BytesToGiB      = 1073741824.0
)

type RebalanceConfig struct {
	UploadConfig                bool   `json:"uploadConfig"`
	Enable                      bool   `json:"enable"`
	EnableDiversityInstanceType bool   `json:"enableDiversityInstanceType"`
	RebalanceType               string `json:"rebalanceType"`

	State                    string      `json:"state"`
	Message                  string      `json:"message"`
	LastComponentsActiveTime metav1.Time `json:"lastComponentsActiveTime"`
}

type Workload struct {
	Name               string `json:"name"`
	Type               string `json:"type"`
	Namespace          string `json:"namespace"`
	Replicas           int    `json:"replicas"`
	RebalanceAble      bool   `json:"rebalanceAble"`
	SpotFriendly       bool   `json:"spotFriendly"`
	MinNonSpotReplicas int    `json:"minNonSpotReplicas"`
}

func (w *Workload) ToWorkloadModel() *WorkloadModel {
	if w == nil {
		return nil
	}

	var workloadModel WorkloadModel
	workloadModel.Name = types.StringValue(w.Name)
	workloadModel.Type = types.StringValue(w.Type)
	workloadModel.Namespace = types.StringValue(w.Namespace)
	workloadModel.RebalanceAble = types.BoolValue(w.RebalanceAble)
	workloadModel.SpotFriendly = types.BoolValue(w.SpotFriendly)
	workloadModel.MinNonSpotReplicas = types.Int64Value(int64(w.MinNonSpotReplicas))

	return &workloadModel
}

type ClusterWorkloadSpec struct {
	Workloads []Workload `json:"workloads"`
}

type RebalanceNodePool struct {
	EC2NodePool *EC2NodePool `json:"ec2NodePool"`
	ECSNodePool *ECSNodePool `json:"ecsNodePool"`
}

type RebalanceNodeClass struct {
	EC2NodeClass *EC2NodeClass `json:"ec2NodeClass"`
	ECSNodeClass *ECSNodeClass `json:"ecsNodeClass"`
}

type RebalanceNodeClassList struct {
	EC2NodeClasses []EC2NodeClass `json:"ec2NodeClasses"`
	ECSNodeClasses []ECSNodeClass `json:"ecsNodeClasses"`
}

type EC2NodePool struct {
	Name               string                  `json:"name"`
	Enable             bool                    `json:"enable"`
	NodePoolAnnotation map[string]string       `json:"nodePoolAnnotation"`
	NodePoolSpec       *awscorev1.NodePoolSpec `json:"nodePoolSpec"`
}

type RebalanceNodePoolList struct {
	EC2NodePools []EC2NodePool `json:"ec2NodePools"`
	ECSNodePools []ECSNodePool `json:"ecsNodePools"`
}

type EC2NodeClass struct {
	Name                string                          `json:"name"`
	NodeClassAnnotation map[string]string               `json:"nodeClassAnnotation"`
	NodeClassSpec       *awsproviderv1.EC2NodeClassSpec `json:"nodeClassSpec"`
}

type ECSNodePool struct {
	Name         string                           `json:"name"`
	Enable       bool                             `json:"enable"`
	NodePoolSpec *alibabacloudcorev1.NodePoolSpec `json:"nodePoolSpec"`
}

type ECSNodeClass struct {
	Name          string                                         `json:"name"`
	NodeClassSpec *alibabacloudproviderv1alpha1.ECSNodeClassSpec `json:"nodeClassSpec"`
}

func (e *EC2NodeClass) ToEC2NodeClassModel(ctx context.Context) (*EC2NodeClassModel, error) {
	if e == nil {
		return nil, nil
	}

	var nodeClassModel EC2NodeClassModel
	nodeClassModel.Name = types.StringValue(e.Name)
	nodeClassModel.OriginNodeClassJSON = types.StringValue("")

	if e.NodeClassSpec.Tags != nil {
		tagsMap := make(map[string]types.String)
		for k, v := range e.NodeClassSpec.Tags {
			tagsMap[k] = types.StringValue(v)
		}
		instanceTags, diagnostic := customfield.NewMap[types.String](ctx, tagsMap)
		if diagnostic.HasError() {
			return nil, fmt.Errorf("failed to create instance tags map: %v", diagnostic)
		}
		nodeClassModel.InstanceTags = instanceTags
	} else {
		nodeClassModel.InstanceTags = customfield.NullMap[types.String](ctx)
	}

	if len(e.NodeClassSpec.BlockDeviceMappings) > 0 &&
		e.NodeClassSpec.BlockDeviceMappings[0] != nil &&
		e.NodeClassSpec.BlockDeviceMappings[0].EBS != nil {
		nodeClassModel.SystemDiskSizeGib = types.Int64Value(e.NodeClassSpec.BlockDeviceMappings[0].EBS.VolumeSize.Value() / BytesToGiB)
	}

	if e.NodeClassSpec.Kubelet != nil &&
		e.NodeClassSpec.Kubelet.KubeReserved != nil {
		if cpuStr, ok := e.NodeClassSpec.Kubelet.KubeReserved["cpu"]; ok {
			var cpuMilliCore int64
			_, err := fmt.Sscanf(cpuStr, "%dm", &cpuMilliCore)
			if err != nil {
				return nil, fmt.Errorf("failed to parse extra_cpu_allocation_mcore from kubeReserved cpu: %w", err)
			}
			nodeClassModel.ExtraCPUAllocationMCore = types.Int64Value(cpuMilliCore)
		} else {
			nodeClassModel.ExtraCPUAllocationMCore = types.Int64Value(0)
		}

		if memoryStr, ok := e.NodeClassSpec.Kubelet.KubeReserved["memory"]; ok {
			var memoryMiB int64
			_, err := fmt.Sscanf(memoryStr, "%dMi", &memoryMiB)
			if err != nil {
				return nil, fmt.Errorf("failed to parse extra_memory_allocation_mib from kubeReserved memory: %w", err)
			}
			nodeClassModel.ExtraMemoryAllocationMib = types.Int64Value(memoryMiB)
		} else {
			nodeClassModel.ExtraMemoryAllocationMib = types.Int64Value(0)
		}
	}

	return &nodeClassModel, nil
}

func (e *EC2NodePool) ToEC2NodePoolModel() (*EC2NodePoolModel, error) {
	if e == nil {
		return nil, nil
	}

	var (
		nodePoolModel EC2NodePoolModel
		err           error
	)

	nodePoolModel.Name = types.StringValue(e.Name)
	nodePoolModel.Enable = types.BoolValue(e.Enable)
	nodePoolModel.OriginNodePoolJSON = types.StringValue("")

	if e.NodePoolSpec == nil {
		return &nodePoolModel, nil
	}

	if e.NodePoolSpec.Template.Spec.NodeClassRef != nil {
		nodePoolModel.NodeClass = types.StringValue(e.NodePoolSpec.Template.Spec.NodeClassRef.Name)
	}

	nodePoolModel.EnableGPU = enableGPUToBool(e.NodePoolSpec.Template.Spec.Requirements)

	nodePoolModel.ProvisionPriority = types.Int32Value(lo.FromPtr(e.NodePoolSpec.Weight))
	nodePoolModel.InstanceFamily = requirementsToStrings(e.NodePoolSpec.Template.Spec.Requirements, awsproviderv1.LabelInstanceFamily, corev1.NodeSelectorOpIn)
	nodePoolModel.InstanceArch = requirementsToStrings(e.NodePoolSpec.Template.Spec.Requirements, corev1.LabelArchStable, corev1.NodeSelectorOpIn)
	nodePoolModel.CapacityType = requirementsToStrings(e.NodePoolSpec.Template.Spec.Requirements, awscorev1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn)
	nodePoolModel.Zone = requirementsToStrings(e.NodePoolSpec.Template.Spec.Requirements, corev1.LabelTopologyZone, corev1.NodeSelectorOpIn)
	nodePoolModel.InstanceCPUMAX, err = requirementsToInt64(e.NodePoolSpec.Template.Spec.Requirements, awsproviderv1.LabelInstanceCPU, corev1.NodeSelectorOpLt)
	if err != nil {
		return nil, err
	}
	nodePoolModel.InstanceCPUMIN, err = requirementsToInt64(e.NodePoolSpec.Template.Spec.Requirements, awsproviderv1.LabelInstanceCPU, corev1.NodeSelectorOpGt)
	if err != nil {
		return nil, err
	}
	nodePoolModel.InstanceMemoryMAX, err = requirementsToInt64(e.NodePoolSpec.Template.Spec.Requirements, awsproviderv1.LabelInstanceMemory, corev1.NodeSelectorOpLt)
	if err != nil {
		return nil, err
	}
	nodePoolModel.InstanceMemoryMIN, err = requirementsToInt64(e.NodePoolSpec.Template.Spec.Requirements, awsproviderv1.LabelInstanceMemory, corev1.NodeSelectorOpGt)
	if err != nil {
		return nil, err
	}

	if len(e.NodePoolSpec.Disruption.Budgets) != 0 {
		nodePoolModel.NodeDisruptionLimit = types.StringValue(e.NodePoolSpec.Disruption.Budgets[0].Nodes)
	}

	nodePoolModel.NodeDisruptionDelay = types.StringValue(string(e.NodePoolSpec.Disruption.ConsolidateAfter.Raw))

	return &nodePoolModel, nil
}

func requirementsToStrings(requirements []awscorev1.NodeSelectorRequirementWithMinValues, key string, operator corev1.NodeSelectorOperator) *[]types.String {
	v, found := lo.Find(requirements, func(r awscorev1.NodeSelectorRequirementWithMinValues) bool {
		if r.Key == key && r.Operator == operator {
			return true
		}

		return false
	})
	if !found {
		return nil
	}

	instanceFamilies := make([]types.String, len(v.Values))
	for i, value := range v.Values {
		instanceFamilies[i] = types.StringValue(value)
	}

	return &instanceFamilies
}

func requirementsToInt64(requirements []awscorev1.NodeSelectorRequirementWithMinValues, key string, operator corev1.NodeSelectorOperator) (types.Int64, error) {
	v, found := lo.Find(requirements, func(r awscorev1.NodeSelectorRequirementWithMinValues) bool {
		if r.Key == key && r.Operator == operator {
			return true
		}

		return false
	})
	if !found || len(v.Values) == 0 {
		return types.Int64Value(0), nil
	}

	intValue, err := strconv.ParseInt(v.Values[0], 10, 64)
	if err != nil {
		return types.Int64Value(0), err
	}

	return types.Int64Value(intValue), nil
}

func enableGPUToBool(requirements []awscorev1.NodeSelectorRequirementWithMinValues) types.Bool {
	_, found := lo.Find(requirements, func(r awscorev1.NodeSelectorRequirementWithMinValues) bool {
		if r.Key == awsproviderv1.LabelInstanceGPUCount && r.Operator == corev1.NodeSelectorOpExists {
			return true
		}

		return false
	})

	return lo.Ternary(found, types.BoolValue(true), types.BoolValue(false))
}
