package eks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	commonschemas "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/schemas"
	commonvalidators "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/validators"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func Schema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Description: "Manages an Amazon EKS cluster registration, CloudPilot agents, cluster settings, NodeClasses, NodePools, workloads, and scheduled rebalances.",
		Attributes: map[string]schema.Attribute{
			"aws_profile": schema.StringAttribute{
				Description: "AWS CLI named profile to use as the source credential for AWS operations. If empty, the default AWS credential chain is used.",
				Optional:    true,
			},

			"aws_assume_role": schema.SingleNestedAttribute{
				Description: "Optional IAM role to assume for CloudPilot AWS CLI and kubeconfig operations. Source credentials still come from aws_profile or the ambient AWS credential chain.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectType[AWSAssumeRoleModel](ctx),
				Attributes: map[string]schema.Attribute{
					"role_arn": schema.StringAttribute{
						Description: "IAM role ARN to assume before CloudPilot performs AWS CLI, kubectl, or helm operations.",
						Required:    true,
					},
					"session_name": schema.StringAttribute{
						Description: "Optional STS session name used when assuming the role. Defaults to cloudpilotai-terraform when omitted.",
						Optional:    true,
					},
				},
			},

			"kubeconfig": schema.StringAttribute{
				Description: "Optional Kubernetes configuration file path for accessing the EKS cluster. If not set, the provider generates an execution-local kubeconfig for each operation without storing its path in Terraform state.",
				Optional:    true,
			},

			"cluster_name": schema.StringAttribute{
				Description: "Name of the EKS cluster to be managed",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "AWS region where the EKS cluster is located",
				Required:    true,
			},

			"skip_restore": schema.BoolAttribute{
				Description: "When set to true, skip the node restore step during resource destruction. The cluster will be uninstalled without restoring original nodes first. Takes precedence over `restore_node_number`.",
				Optional:    true,
			},
			"restore_node_number": schema.Int64Attribute{
				Description: "Number of nodes to provision from the original node group when destroying the CloudPilot AI resource. Set to 0 (the default) to leave the cluster in its current optimized state without restoring original nodes. Set to a positive integer to restore that many nodes before uninstalling. Only effective when `skip_restore` is false.",
				Optional:    true,
				Validators:  commonvalidators.Int64AtLeast(0),
			},

			"cluster_id": schema.StringAttribute{
				Description: "Unique identifier of the EKS cluster. Optional override for existing clusters when the caller already knows the server-side cluster ID.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					useStateForUnknownString(),
				},
			},
			"account_id": schema.StringAttribute{
				Description: "AWS account ID where the cluster is deployed (computed)",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					useStateForUnknownNonNullString(),
				},
			},
			"agent_version": schema.StringAttribute{
				Description: "Version of the CloudPilot AI agent currently installed on the cluster (computed).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					useStateForUnknownString(),
				},
			},
			"onboard_manifest_version": schema.StringAttribute{
				Description: "Latest CloudPilot onboard manifest version reported by the service (computed).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					useStateForUnknownString(),
				},
			},
			"need_upgrade": schema.BoolAttribute{
				Description: "Whether the CloudPilot service currently reports that this cluster needs an upgrade (computed).",
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					useStateForUnknownBool(),
				},
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

			// agent configurations
			"disable_workload_uploading": schema.BoolAttribute{
				Description: "Disable automatic uploading of workload information to CloudPilot AI",
				Optional:    true,
			},

			"only_install_agent": schema.BoolAttribute{
				Description: "Only install the CloudPilot AI agent without additional configuration",
				Optional:    true,
			},

			"enable_upgrade": schema.BoolAttribute{
				Description: "Enable upgrading CloudPilot AI components through the cluster upgrade script. The provider checks whether the cluster needs upgrade first, and only runs the upgrade when required.",
				Optional:    true,
			},

			// rebalance configurations
			"enable_rebalance": schema.BoolAttribute{
				Description: "Enable automatic workload rebalancing across node pools. Ignores `only_install_agent` if set to true.",
				Optional:    true,
			},

			"custom_node_role": schema.StringAttribute{
				Description: "Custom IAM role name for EC2 instances. When set, this role will be added to the CloudPilot controller's PassNodeIAMRole policy during installation, allowing the controller to pass this role to EC2 instances.",
				Optional:    true,
			},

			"workload_templates": commonschemas.WorkloadTemplateSchema(ctx),
			"workloads":          commonschemas.WorkloadSchema(ctx),

			"nodeclass_templates": schema.ListNestedAttribute{
				Description:        "Deprecated provider-side NodeClass templates. Omit to leave provider-side templates unmanaged. Use the EKS module for reusable template composition.",
				Optional:           true,
				DeprecationMessage: commonschemas.ProviderSideTemplateDeprecationMessage,
				CustomType:         customfield.NewNestedObjectListType[api.EC2NodeClassTemplateModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: lo.Assign(map[string]schema.Attribute{
						"template_name": schema.StringAttribute{
							Description:        "Unique provider-side NodeClass template name.",
							Required:           true,
							DeprecationMessage: commonschemas.ProviderSideTemplateDeprecationMessage,
						},
					}, nodeClassTemplateSchema(ctx)),
				},
			},

			"nodeclasses": schema.ListNestedAttribute{
				Description: "EC2NodeClasses managed by Terraform. Omit to leave NodeClasses unmanaged; use an empty list to remove NodeClasses previously managed by this resource.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.EC2NodeClassModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: lo.Assign(map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "EC2NodeClass name. NodePools reference this value through `nodeclass`.",
							Required:    true,
						},

						"template_name": schema.StringAttribute{
							Description:        "Deprecated provider-side NodeClass template name to merge before applying this NodeClass.",
							Optional:           true,
							DeprecationMessage: commonschemas.ProviderSideTemplateDeprecationMessage,
						},

						"origin_nodeclass_json": schema.StringAttribute{
							Description: "Raw EC2NodeClass JSON. When configured, it replaces the generated NodeClass and all other typed fields on this object are ignored. The JSON must contain a valid EC2NodeClass object.",
							Optional:    true,
						},
					}, nodeClassTemplateSchema(ctx)),
				},
			},

			"nodepool_templates": schema.ListNestedAttribute{
				Description:        "Deprecated provider-side NodePool templates. Omit to leave provider-side templates unmanaged. Use the EKS module for reusable template composition.",
				Optional:           true,
				DeprecationMessage: commonschemas.ProviderSideTemplateDeprecationMessage,
				CustomType:         customfield.NewNestedObjectListType[api.EC2NodePoolTemplateModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: lo.Assign(map[string]schema.Attribute{
						"template_name": schema.StringAttribute{
							Description:        "Unique provider-side NodePool template name.",
							Required:           true,
							DeprecationMessage: commonschemas.ProviderSideTemplateDeprecationMessage,
						},
					}, nodePoolTemplateSchema(ctx)),
				},
			},

			"nodepools": schema.ListNestedAttribute{
				Description: "Karpenter NodePools managed by Terraform. Omit to leave NodePools unmanaged; use an empty list to remove NodePools previously managed by this resource.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.EC2NodePoolModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: lo.Assign(map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Karpenter NodePool name.",
							Required:    true,
						},

						"template_name": schema.StringAttribute{
							Description:        "Deprecated provider-side NodePool template name to merge before applying this NodePool.",
							Optional:           true,
							DeprecationMessage: commonschemas.ProviderSideTemplateDeprecationMessage,
						},

						"origin_nodepool_json": schema.StringAttribute{
							Description: "Raw Karpenter NodePool JSON. When configured, it replaces the generated NodePool and all other typed fields on this object are ignored. The JSON must contain a valid NodePool object.",
							Optional:    true,
						},
					}, nodePoolTemplateSchema(ctx)),
				},
			},
			"scheduled_rebalances": commonschemas.ScheduledRebalancesSchema(ctx),
		},
	}
}

func nodeClassTemplateSchema(ctx context.Context) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"role": schema.StringAttribute{
			Description: "IAM role name for the EC2 instances launched by this NodeClass. Defaults to `CloudPilotNodeRole-{cluster_name}` if not set.",
			Optional:    true,
		},
		"enable_image_accelerator": schema.BoolAttribute{
			Description: "Enable image accelerator (for example Spegel) for this nodeclass.",
			Optional:    true,
		},
		"enable_local_ssd_ephemeral_storage": schema.BoolAttribute{
			Description: "Use EC2 instance-store NVMe disks as kubelet ephemeral storage. When omitted, Terraform preserves the server setting.",
			Optional:    true,
		},
		"ami_alias": schema.StringAttribute{
			Description: "EKS optimized AMI alias, for example 'al2023@latest'. Maps to spec.amiSelectorTerms alias.",
			Optional:    true,
		},
		"user_data": schema.StringAttribute{
			Description: "NodeClass userData passed to Karpenter EC2NodeClass spec.userData.",
			Optional:    true,
		},
		"subnet_selector_terms": schema.ListNestedAttribute{
			Description: "Subnet selector terms (ORed). Each block uses non-empty `tags` or `id` (mutually exclusive). If omitted, defaults to one tag selector `{\"cluster.cloudpilot.ai/{cluster_name}\": \"true\"}`.",
			Optional:    true,
			CustomType:  customfield.NewNestedObjectListType[api.SubnetSelectorTermModel](ctx),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"tags": schema.MapAttribute{
						Description: "Tag key/value map to select subnets (AND within this block). Mutually exclusive with `id`.",
						Optional:    true,
						CustomType:  customfield.NewMapType[types.String](ctx),
						ElementType: types.StringType,
					},
					"id": schema.StringAttribute{
						Description: "EC2 subnet ID (for example `subnet-0123456789abcdef0`). Mutually exclusive with `tags`.",
						Optional:    true,
					},
				},
			},
		},
		"security_group_selector_terms": schema.ListNestedAttribute{
			Description: "Security group selector terms (ORed). Each block sets exactly one of non-empty `tags`, `id`, or `name`. If omitted, defaults to one tag selector `{\"cluster.cloudpilot.ai/{cluster_name}\": \"true\"}`.",
			Optional:    true,
			CustomType:  customfield.NewNestedObjectListType[api.SecurityGroupSelectorTermModel](ctx),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"tags": schema.MapAttribute{
						Description: "Tag key/value map to select security groups (AND within this block). Mutually exclusive with `id` and `name`.",
						Optional:    true,
						CustomType:  customfield.NewMapType[types.String](ctx),
						ElementType: types.StringType,
					},
					"id": schema.StringAttribute{
						Description: "EC2 security group ID (for example `sg-0123456789abcdef0`). Mutually exclusive with `tags` and `name`.",
						Optional:    true,
					},
					"name": schema.StringAttribute{
						Description: "Security group name (the EC2 name field, not the name tag). Mutually exclusive with `tags` and `id`.",
						Optional:    true,
					},
				},
			},
		},
		"instance_tags": schema.MapAttribute{
			Description: "Each provisioned EC2 instance will have the configured tags as key-value pairs. If omitted, CloudPilot keeps its default managed instance tag configuration.",
			Optional:    true,
			CustomType:  customfield.NewMapType[types.String](ctx),
			ElementType: types.StringType,
		},
		"system_disk_size_gib": schema.Int64Attribute{
			Description: "System disk size in GiB. Must be at least 1. Do not combine with `block_device_mappings` on the same NodeClass.",
			Optional:    true,
			Validators:  commonvalidators.Int64AtLeast(1),
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"block_device_mappings": schema.ListNestedAttribute{
			Description: "EC2 block device mappings. Use an empty list to clear inherited mappings; a non-empty list supports at most 50 mappings and at most one mapping with `root_volume = true`. Do not combine with `system_disk_size_gib` on the same NodeClass.",
			Optional:    true,
			Validators:  commonvalidators.ListSizeAtMost(50),
			CustomType:  customfield.NewNestedObjectListType[api.BlockDeviceMappingModel](ctx),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"device_name": schema.StringAttribute{
						Description: "Device name, for example /dev/xvda.",
						Optional:    true,
					},
					"root_volume": schema.BoolAttribute{
						Description: "Whether this mapping is the kubelet root volume.",
						Optional:    true,
					},
					"ebs": schema.SingleNestedAttribute{
						Description: "EBS settings for this block device.",
						Optional:    true,
						CustomType:  customfield.NewNestedObjectType[api.BlockDeviceModel](ctx),
						Attributes: map[string]schema.Attribute{
							"encrypted": schema.BoolAttribute{
								Description: "Whether the EBS volume is encrypted. When omitted, the EC2NodeClass or AWS default applies.",
								Optional:    true,
							},
							"volume_size": schema.StringAttribute{
								Description: "EBS volume size as a Kubernetes quantity using `Gi`, `G`, `Ti`, or `T`, for example `80Gi`. Required by the generated mapping because this provider does not expose `snapshot_id`.",
								Optional:    true,
							},
							"volume_type": schema.StringAttribute{
								Description: "EBS volume type. Allowed values: `standard`, `io1`, `io2`, `gp2`, `sc1`, `st1`, `gp3`.",
								Optional:    true,
								Validators:  commonvalidators.StringOneOf("standard", "io1", "io2", "gp2", "sc1", "st1", "gp3"),
							},
						},
					},
				},
			},
		},
		"extra_cpu_allocation_mcore": schema.Int64Attribute{
			Description: "Additional allocatable CPU in millicores reserved for burstable pods. Must be non-negative. When omitted, Terraform preserves the server value.",
			Optional:    true,
			Validators:  commonvalidators.Int64AtLeast(0),
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"extra_memory_allocation_mib": schema.Int64Attribute{
			Description: "Additional allocatable memory in MiB reserved for burstable pods. Must be non-negative. When omitted, Terraform preserves the server value.",
			Optional:    true,
			Validators:  commonvalidators.Int64AtLeast(0),
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
	}
}

func nodePoolTemplateSchema(ctx context.Context) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"enable": schema.BoolAttribute{
			Description: "Whether this NodePool is enabled for provisioning. When omitted, Terraform preserves the server value.",
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
		"enable_image_accelerator": schema.BoolAttribute{
			Description: "Enable image accelerator (for example Spegel) in this nodepool.",
			Optional:    true,
		},

		"provision_priority": schema.Int32Attribute{
			Description: "Karpenter NodePool weight from 1 to 100. A larger number gives this NodePool higher provisioning priority. Omit to use Karpenter's unweighted behavior.",
			Optional:    true,
			Validators:  commonvalidators.Int32Between(1, 100),
		},
		"instance_family": schema.ListAttribute{
			Description: "Allowed EC2 instance families, for example `[\"t3\", \"m5\"]`. When configured, this filter takes precedence over `instance_arch`.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"instance_arch": schema.ListAttribute{
			Description: "Allowed CPU architectures. Allowed values: `amd64`, `arm64`. Ignored when `instance_family` is configured.",
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
			Description: "AWS availability zones where nodes may be provisioned, for example `[\"us-west-2a\", \"us-west-2b\"]`.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"instance_cpu_min": schema.Int64Attribute{
			Description: "Minimum CPU cores per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.",
			Optional:    true,
			Validators:  commonvalidators.Int64AtLeast(0),
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"instance_cpu_max": schema.Int64Attribute{
			Description: "Maximum CPU cores per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.",
			Optional:    true,
			Validators:  commonvalidators.Int64AtLeast(0),
		},
		"instance_memory_min": schema.Int64Attribute{
			Description: "Minimum memory in MiB per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.",
			Optional:    true,
			Validators:  commonvalidators.Int64AtLeast(0),
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"instance_memory_max": schema.Int64Attribute{
			Description: "Maximum memory in MiB per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.",
			Optional:    true,
			Validators:  commonvalidators.Int64AtLeast(0),
		},
		"node_disruption_limit": schema.StringAttribute{
			Description:        "This specifies the maximum number of nodes that can be terminated at once, either as a fixed number (e.g., 2) or a percentage (e.g., 10%). Use node_disruption_budgets instead.",
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
	}
}
