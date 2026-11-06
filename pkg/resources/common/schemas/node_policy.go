package schemas

import (
	"context"
	"math"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	commonvalidators "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/resources/common/validators"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func DisruptionCooldownSchema() schema.StringAttribute {
	return schema.StringAttribute{Description: "Minimum delay between voluntary disruption batches. Omit to preserve the existing pool override; set 0s to disable the extra delay.", Optional: true, Validators: commonvalidators.StringMatches(`^([0-9]+(h|m|s))+$`, "Use a non-negative duration such as 0s or 30m.")}
}

func NodePoolDisruptionCooldownSchema() schema.StringAttribute {
	return schema.StringAttribute{Description: "Minimum delay between voluntary disruption batches. Omit or set null to preserve the existing override; set an empty string to inherit the cluster setting, or 0s to disable the extra delay.", Optional: true, Validators: commonvalidators.StringMatches(`^$|^([0-9]+(h|m|s))+$`, "Use an empty string to inherit, or a non-negative duration such as 0s or 30m.")}
}

func JobNodePoolOptimizationSchema(ctx context.Context) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Description: "Job workload packing and provisioning settings. Omit or set null to preserve the existing policy; set an empty object to clear it. In a nonempty object, null children preserve their current values.", Optional: true,
		CustomType: customfield.NewNestedObjectType[api.JobNodePoolOptimizationModel](ctx),
		Attributes: map[string]schema.Attribute{
			"min_active_nodes":   schema.Int32Attribute{Description: "Minimum preferred active nodes; must be positive.", Optional: true, Validators: commonvalidators.Int32Between(1, math.MaxInt32)},
			"provisioning_delay": schema.StringAttribute{Description: "Positive duration to delay provisioning while current nodes become available, for example 30s. Set an empty string to clear the delay.", Optional: true},
		},
	}
}

func NRISwapPolicySchema(ctx context.Context) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Description: "Automatic NRI swap policy for EKS 1.34 or later. Requires an enabled Memory Extension policy on the referenced NodeClass. Omit to preserve the current policy; set an empty object or enabled=false to disable it.", Optional: true,
		CustomType: customfield.NewNestedObjectType[api.NRISwapPolicyModel](ctx),
		Attributes: map[string]schema.Attribute{
			"enabled":                 schema.BoolAttribute{Description: "Enable automatic container swap allowances. Omit or set null to preserve the existing value.", Optional: true},
			"memory_limit_multiplier": schema.Float64Attribute{Description: "Memory limit multiplier, greater than 1 and at most 10, with at most three decimal places. Required when enabling a new policy.", Optional: true},
		},
	}
}

func MemoryExtensionSchema(ctx context.Context) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Description: "Cold memory and compressed cached memory policy for AL2023 EKS nodes. Omit to preserve the existing policy; set an empty object or enabled=false to disable it.", Optional: true,
		CustomType: customfield.NewNestedObjectType[api.MemoryExtensionModel](ctx),
		Attributes: map[string]schema.Attribute{
			"enabled":        schema.BoolAttribute{Description: "Enable Memory Extension on newly provisioned nodes. Omit or set null to preserve the existing value.", Optional: true},
			"reclaim_policy": schema.StringAttribute{Description: "Memory reclaim policy. Currently Conservative is supported. Set an empty string to restore the server default.", Optional: true, Validators: commonvalidators.StringOneOf("", "Conservative")},
			"cold_memory":    coldMemorySchema(ctx),
			"cached_memory": schema.SingleNestedAttribute{Description: "Compressed cached memory settings. Set an empty object to restore server defaults.", Optional: true, CustomType: customfield.NewNestedObjectType[api.CachedMemoryModel](ctx), Attributes: map[string]schema.Attribute{
				"max_memory_percent": schema.Int32Attribute{Description: "Maximum compressed cache as a percentage of memory, from 1 to 40.", Optional: true, Validators: commonvalidators.Int32Between(1, 40)},
			}},
		},
	}
}

func coldMemorySchema(ctx context.Context) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Description: "Disk-backed cold memory settings. Null children preserve existing values. Set an empty object to restore server defaults.", Optional: true,
		CustomType: customfield.NewNestedObjectType[api.ColdMemoryModel](ctx),
		Attributes: map[string]schema.Attribute{
			"sizing_mode":          schema.StringAttribute{Description: "Cold memory sizing mode. Currently Auto is supported. Set an empty string to restore the server default.", Optional: true, Validators: commonvalidators.StringOneOf("", "Auto")},
			"memory_ratio_percent": schema.Int32Attribute{Description: "Cold memory as a percentage of node memory, from 1 to 100.", Optional: true, Validators: commonvalidators.Int32Between(1, 100)},
			"min_gib":              schema.Int32Attribute{Description: "Minimum cold memory in GiB, from 1 to 64.", Optional: true, Validators: commonvalidators.Int32Between(1, 64)},
			"max_gib":              schema.Int32Attribute{Description: "Maximum cold memory in GiB, from 1 to 64 and at least min_gib.", Optional: true, Validators: commonvalidators.Int32Between(1, 64)},
			"max_disk_percent":     schema.Int32Attribute{Description: "Maximum disk percentage for cold memory, from 1 to 50.", Optional: true, Validators: commonvalidators.Int32Between(1, 50)},
		},
	}
}
