package api

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	gcpproviderv1alpha1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter-provider-gcp/apis/v1alpha1"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api/jsonmerge"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
)

const (
	gceNodeClassRefGroup          = "karpenter.k8s.gcp"
	gceNodeClassRefKind           = "GCENodeClass"
	gceLabelInstanceFamily        = "karpenter.k8s.gcp/instance-family"
	gceLabelInstanceCPU           = "karpenter.k8s.gcp/instance-cpu"
	gceLabelInstanceMemory        = "karpenter.k8s.gcp/instance-memory"
	gceLabelInstanceGPUCount      = "karpenter.k8s.gcp/instance-gpu-count"
	gceLegacyLabelTopologyZoneID  = "topology.k8s.gcp/zone-id"
	maxGCPDisruptionBudgetReasons = 50
)

type GCEImageSelectorTerm = gcpproviderv1alpha1.ImageSelectorTerm
type GCEDisk = gcpproviderv1alpha1.Disk
type GCEAdditionalNetworkInterface = gcpproviderv1alpha1.AdditionalNetworkInterface
type GCENetworkConfig = gcpproviderv1alpha1.NetworkConfig
type GCEKubeletConfiguration = gcpproviderv1alpha1.KubeletConfiguration
type GCENodeClassSpec = gcpproviderv1alpha1.GCENodeClassSpec
type GCEEphemeralStorageLocalSSD = gcpproviderv1alpha1.EphemeralStorageLocalSSDConfig

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
	SizeGiB    types.Int64                                     `tfsdk:"size_gib"`
	SizePolicy customfield.NestedObject[VolumeSizePolicyModel] `tfsdk:"size_policy"`
	Category   types.String                                    `tfsdk:"category"`
	Boot       types.Bool                                      `tfsdk:"boot"`
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

type GCEEphemeralStorageLocalSSDModel struct {
	AutoCount types.Bool  `tfsdk:"auto_count"`
	Count     types.Int32 `tfsdk:"count"`
}

type GCENodeClassModel struct {
	Name                           types.String                                               `tfsdk:"name"`
	EnableImageAccelerator         types.Bool                                                 `tfsdk:"enable_image_accelerator"`
	EnableLocalSSDEphemeralStorage types.Bool                                                 `tfsdk:"enable_local_ssd_ephemeral_storage"`
	EphemeralStorageLocalSSD       customfield.NestedObject[GCEEphemeralStorageLocalSSDModel] `tfsdk:"ephemeral_storage_local_ssd"`
	ServiceAccount                 types.String                                               `tfsdk:"service_account"`
	Disks                          customfield.NestedObjectList[GCEDiskModel]                 `tfsdk:"disks"`
	ImageSelectorTerms             customfield.NestedObjectList[GCEImageSelectorTermModel]    `tfsdk:"image_selector_terms"`
	SubnetRangeName                types.String                                               `tfsdk:"subnet_range_name"`
	KubeletConfiguration           customfield.NestedObject[GCEKubeletConfigurationModel]     `tfsdk:"kubelet_configuration"`
	Labels                         customfield.Map[types.String]                              `tfsdk:"labels"`
	Metadata                       customfield.Map[types.String]                              `tfsdk:"metadata"`
	NetworkTags                    []types.String                                             `tfsdk:"network_tags"`
	ConfidentialInstanceType       types.String                                               `tfsdk:"confidential_instance_type"`
	NetworkConfig                  customfield.NestedObject[GCENetworkConfigModel]            `tfsdk:"network_config"`
	AutoGPUTaint                   types.Bool                                                 `tfsdk:"auto_gpu_taint"`
	GPUDriverVersion               types.String                                               `tfsdk:"gpu_driver_version"`
	OriginNodeClassJSON            types.String                                               `tfsdk:"origin_nodeclass_json"`
}

type GCENodePoolModel struct {
	DisruptionCooldown      types.String                                           `tfsdk:"disruption_cooldown"`
	JobNodePoolOptimization customfield.NestedObject[JobNodePoolOptimizationModel] `tfsdk:"job_nodepool_optimization"`
	Name                    types.String                                           `tfsdk:"name"`
	Enable                  types.Bool                                             `tfsdk:"enable"`
	EnableImageAccelerator  types.Bool                                             `tfsdk:"enable_image_accelerator"`
	NodeClass               types.String                                           `tfsdk:"nodeclass"`
	EnableGPU               types.Bool                                             `tfsdk:"enable_gpu"`
	ProvisionPriority       types.Int32                                            `tfsdk:"provision_priority"`
	InstanceFamily          *[]types.String                                        `tfsdk:"instance_family"`
	InstanceArch            *[]types.String                                        `tfsdk:"instance_arch"`
	CapacityType            *[]types.String                                        `tfsdk:"capacity_type"`
	Zone                    *[]types.String                                        `tfsdk:"zone"`
	InstanceCPUMAX          types.Int64                                            `tfsdk:"instance_cpu_max"`
	InstanceCPUMIN          types.Int64                                            `tfsdk:"instance_cpu_min"`
	InstanceMemoryMAX       types.Int64                                            `tfsdk:"instance_memory_max"`
	InstanceMemoryMIN       types.Int64                                            `tfsdk:"instance_memory_min"`
	Labels                  customfield.Map[types.String]                          `tfsdk:"labels"`
	Taints                  customfield.NestedObjectList[TaintModel]               `tfsdk:"taints"`
	NodeDisruptionLimit     types.String                                           `tfsdk:"node_disruption_limit"`
	NodeDisruptionBudgets   customfield.NestedObjectList[DisruptionBudgetModel]    `tfsdk:"node_disruption_budgets"`
	NodeDisruptionDelay     types.String                                           `tfsdk:"node_disruption_delay"`
	OriginNodePoolJSON      types.String                                           `tfsdk:"origin_nodepool_json"`
}

func (g *GCENodeClass) localSSDEphemeralStorageEnabled() bool {
	if g == nil {
		return false
	}
	if g.EnableLocalSSDEphemeralStorage != nil {
		return *g.EnableLocalSSDEphemeralStorage
	}
	return g.NodeClassSpec != nil && g.NodeClassSpec.EphemeralStorageLocalSSD != nil
}

func (g *GCENodeClass) ToGCENodeClassModel(ctx context.Context) (*GCENodeClassModel, error) {
	if g == nil {
		return nil, nil
	}

	model := &GCENodeClassModel{
		Name:                     types.StringValue(g.Name),
		Disks:                    customfield.NullObjectList[GCEDiskModel](ctx),
		ImageSelectorTerms:       customfield.NullObjectList[GCEImageSelectorTermModel](ctx),
		KubeletConfiguration:     customfield.NullObject[GCEKubeletConfigurationModel](ctx),
		Labels:                   customfield.NullMap[types.String](ctx),
		Metadata:                 customfield.NullMap[types.String](ctx),
		NetworkConfig:            customfield.NullObject[GCENetworkConfigModel](ctx),
		EphemeralStorageLocalSSD: customfield.NullObject[GCEEphemeralStorageLocalSSDModel](ctx),
		OriginNodeClassJSON:      types.StringNull(),
	}
	model.EnableImageAccelerator = types.BoolValue(g.EnableImageAccelerator)
	model.EnableLocalSSDEphemeralStorage = types.BoolValue(g.localSSDEphemeralStorageEnabled())
	if g.NodeClassSpec == nil {
		return model, nil
	}
	if g.NodeClassSpec.EphemeralStorageLocalSSD != nil {
		localSSD := GCEEphemeralStorageLocalSSDModel{Count: types.Int32Null(), AutoCount: types.BoolValue(g.NodeClassSpec.EphemeralStorageLocalSSD.AutoCount)}
		if g.NodeClassSpec.EphemeralStorageLocalSSD.Count != nil {
			localSSD.Count = types.Int32Value(*g.NodeClassSpec.EphemeralStorageLocalSSD.Count)
		}
		model.EphemeralStorageLocalSSD = customfield.NewObjectMust(ctx, &localSSD)
	}

	if g.NodeClassSpec.ServiceAccount != "" {
		model.ServiceAccount = types.StringValue(g.NodeClassSpec.ServiceAccount)
	}
	if len(g.NodeClassSpec.Disks) > 0 {
		model.Disks = customfield.NewObjectListMust(ctx, lo.Map(g.NodeClassSpec.Disks, func(d GCEDisk, _ int) GCEDiskModel {
			model := GCEDiskModel{
				SizeGiB:    types.Int64Value(int64(d.SizeGiB)),
				SizePolicy: customfield.NullObject[VolumeSizePolicyModel](ctx),
				Category:   stringValueOrNull(string(d.Category)),
				Boot:       types.BoolValue(d.Boot),
			}
			if d.SizePolicy != nil {
				model.SizePolicy = customfield.NewObjectMust(ctx, &VolumeSizePolicyModel{
					PerVCPUGiB: types.Int64Value(int64(d.SizePolicy.PerVCPUGiB)),
					MaxSizeGiB: types.Int64Value(int64(d.SizePolicy.MaxSizeGiB)),
				})
			}
			return model
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
	g.readGCERuntimeModel(ctx, model)

	return model, nil
}

func (m *GCENodeClassModel) ToGCENodeClass(ctx context.Context, current GCENodeClass) (*GCENodeClass, error) {
	if !m.OriginNodeClassJSON.IsNull() && !m.OriginNodeClassJSON.IsUnknown() && m.OriginNodeClassJSON.ValueString() != "" {
		var raw GCENodeClass
		if err := json.Unmarshal([]byte(m.OriginNodeClassJSON.ValueString()), &raw); err != nil {
			return nil, err
		}
		raw.originJSON = true
		return &raw, nil
	}
	var localSSD *GCEEphemeralStorageLocalSSDModel
	if !m.EphemeralStorageLocalSSD.IsNull() && !m.EphemeralStorageLocalSSD.IsUnknown() {
		value, diags := m.EphemeralStorageLocalSSD.Value(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("ephemeral_storage_local_ssd: %v", diags)
		}
		localSSD = value
		if localSSD != nil && (localSSD.hasManagedValues() || IsKnownEmptyObject(m.EphemeralStorageLocalSSD)) {
			if !m.EnableLocalSSDEphemeralStorage.IsNull() && !m.EnableLocalSSDEphemeralStorage.IsUnknown() {
				if !m.EnableLocalSSDEphemeralStorage.ValueBool() {
					return nil, fmt.Errorf("ephemeral_storage_local_ssd cannot be configured when enable_local_ssd_ephemeral_storage is false")
				}
			} else if !current.localSSDEphemeralStorageEnabled() {
				return nil, fmt.Errorf("ephemeral_storage_local_ssd cannot be configured while the existing enable_local_ssd_ephemeral_storage value is false; set enable_local_ssd_ephemeral_storage to true explicitly")
			}
		}
	}

	out := current
	out.originJSON = false
	if current.NodeClassSpec != nil {
		out.NodeClassSpec = current.NodeClassSpec.DeepCopy()
	}
	out.Name = m.Name.ValueString()
	if !m.EnableImageAccelerator.IsNull() && !m.EnableImageAccelerator.IsUnknown() {
		out.EnableImageAccelerator = m.EnableImageAccelerator.ValueBool()
	}
	if !m.EnableLocalSSDEphemeralStorage.IsNull() && !m.EnableLocalSSDEphemeralStorage.IsUnknown() {
		enabled := m.EnableLocalSSDEphemeralStorage.ValueBool()
		out.EnableLocalSSDEphemeralStorage = &enabled
	}
	if out.NodeClassSpec == nil {
		out.NodeClassSpec = &GCENodeClassSpec{}
	}
	if localSSD != nil {
		applyGCELocalSSDModel(out.NodeClassSpec, *localSSD)
	}
	if !m.EnableLocalSSDEphemeralStorage.IsNull() && !m.EnableLocalSSDEphemeralStorage.IsUnknown() {
		if m.EnableLocalSSDEphemeralStorage.ValueBool() {
			if out.NodeClassSpec.EphemeralStorageLocalSSD == nil {
				out.NodeClassSpec.EphemeralStorageLocalSSD = &GCEEphemeralStorageLocalSSD{}
			}
		} else {
			out.NodeClassSpec.EphemeralStorageLocalSSD = nil
		}
	}
	if !m.ServiceAccount.IsNull() && !m.ServiceAccount.IsUnknown() {
		out.NodeClassSpec.ServiceAccount = m.ServiceAccount.ValueString()
	}
	if err := m.applyGCEDisks(ctx, &out); err != nil {
		return nil, err
	}
	if err := m.applyGCEImageSelectors(ctx, current, &out); err != nil {
		return nil, err
	}
	if err := m.applyGCERuntime(ctx, &out); err != nil {
		return nil, err
	}
	if !m.AutoGPUTaint.IsNull() && !m.AutoGPUTaint.IsUnknown() {
		out.NodeClassSpec.AutoGPUTaint = m.AutoGPUTaint.ValueBool()
	}
	if !m.GPUDriverVersion.IsNull() && !m.GPUDriverVersion.IsUnknown() {
		out.NodeClassSpec.GPUDriverVersion = m.GPUDriverVersion.ValueString()
	}
	if err := validateGCENodeClassLocalSSD(out.NodeClassSpec); err != nil {
		return nil, err
	}
	if err := validateGCENodeClassDiskSizePolicy(out.NodeClassSpec); err != nil {
		return nil, err
	}

	return &out, nil
}

func validateGCENodeClassLocalSSD(spec *GCENodeClassSpec) error {
	if spec != nil && spec.EphemeralStorageLocalSSD != nil && spec.EphemeralStorageLocalSSD.Count != nil && spec.EphemeralStorageLocalSSD.AutoCount {
		return fmt.Errorf("ephemeral_storage_local_ssd.count and auto_count cannot both be configured")
	}
	if spec == nil || spec.EphemeralStorageLocalSSD == nil {
		return nil
	}
	if count := spec.EphemeralStorageLocalSSD.Count; count != nil && (*count < 1 || *count > 32) {
		return fmt.Errorf("ephemeral_storage_local_ssd.count must be between 1 and 32")
	}
	for index, disk := range spec.Disks {
		if disk.Category == gcpproviderv1alpha1.DiskCategory("local-ssd") {
			return fmt.Errorf("ephemeral_storage_local_ssd cannot be combined with disks[%d].category = local-ssd", index)
		}
	}
	return nil
}

func validateGCENodeClassDiskSizePolicy(spec *GCENodeClassSpec) error {
	if spec == nil {
		return nil
	}
	policyCount := 0
	for index, disk := range spec.Disks {
		if disk.SizePolicy == nil {
			continue
		}
		policyCount++
		if disk.SizeGiB < 10 {
			return fmt.Errorf("disks[%d].size_gib is required with size_policy", index)
		}
		if disk.SizePolicy.PerVCPUGiB < 0 || disk.SizePolicy.PerVCPUGiB > 65536 || disk.SizePolicy.MaxSizeGiB < 10 || disk.SizePolicy.MaxSizeGiB > 65536 {
			return fmt.Errorf("disks[%d].size_policy exceeds supported size limits", index)
		}
		if !disk.Boot {
			return fmt.Errorf("disks[%d].size_policy requires boot = true", index)
		}
		if disk.SizePolicy.MaxSizeGiB < disk.SizeGiB {
			return fmt.Errorf("disks[%d].size_policy.max_size_gib must be at least size_gib", index)
		}
		if disk.Category == gcpproviderv1alpha1.DiskCategory("hyperdisk-throughput") && disk.SizePolicy.MaxSizeGiB > 32768 {
			return fmt.Errorf("disks[%d].size_policy.max_size_gib cannot exceed 32768 for hyperdisk-throughput", index)
		}
	}
	if policyCount > 1 {
		return fmt.Errorf("disks supports at most one size_policy")
	}
	return nil
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

func GCEPhysicalDiskFromCurrent(disks []GCEDisk, index int) GCEDisk {
	if index < len(disks) {
		return disks[index]
	}
	return GCEDisk{}
}

func (m GCEEphemeralStorageLocalSSDModel) hasManagedValues() bool {
	return (!m.Count.IsNull() && !m.Count.IsUnknown()) || (!m.AutoCount.IsNull() && !m.AutoCount.IsUnknown())
}

func applyGCELocalSSDModel(spec *GCENodeClassSpec, model GCEEphemeralStorageLocalSSDModel) {
	if model.Count.IsNull() && model.AutoCount.IsNull() {
		spec.EphemeralStorageLocalSSD = &GCEEphemeralStorageLocalSSD{}
		return
	}
	countKnown := !model.Count.IsNull() && !model.Count.IsUnknown()
	autoKnown := !model.AutoCount.IsNull() && !model.AutoCount.IsUnknown()
	if !countKnown && !autoKnown {
		return
	}
	if spec.EphemeralStorageLocalSSD == nil {
		spec.EphemeralStorageLocalSSD = &GCEEphemeralStorageLocalSSD{}
	}
	out := spec.EphemeralStorageLocalSSD
	if countKnown {
		v := model.Count.ValueInt32()
		out.Count = &v
		if !autoKnown {
			out.AutoCount = false
		}
	}
	if autoKnown {
		out.AutoCount = model.AutoCount.ValueBool()
		if out.AutoCount && !countKnown {
			out.Count = nil
		}
	}
}

func (g *GCENodeClass) readGCERuntimeModel(ctx context.Context, model *GCENodeClassModel) {
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

}

func (m *GCENodeClassModel) applyGCEDisks(ctx context.Context, out *GCENodeClass) error {
	if !m.Disks.IsNull() && !m.Disks.IsUnknown() {
		disks, diags := m.Disks.AsStructSliceT(ctx)
		if diags.HasError() {
			return fmt.Errorf("disks: %v", diags)
		}
		currentDisks := out.NodeClassSpec.Disks
		out.NodeClassSpec.Disks = make([]GCEDisk, 0, len(disks))
		for index, diskModel := range disks {
			disk := GCEPhysicalDiskFromCurrent(currentDisks, index)
			if !diskModel.SizeGiB.IsNull() && !diskModel.SizeGiB.IsUnknown() {
				size := diskModel.SizeGiB.ValueInt64()
				if size < 10 || size > 65536 {
					return fmt.Errorf("disks[%d].size_gib must be between 10 and 65536", index)
				}
				disk.SizeGiB = int32(size)
			}
			if !diskModel.Category.IsNull() && !diskModel.Category.IsUnknown() {
				disk.Category = gcpproviderv1alpha1.DiskCategory(diskModel.Category.ValueString())
			}
			if !diskModel.Boot.IsNull() && !diskModel.Boot.IsUnknown() {
				disk.Boot = diskModel.Boot.ValueBool()
			}
			policy, err := gceVolumePolicyFromModel(ctx, diskModel.SizePolicy, disk.SizePolicy)
			if err != nil {
				return fmt.Errorf("disks[%d].size_policy: %w", index, err)
			}
			disk.SizePolicy = policy

			out.NodeClassSpec.Disks = append(out.NodeClassSpec.Disks, disk)
		}
	}

	return nil
}

func (m *GCENodeClassModel) applyGCERuntime(ctx context.Context, out *GCENodeClass) error {
	if !m.SubnetRangeName.IsNull() && !m.SubnetRangeName.IsUnknown() {
		value := m.SubnetRangeName.ValueString()
		out.NodeClassSpec.SubnetRangeName = &value
	}
	if !m.KubeletConfiguration.IsNull() && !m.KubeletConfiguration.IsUnknown() {
		kubeletConfig, diags := m.KubeletConfiguration.Value(ctx)
		if diags.HasError() {
			return fmt.Errorf("kubelet_configuration: %v", diags)
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
			return fmt.Errorf("network_config: %v", diags)
		}
		if networkConfig != nil {
			if out.NodeClassSpec.NetworkConfig == nil {
				out.NodeClassSpec.NetworkConfig = &GCENetworkConfig{}
			}
			if !networkConfig.AdditionalNetworkInterfaces.IsNull() && !networkConfig.AdditionalNetworkInterfaces.IsUnknown() {
				nics, nicDiags := networkConfig.AdditionalNetworkInterfaces.AsStructSliceT(ctx)
				if nicDiags.HasError() {
					return fmt.Errorf("network_config.additional_network_interfaces: %v", nicDiags)
				}
				out.NodeClassSpec.NetworkConfig.AdditionalNetworkInterfaces = make([]GCEAdditionalNetworkInterface, 0, len(nics))
				for index, nic := range nics {
					subnetwork := strings.TrimSpace(stringFromTerraform(nic.Subnetwork))
					if subnetwork == "" {
						return fmt.Errorf("network_config.additional_network_interfaces[%d].subnetwork is required", index)
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

	return nil
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

func (m *GCENodeClassModel) applyGCEImageSelectors(ctx context.Context, current GCENodeClass, out *GCENodeClass) error {
	currentHasLegacyAliasImageSelector := current.NodeClassSpec != nil && hasLegacyAliasImageSelectorTerm(current.NodeClassSpec.ImageSelectorTerms)
	if !m.ImageSelectorTerms.IsNull() && !m.ImageSelectorTerms.IsUnknown() {
		terms, diags := m.ImageSelectorTerms.AsStructSliceT(ctx)
		if diags.HasError() {
			return fmt.Errorf("image_selector_terms: %v", diags)
		}
		if len(terms) == 0 {
			return fmt.Errorf("image_selector_terms cannot be empty")
		}
		if currentHasLegacyAliasImageSelector && gcpDesiredImageSelectorTermsMatchCurrentNormalized(terms, current.NodeClassSpec.ImageSelectorTerms) {
			out.NodeClassSpec.ImageSelectorTerms = current.NodeClassSpec.ImageSelectorTerms
		} else {
			out.NodeClassSpec.ImageSelectorTerms = make([]GCEImageSelectorTerm, 0, len(terms))
			for index, term := range terms {
				specTerm, err := gcpImageSelectorTermModelToSpec(term, index)
				if err != nil {
					return err
				}
				out.NodeClassSpec.ImageSelectorTerms = append(out.NodeClassSpec.ImageSelectorTerms, specTerm)
			}
			if err := validateGCPImageSelectorTermList(out.NodeClassSpec.ImageSelectorTerms); err != nil {
				return err
			}
			out.NodeClassSpec.ImageFamily = nil
		}
	}
	if len(current.rawJSON) == 0 && current.NodeClassSpec == nil && len(out.NodeClassSpec.ImageSelectorTerms) == 0 {
		return fmt.Errorf("image_selector_terms must be set for a new gce nodeclass")
	}

	return nil
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
	if g.originJSON {
		return append([]byte(nil), g.rawJSON...), nil
	}
	type alias GCENodeClass
	return jsonmerge.MarshalPreservingUnknown(g.rawJSON, alias(g), jsonmerge.WithPositionalArrayElements(GCEDisk{}))
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
	if g.originJSON {
		return append([]byte(nil), g.rawJSON...), nil
	}
	type alias GCENodePool
	return jsonmerge.MarshalPreservingUnknown(g.rawJSON, alias(g))
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
