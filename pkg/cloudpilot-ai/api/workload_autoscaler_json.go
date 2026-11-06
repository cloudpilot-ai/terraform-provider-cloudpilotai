package api

import (
	"encoding/json"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api/jsonmerge"
)

func (r *RecommendationPolicyResource) UnmarshalJSON(data []byte) error {
	type plain RecommendationPolicyResource
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = RecommendationPolicyResource(decoded)
	r.originalJSON = append(json.RawMessage(nil), data...)
	return nil
}

func (r RecommendationPolicyResource) MarshalJSON() ([]byte, error) {
	type plain RecommendationPolicyResource
	return jsonmerge.MarshalPreservingUnknown(r.originalJSON, plain(r))
}

func (r *AutoscalingPolicyResource) UnmarshalJSON(data []byte) error {
	type plain AutoscalingPolicyResource
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = AutoscalingPolicyResource(decoded)
	r.originalJSON = append(json.RawMessage(nil), data...)
	return nil
}

func (r AutoscalingPolicyResource) MarshalJSON() ([]byte, error) {
	type plain AutoscalingPolicyResource
	return jsonmerge.MarshalPreservingUnknown(r.originalJSON, plain(r))
}

func (s AutoscalingPolicySpec) MarshalJSON() ([]byte, error) {
	type plain AutoscalingPolicySpec
	if s.UpdateResources == nil || len(s.UpdateResources) > 0 {
		return json.Marshal(plain(s))
	}
	// An explicit empty scope disables resource management. Omitting it would
	// restore the server default of CPU and memory.
	return json.Marshal(struct {
		plain
		UpdateResources []string `json:"updateResources"`
	}{plain(s), s.UpdateResources})
}

func (s WorkloadStartupResourceBoost) MarshalJSON() ([]byte, error) {
	type plain WorkloadStartupResourceBoost
	if s.UpdateResources == nil || len(s.UpdateResources) > 0 {
		return json.Marshal(plain(s))
	}
	// Preserve the distinction so unsupported empty startup scopes cannot
	// silently become the default scope during encoding.
	return json.Marshal(struct {
		plain
		UpdateResources []string `json:"updateResources"`
	}{plain(s), s.UpdateResources})
}
