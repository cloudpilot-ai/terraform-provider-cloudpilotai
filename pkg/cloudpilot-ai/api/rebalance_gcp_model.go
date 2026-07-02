package api

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	gcpproviderv1alpha1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter-provider-gcp/apis/v1alpha1"
	gcpcorev1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter/apis/v1"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

const (
	gceNodeClassRefGroup     = "karpenter.k8s.gcp"
	gceNodeClassRefKind      = "GCENodeClass"
	gceLabelInstanceFamily   = "karpenter.k8s.gcp/instance-family"
	gceLabelInstanceCPU      = "karpenter.k8s.gcp/instance-cpu"
	gceLabelInstanceMemory   = "karpenter.k8s.gcp/instance-memory"
	gceLabelInstanceGPUCount = "karpenter.k8s.gcp/instance-gpu-count"
	gceLabelTopologyZoneID   = "topology.k8s.gcp/zone-id"
)

type GCEImageSelectorTerm = gcpproviderv1alpha1.ImageSelectorTerm
type GCEDisk = gcpproviderv1alpha1.Disk
type GCEAdditionalNetworkInterface = gcpproviderv1alpha1.AdditionalNetworkInterface
type GCENetworkConfig = gcpproviderv1alpha1.NetworkConfig
type GCEKubeletConfiguration = gcpproviderv1alpha1.KubeletConfiguration
type GCENodeClassSpec = gcpproviderv1alpha1.GCENodeClassSpec

type GCEImageSelectorTermModel struct {
	ID      types.String `tfsdk:"id"`
	Family  types.String `tfsdk:"family"`
	Channel types.String `tfsdk:"channel"`
	Version types.String `tfsdk:"version"`
}

type gceAliasCompatibility struct {
	family  string
	version string
}

var (
	gcpAllowedImageSelectorFamilies = map[string]struct{}{
		"ContainerOptimizedOS": {},
		"Ubuntu2404":           {},
		"Ubuntu2204":           {},
	}
	gcpAllowedImageSelectorChannels = map[string]struct{}{
		"rapid":    {},
		"regular":  {},
		"stable":   {},
		"extended": {},
		"cluster":  {},
	}
	gcpCOSVersionPattern    = regexp.MustCompile(`^[0-9]+[.][0-9]+[.][0-9]+[.][0-9]+$`)
	gcpUbuntuVersionPattern = regexp.MustCompile(`^v[0-9]{8}$`)
)

type GCEDiskModel struct {
	SizeGiB  types.Int64  `tfsdk:"size_gib"`
	Category types.String `tfsdk:"category"`
	Boot     types.Bool   `tfsdk:"boot"`
}

type GCEAdditionalNetworkInterfaceModel struct {
	Network    types.String `tfsdk:"network"`
	Subnetwork types.String `tfsdk:"subnetwork"`
}

type GCENetworkConfigModel struct {
	EnablePrivateNodes          types.Bool                                                       `tfsdk:"enable_private_nodes"`
	Subnetwork                  types.String                                                     `tfsdk:"subnetwork"`
	AdditionalNetworkInterfaces customfield.NestedObjectList[GCEAdditionalNetworkInterfaceModel] `tfsdk:"additional_network_interfaces"`
}

type GCEKubeletConfigurationModel struct {
	KubeReserved   customfield.Map[types.String] `tfsdk:"kube_reserved"`
	SystemReserved customfield.Map[types.String] `tfsdk:"system_reserved"`
	EvictionHard   customfield.Map[types.String] `tfsdk:"eviction_hard"`
	EvictionSoft   customfield.Map[types.String] `tfsdk:"eviction_soft"`
}

type GCENodeClassModel struct {
	Name                     types.String                                            `tfsdk:"name"`
	EnableImageAccelerator   types.Bool                                              `tfsdk:"enable_image_accelerator"`
	ServiceAccount           types.String                                            `tfsdk:"service_account"`
	Disks                    customfield.NestedObjectList[GCEDiskModel]              `tfsdk:"disks"`
	ImageSelectorTerms       customfield.NestedObjectList[GCEImageSelectorTermModel] `tfsdk:"image_selector_terms"`
	SubnetRangeName          types.String                                            `tfsdk:"subnet_range_name"`
	KubeletConfiguration     customfield.NestedObject[GCEKubeletConfigurationModel]  `tfsdk:"kubelet_configuration"`
	Labels                   customfield.Map[types.String]                           `tfsdk:"labels"`
	Metadata                 customfield.Map[types.String]                           `tfsdk:"metadata"`
	NetworkTags              []types.String                                          `tfsdk:"network_tags"`
	ConfidentialInstanceType types.String                                            `tfsdk:"confidential_instance_type"`
	NetworkConfig            customfield.NestedObject[GCENetworkConfigModel]         `tfsdk:"network_config"`
	AutoGPUTaint             types.Bool                                              `tfsdk:"auto_gpu_taint"`
	GPUDriverVersion         types.String                                            `tfsdk:"gpu_driver_version"`
	OriginNodeClassJSON      types.String                                            `tfsdk:"origin_nodeclass_json"`
}

type GCENodePoolModel struct {
	Name                   types.String                             `tfsdk:"name"`
	Enable                 types.Bool                               `tfsdk:"enable"`
	EnableImageAccelerator types.Bool                               `tfsdk:"enable_image_accelerator"`
	NodeClass              types.String                             `tfsdk:"nodeclass"`
	EnableGPU              types.Bool                               `tfsdk:"enable_gpu"`
	ProvisionPriority      types.Int32                              `tfsdk:"provision_priority"`
	InstanceFamily         *[]types.String                          `tfsdk:"instance_family"`
	InstanceArch           *[]types.String                          `tfsdk:"instance_arch"`
	CapacityType           *[]types.String                          `tfsdk:"capacity_type"`
	Zone                   *[]types.String                          `tfsdk:"zone"`
	InstanceCPUMAX         types.Int64                              `tfsdk:"instance_cpu_max"`
	InstanceCPUMIN         types.Int64                              `tfsdk:"instance_cpu_min"`
	InstanceMemoryMAX      types.Int64                              `tfsdk:"instance_memory_max"`
	InstanceMemoryMIN      types.Int64                              `tfsdk:"instance_memory_min"`
	Labels                 customfield.Map[types.String]            `tfsdk:"labels"`
	Taints                 customfield.NestedObjectList[TaintModel] `tfsdk:"taints"`
	NodeDisruptionLimit    types.String                             `tfsdk:"node_disruption_limit"`
	NodeDisruptionDelay    types.String                             `tfsdk:"node_disruption_delay"`
	OriginNodePoolJSON     types.String                             `tfsdk:"origin_nodepool_json"`
}

func (g *GCENodeClass) UnmarshalJSON(data []byte) error {
	type alias GCENodeClass
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*g = GCENodeClass(decoded)
	g.rawJSON = append([]byte(nil), data...)
	return nil
}

func (g GCENodeClass) MarshalJSON() ([]byte, error) {
	if len(g.rawJSON) > 0 {
		return append([]byte(nil), g.rawJSON...), nil
	}
	type alias GCENodeClass
	return json.Marshal(alias(g))
}

func (g *GCENodePool) UnmarshalJSON(data []byte) error {
	type alias GCENodePool
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*g = GCENodePool(decoded)
	g.rawJSON = append([]byte(nil), data...)
	return nil
}

func (g GCENodePool) MarshalJSON() ([]byte, error) {
	if len(g.rawJSON) > 0 {
		return append([]byte(nil), g.rawJSON...), nil
	}
	type alias GCENodePool
	return json.Marshal(alias(g))
}

func (g *GCENodeClass) ToGCENodeClassModel(ctx context.Context) (*GCENodeClassModel, error) {
	if g == nil {
		return nil, nil
	}

	model := &GCENodeClassModel{
		Name:                 types.StringValue(g.Name),
		Disks:                customfield.NullObjectList[GCEDiskModel](ctx),
		ImageSelectorTerms:   customfield.NullObjectList[GCEImageSelectorTermModel](ctx),
		KubeletConfiguration: customfield.NullObject[GCEKubeletConfigurationModel](ctx),
		Labels:               customfield.NullMap[types.String](ctx),
		Metadata:             customfield.NullMap[types.String](ctx),
		NetworkConfig:        customfield.NullObject[GCENetworkConfigModel](ctx),
		OriginNodeClassJSON:  types.StringNull(),
	}
	if g.NodeClassSpec == nil {
		model.EnableImageAccelerator = types.BoolValue(g.EnableImageAccelerator)
		return model, nil
	}
	model.EnableImageAccelerator = types.BoolValue(g.EnableImageAccelerator)

	if g.NodeClassSpec.ServiceAccount != "" {
		model.ServiceAccount = types.StringValue(g.NodeClassSpec.ServiceAccount)
	}
	if len(g.NodeClassSpec.Disks) > 0 {
		model.Disks = customfield.NewObjectListMust(ctx, lo.Map(g.NodeClassSpec.Disks, func(d GCEDisk, _ int) GCEDiskModel {
			return GCEDiskModel{
				SizeGiB:  types.Int64Value(int64(d.SizeGiB)),
				Category: stringValueOrNull(string(d.Category)),
				Boot:     types.BoolValue(d.Boot),
			}
		}))
	}
	if len(g.NodeClassSpec.ImageSelectorTerms) > 0 {
		model.ImageSelectorTerms = customfield.NewObjectListMust(ctx, lo.Map(g.NodeClassSpec.ImageSelectorTerms, func(term GCEImageSelectorTerm, _ int) GCEImageSelectorTermModel {
			if compat, ok := gcpImageSelectorAliasCompatibility(term.Alias); ok {
				return GCEImageSelectorTermModel{
					ID:      stringValueOrNull(term.ID),
					Family:  types.StringValue(compat.family),
					Channel: stringValueOrNull(term.Channel),
					Version: types.StringValue(compat.version),
				}
			}
			return GCEImageSelectorTermModel{
				ID:      stringValueOrNull(term.ID),
				Family:  stringValueOrNull(term.Family),
				Channel: stringValueOrNull(term.Channel),
				Version: stringValueOrNull(term.Version),
			}
		}))
	}
	if g.NodeClassSpec.SubnetRangeName != nil {
		model.SubnetRangeName = types.StringValue(*g.NodeClassSpec.SubnetRangeName)
	}
	if g.NodeClassSpec.KubeletConfiguration != nil {
		model.KubeletConfiguration = customfield.NewObjectMust(ctx, &GCEKubeletConfigurationModel{
			KubeReserved:   kubeletQuantityMapToTerraformMap(ctx, g.NodeClassSpec.KubeletConfiguration.KubeReserved),
			SystemReserved: kubeletQuantityMapToTerraformMap(ctx, g.NodeClassSpec.KubeletConfiguration.SystemReserved),
			EvictionHard:   kubeletQuantityMapToTerraformMap(ctx, g.NodeClassSpec.KubeletConfiguration.EvictionHard),
			EvictionSoft:   kubeletQuantityMapToTerraformMap(ctx, g.NodeClassSpec.KubeletConfiguration.EvictionSoft),
		})
	}
	model.Labels = stringMapToTerraformMap(ctx, g.NodeClassSpec.Labels)
	model.Metadata = stringMapToTerraformMap(ctx, g.NodeClassSpec.Metadata)
	if len(g.NodeClassSpec.NetworkTags) > 0 {
		model.NetworkTags = lo.Map(g.NodeClassSpec.NetworkTags, func(tag gcpproviderv1alpha1.NetworkTag, _ int) types.String {
			return types.StringValue(string(tag))
		})
	}
	if g.NodeClassSpec.ConfidentialInstanceType != nil {
		model.ConfidentialInstanceType = types.StringValue(*g.NodeClassSpec.ConfidentialInstanceType)
	}
	if g.NodeClassSpec.NetworkConfig != nil {
		networkConfig := &GCENetworkConfigModel{
			AdditionalNetworkInterfaces: customfield.NullObjectList[GCEAdditionalNetworkInterfaceModel](ctx),
		}
		if g.NodeClassSpec.NetworkConfig.EnablePrivateNodes != nil {
			networkConfig.EnablePrivateNodes = types.BoolValue(*g.NodeClassSpec.NetworkConfig.EnablePrivateNodes)
		}
		if g.NodeClassSpec.NetworkConfig.Subnetwork != "" {
			networkConfig.Subnetwork = types.StringValue(g.NodeClassSpec.NetworkConfig.Subnetwork)
		}
		if len(g.NodeClassSpec.NetworkConfig.AdditionalNetworkInterfaces) > 0 {
			networkConfig.AdditionalNetworkInterfaces = customfield.NewObjectListMust(ctx, lo.Map(g.NodeClassSpec.NetworkConfig.AdditionalNetworkInterfaces, func(nic GCEAdditionalNetworkInterface, _ int) GCEAdditionalNetworkInterfaceModel {
				return GCEAdditionalNetworkInterfaceModel{
					Network:    stringValueOrNull(nic.Network),
					Subnetwork: stringValueOrNull(nic.Subnetwork),
				}
			}))
		}
		model.NetworkConfig = customfield.NewObjectMust(ctx, networkConfig)
	}
	model.AutoGPUTaint = types.BoolValue(g.NodeClassSpec.AutoGPUTaint)
	if g.NodeClassSpec.GPUDriverVersion != "" {
		model.GPUDriverVersion = types.StringValue(g.NodeClassSpec.GPUDriverVersion)
	}

	return model, nil
}

func (m *GCENodeClassModel) ToGCENodeClass(ctx context.Context, current GCENodeClass) (*GCENodeClass, error) {
	if !m.OriginNodeClassJSON.IsNull() && !m.OriginNodeClassJSON.IsUnknown() && m.OriginNodeClassJSON.ValueString() != "" {
		var raw GCENodeClass
		if err := json.Unmarshal([]byte(m.OriginNodeClassJSON.ValueString()), &raw); err != nil {
			return nil, err
		}
		return &raw, nil
	}

	currentHasLegacyAliasImageSelector := current.NodeClassSpec != nil && hasLegacyAliasImageSelectorTerm(current.NodeClassSpec.ImageSelectorTerms)

	out := current
	out.Name = m.Name.ValueString()
	if !m.EnableImageAccelerator.IsNull() && !m.EnableImageAccelerator.IsUnknown() {
		out.EnableImageAccelerator = m.EnableImageAccelerator.ValueBool()
	}
	if out.NodeClassSpec == nil {
		out.NodeClassSpec = &GCENodeClassSpec{}
	}

	if !m.ServiceAccount.IsNull() && !m.ServiceAccount.IsUnknown() {
		out.NodeClassSpec.ServiceAccount = m.ServiceAccount.ValueString()
	}
	if !m.Disks.IsNull() && !m.Disks.IsUnknown() {
		disks, diags := m.Disks.AsStructSliceT(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("disks: %v", diags)
		}
		currentDisks := out.NodeClassSpec.Disks
		out.NodeClassSpec.Disks = make([]GCEDisk, 0, len(disks))
		for index, diskModel := range disks {
			disk := GCEPhysicalDiskFromCurrent(currentDisks, index)
			if !diskModel.SizeGiB.IsNull() && !diskModel.SizeGiB.IsUnknown() {
				disk.SizeGiB = int32(diskModel.SizeGiB.ValueInt64())
			}
			if !diskModel.Category.IsNull() && !diskModel.Category.IsUnknown() {
				disk.Category = gcpproviderv1alpha1.DiskCategory(diskModel.Category.ValueString())
			}
			if !diskModel.Boot.IsNull() && !diskModel.Boot.IsUnknown() {
				disk.Boot = diskModel.Boot.ValueBool()
			}
			out.NodeClassSpec.Disks = append(out.NodeClassSpec.Disks, disk)
		}
	}
	if !m.ImageSelectorTerms.IsNull() && !m.ImageSelectorTerms.IsUnknown() {
		terms, diags := m.ImageSelectorTerms.AsStructSliceT(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("image_selector_terms: %v", diags)
		}
		if len(terms) == 0 {
			return nil, fmt.Errorf("image_selector_terms cannot be empty")
		}
		if currentHasLegacyAliasImageSelector && gcpDesiredImageSelectorTermsMatchCurrentNormalized(terms, current.NodeClassSpec.ImageSelectorTerms) {
			out.NodeClassSpec.ImageSelectorTerms = current.NodeClassSpec.ImageSelectorTerms
		} else {
			out.NodeClassSpec.ImageSelectorTerms = make([]GCEImageSelectorTerm, 0, len(terms))
			for index, term := range terms {
				specTerm, err := gcpImageSelectorTermModelToSpec(term, index)
				if err != nil {
					return nil, err
				}
				out.NodeClassSpec.ImageSelectorTerms = append(out.NodeClassSpec.ImageSelectorTerms, specTerm)
			}
			if err := validateGCPImageSelectorTermList(out.NodeClassSpec.ImageSelectorTerms); err != nil {
				return nil, err
			}
			out.NodeClassSpec.ImageFamily = nil
		}
	}
	if len(current.rawJSON) == 0 && current.NodeClassSpec == nil && len(out.NodeClassSpec.ImageSelectorTerms) == 0 {
		return nil, fmt.Errorf("image_selector_terms must be set for a new gce nodeclass")
	}
	if !m.SubnetRangeName.IsNull() && !m.SubnetRangeName.IsUnknown() {
		value := m.SubnetRangeName.ValueString()
		out.NodeClassSpec.SubnetRangeName = &value
	}
	if !m.KubeletConfiguration.IsNull() && !m.KubeletConfiguration.IsUnknown() {
		kubeletConfig, diags := m.KubeletConfiguration.Value(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("kubelet_configuration: %v", diags)
		}
		if kubeletConfig != nil {
			if out.NodeClassSpec.KubeletConfiguration == nil {
				out.NodeClassSpec.KubeletConfiguration = &GCEKubeletConfiguration{}
			}
			out.NodeClassSpec.KubeletConfiguration.KubeReserved = terraformMapToKubeletQuantityMap(ctx, kubeletConfig.KubeReserved)
			out.NodeClassSpec.KubeletConfiguration.SystemReserved = terraformMapToKubeletQuantityMap(ctx, kubeletConfig.SystemReserved)
			out.NodeClassSpec.KubeletConfiguration.EvictionHard = terraformMapToKubeletQuantityMap(ctx, kubeletConfig.EvictionHard)
			out.NodeClassSpec.KubeletConfiguration.EvictionSoft = terraformMapToKubeletQuantityMap(ctx, kubeletConfig.EvictionSoft)
		}
	}
	if !m.Labels.IsNull() && !m.Labels.IsUnknown() {
		out.NodeClassSpec.Labels = terraformMapToStringMap(ctx, m.Labels)
	}
	if !m.Metadata.IsNull() && !m.Metadata.IsUnknown() {
		out.NodeClassSpec.Metadata = terraformMapToStringMap(ctx, m.Metadata)
	}
	if m.NetworkTags != nil {
		out.NodeClassSpec.NetworkTags = lo.Map(m.NetworkTags, func(tag types.String, _ int) gcpproviderv1alpha1.NetworkTag {
			return gcpproviderv1alpha1.NetworkTag(tag.ValueString())
		})
	}
	if !m.ConfidentialInstanceType.IsNull() && !m.ConfidentialInstanceType.IsUnknown() {
		value := m.ConfidentialInstanceType.ValueString()
		out.NodeClassSpec.ConfidentialInstanceType = &value
	}
	if !m.NetworkConfig.IsNull() && !m.NetworkConfig.IsUnknown() {
		networkConfig, diags := m.NetworkConfig.Value(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("network_config: %v", diags)
		}
		if networkConfig != nil {
			if out.NodeClassSpec.NetworkConfig == nil {
				out.NodeClassSpec.NetworkConfig = &GCENetworkConfig{}
			}
			if !networkConfig.AdditionalNetworkInterfaces.IsNull() && !networkConfig.AdditionalNetworkInterfaces.IsUnknown() {
				nics, nicDiags := networkConfig.AdditionalNetworkInterfaces.AsStructSliceT(ctx)
				if nicDiags.HasError() {
					return nil, fmt.Errorf("network_config.additional_network_interfaces: %v", nicDiags)
				}
				out.NodeClassSpec.NetworkConfig.AdditionalNetworkInterfaces = make([]GCEAdditionalNetworkInterface, 0, len(nics))
				for index, nic := range nics {
					subnetwork := strings.TrimSpace(stringFromTerraform(nic.Subnetwork))
					if subnetwork == "" {
						return nil, fmt.Errorf("network_config.additional_network_interfaces[%d].subnetwork is required", index)
					}
					out.NodeClassSpec.NetworkConfig.AdditionalNetworkInterfaces = append(out.NodeClassSpec.NetworkConfig.AdditionalNetworkInterfaces, GCEAdditionalNetworkInterface{
						Network:    stringFromTerraform(nic.Network),
						Subnetwork: subnetwork,
					})
				}
			}
			if !networkConfig.EnablePrivateNodes.IsNull() && !networkConfig.EnablePrivateNodes.IsUnknown() {
				value := networkConfig.EnablePrivateNodes.ValueBool()
				out.NodeClassSpec.NetworkConfig.EnablePrivateNodes = &value
			}
			if !networkConfig.Subnetwork.IsNull() && !networkConfig.Subnetwork.IsUnknown() {
				out.NodeClassSpec.NetworkConfig.Subnetwork = networkConfig.Subnetwork.ValueString()
			}
		}
	}
	if !m.AutoGPUTaint.IsNull() && !m.AutoGPUTaint.IsUnknown() {
		out.NodeClassSpec.AutoGPUTaint = m.AutoGPUTaint.ValueBool()
	}
	if !m.GPUDriverVersion.IsNull() && !m.GPUDriverVersion.IsUnknown() {
		out.NodeClassSpec.GPUDriverVersion = m.GPUDriverVersion.ValueString()
	}

	if len(current.rawJSON) > 0 {
		mergedRawJSON, err := mergeGCENodeClassModelIntoRawJSON(m, &out)
		if err != nil {
			return nil, err
		}
		out.rawJSON = mergedRawJSON
	}

	return &out, nil
}

func (g *GCENodePool) ToGCENodePoolModel(ctx context.Context) (*GCENodePoolModel, error) {
	if g == nil {
		return nil, nil
	}

	model := &GCENodePoolModel{
		Name:                   types.StringValue(g.Name),
		Enable:                 types.BoolValue(g.Enable),
		EnableImageAccelerator: types.BoolValue(g.EnableImageAccelerator),
		Labels:                 customfield.NullMap[types.String](ctx),
		Taints:                 customfield.NullObjectList[TaintModel](ctx),
		OriginNodePoolJSON:     types.StringNull(),
	}
	if g.NodePoolSpec == nil {
		return model, nil
	}

	if g.NodePoolSpec.Template.Spec.NodeClassRef != nil && g.NodePoolSpec.Template.Spec.NodeClassRef.Name != "" {
		model.NodeClass = types.StringValue(g.NodePoolSpec.Template.Spec.NodeClassRef.Name)
	}
	model.EnableGPU = gcpEnableGPUToBoolByKey(g.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceGPUCount)
	if g.NodePoolSpec.Weight != nil {
		model.ProvisionPriority = types.Int32Value(*g.NodePoolSpec.Weight)
	} else {
		model.ProvisionPriority = types.Int32Null()
	}
	model.InstanceFamily = gcpRequirementsToStrings(g.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceFamily, corev1.NodeSelectorOpIn)
	model.InstanceArch = gcpRequirementsToStrings(g.NodePoolSpec.Template.Spec.Requirements, corev1.LabelArchStable, corev1.NodeSelectorOpIn)
	model.CapacityType = gcpRequirementsToStrings(g.NodePoolSpec.Template.Spec.Requirements, gcpcorev1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn)
	model.Zone = gcpRequirementsToStringsByKeys(g.NodePoolSpec.Template.Spec.Requirements, corev1.NodeSelectorOpIn, gceLabelTopologyZoneID, corev1.LabelTopologyZone)

	var err error
	model.InstanceCPUMAX, err = gcpRequirementsToInt64(g.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceCPU, corev1.NodeSelectorOpLt)
	if err != nil {
		return nil, err
	}
	model.InstanceCPUMIN, err = gcpRequirementsToOptionalInt64(g.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceCPU, corev1.NodeSelectorOpGt)
	if err != nil {
		return nil, err
	}
	model.InstanceMemoryMAX, err = gcpRequirementsToInt64(g.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceMemory, corev1.NodeSelectorOpLt)
	if err != nil {
		return nil, err
	}
	model.InstanceMemoryMIN, err = gcpRequirementsToOptionalInt64(g.NodePoolSpec.Template.Spec.Requirements, gceLabelInstanceMemory, corev1.NodeSelectorOpGt)
	if err != nil {
		return nil, err
	}
	if len(g.NodePoolSpec.Template.ObjectMeta.Labels) > 0 {
		model.Labels = stringMapToTerraformMap(ctx, g.NodePoolSpec.Template.ObjectMeta.Labels)
	}
	if len(g.NodePoolSpec.Template.Spec.Taints) > 0 {
		model.Taints = customfield.NewObjectListMust(ctx, lo.Map(g.NodePoolSpec.Template.Spec.Taints, func(taint corev1.Taint, _ int) TaintModel {
			return TaintModel{
				Key:    types.StringValue(taint.Key),
				Value:  types.StringValue(taint.Value),
				Effect: types.StringValue(string(taint.Effect)),
			}
		}))
	}
	if len(g.NodePoolSpec.Disruption.Budgets) > 0 {
		model.NodeDisruptionLimit = types.StringValue(g.NodePoolSpec.Disruption.Budgets[0].Nodes)
	}
	if consolidateAfter, ok := gcpNillableDurationToString(g.NodePoolSpec.Disruption.ConsolidateAfter); ok {
		model.NodeDisruptionDelay = types.StringValue(consolidateAfter)
	}

	return model, nil
}

func (m *GCENodePoolModel) ToGCENodePool(ctx context.Context, current GCENodePool) (*GCENodePool, error) {
	if !m.OriginNodePoolJSON.IsNull() && !m.OriginNodePoolJSON.IsUnknown() && m.OriginNodePoolJSON.ValueString() != "" {
		var raw GCENodePool
		if err := json.Unmarshal([]byte(m.OriginNodePoolJSON.ValueString()), &raw); err != nil {
			return nil, err
		}
		return &raw, nil
	}

	out := current
	out.rawJSON = nil
	out.Name = m.Name.ValueString()
	if out.NodePoolSpec == nil {
		enableGPU := false
		if !m.EnableGPU.IsNull() && !m.EnableGPU.IsUnknown() {
			enableGPU = m.EnableGPU.ValueBool()
		}
		out.NodePoolSpec = enableGPUGCPNodePoolSpec(nil, enableGPU)
	}

	if !m.Enable.IsNull() && !m.Enable.IsUnknown() {
		out.Enable = m.Enable.ValueBool()
	}
	if !m.EnableImageAccelerator.IsNull() && !m.EnableImageAccelerator.IsUnknown() {
		out.EnableImageAccelerator = m.EnableImageAccelerator.ValueBool()
	}

	if !m.EnableGPU.IsNull() && !m.EnableGPU.IsUnknown() {
		out.NodePoolSpec = enableGPUGCPNodePoolSpec(out.NodePoolSpec, m.EnableGPU.ValueBool())
	}

	if out.NodePoolSpec.Template.Spec.NodeClassRef == nil {
		out.NodePoolSpec.Template.Spec.NodeClassRef = &gcpcorev1.NodeClassReference{
			Group: gceNodeClassRefGroup,
			Kind:  gceNodeClassRefKind,
		}
	}
	out.NodePoolSpec.Template.Spec.NodeClassRef.Group = gceNodeClassRefGroup
	out.NodePoolSpec.Template.Spec.NodeClassRef.Kind = gceNodeClassRefKind

	if !m.NodeClass.IsNull() && !m.NodeClass.IsUnknown() {
		nodeClassName := strings.TrimSpace(m.NodeClass.ValueString())
		if nodeClassName == "" {
			return nil, fmt.Errorf("nodepool %s must reference a valid nodeclass", out.Name)
		}
		out.NodePoolSpec.Template.Spec.NodeClassRef.Name = nodeClassName
	}
	if strings.TrimSpace(out.NodePoolSpec.Template.Spec.NodeClassRef.Name) == "" {
		return nil, fmt.Errorf("nodepool %s must reference a valid nodeclass", out.Name)
	}

	if !m.ProvisionPriority.IsNull() && !m.ProvisionPriority.IsUnknown() {
		weight := m.ProvisionPriority.ValueInt32()
		out.NodePoolSpec.Weight = &weight
	}
	if m.InstanceFamily != nil {
		values := lo.Map(*m.InstanceFamily, func(item types.String, _ int) string { return item.ValueString() })
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gceLabelInstanceFamily, corev1.NodeSelectorOpIn, values, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if m.InstanceArch != nil {
		values := lo.Map(*m.InstanceArch, func(item types.String, _ int) string { return item.ValueString() })
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(corev1.LabelArchStable, corev1.NodeSelectorOpIn, values, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if m.CapacityType != nil {
		values := lo.Map(*m.CapacityType, func(item types.String, _ int) string { return item.ValueString() })
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gcpcorev1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, values, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if m.Zone != nil {
		values := lo.Map(*m.Zone, func(item types.String, _ int) string { return item.ValueString() })
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gceLabelTopologyZoneID, corev1.NodeSelectorOpIn, values, out.NodePoolSpec.Template.Spec.Requirements)
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, nil, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if !m.InstanceCPUMAX.IsNull() && !m.InstanceCPUMAX.IsUnknown() {
		values := []string{strconv.FormatInt(m.InstanceCPUMAX.ValueInt64(), 10)}
		if m.InstanceCPUMAX.ValueInt64() == 0 {
			values = nil
		}
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gceLabelInstanceCPU, corev1.NodeSelectorOpLt, values, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if !m.InstanceCPUMIN.IsNull() && !m.InstanceCPUMIN.IsUnknown() {
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gceLabelInstanceCPU, corev1.NodeSelectorOpGt, []string{strconv.FormatInt(m.InstanceCPUMIN.ValueInt64(), 10)}, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if !m.InstanceMemoryMAX.IsNull() && !m.InstanceMemoryMAX.IsUnknown() {
		values := []string{strconv.FormatInt(m.InstanceMemoryMAX.ValueInt64(), 10)}
		if m.InstanceMemoryMAX.ValueInt64() == 0 {
			values = nil
		}
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gceLabelInstanceMemory, corev1.NodeSelectorOpLt, values, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if !m.InstanceMemoryMIN.IsNull() && !m.InstanceMemoryMIN.IsUnknown() {
		out.NodePoolSpec.Template.Spec.Requirements = gcpUpdateRequirements(gceLabelInstanceMemory, corev1.NodeSelectorOpGt, []string{strconv.FormatInt(m.InstanceMemoryMIN.ValueInt64(), 10)}, out.NodePoolSpec.Template.Spec.Requirements)
	}
	if !m.NodeDisruptionLimit.IsNull() && !m.NodeDisruptionLimit.IsUnknown() {
		ensureFirstGCPBudget(out.NodePoolSpec).Nodes = m.NodeDisruptionLimit.ValueString()
	}
	if !m.NodeDisruptionDelay.IsNull() && !m.NodeDisruptionDelay.IsUnknown() {
		out.NodePoolSpec.Disruption.ConsolidateAfter = gcpcorev1.MustParseNillableDuration(m.NodeDisruptionDelay.ValueString())
	}
	if err := applyGCPNodePoolLabels(ctx, out.NodePoolSpec, m.Labels); err != nil {
		return nil, err
	}
	if err := applyGCPNodePoolTaints(ctx, out.NodePoolSpec, m.Taints); err != nil {
		return nil, err
	}

	return &out, nil
}

func gcpRequirementsToStringsByKeys(requirements []gcpcorev1.NodeSelectorRequirementWithMinValues, operator corev1.NodeSelectorOperator, keys ...string) *[]types.String {
	for _, key := range keys {
		if values := gcpRequirementsToStrings(requirements, key, operator); values != nil {
			return values
		}
	}
	return nil
}

func gcpEnableGPUToBoolByKey(requirements []gcpcorev1.NodeSelectorRequirementWithMinValues, key string) types.Bool {
	_, found := lo.Find(requirements, func(r gcpcorev1.NodeSelectorRequirementWithMinValues) bool {
		return r.Key == key && r.Operator == corev1.NodeSelectorOpExists
	})
	return lo.Ternary(found, types.BoolValue(true), types.BoolValue(false))
}

func defaultGCPNodePoolSpec() *gcpcorev1.NodePoolSpec {
	return &gcpcorev1.NodePoolSpec{
		Template: gcpcorev1.NodeClaimTemplate{
			ObjectMeta: gcpcorev1.ObjectMeta{
				Labels: map[string]string{
					CloudPilotManagedNodeLabelKey: "true",
				},
			},
			Spec: gcpcorev1.NodeClaimTemplateSpec{
				Requirements: []gcpcorev1.NodeSelectorRequirementWithMinValues{
					{
						Key:      corev1.LabelArchStable,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"amd64"},
					},
					{
						Key:      corev1.LabelOSStable,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"linux"},
					},
					{
						Key:      gcpcorev1.CapacityTypeLabelKey,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"spot", "on-demand"},
					},
				},
				NodeClassRef: &gcpcorev1.NodeClassReference{
					Group: gceNodeClassRefGroup,
					Kind:  gceNodeClassRefKind,
					Name:  DefaultNodeClassName,
				},
				ExpireAfter: gcpcorev1.MustParseNillableDuration("Never"),
			},
		},
		Disruption: gcpcorev1.Disruption{
			ConsolidationPolicy: gcpcorev1.ConsolidationPolicyWhenEmptyOrUnderutilized,
			ConsolidateAfter:    gcpcorev1.MustParseNillableDuration("0s"),
			Budgets: []gcpcorev1.Budget{{
				Nodes: "10%",
			}},
		},
	}
}

func enableGPUGCPNodePoolSpec(nodePoolSpec *gcpcorev1.NodePoolSpec, enableGPU bool) *gcpcorev1.NodePoolSpec {
	if nodePoolSpec == nil {
		nodePoolSpec = defaultGCPNodePoolSpec()
	}

	_, index, found := lo.FindIndexOf(nodePoolSpec.Template.Spec.Requirements, func(req gcpcorev1.NodeSelectorRequirementWithMinValues) bool {
		return req.Key == gceLabelInstanceGPUCount
	})

	operator := corev1.NodeSelectorOpDoesNotExist
	if enableGPU {
		operator = corev1.NodeSelectorOpExists
	}

	if found {
		nodePoolSpec.Template.Spec.Requirements[index].Operator = operator
		nodePoolSpec.Template.Spec.Requirements[index].Values = nil
		return nodePoolSpec
	}

	nodePoolSpec.Template.Spec.Requirements = append(nodePoolSpec.Template.Spec.Requirements, gcpcorev1.NodeSelectorRequirementWithMinValues{
		Key:      gceLabelInstanceGPUCount,
		Operator: operator,
	})
	return nodePoolSpec
}

func ensureFirstGCPBudget(nodePoolSpec *gcpcorev1.NodePoolSpec) *gcpcorev1.Budget {
	if len(nodePoolSpec.Disruption.Budgets) == 0 {
		nodePoolSpec.Disruption.Budgets = []gcpcorev1.Budget{{}}
	}
	return &nodePoolSpec.Disruption.Budgets[0]
}

func applyGCPNodePoolLabels(ctx context.Context, nodePoolSpec *gcpcorev1.NodePoolSpec, labels customfield.Map[types.String]) error {
	if labels.IsNull() || labels.IsUnknown() {
		return nil
	}
	values, diags := labels.Value(ctx)
	if diags.HasError() {
		return fmt.Errorf("labels: %v", diags)
	}
	nodePoolSpec.Template.ObjectMeta.Labels = map[string]string{}
	for key, value := range values {
		nodePoolSpec.Template.ObjectMeta.Labels[key] = value.ValueString()
	}
	if _, ok := nodePoolSpec.Template.ObjectMeta.Labels[CloudPilotManagedNodeLabelKey]; !ok {
		nodePoolSpec.Template.ObjectMeta.Labels[CloudPilotManagedNodeLabelKey] = "true"
	}
	return nil
}

func applyGCPNodePoolTaints(ctx context.Context, nodePoolSpec *gcpcorev1.NodePoolSpec, taints customfield.NestedObjectList[TaintModel]) error {
	if taints.IsNullOrUnknown() {
		return nil
	}
	values, diags := taints.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("taints: %v", diags)
	}
	nodePoolSpec.Template.Spec.Taints = lo.Map(values, func(taint TaintModel, _ int) corev1.Taint {
		return corev1.Taint{
			Key:    taint.Key.ValueString(),
			Value:  taint.Value.ValueString(),
			Effect: corev1.TaintEffect(taint.Effect.ValueString()),
		}
	})
	return nil
}

func gcpRequirementsToStrings(requirements []gcpcorev1.NodeSelectorRequirementWithMinValues, key string, operator corev1.NodeSelectorOperator) *[]types.String {
	v, found := lo.Find(requirements, func(r gcpcorev1.NodeSelectorRequirementWithMinValues) bool {
		return r.Key == key && r.Operator == operator
	})
	if !found {
		return nil
	}

	values := make([]types.String, len(v.Values))
	for i, value := range v.Values {
		values[i] = types.StringValue(value)
	}
	return &values
}

func gcpRequirementsToInt64(requirements []gcpcorev1.NodeSelectorRequirementWithMinValues, key string, operator corev1.NodeSelectorOperator) (types.Int64, error) {
	v, found := lo.Find(requirements, func(r gcpcorev1.NodeSelectorRequirementWithMinValues) bool {
		return r.Key == key && r.Operator == operator
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

func gcpRequirementsToOptionalInt64(requirements []gcpcorev1.NodeSelectorRequirementWithMinValues, key string, operator corev1.NodeSelectorOperator) (types.Int64, error) {
	v, found := lo.Find(requirements, func(r gcpcorev1.NodeSelectorRequirementWithMinValues) bool {
		return r.Key == key && r.Operator == operator
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

func gcpUpdateRequirements(key string, operator corev1.NodeSelectorOperator, values []string, requirements []gcpcorev1.NodeSelectorRequirementWithMinValues) []gcpcorev1.NodeSelectorRequirementWithMinValues {
	_, index, found := lo.FindIndexOf(requirements, func(item gcpcorev1.NodeSelectorRequirementWithMinValues) bool {
		return item.Key == key && item.Operator == operator
	})

	if found {
		if len(values) == 0 {
			requirements = append(requirements[:index], requirements[index+1:]...)
			return requirements
		}

		requirements[index].Values = values
		return requirements
	}

	if len(values) == 0 {
		return requirements
	}

	requirements = append(requirements, gcpcorev1.NodeSelectorRequirementWithMinValues{
		Key:      key,
		Operator: operator,
		Values:   values,
	})
	return requirements
}

func stringMapToTerraformMap(ctx context.Context, in map[string]string) customfield.Map[types.String] {
	if len(in) == 0 {
		return customfield.NullMap[types.String](ctx)
	}
	values := make(map[string]types.String, len(in))
	for key, value := range in {
		values[key] = types.StringValue(value)
	}
	return customfield.NewMapMust[types.String](ctx, values)
}

func terraformMapToStringMap(ctx context.Context, in customfield.Map[types.String]) map[string]string {
	if in.IsNull() || in.IsUnknown() {
		return nil
	}
	values, diags := in.Value(ctx)
	if diags.HasError() {
		return nil
	}
	if len(values) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value.ValueString()
	}
	return out
}

func kubeletQuantityMapToTerraformMap(ctx context.Context, in map[string]gcpproviderv1alpha1.KubeletQuantity) customfield.Map[types.String] {
	if len(in) == 0 {
		return customfield.NullMap[types.String](ctx)
	}
	values := make(map[string]types.String, len(in))
	for key, value := range in {
		values[key] = types.StringValue(string(value))
	}
	return customfield.NewMapMust[types.String](ctx, values)
}

func terraformMapToKubeletQuantityMap(ctx context.Context, in customfield.Map[types.String]) map[string]gcpproviderv1alpha1.KubeletQuantity {
	if in.IsNull() || in.IsUnknown() {
		return nil
	}
	values, diags := in.Value(ctx)
	if diags.HasError() {
		return nil
	}
	if len(values) == 0 {
		return map[string]gcpproviderv1alpha1.KubeletQuantity{}
	}
	out := make(map[string]gcpproviderv1alpha1.KubeletQuantity, len(values))
	for key, value := range values {
		out[key] = gcpproviderv1alpha1.KubeletQuantity(value.ValueString())
	}
	return out
}

func stringValueOrNull(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func stringFromTerraform(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}

func boolFromTerraform(value types.Bool) bool {
	return !value.IsNull() && !value.IsUnknown() && value.ValueBool()
}

func GCEPhysicalDiskFromCurrent(disks []GCEDisk, index int) GCEDisk {
	if index < len(disks) {
		return disks[index]
	}
	return GCEDisk{}
}

func gcpImageSelectorAliasCompatibility(alias string) (gceAliasCompatibility, bool) {
	trimmed := strings.TrimSpace(alias)
	if trimmed == "" {
		return gceAliasCompatibility{}, false
	}

	parts := strings.SplitN(trimmed, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return gceAliasCompatibility{}, false
	}

	switch parts[0] {
	case "ContainerOptimizedOS":
		return gceAliasCompatibility{family: "ContainerOptimizedOS", version: parts[1]}, true
	default:
		return gceAliasCompatibility{}, false
	}
}

func gcpImageSelectorTermModelToSpec(term GCEImageSelectorTermModel, index int) (GCEImageSelectorTerm, error) {
	id := strings.TrimSpace(stringFromTerraform(term.ID))
	family := strings.TrimSpace(stringFromTerraform(term.Family))
	channel := strings.TrimSpace(stringFromTerraform(term.Channel))
	version := strings.TrimSpace(stringFromTerraform(term.Version))

	hasID := id != ""
	hasFamily := family != ""
	hasChannel := channel != ""
	hasVersion := version != ""

	if hasID && (hasFamily || hasChannel || hasVersion) {
		return GCEImageSelectorTerm{}, fmt.Errorf("image_selector_terms[%d]: id is mutually exclusive with family, channel, and version", index)
	}
	if !hasID && !hasFamily {
		return GCEImageSelectorTerm{}, fmt.Errorf("image_selector_terms[%d]: set either id or family", index)
	}
	if hasFamily && !hasChannel && !hasVersion {
		return GCEImageSelectorTerm{}, fmt.Errorf("image_selector_terms[%d]: family requires channel or version", index)
	}
	if !hasFamily && (hasChannel || hasVersion) {
		return GCEImageSelectorTerm{}, fmt.Errorf("image_selector_terms[%d]: channel and version require family", index)
	}
	if hasChannel && hasVersion {
		return GCEImageSelectorTerm{}, fmt.Errorf("image_selector_terms[%d]: channel and version are mutually exclusive", index)
	}
	if hasChannel && family != "ContainerOptimizedOS" {
		return GCEImageSelectorTerm{}, fmt.Errorf("image_selector_terms[%d]: channel is only supported with family ContainerOptimizedOS", index)
	}
	if hasFamily {
		if family == "Ubuntu" {
			return GCEImageSelectorTerm{}, fmt.Errorf("image_selector_terms[%d]: family 'Ubuntu' is not valid; use Ubuntu2404 or Ubuntu2204", index)
		}
		if _, ok := gcpAllowedImageSelectorFamilies[family]; !ok {
			return GCEImageSelectorTerm{}, fmt.Errorf("image_selector_terms[%d]: unsupported family %q", index, family)
		}
	}
	if hasChannel {
		if _, ok := gcpAllowedImageSelectorChannels[channel]; !ok {
			return GCEImageSelectorTerm{}, fmt.Errorf("image_selector_terms[%d]: unsupported channel %q", index, channel)
		}
	}
	if hasVersion {
		switch family {
		case "ContainerOptimizedOS":
			if version != "latest" && !gcpCOSVersionPattern.MatchString(version) {
				return GCEImageSelectorTerm{}, fmt.Errorf("image_selector_terms[%d]: invalid ContainerOptimizedOS version %q", index, version)
			}
		case "Ubuntu2404", "Ubuntu2204":
			if version != "latest" && !gcpUbuntuVersionPattern.MatchString(version) {
				return GCEImageSelectorTerm{}, fmt.Errorf("image_selector_terms[%d]: invalid %s version %q", index, family, version)
			}
		}
	}

	return GCEImageSelectorTerm{
		ID:      id,
		Family:  family,
		Channel: channel,
		Version: version,
	}, nil
}

func validateGCPImageSelectorTermList(terms []GCEImageSelectorTerm) error {
	hasChannelBasedTerm := false
	hasCOSLatestVersionTerm := false
	for _, term := range terms {
		if term.Channel != "" {
			hasChannelBasedTerm = true
		}
		if term.Family == "ContainerOptimizedOS" && term.Version == "latest" && term.Alias == "" {
			hasCOSLatestVersionTerm = true
		}
	}
	if hasChannelBasedTerm && hasCOSLatestVersionTerm {
		return fmt.Errorf("image_selector_terms: channel-based terms cannot be mixed with ContainerOptimizedOS version=latest terms")
	}
	return nil
}

func hasLegacyAliasImageSelectorTerm(terms []GCEImageSelectorTerm) bool {
	for _, term := range terms {
		if strings.TrimSpace(term.Alias) != "" {
			return true
		}
	}
	return false
}

func gcpDesiredImageSelectorTermsMatchCurrentNormalized(desired []GCEImageSelectorTermModel, current []GCEImageSelectorTerm) bool {
	if len(desired) != len(current) {
		return false
	}
	for i := range desired {
		currentID := strings.TrimSpace(current[i].ID)
		currentFamily := strings.TrimSpace(current[i].Family)
		currentChannel := strings.TrimSpace(current[i].Channel)
		currentVersion := strings.TrimSpace(current[i].Version)
		if compat, ok := gcpImageSelectorAliasCompatibility(current[i].Alias); ok {
			currentFamily = compat.family
			currentVersion = compat.version
			currentChannel = ""
		}

		if strings.TrimSpace(stringFromTerraform(desired[i].ID)) != currentID {
			return false
		}
		if strings.TrimSpace(stringFromTerraform(desired[i].Family)) != currentFamily {
			return false
		}
		if strings.TrimSpace(stringFromTerraform(desired[i].Channel)) != currentChannel {
			return false
		}
		if strings.TrimSpace(stringFromTerraform(desired[i].Version)) != currentVersion {
			return false
		}
	}
	return true
}

func gcpNillableDurationToString(value gcpcorev1.NillableDuration) (string, bool) {
	if len(value.Raw) > 0 {
		var raw string
		if err := json.Unmarshal(value.Raw, &raw); err == nil && raw != "" {
			if raw == gcpcorev1.Never {
				return gcpcorev1.Never, true
			}
			return NormalizeDuration(raw), true
		}
	}
	if value.Duration != nil {
		return NormalizeDuration(value.Duration.String()), true
	}
	return gcpcorev1.Never, true
}

func mergeGCENodeClassModelIntoRawJSON(model *GCENodeClassModel, nodeClass *GCENodeClass) ([]byte, error) {
	payload := map[string]json.RawMessage{}
	if err := json.Unmarshal(nodeClass.rawJSON, &payload); err != nil {
		return nil, fmt.Errorf("merge existing gce nodeclass payload: %w", err)
	}

	nameJSON, err := json.Marshal(nodeClass.Name)
	if err != nil {
		return nil, fmt.Errorf("marshal gce nodeclass name: %w", err)
	}
	payload["name"] = nameJSON
	if !model.EnableImageAccelerator.IsNull() && !model.EnableImageAccelerator.IsUnknown() {
		enableImageAcceleratorJSON, err := json.Marshal(nodeClass.EnableImageAccelerator)
		if err != nil {
			return nil, fmt.Errorf("marshal gce nodeclass enableImageAccelerator: %w", err)
		}
		payload["enableImageAccelerator"] = enableImageAcceleratorJSON
	}

	specPayload := map[string]json.RawMessage{}
	if rawSpec, ok := payload["nodeClassSpec"]; ok && len(rawSpec) > 0 && string(rawSpec) != "null" {
		if err := json.Unmarshal(rawSpec, &specPayload); err != nil {
			return nil, fmt.Errorf("merge existing gce nodeclass spec payload: %w", err)
		}
	}

	if err := mergeGCENodeClassSpecField(specPayload, "serviceAccount",
		!model.ServiceAccount.IsNull() && !model.ServiceAccount.IsUnknown(),
		nodeClass.NodeClassSpec.ServiceAccount != "",
		nodeClass.NodeClassSpec.ServiceAccount); err != nil {
		return nil, err
	}
	if err := mergeGCENodeClassSpecField(specPayload, "disks",
		!model.Disks.IsNull() && !model.Disks.IsUnknown(),
		len(nodeClass.NodeClassSpec.Disks) > 0,
		nodeClass.NodeClassSpec.Disks); err != nil {
		return nil, err
	}
	if err := mergeGCENodeClassSpecField(specPayload, "imageSelectorTerms",
		!model.ImageSelectorTerms.IsNull() && !model.ImageSelectorTerms.IsUnknown(),
		len(nodeClass.NodeClassSpec.ImageSelectorTerms) > 0,
		nodeClass.NodeClassSpec.ImageSelectorTerms); err != nil {
		return nil, err
	}
	if !model.ImageSelectorTerms.IsNull() && !model.ImageSelectorTerms.IsUnknown() {
		delete(specPayload, "imageFamily")
	}
	if err := mergeGCENodeClassSpecField(specPayload, "subnetRangeName",
		!model.SubnetRangeName.IsNull() && !model.SubnetRangeName.IsUnknown(),
		nodeClass.NodeClassSpec.SubnetRangeName != nil,
		nodeClass.NodeClassSpec.SubnetRangeName); err != nil {
		return nil, err
	}
	if err := mergeGCENodeClassSpecField(specPayload, "kubeletConfiguration",
		!model.KubeletConfiguration.IsNull() && !model.KubeletConfiguration.IsUnknown(),
		nodeClass.NodeClassSpec.KubeletConfiguration != nil,
		nodeClass.NodeClassSpec.KubeletConfiguration); err != nil {
		return nil, err
	}
	if err := mergeGCENodeClassSpecField(specPayload, "labels",
		!model.Labels.IsNull() && !model.Labels.IsUnknown(),
		len(nodeClass.NodeClassSpec.Labels) > 0,
		nodeClass.NodeClassSpec.Labels); err != nil {
		return nil, err
	}
	if err := mergeGCENodeClassSpecField(specPayload, "metadata",
		!model.Metadata.IsNull() && !model.Metadata.IsUnknown(),
		len(nodeClass.NodeClassSpec.Metadata) > 0,
		nodeClass.NodeClassSpec.Metadata); err != nil {
		return nil, err
	}
	if err := mergeGCENodeClassSpecField(specPayload, "networkTags",
		model.NetworkTags != nil,
		len(nodeClass.NodeClassSpec.NetworkTags) > 0,
		nodeClass.NodeClassSpec.NetworkTags); err != nil {
		return nil, err
	}
	if err := mergeGCENodeClassSpecField(specPayload, "confidentialInstanceType",
		!model.ConfidentialInstanceType.IsNull() && !model.ConfidentialInstanceType.IsUnknown(),
		nodeClass.NodeClassSpec.ConfidentialInstanceType != nil,
		nodeClass.NodeClassSpec.ConfidentialInstanceType); err != nil {
		return nil, err
	}
	if err := mergeGCENodeClassSpecField(specPayload, "networkConfig",
		!model.NetworkConfig.IsNull() && !model.NetworkConfig.IsUnknown(),
		nodeClass.NodeClassSpec.NetworkConfig != nil,
		nodeClass.NodeClassSpec.NetworkConfig); err != nil {
		return nil, err
	}
	if err := mergeGCENodeClassSpecField(specPayload, "autoGPUTaint",
		!model.AutoGPUTaint.IsNull() && !model.AutoGPUTaint.IsUnknown(),
		nodeClass.NodeClassSpec.AutoGPUTaint,
		nodeClass.NodeClassSpec.AutoGPUTaint); err != nil {
		return nil, err
	}
	if err := mergeGCENodeClassSpecField(specPayload, "gpuDriverVersion",
		!model.GPUDriverVersion.IsNull() && !model.GPUDriverVersion.IsUnknown(),
		nodeClass.NodeClassSpec.GPUDriverVersion != "",
		nodeClass.NodeClassSpec.GPUDriverVersion); err != nil {
		return nil, err
	}

	specJSON, err := json.Marshal(specPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal merged gce nodeclass spec: %w", err)
	}
	payload["nodeClassSpec"] = specJSON

	mergedJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal merged gce nodeclass payload: %w", err)
	}
	return mergedJSON, nil
}

func mergeGCENodeClassSpecField(specPayload map[string]json.RawMessage, key string, touched, keep bool, value any) error {
	if !touched {
		return nil
	}
	if !keep {
		delete(specPayload, key)
		return nil
	}

	fieldJSON, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal gce nodeclass field %s: %w", key, err)
	}
	specPayload[key] = fieldJSON
	return nil
}
