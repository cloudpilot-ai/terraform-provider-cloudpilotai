package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	alibabacloudproviderv1alpha1 "github.com/cloudpilot-ai/lib/pkg/alibabacloud/karpenter-provider-alibabacloud/apis/v1alpha1"
	alibabacloudcorev1 "github.com/cloudpilot-ai/lib/pkg/alibabacloud/karpenter/apis/v1"
	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	awscorev1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter/apis/v1"
	gcpproviderv1alpha1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter-provider-gcp/apis/v1alpha1"
	gcpcorev1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter/apis/v1"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

const (
	MilliCoreToCore   = 1000.0
	BytesToGiB        = 1073741824.0
	RebalanceTypeAll  = "all"
	RebalanceTypeNode = "node"
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
	GCENodePool *GCENodePool `json:"gceNodePool"`
}

type RebalanceNodeClass struct {
	EC2NodeClass *EC2NodeClass `json:"ec2NodeClass"`
	ECSNodeClass *ECSNodeClass `json:"ecsNodeClass"`
	GCENodeClass *GCENodeClass `json:"gceNodeClass"`
}

type RebalanceNodeClassList struct {
	EC2NodeClasses []EC2NodeClass `json:"ec2NodeClasses"`
	ECSNodeClasses []ECSNodeClass `json:"ecsNodeClasses"`
	GCENodeClasses []GCENodeClass `json:"gceNodeClasses"`
}

type EC2NodePool struct {
	Name                   string                  `json:"name"`
	Enable                 bool                    `json:"enable"`
	EnableImageAccelerator bool                    `json:"enableImageAccelerator"`
	NodePoolAnnotation     map[string]string       `json:"nodePoolAnnotation"`
	NodePoolSpec           *awscorev1.NodePoolSpec `json:"nodePoolSpec"`
}

type RebalanceNodePoolList struct {
	EC2NodePools []EC2NodePool `json:"ec2NodePools"`
	ECSNodePools []ECSNodePool `json:"ecsNodePools"`
	GCENodePools []GCENodePool `json:"gceNodePools"`
}

type EC2NodeClass struct {
	Name                   string                          `json:"name"`
	NodeClassAnnotation    map[string]string               `json:"nodeClassAnnotation"`
	NodeClassSpec          *awsproviderv1.EC2NodeClassSpec `json:"nodeClassSpec"`
	EnableImageAccelerator bool                            `json:"enableImageAccelerator"`
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

type GCENodePool struct {
	Name                   string                  `json:"name"`
	Enable                 bool                    `json:"enable"`
	EnableImageAccelerator bool                    `json:"enableImageAccelerator"`
	NodePoolSpec           *gcpcorev1.NodePoolSpec `json:"nodePoolSpec"`
	rawJSON                []byte                  `json:"-"`
}

type GCENodeClass struct {
	Name                   string                                `json:"name"`
	EnableImageAccelerator bool                                  `json:"enableImageAccelerator"`
	NodeClassSpec          *gcpproviderv1alpha1.GCENodeClassSpec `json:"nodeClassSpec"`
	rawJSON                []byte                                `json:"-"`
}

func (e *EC2NodeClass) ToEC2NodeClassModel(ctx context.Context) (*EC2NodeClassModel, error) {
	if e == nil {
		return nil, nil
	}

	var nodeClassModel EC2NodeClassModel
	nodeClassModel.Name = types.StringValue(e.Name)
	nodeClassModel.EnableImageAccelerator = types.BoolValue(e.EnableImageAccelerator)
	nodeClassModel.OriginNodeClassJSON = types.StringNull()

	if e.NodeClassSpec.Role != "" {
		nodeClassModel.Role = types.StringValue(e.NodeClassSpec.Role)
	}

	if len(e.NodeClassSpec.SubnetSelectorTerms) > 0 {
		models := make([]SubnetSelectorTermModel, len(e.NodeClassSpec.SubnetSelectorTerms))
		for i, t := range e.NodeClassSpec.SubnetSelectorTerms {
			m := SubnetSelectorTermModel{
				Tags: customfield.NullMap[types.String](ctx),
				ID:   types.StringNull(),
			}
			if t.ID != "" {
				m.ID = types.StringValue(t.ID)
			}
			if len(t.Tags) > 0 {
				tagsMap := make(map[string]types.String, len(t.Tags))
				for k, v := range t.Tags {
					tagsMap[k] = types.StringValue(v)
				}
				subnetTags, diags := customfield.NewMap[types.String](ctx, tagsMap)
				if diags.HasError() {
					return nil, fmt.Errorf("subnet_selector_terms: %v", diags)
				}
				m.Tags = subnetTags
			}
			models[i] = m
		}
		listVal, diags := customfield.NewObjectList(ctx, models)
		if diags.HasError() {
			return nil, fmt.Errorf("subnet_selector_terms: %v", diags)
		}
		nodeClassModel.SubnetSelectorTerms = listVal
	} else {
		nodeClassModel.SubnetSelectorTerms = customfield.NullObjectList[SubnetSelectorTermModel](ctx)
	}

	if len(e.NodeClassSpec.SecurityGroupSelectorTerms) > 0 {
		models := make([]SecurityGroupSelectorTermModel, len(e.NodeClassSpec.SecurityGroupSelectorTerms))
		for i, t := range e.NodeClassSpec.SecurityGroupSelectorTerms {
			m := SecurityGroupSelectorTermModel{
				Tags: customfield.NullMap[types.String](ctx),
				ID:   types.StringNull(),
				Name: types.StringNull(),
			}
			if t.ID != "" {
				m.ID = types.StringValue(t.ID)
			}
			if t.Name != "" {
				m.Name = types.StringValue(t.Name)
			}
			if len(t.Tags) > 0 {
				tagsMap := make(map[string]types.String, len(t.Tags))
				for k, v := range t.Tags {
					tagsMap[k] = types.StringValue(v)
				}
				sgTags, diags := customfield.NewMap[types.String](ctx, tagsMap)
				if diags.HasError() {
					return nil, fmt.Errorf("security_group_selector_terms: %v", diags)
				}
				m.Tags = sgTags
			}
			models[i] = m
		}
		listVal, diags := customfield.NewObjectList(ctx, models)
		if diags.HasError() {
			return nil, fmt.Errorf("security_group_selector_terms: %v", diags)
		}
		nodeClassModel.SecurityGroupSelectorTerms = listVal
	} else {
		nodeClassModel.SecurityGroupSelectorTerms = customfield.NullObjectList[SecurityGroupSelectorTermModel](ctx)
	}

	if e.NodeClassSpec.Tags != nil {
		tagsMap := make(map[string]types.String)
		for k, v := range e.NodeClassSpec.Tags {
			if k == CloudPilotManagedNodeLabelKey {
				continue
			}
			v = strings.Trim(v, "\"")
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

	for _, term := range e.NodeClassSpec.AMISelectorTerms {
		if term.Alias != "" {
			nodeClassModel.AmiAlias = types.StringValue(term.Alias)
			break
		}
	}
	if e.NodeClassSpec.UserData != nil {
		nodeClassModel.UserData = types.StringValue(*e.NodeClassSpec.UserData)
	}
	blockDeviceMappings := blockDeviceMappingModelsFromAWS(ctx, e.NodeClassSpec.BlockDeviceMappings)
	if len(blockDeviceMappings) > 0 {
		nodeClassModel.BlockDeviceMappings = customfield.NewObjectListMust(ctx, blockDeviceMappings)
	} else {
		nodeClassModel.BlockDeviceMappings = customfield.NullObjectList[BlockDeviceMappingModel](ctx)
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
		}

		if memoryStr, ok := e.NodeClassSpec.Kubelet.KubeReserved["memory"]; ok {
			var memoryMiB int64
			_, err := fmt.Sscanf(memoryStr, "%dMi", &memoryMiB)
			if err != nil {
				return nil, fmt.Errorf("failed to parse extra_memory_allocation_mib from kubeReserved memory: %w", err)
			}
			nodeClassModel.ExtraMemoryAllocationMib = types.Int64Value(memoryMiB)
		}
	}

	return &nodeClassModel, nil
}

func blockDeviceMappingModelsFromAWS(ctx context.Context, in []*awsproviderv1.BlockDeviceMapping) []BlockDeviceMappingModel {
	out := make([]BlockDeviceMappingModel, 0, len(in))
	for _, m := range in {
		if m == nil {
			continue
		}
		model := BlockDeviceMappingModel{
			RootVolume: types.BoolValue(m.RootVolume),
			EBS:        customfield.NullObject[BlockDeviceModel](ctx),
		}
		if m.DeviceName != nil {
			model.DeviceName = types.StringValue(*m.DeviceName)
		} else {
			model.DeviceName = types.StringValue("")
		}
		if m.EBS != nil {
			model.EBS = customfield.NewObjectMust(ctx, blockDeviceModelFromAWS(m.EBS))
		}
		out = append(out, model)
	}
	return out
}

func blockDeviceModelFromAWS(in *awsproviderv1.BlockDevice) *BlockDeviceModel {
	model := &BlockDeviceModel{}
	if in.Encrypted != nil {
		model.Encrypted = types.BoolValue(*in.Encrypted)
	}
	if in.VolumeSize != nil {
		model.VolumeSize = types.StringValue(in.VolumeSize.String())
	}
	if in.VolumeType != nil {
		model.VolumeType = types.StringValue(*in.VolumeType)
	}
	return model
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
	nodePoolModel.EnableImageAccelerator = types.BoolValue(e.EnableImageAccelerator)
	nodePoolModel.OriginNodePoolJSON = types.StringNull()

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
	nodePoolModel.InstanceCPUMIN, err = requirementsToOptionalInt64(e.NodePoolSpec.Template.Spec.Requirements, awsproviderv1.LabelInstanceCPU, corev1.NodeSelectorOpGt)
	if err != nil {
		return nil, err
	}
	nodePoolModel.InstanceMemoryMAX, err = requirementsToInt64(e.NodePoolSpec.Template.Spec.Requirements, awsproviderv1.LabelInstanceMemory, corev1.NodeSelectorOpLt)
	if err != nil {
		return nil, err
	}
	nodePoolModel.InstanceMemoryMIN, err = requirementsToOptionalInt64(e.NodePoolSpec.Template.Spec.Requirements, awsproviderv1.LabelInstanceMemory, corev1.NodeSelectorOpGt)
	if err != nil {
		return nil, err
	}

	if len(e.NodePoolSpec.Template.ObjectMeta.Labels) > 0 {
		labelModels := map[string]types.String{}
		for k, v := range e.NodePoolSpec.Template.ObjectMeta.Labels {
			labelModels[k] = types.StringValue(v)
		}
		nodePoolModel.Labels = customfield.NewMapMust[types.String](context.Background(), labelModels)
	} else {
		nodePoolModel.Labels = customfield.NullMap[types.String](context.Background())
	}

	if len(e.NodePoolSpec.Template.Spec.Taints) > 0 {
		taintModels := make([]TaintModel, 0, len(e.NodePoolSpec.Template.Spec.Taints))
		for _, taint := range e.NodePoolSpec.Template.Spec.Taints {
			taintModels = append(taintModels, TaintModel{
				Key:    types.StringValue(taint.Key),
				Value:  types.StringValue(taint.Value),
				Effect: types.StringValue(string(taint.Effect)),
			})
		}
		nodePoolModel.Taints = customfield.NewObjectListMust(context.Background(), taintModels)
	} else {
		nodePoolModel.Taints = customfield.NullObjectList[TaintModel](context.Background())
	}

	if len(e.NodePoolSpec.Disruption.Budgets) != 0 {
		nodePoolModel.NodeDisruptionLimit = types.StringValue(e.NodePoolSpec.Disruption.Budgets[0].Nodes)
	}

	if e.NodePoolSpec.Disruption.ConsolidateAfter.Duration != nil {
		nodePoolModel.NodeDisruptionDelay = types.StringValue(NormalizeDuration(e.NodePoolSpec.Disruption.ConsolidateAfter.Duration.String()))
	}

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

func requirementsToOptionalInt64(requirements []awscorev1.NodeSelectorRequirementWithMinValues, key string, operator corev1.NodeSelectorOperator) (types.Int64, error) {
	v, found := lo.Find(requirements, func(r awscorev1.NodeSelectorRequirementWithMinValues) bool {
		if r.Key == key && r.Operator == operator {
			return true
		}

		return false
	})
	if !found || len(v.Values) == 0 {
		return types.Int64Null(), nil
	}

	intValue, err := strconv.ParseInt(v.Values[0], 10, 64)
	if err != nil {
		return types.Int64Null(), err
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
