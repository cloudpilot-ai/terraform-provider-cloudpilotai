package gke

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	commonvalidators "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/validators"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func Schema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Description: "GKE Cluster",
		Attributes: map[string]schema.Attribute{
			"kubeconfig": schema.StringAttribute{
				Description: "Kubernetes configuration file path for accessing the GKE cluster. If not set, the provider tries to generate one during CRUD flows, including import-driven GKE operations where cluster metadata is sufficient to infer project access.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					useStateForUnknownNonNullString(),
				},
			},
			"cluster_name": schema.StringAttribute{
				Description: "Name of the GKE cluster to be managed.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					requiresReplaceString(),
				},
			},
			"region": schema.StringAttribute{
				Description: "GCP region where the GKE cluster is located.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					requiresReplaceString(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "GCP project ID where the GKE cluster is located. When unset, the provider first tries to infer it from GKE metadata it already knows, then falls back to the active local gcloud project.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					useStateForUnknownString(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Description: "Optional override for the CloudPilot cluster ID. When omitted, the provider generates the same GCP cluster ID as the server from cluster_name, region, and cluster_uid.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					useStateForUnknownString(),
					requiresReplaceString(),
				},
			},
			"cluster_uid": schema.StringAttribute{
				Description: "Kubernetes cluster UID used to derive the deterministic CloudPilot cluster ID. For GKE, this matches the kube-system namespace UID. When unset, the provider tries to read it from the target cluster through kubeconfig before requiring manual input.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					useStateForUnknownString(),
					requiresReplaceString(),
				},
			},
			"cluster_location": schema.StringAttribute{
				Description: "Optional GKE cluster location override for zonal clusters.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					requiresReplaceString(),
				},
			},
			"agent_version": schema.StringAttribute{
				Description: "Version of the CloudPilot agent currently installed on the cluster.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					useStateForUnknownString(),
				},
			},
			"onboard_manifest_version": schema.StringAttribute{
				Description: "Latest CloudPilot onboard manifest version reported by the service.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					useStateForUnknownString(),
				},
			},
			"need_upgrade": schema.BoolAttribute{
				Description: "Whether the CloudPilot service currently reports that this cluster needs an upgrade.",
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					useStateForUnknownBool(),
				},
			},
			"disable_workload_uploading": schema.BoolAttribute{
				Description: "Disable automatic uploading of workload information to CloudPilot AI.",
				Optional:    true,
			},
			"only_install_agent": schema.BoolAttribute{
				Description: "Only install the CloudPilot AI agent without additional node autoscaler configuration.",
				Optional:    true,
			},
			"enable_upgrade": schema.BoolAttribute{
				Description: "Enable upgrading CloudPilot AI components when the service reports this cluster needs an upgrade.",
				Optional:    true,
			},
			"enable_rebalance": schema.BoolAttribute{
				Description: "Enable CloudPilot node autoscaler / rebalance behavior for the cluster. This overrides only_install_agent when true.",
				Optional:    true,
			},
			"skip_restore": schema.BoolAttribute{
				Description: "When set to true, skip restoring the original regular GKE node pools during cluster destroy. This matches the EKS-style destroy switch and leaves the current optimized nodes untouched while uninstalling CloudPilot.",
				Optional:    true,
			},
			"restore_node_number": schema.Int64Attribute{
				Description: "Total number of regular GKE node-pool nodes to restore during cluster destroy. For regional or multi-zone GKE node pools, this is the desired total across all locations. Set to 0 to skip restore unless restore_desired_sizes is set.",
				Optional:    true,
			},
			"restore_desired_sizes": schema.MapAttribute{
				Description: "Optional per-node-pool desired total node counts used during cluster destroy. Keys are GKE node-pool names and values are desired total nodes for each pool.",
				Optional:    true,
				ElementType: types.Int64Type,
			},
			"cluster_setting": schema.SingleNestedAttribute{
				Description: "Optional cluster-level setting fields exposed by /api/v1/clusters/{cluster_id}/setting.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectType[ClusterSettingModel](ctx),
				Attributes: map[string]schema.Attribute{
					"enable_node_repair": schema.BoolAttribute{
						Description: "Enable CloudPilot AI node repair for this cluster.",
						Optional:    true,
					},
					"enable_disk_monitor": schema.BoolAttribute{
						Description: "Enable disk monitor for this cluster.",
						Optional:    true,
					},
					"discount": schema.Float64Attribute{
						Description: "Cluster-level discount ratio used by cost calculations.",
						Optional:    true,
					},
					"pre_run_command": schema.StringAttribute{
						Description: "Command run before maintenance or repair actions.",
						Optional:    true,
					},
					"post_run_command": schema.StringAttribute{
						Description: "Command run after maintenance or repair actions.",
						Optional:    true,
					},
				},
			},
			"nodeclasses": schema.ListNestedAttribute{
				Description: "GCENodeClasses configuration managed by the GKE cluster resource.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.GCENodeClassModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: gkeNodeClassSchema(ctx),
				},
			},
			"nodepools": schema.ListNestedAttribute{
				Description: "GCENodePools configuration managed by the GKE cluster resource.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.GCENodePoolModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: gkeNodePoolSchema(ctx),
				},
			},
		},
	}
}

func gkeNodeClassSchema(ctx context.Context) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name": schema.StringAttribute{
			Description: "NodeClass name.",
			Required:    true,
		},
		"enable_image_accelerator": schema.BoolAttribute{
			Description: "Enable Image Accelerator bootstrap for nodes launched from this NodeClass.",
			Optional:    true,
		},
		"service_account": schema.StringAttribute{
			Description: "Service account used by nodes launched from this NodeClass.",
			Optional:    true,
		},
		"disks": schema.ListNestedAttribute{
			Description: "GCE disks attached to provisioned nodes.",
			Optional:    true,
			CustomType:  customfield.NewNestedObjectListType[api.GCEDiskModel](ctx),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"size_gib": schema.Int64Attribute{
						Description: "Disk size in GiB.",
						Optional:    true,
					},
					"category": schema.StringAttribute{
						Description: "Disk category, for example pd-balanced.",
						Optional:    true,
					},
					"boot": schema.BoolAttribute{
						Description: "Whether this is the boot disk.",
						Optional:    true,
					},
				},
			},
		},
		"image_selector_terms": schema.ListNestedAttribute{
			Description: "Image selector terms for the GCENodeClass.",
			Optional:    true,
			CustomType:  customfield.NewNestedObjectListType[api.GCEImageSelectorTermModel](ctx),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Description: "Image ID selector.",
						Optional:    true,
					},
					"family": schema.StringAttribute{
						Description: "Image family selector.",
						Optional:    true,
					},
					"channel": schema.StringAttribute{
						Description: "ContainerOptimizedOS channel selector.",
						Optional:    true,
					},
					"version": schema.StringAttribute{
						Description: "Image version selector.",
						Optional:    true,
					},
				},
			},
		},
		"subnet_range_name": schema.StringAttribute{
			Description: "Alias IP secondary range name used for pods.",
			Optional:    true,
		},
		"kubelet_configuration": schema.SingleNestedAttribute{
			Description: "Kubelet configuration overrides for nodes in this NodeClass.",
			Optional:    true,
			CustomType:  customfield.NewNestedObjectType[api.GCEKubeletConfigurationModel](ctx),
			Attributes: map[string]schema.Attribute{
				"kube_reserved": schema.MapAttribute{
					Description: "Kubelet kubeReserved map.",
					Optional:    true,
					ElementType: types.StringType,
					CustomType:  customfield.NewMapType[types.String](ctx),
				},
				"system_reserved": schema.MapAttribute{
					Description: "Kubelet systemReserved map.",
					Optional:    true,
					ElementType: types.StringType,
					CustomType:  customfield.NewMapType[types.String](ctx),
				},
				"eviction_hard": schema.MapAttribute{
					Description: "Kubelet evictionHard map.",
					Optional:    true,
					ElementType: types.StringType,
					CustomType:  customfield.NewMapType[types.String](ctx),
				},
				"eviction_soft": schema.MapAttribute{
					Description: "Kubelet evictionSoft map.",
					Optional:    true,
					ElementType: types.StringType,
					CustomType:  customfield.NewMapType[types.String](ctx),
				},
			},
		},
		"labels": schema.MapAttribute{
			Description: "Labels applied through the GCENodeClass spec.",
			Optional:    true,
			ElementType: types.StringType,
			CustomType:  customfield.NewMapType[types.String](ctx),
		},
		"metadata": schema.MapAttribute{
			Description: "Instance metadata applied through the GCENodeClass spec.",
			Optional:    true,
			ElementType: types.StringType,
			CustomType:  customfield.NewMapType[types.String](ctx),
		},
		"network_tags": schema.ListAttribute{
			Description: "Network tags applied to provisioned instances.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"confidential_instance_type": schema.StringAttribute{
			Description: "Confidential instance type setting for this NodeClass.",
			Optional:    true,
		},
		"network_config": schema.SingleNestedAttribute{
			Description: "Network configuration for this NodeClass.",
			Optional:    true,
			CustomType:  customfield.NewNestedObjectType[api.GCENetworkConfigModel](ctx),
			Attributes: map[string]schema.Attribute{
				"enable_private_nodes": schema.BoolAttribute{
					Description: "Enable private nodes for this NodeClass.",
					Optional:    true,
				},
				"subnetwork": schema.StringAttribute{
					Description: "Subnetwork self link or resource path.",
					Optional:    true,
				},
				"additional_network_interfaces": schema.ListNestedAttribute{
					Description: "Additional network interfaces to attach.",
					Optional:    true,
					CustomType:  customfield.NewNestedObjectListType[api.GCEAdditionalNetworkInterfaceModel](ctx),
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"network": schema.StringAttribute{
								Description: "Additional interface network.",
								Optional:    true,
							},
							"subnetwork": schema.StringAttribute{
								Description: "Additional interface subnetwork.",
								Optional:    true,
							},
						},
					},
				},
			},
		},
		"auto_gpu_taint": schema.BoolAttribute{
			Description: "Automatically add taints for GPU nodes.",
			Optional:    true,
		},
		"gpu_driver_version": schema.StringAttribute{
			Description: "GPU driver version to configure on provisioned nodes.",
			Optional:    true,
		},
		"origin_nodeclass_json": schema.StringAttribute{
			Description: "The origin node class JSON. If configured, the other nodeclass configuration items are ignored.",
			Optional:    true,
		},
	}
}

func gkeNodePoolSchema(ctx context.Context) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name": schema.StringAttribute{
			Description: "NodePool name.",
			Required:    true,
		},
		"enable": schema.BoolAttribute{
			Description: "Enable this nodepool.",
			Optional:    true,
		},
		"enable_image_accelerator": schema.BoolAttribute{
			Description: "Enable Image Accelerator scheduling markers for this nodepool.",
			Optional:    true,
		},
		"nodeclass": schema.StringAttribute{
			Description: "Select the nodeclass to use for this nodepool.",
			Optional:    true,
		},
		"enable_gpu": schema.BoolAttribute{
			Description: "Enable GPU instances in this nodepool.",
			Optional:    true,
		},
		"provision_priority": schema.Int32Attribute{
			Description: "The priority level of this nodepool. A larger number means a higher priority.",
			Optional:    true,
		},
		"instance_family": schema.ListAttribute{
			Description: "Target GCE instance families for this nodepool.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"instance_arch": schema.ListAttribute{
			Description: "Target instance architectures for this nodepool.",
			Optional:    true,
			ElementType: types.StringType,
			Validators:  commonvalidators.ArchValidators(),
		},
		"capacity_type": schema.ListAttribute{
			Description: "Provisioned node capacity types, such as on-demand or spot.",
			Optional:    true,
			ElementType: types.StringType,
			Validators:  commonvalidators.CapacityTypeValidators(),
		},
		"zone": schema.ListAttribute{
			Description: "Zones where nodes may be provisioned.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"instance_cpu_min": schema.Int64Attribute{
			Description: "Minimum CPU cores per node. Set to 0 for unlimited.",
			Optional:    true,
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"instance_cpu_max": schema.Int64Attribute{
			Description: "Maximum CPU cores per node. Set to 0 for unlimited.",
			Optional:    true,
		},
		"instance_memory_min": schema.Int64Attribute{
			Description: "Minimum memory in MiB per node. Set to 0 for unlimited.",
			Optional:    true,
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"instance_memory_max": schema.Int64Attribute{
			Description: "Maximum memory in MiB per node. Set to 0 for unlimited.",
			Optional:    true,
		},
		"labels": schema.MapAttribute{
			Description: "Labels applied to provisioned nodes through spec.template.metadata.labels.",
			Optional:    true,
			ElementType: types.StringType,
			CustomType:  customfield.NewMapType[types.String](ctx),
		},
		"taints": schema.ListNestedAttribute{
			Description: "Taints applied to provisioned nodes through spec.template.spec.taints.",
			Optional:    true,
			CustomType:  customfield.NewNestedObjectListType[api.TaintModel](ctx),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"key": schema.StringAttribute{
						Description: "Taint key.",
						Required:    true,
					},
					"value": schema.StringAttribute{
						Description: "Taint value.",
						Optional:    true,
					},
					"effect": schema.StringAttribute{
						Description: "Taint effect: NoSchedule, PreferNoSchedule, or NoExecute.",
						Required:    true,
					},
				},
			},
		},
		"node_disruption_limit": schema.StringAttribute{
			Description: "Maximum number of nodes that can be terminated at once, either as a fixed number or percentage.",
			Optional:    true,
		},
		"node_disruption_delay": schema.StringAttribute{
			Description: "Duration the controller waits before terminating underutilized nodes.",
			Optional:    true,
		},
		"origin_nodepool_json": schema.StringAttribute{
			Description: "The origin nodepool JSON. If configured, the other nodepool configuration items are ignored.",
			Optional:    true,
		},
	}
}

func resolveClusterUID(preferred, fallback, clusterName, region, clusterUID types.String) string {
	if !preferred.IsNull() && !preferred.IsUnknown() && preferred.ValueString() != "" {
		return preferred.ValueString()
	}
	if !fallback.IsNull() && !fallback.IsUnknown() && fallback.ValueString() != "" {
		return fallback.ValueString()
	}
	return api.GenerateClusterUID(api.CloudProviderGCP, clusterName.ValueString(), region.ValueString(), clusterUID.ValueString())
}

func useStateForUnknownInt64() planmodifier.Int64 {
	return useStateForUnknownInt64Modifier{}
}

func useStateForUnknownBool() planmodifier.Bool {
	return useStateForUnknownBoolModifier{}
}

func useStateForUnknownString() planmodifier.String {
	return useStateForUnknownStringModifier{}
}

func useStateForUnknownNonNullString() planmodifier.String {
	return useStateForUnknownNonNullStringModifier{}
}

func requiresReplaceString() planmodifier.String {
	return requiresReplaceStringModifier{}
}

type useStateForUnknownInt64Modifier struct{}
type useStateForUnknownBoolModifier struct{}
type useStateForUnknownStringModifier struct{}
type useStateForUnknownNonNullStringModifier struct{}
type requiresReplaceStringModifier struct{}

func (m useStateForUnknownInt64Modifier) Description(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownInt64Modifier) MarkdownDescription(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownBoolModifier) Description(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownBoolModifier) MarkdownDescription(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownStringModifier) Description(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownStringModifier) MarkdownDescription(_ context.Context) string {
	return "Preserve the prior state value when the planned value is unknown."
}

func (m useStateForUnknownNonNullStringModifier) Description(_ context.Context) string {
	return "Preserve the prior non-null state value when the planned value is unknown."
}

func (m useStateForUnknownNonNullStringModifier) MarkdownDescription(_ context.Context) string {
	return "Preserve the prior non-null state value when the planned value is unknown."
}

func (m requiresReplaceStringModifier) Description(_ context.Context) string {
	return "Changing this value requires replacing the resource."
}

func (m requiresReplaceStringModifier) MarkdownDescription(_ context.Context) string {
	return "Changing this value requires replacing the resource."
}

func (m useStateForUnknownInt64Modifier) PlanModifyInt64(_ context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if req.State.Raw.IsNull() {
		return
	}
	if !req.PlanValue.IsUnknown() {
		return
	}
	if req.ConfigValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.StateValue
}

func (m useStateForUnknownBoolModifier) PlanModifyBool(_ context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if req.State.Raw.IsNull() {
		return
	}
	if !req.PlanValue.IsUnknown() {
		return
	}
	if req.ConfigValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.StateValue
}

func (m useStateForUnknownStringModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() {
		return
	}
	if !req.PlanValue.IsUnknown() {
		return
	}
	if req.ConfigValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.StateValue
}

func (m useStateForUnknownNonNullStringModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() {
		return
	}
	if !req.PlanValue.IsUnknown() {
		return
	}
	if req.ConfigValue.IsUnknown() {
		return
	}
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.StateValue
}

func (m requiresReplaceStringModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	if req.PlanValue.Equal(req.StateValue) {
		return
	}
	resp.RequiresReplace = true
}
