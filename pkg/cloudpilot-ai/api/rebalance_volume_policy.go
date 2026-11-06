package api

import (
	"context"
	"fmt"

	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	gcpproviderv1alpha1 "github.com/cloudpilot-ai/lib/pkg/gcp/karpenter-provider-gcp/apis/v1alpha1"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
)

func volumePolicyValues(ctx context.Context, model customfield.NestedObject[VolumeSizePolicyModel], per, max int64, minimum, perMaximum int64) (int64, int64, error) {
	if model.IsNull() || model.IsUnknown() {
		return per, max, nil
	}
	value, diags := model.Value(ctx)
	if diags.HasError() {
		return 0, 0, fmt.Errorf("volume size policy: %v", diags)
	}
	if value == nil {
		return per, max, nil
	}
	if (value.PerVCPUGiB.IsNull() || value.PerVCPUGiB.IsUnknown()) &&
		(value.MaxSizeGiB.IsNull() || value.MaxSizeGiB.IsUnknown()) {
		return per, max, nil
	}
	if !value.PerVCPUGiB.IsNull() && !value.PerVCPUGiB.IsUnknown() {
		per = value.PerVCPUGiB.ValueInt64()
	}
	if !value.MaxSizeGiB.IsNull() && !value.MaxSizeGiB.IsUnknown() {
		max = value.MaxSizeGiB.ValueInt64()
	}
	if per < 0 || per > perMaximum {
		return 0, 0, fmt.Errorf("per_vcpu_gib must be between 0 and %d", perMaximum)
	}
	if value.MaxSizeGiB.IsUnknown() {
		return per, max, nil
	}
	if max < minimum || max > 65536 {
		return 0, 0, fmt.Errorf("max_size_gib must be between %d and 65536", minimum)
	}
	return per, max, nil
}

func awsVolumePolicyFromModel(ctx context.Context, model customfield.NestedObject[VolumeSizePolicyModel], current *awsproviderv1.VolumeSizePolicy) (*awsproviderv1.VolumeSizePolicy, error) {
	if model.IsNull() || model.IsUnknown() {
		return current, nil
	}
	if IsKnownEmptyObject(model) {
		return nil, nil
	}
	var per, max int64
	if current != nil {
		per, max = current.PerVCPUGiB, current.MaxSizeGiB
	}
	per, max, err := volumePolicyValues(ctx, model, per, max, 1, 16384)
	if err != nil {
		return nil, err
	}
	if max == 0 {
		return current, nil
	}
	return &awsproviderv1.VolumeSizePolicy{PerVCPUGiB: per, MaxSizeGiB: max}, nil
}

func gceVolumePolicyFromModel(ctx context.Context, model customfield.NestedObject[VolumeSizePolicyModel], current *gcpproviderv1alpha1.VolumeSizePolicy) (*gcpproviderv1alpha1.VolumeSizePolicy, error) {
	if model.IsNull() || model.IsUnknown() {
		return current, nil
	}
	if IsKnownEmptyObject(model) {
		return nil, nil
	}
	var per, max int64
	if current != nil {
		per, max = int64(current.PerVCPUGiB), int64(current.MaxSizeGiB)
	}
	per, max, err := volumePolicyValues(ctx, model, per, max, 10, 65536)
	if err != nil {
		return nil, err
	}
	if max == 0 {
		return current, nil
	}
	return &gcpproviderv1alpha1.VolumeSizePolicy{PerVCPUGiB: int32(per), MaxSizeGiB: int32(max)}, nil
}

func validateEC2VolumeSizePolicies(out []*awsproviderv1.BlockDeviceMapping) error {
	policyCount := 0
	for index, mapping := range out {
		if mapping == nil || mapping.EBS == nil || mapping.EBS.VolumeSizePolicy == nil {
			continue
		}
		policyCount++
		if !mapping.RootVolume && len(out) != 1 {
			return fmt.Errorf("block_device_mappings[%d].ebs.volume_size_policy requires root_volume = true when multiple mappings are configured", index)
		}
		ebs := mapping.EBS
		if ebs.VolumeSize == nil || ebs.VolumeSize.Sign() <= 0 {
			return fmt.Errorf("block_device_mappings[%d].ebs.volume_size is required and must be positive with volume_size_policy", index)
		}
		baseGiB := (ebs.VolumeSize.Value() + (1 << 30) - 1) / (1 << 30)
		limit := int64(16384)
		if ebs.VolumeType != nil {
			switch *ebs.VolumeType {
			case "standard":
				limit = 1024
			case "gp3", "io2":
				limit = 65536
			}
		}
		policy := ebs.VolumeSizePolicy
		if policy.PerVCPUGiB < 0 || policy.PerVCPUGiB > 16384 {
			return fmt.Errorf("block_device_mappings[%d].ebs.volume_size_policy.per_vcpu_gib must be between 0 and 16384", index)
		}
		if policy.MaxSizeGiB < baseGiB || policy.MaxSizeGiB > limit {
			return fmt.Errorf("block_device_mappings[%d].ebs.volume_size_policy.max_size_gib must be at least %d and at most %d for this volume type", index, baseGiB, limit)
		}
	}
	if policyCount > 1 {
		return fmt.Errorf("block_device_mappings supports at most one volume_size_policy")
	}
	return nil
}
