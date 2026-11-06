package api

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"time"

	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type JobNodePoolOptimization struct {
	MinActiveNodes    *int32  `json:"minActiveNodes,omitempty"`
	ProvisioningDelay *string `json:"provisioningDelay,omitempty"`
}

type JobNodePoolOptimizationModel struct {
	MinActiveNodes    types.Int32  `tfsdk:"min_active_nodes"`
	ProvisioningDelay types.String `tfsdk:"provisioning_delay"`
}

type NRISwapPolicy struct {
	Enabled               bool    `json:"enabled"`
	MemoryLimitMultiplier float64 `json:"memoryLimitMultiplier,omitempty"`
}

type NRISwapPolicyModel struct {
	Enabled               types.Bool    `tfsdk:"enabled"`
	MemoryLimitMultiplier types.Float64 `tfsdk:"memory_limit_multiplier"`
}

var cooldownPattern = regexp.MustCompile(`^([0-9]+(h|m|s))+$`)

func durationModel(in *metav1.Duration) types.String {
	if in == nil {
		return types.StringNull()
	}
	return types.StringValue(in.Duration.String())
}

func cooldownFromModel(value types.String, current *metav1.Duration) (*metav1.Duration, error) {
	if value.IsNull() || value.IsUnknown() {
		return current, nil
	}
	if value.ValueString() == "" {
		return nil, nil
	}
	duration, err := time.ParseDuration(value.ValueString())
	if err != nil || duration < 0 || !cooldownPattern.MatchString(value.ValueString()) {
		return nil, fmt.Errorf("disruption_cooldown must be a non-negative duration such as 0s or 30m")
	}
	return &metav1.Duration{Duration: duration}, nil
}

func jobOptimizationModelFromAPI(ctx context.Context, in *JobNodePoolOptimization) customfield.NestedObject[JobNodePoolOptimizationModel] {
	if in == nil {
		return customfield.NullObject[JobNodePoolOptimizationModel](ctx)
	}
	model := JobNodePoolOptimizationModel{MinActiveNodes: types.Int32Null(), ProvisioningDelay: types.StringNull()}
	if in.MinActiveNodes != nil {
		model.MinActiveNodes = types.Int32Value(*in.MinActiveNodes)
	}
	if in.ProvisioningDelay != nil {
		model.ProvisioningDelay = types.StringValue(*in.ProvisioningDelay)
	}
	return customfield.NewObjectMust(ctx, &model)
}

func jobOptimizationFromModel(ctx context.Context, model customfield.NestedObject[JobNodePoolOptimizationModel], current *JobNodePoolOptimization) (*JobNodePoolOptimization, error) {
	if model.IsNull() || model.IsUnknown() {
		return current, nil
	}
	if IsKnownEmptyObject(model) {
		return &JobNodePoolOptimization{}, nil
	}
	value, diags := model.Value(ctx)
	if diags.HasError() {
		return nil, fmt.Errorf("job_nodepool_optimization: %v", diags)
	}
	if value == nil {
		return current, nil
	}
	if (value.MinActiveNodes.IsNull() || value.MinActiveNodes.IsUnknown()) &&
		(value.ProvisioningDelay.IsNull() || value.ProvisioningDelay.IsUnknown()) {
		return current, nil
	}
	out := &JobNodePoolOptimization{}
	if current != nil {
		*out = *current
	}
	if !value.MinActiveNodes.IsNull() && !value.MinActiveNodes.IsUnknown() {
		n := value.MinActiveNodes.ValueInt32()
		if n <= 0 {
			return nil, fmt.Errorf("job_nodepool_optimization.min_active_nodes must be positive")
		}
		out.MinActiveNodes = &n
	}
	if !value.ProvisioningDelay.IsNull() && !value.ProvisioningDelay.IsUnknown() {
		delay := value.ProvisioningDelay.ValueString()
		out.ProvisioningDelay = nil
		if delay != "" {
			duration, err := time.ParseDuration(delay)
			if err != nil || duration <= 0 {
				return nil, fmt.Errorf("job_nodepool_optimization.provisioning_delay must be a positive duration or an empty string to clear it")
			}
			out.ProvisioningDelay = &delay
		}
	}
	return out, nil
}

func nriSwapModelFromAPI(ctx context.Context, in *NRISwapPolicy) customfield.NestedObject[NRISwapPolicyModel] {
	if in == nil {
		return customfield.NullObject[NRISwapPolicyModel](ctx)
	}
	return customfield.NewObjectMust(ctx, &NRISwapPolicyModel{Enabled: types.BoolValue(in.Enabled), MemoryLimitMultiplier: types.Float64Value(in.MemoryLimitMultiplier)})
}

func nriSwapFromModel(ctx context.Context, model customfield.NestedObject[NRISwapPolicyModel], current *NRISwapPolicy, overrides ...customfield.NestedObject[NRISwapPolicyModel]) (*NRISwapPolicy, error) {
	out := current
	managed := false
	for _, layer := range append([]customfield.NestedObject[NRISwapPolicyModel]{model}, overrides...) {
		if layer.IsNull() || layer.IsUnknown() {
			continue
		}
		if IsKnownEmptyObject(layer) {
			out = &NRISwapPolicy{Enabled: false}
			managed = true
			continue
		}
		value, diags := layer.Value(ctx)
		if diags.HasError() {
			return nil, fmt.Errorf("nri_swap_policy: %v", diags)
		}
		if value == nil || ((value.Enabled.IsNull() || value.Enabled.IsUnknown()) &&
			(value.MemoryLimitMultiplier.IsNull() || value.MemoryLimitMultiplier.IsUnknown())) {
			continue
		}
		next := NRISwapPolicy{}
		if out != nil {
			next = *out
		}
		if !value.Enabled.IsNull() && !value.Enabled.IsUnknown() {
			next.Enabled = value.Enabled.ValueBool()
		}
		if !value.MemoryLimitMultiplier.IsNull() && !value.MemoryLimitMultiplier.IsUnknown() {
			next.MemoryLimitMultiplier = value.MemoryLimitMultiplier.ValueFloat64()
		}
		out = &next
		managed = true
	}
	if !managed {
		return current, nil
	}
	ratio := out.MemoryLimitMultiplier
	if out.Enabled && (math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio <= 1 || ratio > 10 || ratio != math.Round(ratio*1000)/1000) {
		return nil, fmt.Errorf("nri_swap_policy.memory_limit_multiplier must be greater than 1 and at most 10, with at most 3 decimal places")
	}
	return out, nil
}

func applyEC2NodePoolPolicies(ctx context.Context, pool *EC2NodePool, model *EC2NodePoolTemplateModel) error {
	var err error
	pool.NodePoolSpec.Disruption.DisruptionCooldown, err = cooldownFromModel(model.DisruptionCooldown, pool.NodePoolSpec.Disruption.DisruptionCooldown)
	if err != nil {
		return err
	}
	pool.JobNodePoolOptimization, err = jobOptimizationFromModel(ctx, model.JobNodePoolOptimization, pool.JobNodePoolOptimization)
	if err != nil {
		return err
	}
	return nil
}

func applyGCENodePoolPolicies(ctx context.Context, pool *GCENodePool, model *GCENodePoolModel) error {
	var err error
	pool.NodePoolSpec.Disruption.DisruptionCooldown, err = cooldownFromModel(model.DisruptionCooldown, pool.NodePoolSpec.Disruption.DisruptionCooldown)
	if err != nil {
		return err
	}
	pool.JobNodePoolOptimization, err = jobOptimizationFromModel(ctx, model.JobNodePoolOptimization, pool.JobNodePoolOptimization)
	return err
}
