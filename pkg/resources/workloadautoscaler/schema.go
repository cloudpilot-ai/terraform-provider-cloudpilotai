package workloadautoscaler

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	commonvalidators "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/validators"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func Schema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Description: "Installs and configures CloudPilot Workload Autoscaler, recommendation policies, autoscaling policies, and proactive-update filters for one registered cluster.",
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
				Description: "StorageClass name for the VictoriaMetrics persistent volume. An empty string uses the cluster default. Changing a managed value reinstalls the Workload Autoscaler components.",
				Optional:    true,
			},
			"enable_node_agent": schema.BoolAttribute{
				Description: "Whether to install the Node Agent DaemonSet for per-node metrics collection. When omitted during creation, the install script defaults to true. Changing a managed value reinstalls the Workload Autoscaler components.",
				Optional:    true,
			},
			"enable_new_workloads_proactive_update": schema.BoolAttribute{
				Description: "Enable proactive update automatically for new workloads once recommendations are ready.",
				Optional:    true,
			},
			"limiter_quota_per_window": schema.Int64Attribute{
				Description: "Maximum Workload Autoscaler operations permitted per limiter window. Must be greater than zero. When omitted, Terraform does not manage the server value.",
				Optional:    true,
				Validators:  commonvalidators.Int64AtLeast(1),
			},
			"limiter_burst": schema.Int64Attribute{
				Description: "Maximum burst size for the Workload Autoscaler limiter. Must be greater than zero. When omitted, Terraform does not manage the server value.",
				Optional:    true,
				Validators:  commonvalidators.Int64AtLeast(1),
			},
			"limiter_window_seconds": schema.Int64Attribute{
				Description: "Workload Autoscaler limiter window in seconds. Must be greater than zero. When omitted, Terraform does not manage the server value.",
				Optional:    true,
				Validators:  commonvalidators.Int64AtLeast(1),
			},
			"enable_preempted_pod_gc": schema.BoolAttribute{
				Description: "Enable garbage collection for preempted pods.",
				Optional:    true,
			},
			"preempted_pod_gc_ttl": schema.StringAttribute{
				Description: "Go-style duration before a preempted pod is garbage-collected, for example `30m` or `1h30m`. When omitted, Terraform does not manage the server value.",
				Optional:    true,
			},
			"enable_initial_optimization_data_window_check": schema.BoolAttribute{
				Description: "Require the initial optimization data window before enabling mutation and update paths for new workloads.",
				Optional:    true,
			},

			"recommendation_policies": schema.ListNestedAttribute{
				Description: "RecommendationPolicy resources managed by Terraform. Omit to leave policies unmanaged; use an empty list to delete policies previously managed by this resource when no managed AutoscalingPolicy still references them.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.RecommendationPolicyModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: recommendationPolicyAttributes(),
				},
			},

			"autoscaling_policies": schema.ListNestedAttribute{
				Description: "AutoscalingPolicy resources managed by Terraform. Omit to leave policies unmanaged; use an empty list to delete policies previously managed by this resource.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.AutoscalingPolicyModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: autoscalingPolicyAttributes(ctx),
				},
			},

			"enable_proactive": schema.ListNestedAttribute{
				Description: "Workload filters for enabling proactive optimization. Each filter is applied as an operation during create and update; it is not a persistent managed policy list.",
				Optional:    true,
				CustomType:  customfield.NewNestedObjectListType[api.EnableProactiveModel](ctx),
				NestedObject: schema.NestedAttributeObject{
					Attributes: proactiveFilterAttributes(),
				},
			},

			"disable_proactive": schema.ListNestedAttribute{
				Description: "Workload filters for disabling proactive optimization. Each filter is applied as an operation during create and update; it is not a persistent managed policy list.",
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
			Description: "Recommendation strategy. Allowed value: `percentile`. When omitted for a new policy, CloudPilot uses `percentile`.",
			Optional:    true,
			Validators:  commonvalidators.StringOneOf("percentile"),
		},
		"percentile_cpu": schema.Int32Attribute{
			Description: "Target CPU usage percentile. Allowed range: 50 to 100. Configure together with `percentile_memory`; new policies default both values to 95 when both are omitted.",
			Optional:    true,
			Validators:  commonvalidators.Int32Between(50, 100),
		},
		"percentile_memory": schema.Int32Attribute{
			Description: "Target memory usage percentile. Allowed range: 50 to 100. Configure together with `percentile_cpu`; new policies default both values to 95 when both are omitted.",
			Optional:    true,
			Validators:  commonvalidators.Int32Between(50, 100),
		},
		"history_window_cpu": schema.StringAttribute{
			Description: "Go-style duration of CPU history used for recommendations, for example `168h`.",
			Required:    true,
		},
		"history_window_memory": schema.StringAttribute{
			Description: "Go-style duration of memory history used for recommendations, for example `168h`.",
			Required:    true,
		},
		"evaluation_period": schema.StringAttribute{
			Description: "Go-style duration between recommendation evaluations, for example `1h`.",
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
		"jvm_min_heap_xms": schema.StringAttribute{
			Optional:    true,
			Description: "Minimum JVM heap size (Xms), for example 512Mi. When omitted, Terraform does not manage this setting.",
		},
		"jvm_min_heap_xms_ratio_of_memory": schema.StringAttribute{
			Description: "Minimum HeapXms ratio relative to the JVM memory recommendation. Use a numeric string in the range `[0, 1)`, for example `0.25`; `0` disables the ratio floor.",
			Optional:    true,
		},
		"jvm_recent_non_heap_window": schema.StringAttribute{
			Description: "Recent non-heap protection window, for example '2h'.",
			Optional:    true,
		},
		"jvm_heap_used_percentile": schema.Int32Attribute{
			Description: "JVM heap-used percentile. Allowed range: 20 to 100. When omitted, the controller defaults to 20.",
			Optional:    true,
			Validators:  commonvalidators.Int32Between(20, 100),
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
			Description: "Signed 32-bit priority used when multiple policies match the same workload. Higher values take precedence; ties use the oldest policy. New policies default to 0.",
			Optional:    true,
			Validators:  commonvalidators.Int64Between(-2147483648, 2147483647),
		},
		"disable_runtime_optimization": schema.BoolAttribute{
			Description: "Disable runtime-based optimization for workloads matched by this AutoscalingPolicy.",
			Optional:    true,
		},
		"update_resources": schema.ListAttribute{
			Description: "Resources to optimize. Allowed values: `cpu`, `memory`. Omit or set an empty list to use the CloudPilot default of both resources.",
			Optional:    true,
			ElementType: types.StringType,
			Validators:  commonvalidators.StringListOneOf("cpu", "memory"),
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
			Description: "How EVPA restores Pods toward baseline resources when the generated policy configuration is removed. Allowed values: `off`, `recreate`, `inplace`. New policies default to `off`.",
			Optional:    true,
			Validators:  commonvalidators.StringOneOf("off", "recreate", "inplace"),
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
						Description: "Supported workload kind. Allowed values: `Deployment`, `StatefulSet`, `DaemonSet`.",
						Required:    true,
						Validators:  commonvalidators.StringOneOf("Deployment", "StatefulSet", "DaemonSet"),
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
											Description: "Label selector operator. Allowed values: `In`, `NotIn`, `Exists`, `DoesNotExist`.",
											Required:    true,
											Validators:  commonvalidators.StringOneOf("In", "NotIn", "Exists", "DoesNotExist"),
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
						Description: "Standard cron expression for the start of the update window. If either `schedule` or `duration` is empty or omitted, the item is always active.",
						Optional:    true,
					},
					"duration": schema.StringAttribute{
						Description: "Go-style duration for the update window, for example `1h`. If either `schedule` or `duration` is empty or omitted, the item is always active.",
						Optional:    true,
					},
					"mode": schema.StringAttribute{
						Description: "Update mode active during this item. Allowed values: `oncreate`, `recreate`, `inplace`, `off`.",
						Required:    true,
						Validators:  commonvalidators.StringOneOf("oncreate", "recreate", "inplace", "off"),
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
						Description: "Resource governed by this limit policy. Allowed values: `cpu`, `memory`.",
						Required:    true,
						Validators:  commonvalidators.StringOneOf("cpu", "memory"),
					},
					"remove_limit": schema.BoolAttribute{
						Description: "Set to true to remove the resource limit. At most one of `remove_limit`, `keep_limit`, `multiplier`, or `auto_headroom` may be configured.",
						Optional:    true,
					},
					"keep_limit": schema.BoolAttribute{
						Description: "Set to true to keep the baseline resource limit unchanged. At most one of `remove_limit`, `keep_limit`, `multiplier`, or `auto_headroom` may be configured.",
						Optional:    true,
					},
					"multiplier": schema.StringAttribute{
						Description: "Numeric string multiplier applied to the recommended request to compute the limit. Allowed range: 1.0 to 5.0. At most one limit-policy action may be configured.",
						Optional:    true,
					},
					"auto_headroom": schema.StringAttribute{
						Description: "Numeric string headroom factor in the range 1.0 to 5.0. Existing limits are only increased, never decreased. At most one limit-policy action may be configured.",
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
			Description: "CPU startup multiplier as a numeric string from 1.0 to 5.0, for example `2.0`.",
			Optional:    true,
		},
		"startup_boost_multiplier_memory": schema.StringAttribute{
			Description: "Memory startup multiplier as a numeric string from 1.0 to 5.0, for example `2.0`.",
			Optional:    true,
		},
		"in_place_fallback_default_policy": schema.StringAttribute{
			Description: "Default action when in-place update cannot continue. Allowed values: `recreate`, `hold`. When omitted, CloudPilot defaults DaemonSet targets to `hold` and other supported targets to `recreate`.",
			Optional:    true,
			Validators:  commonvalidators.StringOneOf("recreate", "hold"),
		},
		"in_place_fallback_reason_policies": schema.MapAttribute{
			Description: "Fallback overrides keyed by failure reason. Allowed keys: `PodResizePending`, `QoSChangeForbidden`, `MemoryLimitsAddForbidden`, `ResourceLimitsRemoveForbidden`, `ResourceRequestsRemoveForbidden`, `ResourceMemoryLimitCannotBeDecreased`, `JVMHeapDrift`. Allowed values: `recreate`, `hold`.",
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
			Description: "Workload kinds to filter. Allowed values: `Deployment`, `StatefulSet`, `DaemonSet`. Leave empty or omit to match all supported kinds.",
			Optional:    true,
			ElementType: types.StringType,
			Validators:  commonvalidators.StringListOneOf("Deployment", "StatefulSet", "DaemonSet"),
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
