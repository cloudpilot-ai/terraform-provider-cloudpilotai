package workloadautoscaler

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
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
				PlanModifiers: []planmodifier.String{
					useStateForUnknownString(),
				},
			},
			"kubeconfig": schema.StringAttribute{
				Description: "Optional path to a kubeconfig file for the target Kubernetes cluster. If not set, the provider generates an execution-local kubeconfig for supported EKS and GKE clusters without storing its path in Terraform state.",
				Optional:    true,
			},
			"aws_profile": schema.StringAttribute{
				Description: "Optional AWS CLI named profile used when an execution-local EKS kubeconfig must be generated.",
				Optional:    true,
			},
			"aws_assume_role": schema.SingleNestedAttribute{
				Description: "Optional IAM role to assume when an execution-local EKS kubeconfig must be generated.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectType[AWSAssumeRoleModel](ctx),
				Attributes: map[string]schema.Attribute{
					"role_arn": schema.StringAttribute{
						Description: "IAM role ARN to assume for AWS CLI and EKS kubeconfig operations.",
						Required:    true,
					},
					"session_name": schema.StringAttribute{
						Description: "Optional STS session name. Defaults to cloudpilotai-terraform when omitted.",
						Optional:    true,
					},
				},
			},
			"gcp_project_id": schema.StringAttribute{
				Description: "Optional GCP project ID used when an execution-local GKE kubeconfig must be generated.",
				Optional:    true,
			},
			"gcp_cluster_location": schema.StringAttribute{
				Description: "Optional GKE region or zone used when an execution-local kubeconfig must be generated.",
				Optional:    true,
			},
			"storage_class": schema.StringAttribute{
				Description: "StorageClass name for VictoriaMetrics persistent volume. If empty, the cluster default StorageClass is used.",
				Optional:    true,
			},
			"enable_node_agent": schema.BoolAttribute{
				Description: "Whether to enable the Node Agent DaemonSet for per-node metrics collection.",
				Optional:    true,
			},
			"enable_new_workloads_proactive_update": schema.BoolAttribute{
				Description: "Enable proactive update automatically for new workloads once recommendations are ready.",
				Optional:    true,
			},
			"limiter_quota_per_window": schema.Int64Attribute{
				Description: "Workload Autoscaler rate-limit quota per limiter window. Server requires a positive value.",
				Optional:    true,
			},
			"limiter_burst": schema.Int64Attribute{
				Description: "Workload Autoscaler rate-limit burst. Server requires a positive value.",
				Optional:    true,
			},
			"limiter_window_seconds": schema.Int64Attribute{
				Description: "Workload Autoscaler limiter window in seconds. Server requires a positive value.",
				Optional:    true,
			},
			"enable_preempted_pod_gc": schema.BoolAttribute{
				Description: "Enable garbage collection for preempted pods.",
				Optional:    true,
			},
			"preempted_pod_gc_ttl": schema.StringAttribute{
				Description: "TTL for preempted pod garbage collection, for example '30m'.",
				Optional:    true,
			},
			"enable_initial_optimization_data_window_check": schema.BoolAttribute{
				Description: "Require the initial optimization data window before enabling mutation and update paths for new workloads.",
				Optional:    true,
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
		},
		"percentile_cpu": schema.Int32Attribute{
			Description: "Target CPU percentile (50-100) when strategy_type is 'percentile'.",
			Optional:    true,
		},
		"percentile_memory": schema.Int32Attribute{
			Description: "Target Memory percentile (50-100) when strategy_type is 'percentile'.",
			Optional:    true,
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
		},
		"buffer_memory": schema.StringAttribute{
			Description: "Memory buffer as a quantity or percent (e.g. '10%' or '128Mi').",
			Optional:    true,
		},
		"request_min_cpu": schema.StringAttribute{
			Description: "Minimum CPU request recommendation (e.g. '10m').",
			Optional:    true,
		},
		"request_min_memory": schema.StringAttribute{
			Description: "Minimum Memory request recommendation (e.g. '32Mi').",
			Optional:    true,
		},
		"request_max_cpu": schema.StringAttribute{
			Description: "Maximum CPU request recommendation (e.g. '8').",
			Optional:    true,
		},
		"request_max_memory": schema.StringAttribute{
			Description: "Maximum Memory request recommendation (e.g. '16Gi').",
			Optional:    true,
		},
		"jvm_heap_buffer": schema.StringAttribute{
			Description: "JVM heap buffer for HeapXmx, for example '25%' or '300Mi'.",
			Optional:    true,
		},
		"jvm_min_heap_xms_ratio_of_memory": schema.StringAttribute{
			Description: "Minimum ratio of HeapXms to JVM memory recommendation, for example '0.25'.",
			Optional:    true,
		},
		"jvm_recent_non_heap_window": schema.StringAttribute{
			Description: "Recent non-heap protection window, for example '2h'.",
			Optional:    true,
		},
		"jvm_heap_used_percentile": schema.Int32Attribute{
			Description: "JVM heap-used percentile, valid server range is 20 to 100.",
			Optional:    true,
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
		},
		"recommendation_policy_name": schema.StringAttribute{
			Description: "Name of the RecommendationPolicy to use.",
			Required:    true,
		},
		"priority": schema.Int64Attribute{
			Description: "Priority level when multiple policies match the same workload. Higher values take precedence.",
			Optional:    true,
		},
		"disable_runtime_optimization": schema.BoolAttribute{
			Description: "Disable runtime-based optimization for workloads matched by this AutoscalingPolicy.",
			Optional:    true,
		},
		"update_resources": schema.ListAttribute{
			Description: "Resources to optimize, e.g. ['cpu', 'memory'].",
			Optional:    true,
			ElementType: types.StringType,
		},
		"drift_threshold_cpu": schema.StringAttribute{
			Description: "CPU drift threshold as a quantity or percent (e.g. '10%').",
			Optional:    true,
		},
		"drift_threshold_memory": schema.StringAttribute{
			Description: "Memory drift threshold as a quantity or percent (e.g. '10%').",
			Optional:    true,
		},
		"on_policy_removal": schema.StringAttribute{
			Description: "Behavior when the policy is removed: 'off', 'recreate', or 'inplace'.",
			Optional:    true,
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
					},
					"namespace": schema.StringAttribute{
						Description: "Namespace. Leave empty to match all namespaces.",
						Optional:    true,
					},
					"label_selector": schema.SingleNestedAttribute{
						Description: "Kubernetes label selector for matching workloads.",
						Optional:    true,
						CustomType:  customfield.NewNestedObjectType[api.LabelSelectorModel](ctx),
						Attributes: map[string]schema.Attribute{
							"match_labels": schema.MapAttribute{
								Description: "Label key/value pairs that selected workloads must match.",
								Optional:    true,
								ElementType: types.StringType,
								CustomType:  customfield.NewMapType[types.String](ctx),
							},
							"match_expressions": schema.ListNestedAttribute{
								Description: "Label selector match expressions.",
								Optional:    true,
								CustomType:  customfield.NewNestedObjectListType[api.LabelSelectorRequirementModel](ctx),
								NestedObject: schema.NestedAttributeObject{
									Attributes: map[string]schema.Attribute{
										"key": schema.StringAttribute{
											Description: "Label key.",
											Required:    true,
										},
										"operator": schema.StringAttribute{
											Description: "Selector operator, for example 'In', 'NotIn', 'Exists', or 'DoesNotExist'.",
											Required:    true,
										},
										"values": schema.ListAttribute{
											Description: "Selector values used by the operator.",
											Optional:    true,
											ElementType: types.StringType,
										},
									},
								},
							},
						},
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
					},
					"duration": schema.StringAttribute{
						Description: "Duration for the schedule window (e.g. '1h').",
						Optional:    true,
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
					},
					"keep_limit": schema.BoolAttribute{
						Description: "Keep the original resource limit.",
						Optional:    true,
					},
					"multiplier": schema.StringAttribute{
						Description: "Multiplier for limit relative to request (e.g. '2.0').",
						Optional:    true,
					},
					"auto_headroom": schema.StringAttribute{
						Description: "Auto headroom multiplier (e.g. '1.5').",
						Optional:    true,
					},
				},
			},
		},

		"startup_boost_enabled": schema.BoolAttribute{
			Description: "Enable startup resource boost for newly created pods.",
			Optional:    true,
		},
		"startup_boost_min_boost_duration": schema.StringAttribute{
			Description: "Minimum duration for the startup boost (e.g. '5m').",
			Optional:    true,
		},
		"startup_boost_min_ready_duration": schema.StringAttribute{
			Description: "Minimum ready duration before removing the boost (e.g. '3m').",
			Optional:    true,
		},
		"startup_boost_multiplier_cpu": schema.StringAttribute{
			Description: "CPU multiplier during startup boost (e.g. '2.0').",
			Optional:    true,
		},
		"startup_boost_multiplier_memory": schema.StringAttribute{
			Description: "Memory multiplier during startup boost (e.g. '2.0').",
			Optional:    true,
		},
		"in_place_fallback_default_policy": schema.StringAttribute{
			Description: "Default fallback policy when in-place update fails: 'recreate' or 'hold'.",
			Optional:    true,
		},
		"in_place_fallback_reason_policies": schema.MapAttribute{
			Description: "Fallback policy overrides keyed by in-place failure reason.",
			Optional:    true,
			ElementType: types.StringType,
			CustomType:  customfield.NewMapType[types.String](ctx),
		},
	}
}

func proactiveFilterAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"workload_name": schema.StringAttribute{
			Description: "Filter by workload name (substring match).",
			Optional:    true,
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
