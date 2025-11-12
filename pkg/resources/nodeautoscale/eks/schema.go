package eks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
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

			"cluster_name": schema.StringAttribute{
				Description: "Name of the EKS cluster to be managed",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "AWS region where the EKS cluster is located",
				Required:    true,
			},

			"restore_node_number": schema.Int64Attribute{
				Description: "When restoring a cluster, set this to the desired number of nodes to restore.",
				Required:    true,
			},

			"cluster_id": schema.StringAttribute{
				Description: "Unique identifier of the EKS cluster (computed)",
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
					}, nodePoolTemplateSchema()),
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
					}, nodePoolTemplateSchema()),
				},
			},
		},
	}
}

func nodeClassTemplateSchema(ctx context.Context) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"instance_tags": schema.MapAttribute{
			Description: "Each provisioned node will have the configured tags as key-value pairs. Defaults to `node.cloudpilot.ai/managed=true` if not specified.",
			Optional:    true,
			CustomType:  customfield.NewMapType[types.String](ctx),
			ElementType: types.StringType,
		},
		"system_disk_size_gib": schema.Int64Attribute{
			Description: "Each provisioned node's system storage size, default to be 20 GiB.",
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(20),
		},
		"extra_cpu_allocation_mcore": schema.Int64Attribute{
			Description: "Each provisioned node will have extra CPU allocation, used only for burstable pods.",
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(0),
		},
		"extra_memory_allocation_mib": schema.Int64Attribute{
			Description: "Each provisioned node will have extra Memory allocation, used only for burstable pods.",
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(0),
		},
	}
}

func nodePoolTemplateSchema() map[string]schema.Attribute {
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
			Default:     int64default.StaticInt64(0),
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
			Default:     int64default.StaticInt64(0),
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
	}
}
