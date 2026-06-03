package eks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	commondefaults "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/defaults"
	commonschemas "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/schemas"
	commonvalidators "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/validators"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func Schema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Description: "EKS Cluster",
		Attributes: map[string]schema.Attribute{
			"kubeconfig": schema.StringAttribute{
				Description: "Kubernetes configuration file content for accessing the EKS cluster",
				Optional:    true,
				Computed:    true,
			},

			"aws_profile": schema.StringAttribute{
				Description: "AWS CLI named profile to use for all AWS operations (sts, eks). If empty, the default profile or environment credentials are used.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
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
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"restore_node_number": schema.Int64Attribute{
				Description: "Number of nodes to provision from the original node group when destroying the CloudPilot AI resource. Set to 0 (the default) to leave the cluster in its current optimized state without restoring original nodes. Set to a positive integer to restore that many nodes before uninstalling. Only effective when `skip_restore` is false.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},

			"cluster_id": schema.StringAttribute{
				Description: "Unique identifier of the EKS cluster. Optional override for existing clusters when the caller already knows the server-side cluster ID.",
				Optional:    true,
				Computed:    true,
			},
			"account_id": schema.StringAttribute{
				Description: "AWS account ID where the cluster is deployed (computed)",
				Computed:    true,
			},

			// agent configurations
			"disable_workload_uploading": schema.BoolAttribute{
				Description: "Disable automatic uploading of workload information to CloudPilot AI",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},

			"only_install_agent": schema.BoolAttribute{
				Description: "Only install the CloudPilot AI agent without additional configuration",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},

			// upgrade configurations
			"enable_upgrade_agent": schema.BoolAttribute{
				Description: "Enable upgrading the CloudPilot AI agent",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"enable_upgrade_rebalance_component": schema.BoolAttribute{
				Description: "Enable upgrading the CloudPilot AI rebalance component. Ignores `only_install_agent` if set to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},

			// rebalance configurations
			"enable_rebalance": schema.BoolAttribute{
				Description: "Enable automatic workload rebalancing across node pools. Ignores `only_install_agent` if set to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"enable_upload_config": schema.BoolAttribute{
				Description: "Enable uploading of nodepool and nodeclass configuration to CloudPilot AI",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"enable_diversity_instance_type": schema.BoolAttribute{
				Description: "Enable diverse instance types for improved fault tolerance and cost optimization",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},

			"custom_node_role": schema.StringAttribute{
				Description: "Custom IAM role name for EC2 instances. When set, this role will be added to the CloudPilot controller's PassNodeIAMRole policy during installation, allowing the controller to pass this role to EC2 instances.",
				Optional:    true,
			},

			"workload_templates": commonschemas.WorkloadTemplateSchema(ctx),
			"workloads":          commonschemas.WorkloadSchema(ctx),

			"nodeclass_templates": schema.ListNestedAttribute{
				Description: "NodeClass templates configuration",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.EC2NodeClassTemplateModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: lo.Assign(map[string]schema.Attribute{
						"template_name": schema.StringAttribute{
							Description: "NodeClass Template Name",
							Required:    true,
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
							Description: "NodeVlass Template Name",
							Optional:    true,
						},

						"origin_nodeclass_json": schema.StringAttribute{
							Description: "The origin node class json, used to override the default configuration. If this field is configured, the other configuration items will be ignored.",
							Optional:    true,
						},
					}, nodeClassTemplateSchema(ctx)),
				},
			},

			"nodepool_templates": schema.ListNestedAttribute{
				Description: "NodePools configuration (no change if not set)",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.EC2NodePoolTemplateModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: lo.Assign(map[string]schema.Attribute{
						"template_name": schema.StringAttribute{
							Description: "NodePool Template Name",
							Required:    true,
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
							Description: "NodePool Template Name",
							Optional:    true,
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
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"ami_alias": schema.StringAttribute{
			Description: "EKS optimized AMI alias, for example 'al2023@latest'. Maps to spec.amiSelectorTerms alias.",
			Optional:    true,
			Computed:    true,
		},
		"user_data": schema.StringAttribute{
			Description: "NodeClass userData passed to Karpenter EC2NodeClass spec.userData.",
			Optional:    true,
			Computed:    true,
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
			Computed:    true,
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
						Computed:    true,
						Default:     stringdefault.StaticString(""),
					},
					"root_volume": schema.BoolAttribute{
						Description: "Whether this mapping is the kubelet root volume.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
					},
					"ebs": schema.SingleNestedAttribute{
						Description: "EBS settings for this block device.",
						Optional:    true,
						CustomType:  customfield.NewNestedObjectType[api.BlockDeviceModel](ctx),
						Attributes: map[string]schema.Attribute{
							"delete_on_termination": schema.BoolAttribute{Optional: true},
							"encrypted":             schema.BoolAttribute{Optional: true},
							"iops":                  schema.Int64Attribute{Optional: true},
							"kms_key_id":            schema.StringAttribute{Optional: true},
							"snapshot_id":           schema.StringAttribute{Optional: true},
							"throughput":            schema.Int64Attribute{Optional: true},
							"volume_size":           schema.StringAttribute{Optional: true},
							"volume_type":           schema.StringAttribute{Optional: true},
						},
					},
				},
			},
		},
		"extra_cpu_allocation_mcore": schema.Int64Attribute{
			Description: "Each provisioned node will have extra CPU allocation, used only for burstable pods.",
			Optional:    true,
			Computed:    true,
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"extra_memory_allocation_mib": schema.Int64Attribute{
			Description: "Each provisioned node will have extra Memory allocation, used only for burstable pods.",
			Optional:    true,
			Computed:    true,
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
			Computed:    true,
			Default:     booldefault.StaticBool(true),
		},
		"nodeclass": schema.StringAttribute{
			Description: "Select the nodeclass to use for this nodepool.",
			Optional:    true,
			Computed:    true,
		},

		"enable_gpu": schema.BoolAttribute{
			Description: "Enable GPU instances in this nodepool.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"enable_image_accelerator": schema.BoolAttribute{
			Description: "Enable image accelerator (for example Spegel) in this nodepool.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},

		"provision_priority": schema.Int32Attribute{
			Description: "The priority level of this nodepool. A larger number means a higher priority.",
			Optional:    true,
			Computed:    true,
			Default:     int32default.StaticInt32(1),
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
			Computed:    true,
			Default:     commondefaults.ArchDefault(),
			Validators:  commonvalidators.ArchValidators(),
		},
		"capacity_type": schema.ListAttribute{
			Description: "The provisioned node's capacity type, on-demand or spot.",
			Optional:    true,
			ElementType: types.StringType,
			Computed:    true,
			Default:     commondefaults.CapacityTypeDefault(),
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
			Computed:    true,
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"instance_cpu_max": schema.Int64Attribute{
			Description: "Maximum CPU cores per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.",
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(17),
		},
		"instance_memory_min": schema.Int64Attribute{
			Description: "Minimum memory in MiB per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.",
			Optional:    true,
			Computed:    true,
			PlanModifiers: []planmodifier.Int64{
				useStateForUnknownInt64(),
			},
		},
		"instance_memory_max": schema.Int64Attribute{
			Description: "Maximum memory in MiB per node. Used to filter instance types during node provisioning. Set to 0 for unlimited.",
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(32769),
		},
		"node_disruption_limit": schema.StringAttribute{
			Description: "This specifies the maximum number of nodes that can be terminated at once, either as a fixed number (e.g., 2) or a percentage (e.g., 10%).",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("2"),
		},
		"node_disruption_delay": schema.StringAttribute{
			Description: "Specify the duration (e.g., 10s, 10m, or 10h) that the controller waits before terminating underutilized nodes.",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("60m"),
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
						Computed:    true,
						Default:     stringdefault.StaticString(""),
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
