package workloadautoscaler

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func Schema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Description: "CloudPilot AI Workload Autoscaler",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Description: "The CloudPilot AI cluster ID to deploy Workload Autoscaler on.",
				Required:    true,
			},
			"kubeconfig": schema.StringAttribute{
				Description: "Path to the kubeconfig file for the target Kubernetes cluster.",
				Required:    true,
			},
			"storage_class": schema.StringAttribute{
				Description: "StorageClass name for VictoriaMetrics persistent volume. If empty, the cluster default StorageClass is used.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"enable_node_agent": schema.BoolAttribute{
				Description: "Whether to enable the Node Agent DaemonSet for per-node metrics collection.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},

			"recommendation_policies": schema.ListNestedAttribute{
				Description: "List of RecommendationPolicy resources to manage.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.RecommendationPolicyModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: recommendationPolicyAttributes(),
				},
			},

			"autoscaling_policies": schema.ListNestedAttribute{
				Description: "List of AutoscalingPolicy resources to manage.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.AutoscalingPolicyModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: autoscalingPolicyAttributes(ctx),
				},
			},

			"enable_proactive": schema.ListNestedAttribute{
				Description: "List of workload filters to enable proactive optimization. Each entry selects workloads by the specified filters and enables proactive update for them.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.EnableProactiveModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: proactiveFilterAttributes(),
				},
			},

			"disable_proactive": schema.ListNestedAttribute{
				Description: "List of workload filters to disable proactive optimization. Each entry selects workloads by the specified filters and disables proactive update for them.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.DisableProactiveModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: proactiveFilterAttributes(),
				},
			},
		},
	}
}

func recommendationPolicyAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name": schema.StringAttribute{
			Description: "RecommendationPolicy name.",
			Required:    true,
		},
		"strategy_type": schema.StringAttribute{
			Description: "Recommendation strategy type. Currently only 'percentile' is supported.",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("percentile"),
		},
		"percentile_cpu": schema.Int64Attribute{
			Description: "Target CPU percentile (50-100) when strategy_type is 'percentile'.",
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(95),
		},
		"percentile_memory": schema.Int64Attribute{
			Description: "Target Memory percentile (50-100) when strategy_type is 'percentile'.",
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(95),
		},
		"history_window_cpu": schema.StringAttribute{
			Description: "Duration of the CPU history window (e.g. '168h').",
			Required:    true,
		},
		"history_window_memory": schema.StringAttribute{
			Description: "Duration of the Memory history window (e.g. '168h').",
			Required:    true,
		},
		"evaluation_period": schema.StringAttribute{
			Description: "Duration of the evaluation period (e.g. '1h').",
			Required:    true,
		},
		"buffer_cpu": schema.StringAttribute{
			Description: "CPU buffer as a quantity or percent (e.g. '10%' or '100m').",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"buffer_memory": schema.StringAttribute{
			Description: "Memory buffer as a quantity or percent (e.g. '10%' or '128Mi').",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"request_min_cpu": schema.StringAttribute{
			Description: "Minimum CPU request recommendation (e.g. '10m').",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"request_min_memory": schema.StringAttribute{
			Description: "Minimum Memory request recommendation (e.g. '32Mi').",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"request_max_cpu": schema.StringAttribute{
			Description: "Maximum CPU request recommendation (e.g. '8').",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"request_max_memory": schema.StringAttribute{
			Description: "Maximum Memory request recommendation (e.g. '16Gi').",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
	}
}

func autoscalingPolicyAttributes(ctx context.Context) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name": schema.StringAttribute{
			Description: "AutoscalingPolicy name.",
			Required:    true,
		},
		"enable": schema.BoolAttribute{
			Description: "Whether this AutoscalingPolicy is enabled.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(true),
		},
		"recommendation_policy_name": schema.StringAttribute{
			Description: "Name of the RecommendationPolicy to use.",
			Required:    true,
		},
		"priority": schema.Int64Attribute{
			Description: "Priority level when multiple policies match the same workload. Higher values take precedence.",
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(0),
		},
		"update_resources": schema.ListAttribute{
			Description: "Resources to optimize, e.g. ['cpu', 'memory'].",
			Optional:    true,
			ElementType: types.StringType,
		},
		"drift_threshold_cpu": schema.StringAttribute{
			Description: "CPU drift threshold as a quantity or percent (e.g. '10%').",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"drift_threshold_memory": schema.StringAttribute{
			Description: "Memory drift threshold as a quantity or percent (e.g. '10%').",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"on_policy_removal": schema.StringAttribute{
			Description: "Behavior when the policy is removed: 'off', 'recreate', or 'inplace'.",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("off"),
		},

		"target_refs": schema.ListNestedAttribute{
			Description: "Target workload references.",
			Optional:    true,
			CustomType:  customfield.NewNestedObjectListType[api.TargetRefModel](ctx),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"api_version": schema.StringAttribute{
						Description: "API version (e.g. 'apps/v1').",
						Required:    true,
					},
					"kind": schema.StringAttribute{
						Description: "Workload kind: 'Deployment' or 'StatefulSet'.",
						Required:    true,
					},
					"name": schema.StringAttribute{
						Description: "Workload name. Leave empty to match all workloads of this kind.",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString(""),
					},
					"namespace": schema.StringAttribute{
						Description: "Namespace. Leave empty to match all namespaces.",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString(""),
					},
				},
			},
		},

		"update_schedules": schema.ListNestedAttribute{
			Description: "Update schedule items controlling when and how updates are applied.",
			Optional:    true,
			CustomType:  customfield.NewNestedObjectListType[api.UpdateScheduleModel](ctx),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Description: "Schedule name.",
						Required:    true,
					},
					"schedule": schema.StringAttribute{
						Description: "Cron expression for scheduling.",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString(""),
					},
					"duration": schema.StringAttribute{
						Description: "Duration for the schedule window (e.g. '1h').",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString(""),
					},
					"mode": schema.StringAttribute{
						Description: "Update mode: 'oncreate', 'recreate', 'inplace', or 'off'.",
						Required:    true,
					},
				},
			},
		},

		"limit_policies": schema.ListNestedAttribute{
			Description: "Per-resource limit policies.",
			Optional:    true,
			CustomType:  customfield.NewNestedObjectListType[api.LimitPolicyModel](ctx),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"resource": schema.StringAttribute{
						Description: "Resource name: 'cpu' or 'memory'.",
						Required:    true,
					},
					"remove_limit": schema.BoolAttribute{
						Description: "Remove the resource limit entirely.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
					},
					"keep_limit": schema.BoolAttribute{
						Description: "Keep the original resource limit.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
					},
					"multiplier": schema.StringAttribute{
						Description: "Multiplier for limit relative to request (e.g. '2.0').",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString(""),
					},
					"auto_headroom": schema.StringAttribute{
						Description: "Auto headroom multiplier (e.g. '1.5').",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString(""),
					},
				},
			},
		},

		"startup_boost_enabled": schema.BoolAttribute{
			Description: "Enable startup resource boost for newly created pods.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"startup_boost_min_boost_duration": schema.StringAttribute{
			Description: "Minimum duration for the startup boost (e.g. '5m').",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"startup_boost_min_ready_duration": schema.StringAttribute{
			Description: "Minimum ready duration before removing the boost (e.g. '3m').",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"startup_boost_multiplier_cpu": schema.StringAttribute{
			Description: "CPU multiplier during startup boost (e.g. '2.0').",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"startup_boost_multiplier_memory": schema.StringAttribute{
			Description: "Memory multiplier during startup boost (e.g. '2.0').",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"in_place_fallback_default_policy": schema.StringAttribute{
			Description: "Default fallback policy when in-place update fails: 'recreate' or 'hold'.",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
	}
}

func proactiveFilterAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"workload_name": schema.StringAttribute{
			Description: "Filter by workload name (substring match).",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"namespaces": schema.ListAttribute{
			Description: "Namespaces to filter workloads. Leave empty to match all namespaces.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"workload_kinds": schema.ListAttribute{
			Description: "Workload kinds to filter (e.g. 'Deployment', 'StatefulSet'). Leave empty to match all kinds.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"autoscaling_policy_names": schema.ListAttribute{
			Description: "Filter by autoscaling policy names.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"workload_state": schema.StringAttribute{
			Description: "Filter by workload state.",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"optimization_states": schema.ListAttribute{
			Description: "Filter by optimization states.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"disable_proactive_update": schema.BoolAttribute{
			Description: "Filter by whether proactive update is disabled on the workload.",
			Optional:    true,
		},
		"recommendation_policy_names": schema.ListAttribute{
			Description: "Filter by recommendation policy names.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"runtime_languages": schema.ListAttribute{
			Description: "Filter by container runtime languages.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"optimized": schema.BoolAttribute{
			Description: "Filter by whether the workload is optimized.",
			Optional:    true,
		},
	}
}
