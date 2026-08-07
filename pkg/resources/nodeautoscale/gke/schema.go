package gke

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	commonschemas "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/schemas"
	commonvalidators "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/validators"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func Schema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Description: "Manages a Google Kubernetes Engine cluster registration, CloudPilot agents, cluster settings, GCENodeClasses, NodePools, and scheduled rebalances.",
		Attributes: map[string]schema.Attribute{
			"kubeconfig": schema.StringAttribute{
				Description: "Optional Kubernetes configuration file path for accessing the GKE cluster. If not set, the provider generates an execution-local kubeconfig when needed without storing its path in Terraform state.",
				Optional:    true,
			},
			"cluster_name": schema.StringAttribute{
				Description: "Name of the GKE cluster to manage. Changing this value replaces the Terraform resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					requiresReplaceString(),
				},
			},
			"region": schema.StringAttribute{
				Description: "GCP region associated with the CloudPilot cluster identity. Changing this value replaces the Terraform resource.",
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
				Description: "Optional CloudPilot cluster ID override. When omitted, the provider derives the ID from `cluster_name`, `region`, and `cluster_uid`. Changing a configured value replaces the Terraform resource.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					useStateForUnknownString(),
					requiresReplaceString(),
				},
			},
			"cluster_uid": schema.StringAttribute{
				Description: "Kubernetes cluster UID used to derive the deterministic CloudPilot cluster ID. For GKE, this is the `kube-system` namespace UID. When unset, the provider tries to discover it through kubeconfig. Changing a configured value replaces the Terraform resource.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					useStateForUnknownString(),
					requiresReplaceString(),
				},
			},
			"cluster_location": schema.StringAttribute{
				Description: "Optional exact GKE location used for `gcloud container clusters get-credentials`. Set this for zonal clusters when it differs from `region`. Changing it replaces the Terraform resource.",
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
				Validators:  commonvalidators.Int64AtLeast(0),
			},
			"restore_desired_sizes": schema.MapAttribute{
				Description: "Optional per-node-pool desired total node counts used during cluster destroy. Keys are GKE node-pool names and values are non-negative desired totals across all locations. Per-pool values take precedence over `restore_node_number`.",
				Optional:    true,
				ElementType: types.Int64Type,
			},
			"cluster_setting": schema.SingleNestedAttribute{
				Description: "Optional cluster-level settings managed through `/api/v1/clusters/{cluster_id}/setting`. When omitted, Terraform does not manage these settings. Omit individual fields to preserve their server values.",
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
					"enable_node_pool_decommission": schema.BoolAttribute{
						Description: "Enable node pool decommissioning. When omitted, Terraform does not manage this setting.",
						Optional:    true,
					},
					"enable_workload_min_non_spot": schema.BoolAttribute{
						Description: "Enable minimum non-spot workload replicas. When omitted, Terraform does not manage this setting.",
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
				Description: "GCENodeClasses managed by Terraform. Omit to leave NodeClasses unmanaged; use an empty list to remove NodeClasses previously managed by this resource.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.GCENodeClassModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: gkeNodeClassSchema(ctx),
				},
			},
			"nodepools": schema.ListNestedAttribute{
				Description: "Karpenter NodePools managed by Terraform. Omit to leave NodePools unmanaged; use an empty list to remove NodePools previously managed by this resource.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.GCENodePoolModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: gkeNodePoolSchema(ctx),
				},
			},
			"scheduled_rebalances": commonschemas.ScheduledRebalancesSchema(ctx),
		},
	}
}

func gkeNodeClassSchema(ctx context.Context) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name": schema.StringAttribute{
			Description: "GCENodeClass name. NodePools reference this value through `nodeclass`.",
			Required:    true,
		},
		"enable_image_accelerator": schema.BoolAttribute{
			Description: "Enable Image Accelerator bootstrap for nodes launched from this NodeClass.",
			Optional:    true,
		},
		"enable_local_ssd_ephemeral_storage": schema.BoolAttribute{
			Description: "Use GCE Local SSDs as kubelet ephemeral storage. When omitted, Terraform preserves the server setting.",
			Optional:    true,
		},
		"ephemeral_storage_local_ssd": schema.SingleNestedAttribute{
			Description: "Optional Local SSD configuration. Configure `enable_local_ssd_ephemeral_storage = true` on the same NodeClass. When omitted, Terraform preserves the server configuration.",
			Optional:    true,
			CustomType:  customfield.NewNestedObjectType[api.GCEEphemeralStorageLocalSSDModel](ctx),
			Attributes: map[string]schema.Attribute{
				"count": schema.Int32Attribute{
					Description: "Number of NVMe Local SSDs to attach. Allowed range: 1 to 32. Omit for machine types with bundled Local SSDs, where the fixed bundled count is used.",
					Optional:    true,
					Validators:  commonvalidators.Int32Between(1, 32),
				},
			},
		},
		"service_account": schema.StringAttribute{
			Description: "IAM service account email used by nodes launched from this NodeClass. When omitted, the cluster or Compute Engine default applies.",
			Optional:    true,
		},
		"disks": schema.ListNestedAttribute{
			Description: "GCE disks attached to provisioned nodes. Supports at most 10 disks.",
			Optional:    true,
			Validators:  commonvalidators.ListSizeAtMost(10),
			CustomType:  customfield.NewNestedObjectListType[api.GCEDiskModel](ctx),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"size_gib": schema.Int64Attribute{
						Description: "Disk size in GiB. Must be at least 10 when configured.",
						Optional:    true,
						Validators:  commonvalidators.Int64AtLeast(10),
					},
					"category": schema.StringAttribute{
						Description: "GCE disk category. Allowed values: `hyperdisk-balanced`, `hyperdisk-balanced-high-availability`, `hyperdisk-extreme`, `hyperdisk-ml`, `hyperdisk-throughput`, `local-ssd`, `pd-balanced`, `pd-extreme`, `pd-ssd`, `pd-standard`.",
						Optional:    true,
						Validators: commonvalidators.StringOneOf(
							"hyperdisk-balanced", "hyperdisk-balanced-high-availability", "hyperdisk-extreme", "hyperdisk-ml", "hyperdisk-throughput",
							"local-ssd", "pd-balanced", "pd-extreme", "pd-ssd", "pd-standard",
						),
					},
					"boot": schema.BoolAttribute{
						Description: "Whether this is the boot disk.",
						Optional:    true,
					},
				},
			},
		},
		"image_selector_terms": schema.ListNestedAttribute{
			Description: "Image selector terms for the GCENodeClass. Configure 1 to 30 terms. Each term must set either `id`, or `family` with exactly one of `channel` or `version`; terms are ORed.",
			Optional:    true,
			Validators:  commonvalidators.ListSizeBetween(1, 30),
			CustomType:  customfield.NewNestedObjectListType[api.GCEImageSelectorTermModel](ctx),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Description: "Full GCE image resource ID. Mutually exclusive with `family`, `channel`, and `version`.",
						Optional:    true,
					},
					"family": schema.StringAttribute{
						Description: "OS image family. Allowed values: `ContainerOptimizedOS`, `Ubuntu2404`, `Ubuntu2204`. Requires exactly one of `channel` or `version`.",
						Optional:    true,
						Validators:  commonvalidators.StringOneOf("ContainerOptimizedOS", "Ubuntu2404", "Ubuntu2204"),
					},
					"channel": schema.StringAttribute{
						Description: "GKE release channel for `ContainerOptimizedOS`. Allowed values: `rapid`, `regular`, `stable`, `extended`, `cluster`. Mutually exclusive with `version`.",
						Optional:    true,
						Validators:  commonvalidators.StringOneOf("rapid", "regular", "stable", "extended", "cluster"),
					},
					"version": schema.StringAttribute{
						Description: "Pinned image version or `latest`. ContainerOptimizedOS uses `milestone.build.build.build`, for example `125.19216.104.126`; Ubuntu uses `vYYYYMMDD`, for example `v20260416`. Mutually exclusive with `channel`.",
						Optional:    true,
					},
				},
			},
		},
		"subnet_range_name": schema.StringAttribute{
			Description: "Alias IP secondary range name used for pod IPs. Must be 1 to 63 RFC 1035 characters, start with a lowercase letter, and contain only lowercase letters, digits, and hyphens.",
			Optional:    true,
			Validators:  commonvalidators.StringMatches(`^[a-z]([-a-z0-9]{0,61}[a-z0-9])?$`, "must be a valid 1-63 character RFC 1035 name"),
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
			Description: "GCE network tags applied to provisioned instances. Supports at most 20 unique RFC 1035 tag values.",
			Optional:    true,
			ElementType: types.StringType,
			Validators: append(
				commonvalidators.ListSizeAtMost(20),
				commonvalidators.StringListMatches(`^[a-z]([-a-z0-9]{0,61}[a-z0-9])?$`, "must be a valid 1-63 character RFC 1035 network tag")...,
			),
		},
		"confidential_instance_type": schema.StringAttribute{
			Description: "Confidential VM technology. Allowed values: `SEV`, `SEV_SNP`, `TDX`. Omit to disable confidential computing; availability depends on the selected machine family.",
			Optional:    true,
			Validators:  commonvalidators.StringOneOf("SEV", "SEV_SNP", "TDX"),
		},
		"network_config": schema.SingleNestedAttribute{
			Description: "Network configuration for this NodeClass.",
			Optional:    true,
			CustomType:  customfield.NewNestedObjectType[api.GCENetworkConfigModel](ctx),
			Attributes: map[string]schema.Attribute{
				"enable_private_nodes": schema.BoolAttribute{
					Description: "Whether provisioned nodes receive internal IP addresses only. When omitted, the cluster's private-node setting applies.",
					Optional:    true,
				},
				"subnetwork": schema.StringAttribute{
					Description: "Primary subnetwork resource path in the form `projects/{project}/regions/{region}/subnetworks/{name}`. When omitted, the cluster primary subnetwork applies.",
					Optional:    true,
				},
				"additional_network_interfaces": schema.ListNestedAttribute{
					Description: "Additional network interfaces to attach. Supports at most 7 interfaces; every entry must set `subnetwork`.",
					Optional:    true,
					Validators:  commonvalidators.ListSizeAtMost(7),
					CustomType:  customfield.NewNestedObjectListType[api.GCEAdditionalNetworkInterfaceModel](ctx),
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"network": schema.StringAttribute{
								Description: "VPC network resource path. When omitted, the cluster network applies.",
								Optional:    true,
							},
							"subnetwork": schema.StringAttribute{
								Description: "Subnetwork resource path for this additional interface. Required for new typed configurations.",
								Optional:    true,
							},
						},
					},
				},
			},
		},
		"auto_gpu_taint": schema.BoolAttribute{
			Description: "Automatically apply `nvidia.com/gpu=present:NoSchedule` to provisioned GPU nodes. When omitted, Terraform preserves the server value.",
			Optional:    true,
		},
		"gpu_driver_version": schema.StringAttribute{
			Description: "GKE NVIDIA driver installation policy. Allowed values: `default`, `latest`, `disabled`. `latest` is supported only by Container-Optimized OS; the setting is ignored for non-GPU nodes.",
			Optional:    true,
			Validators:  commonvalidators.StringOneOf("default", "latest", "disabled"),
		},
		"origin_nodeclass_json": schema.StringAttribute{
			Description: "Raw GCENodeClass JSON. When configured, it replaces the generated NodeClass and all other typed fields on this object are ignored. The JSON must contain a valid GCENodeClass object.",
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
			Description: "Whether this NodePool is enabled for provisioning. When omitted, Terraform preserves the server value.",
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
			Description: "Karpenter NodePool weight from 1 to 100. A larger number gives this NodePool higher provisioning priority. Omit to use Karpenter's unweighted behavior.",
			Optional:    true,
			Validators:  commonvalidators.Int32Between(1, 100),
		},
		"instance_family": schema.ListAttribute{
			Description: "Target GCE instance families for this nodepool.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"instance_arch": schema.ListAttribute{
			Description: "Allowed CPU architectures. Allowed values: `amd64`, `arm64`.",
			Optional:    true,
			ElementType: types.StringType,
			Validators:  commonvalidators.ArchValidators(),
		},
		"capacity_type": schema.ListAttribute{
			Description: "Allowed capacity types. Allowed values: `on-demand`, `spot`.",
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
			Validators:  commonvalidators.Int64AtLeast(0),
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"instance_cpu_max": schema.Int64Attribute{
			Description: "Maximum CPU cores per node. Set to 0 for unlimited.",
			Optional:    true,
			Validators:  commonvalidators.Int64AtLeast(0),
		},
		"instance_memory_min": schema.Int64Attribute{
			Description: "Minimum memory in MiB per node. Set to 0 for unlimited.",
			Optional:    true,
			Validators:  commonvalidators.Int64AtLeast(0),
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"instance_memory_max": schema.Int64Attribute{
			Description: "Maximum memory in MiB per node. Set to 0 for unlimited.",
			Optional:    true,
			Validators:  commonvalidators.Int64AtLeast(0),
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
						Validators:  commonvalidators.StringOneOf("NoSchedule", "PreferNoSchedule", "NoExecute"),
					},
				},
			},
		},
		"node_disruption_limit": schema.StringAttribute{
			Description:        "Maximum number of nodes that can be terminated at once, either as a fixed number or percentage. Use node_disruption_budgets instead.",
			Optional:           true,
			DeprecationMessage: "node_disruption_limit is deprecated; use node_disruption_budgets instead.",
		},
		"node_disruption_budgets": schema.ListNestedAttribute{
			Description: "Complete disruption budget list containing 1 to 50 budgets. When null, Terraform does not manage the list and `node_disruption_limit` keeps its legacy first-budget behavior. If `node_disruption_limit` is also set, it must match the first budget's `nodes` value.",
			Optional:    true,
			Validators:  commonvalidators.ListSizeBetween(1, 50),
			CustomType:  customfield.NewNestedObjectListType[api.DisruptionBudgetModel](ctx),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"nodes": schema.StringAttribute{
						Description: "Maximum nodes that may be disrupted, expressed as a non-negative integer such as `2` or a percentage from `0%` to `100%`.",
						Required:    true,
						Validators:  commonvalidators.StringMatches(`^((100|[0-9]{1,2})%|[0-9]+)$`, "must be a non-negative integer or percentage from 0% to 100%"),
					},
					"reasons": schema.ListAttribute{
						Description: "Optional non-empty disruption reasons: Empty, Underutilized, or Drifted. Omit to apply the budget to all reasons.",
						Optional:    true,
						ElementType: types.StringType,
						Validators:  commonvalidators.StringListOneOfWithSize(1, 50, "Empty", "Underutilized", "Drifted"),
					},
					"schedule": schema.StringAttribute{
						Description: "Optional cron schedule. Must be configured together with duration.",
						Optional:    true,
					},
					"duration": schema.StringAttribute{
						Description: "How long the scheduled budget remains active. Must be configured together with schedule.",
						Optional:    true,
					},
				},
			},
		},
		"node_disruption_delay": schema.StringAttribute{
			Description: "How long Karpenter waits before consolidating an underutilized node. Use one or more integer duration components with `s`, `m`, or `h`, for example `30s`, `10m`, or `1h30m`; use `Never` to disable consolidation.",
			Optional:    true,
			Validators:  commonvalidators.StringMatches(`^(([0-9]+(s|m|h))+|Never)$`, "must be a duration using s, m, or h components, or Never"),
		},
		"origin_nodepool_json": schema.StringAttribute{
			Description: "Raw Karpenter NodePool JSON. When configured, it replaces the generated NodePool and all other typed fields on this object are ignored. The JSON must contain a valid NodePool object.",
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
