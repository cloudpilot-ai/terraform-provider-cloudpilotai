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
		Description: "EKS Cluster",
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
				Description:        "NodeClass templates configuration",
				Optional:           true,
				DeprecationMessage: commonschemas.ProviderSideTemplateDeprecationMessage,
				CustomType:         customfield.NewNestedObjectListType[api.EC2NodeClassTemplateModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: lo.Assign(map[string]schema.Attribute{
						"template_name": schema.StringAttribute{
							Description:        "NodeClass Template Name",
							Required:           true,
							DeprecationMessage: commonschemas.ProviderSideTemplateDeprecationMessage,
						},
					}, nodeClassTemplateSchema(ctx)),
				},
			},

			"nodeclasses": schema.ListNestedAttribute{
				Description: "NodeClasses configuration (no change if not set)",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.EC2NodeClassModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: lo.Assign(map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "NodeClass Name",
							Required:    true,
						},

						"template_name": schema.StringAttribute{
							Description:        "NodeVlass Template Name",
							Optional:           true,
							DeprecationMessage: commonschemas.ProviderSideTemplateDeprecationMessage,
						},

						"origin_nodeclass_json": schema.StringAttribute{
							Description: "The origin node class json, used to override the default configuration. If this field is configured, the other configuration items will be ignored.",
							Optional:    true,
						},
					}, nodeClassTemplateSchema(ctx)),
				},
			},

			"nodepool_templates": schema.ListNestedAttribute{
				Description:        "NodePools configuration (no change if not set)",
				Optional:           true,
				DeprecationMessage: commonschemas.ProviderSideTemplateDeprecationMessage,
				CustomType:         customfield.NewNestedObjectListType[api.EC2NodePoolTemplateModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: lo.Assign(map[string]schema.Attribute{
						"template_name": schema.StringAttribute{
							Description:        "NodePool Template Name",
							Required:           true,
							DeprecationMessage: commonschemas.ProviderSideTemplateDeprecationMessage,
						},
					}, nodePoolTemplateSchema(ctx)),
				},
			},

			"nodepools": schema.ListNestedAttribute{
				Description: "NodePools configuration (no change if not set)",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.EC2NodePoolModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: lo.Assign(map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Name",
							Required:    true,
						},

						"template_name": schema.StringAttribute{
							Description:        "NodePool Template Name",
							Optional:           true,
							DeprecationMessage: commonschemas.ProviderSideTemplateDeprecationMessage,
						},

						"origin_nodepool_json": schema.StringAttribute{
							Description: "The origin nodepool json, used to override the default configuration. If this field is configured, the other configuration items will be ignored.",
							Optional:    true,
						},
					}, nodePoolTemplateSchema(ctx)),
				},
			},
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
			Description: "Each provisioned node's system storage size. Do not combine with block_device_mappings on the same NodeClass.",
			Optional:    true,
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"block_device_mappings": schema.ListNestedAttribute{
			Description: "Full EC2 blockDeviceMappings list. Do not combine with system_disk_size_gib on the same NodeClass.",
			Optional:    true,
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
							"encrypted":   schema.BoolAttribute{Optional: true},
							"volume_size": schema.StringAttribute{Optional: true},
							"volume_type": schema.StringAttribute{Optional: true},
						},
					},
				},
			},
		},
		"extra_cpu_allocation_mcore": schema.Int64Attribute{
			Description: "Each provisioned node will have extra CPU allocation, used only for burstable pods.",
			Optional:    true,
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"extra_memory_allocation_mib": schema.Int64Attribute{
			Description: "Each provisioned node will have extra Memory allocation, used only for burstable pods.",
			Optional:    true,
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
	}
}

func nodePoolTemplateSchema(ctx context.Context) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"enable": schema.BoolAttribute{
			Description: "Enable",
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
			Description: "The priority level of this nodepool. A larger number means a higher priority.",
			Optional:    true,
		},
		"instance_family": schema.ListAttribute{
			Description: "The target instance family, like t3, m5 and so on, split by comma.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"instance_arch": schema.ListAttribute{
			Description: "The target instance architecture, if the instance family is configured, this field will be ignored.",
			Optional:    true,
			ElementType: types.StringType,
			Validators:  commonvalidators.ArchValidators(),
		},
		"capacity_type": schema.ListAttribute{
			Description: "The provisioned node's capacity type, on-demand or spot.",
			Optional:    true,
			ElementType: types.StringType,
			Validators:  commonvalidators.CapacityTypeValidators(),
		},
		"zone": schema.ListAttribute{
			Description: "Each provisioned node will located in the configured zone, formatted as us-west-1a,us-west-1b.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"instance_cpu_min": schema.Int64Attribute{
			Description: "Minimum CPU cores per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.",
			Optional:    true,
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"instance_cpu_max": schema.Int64Attribute{
			Description: "Maximum CPU cores per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.",
			Optional:    true,
		},
		"instance_memory_min": schema.Int64Attribute{
			Description: "Minimum memory in MiB per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.",
			Optional:    true,
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"instance_memory_max": schema.Int64Attribute{
			Description: "Maximum memory in MiB per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.",
			Optional:    true,
		},
		"node_disruption_limit": schema.StringAttribute{
			Description: "This specifies the maximum number of nodes that can be terminated at once, either as a fixed number (e.g., 2) or a percentage (e.g., 10%).",
			Optional:    true,
		},
		"node_disruption_delay": schema.StringAttribute{
			Description: "Specify the duration (e.g., 10s, 10m, or 10h) that the controller waits before terminating underutilized nodes.",
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
	}
}
