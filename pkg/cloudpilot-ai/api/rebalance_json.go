package api

import (
	"encoding/json"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api/jsonmerge"
)

func (e *EC2NodeClass) UnmarshalJSON(data []byte) error {
	type alias EC2NodeClass
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*e = EC2NodeClass(decoded)
	e.rawJSON = append([]byte(nil), data...)
	return nil
}

func (e EC2NodeClass) MarshalJSON() ([]byte, error) {
	if e.originJSON {
		return append([]byte(nil), e.rawJSON...), nil
	}
	type alias EC2NodeClass
	return jsonmerge.MarshalPreservingUnknown(e.rawJSON, alias(e))
}

func (e *EC2NodePool) UnmarshalJSON(data []byte) error {
	type alias EC2NodePool
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*e = EC2NodePool(decoded)
	e.rawJSON = append([]byte(nil), data...)
	return nil
}

func (e EC2NodePool) MarshalJSON() ([]byte, error) {
	if e.originJSON {
		return append([]byte(nil), e.rawJSON...), nil
	}
	type alias EC2NodePool
	return jsonmerge.MarshalPreservingUnknown(e.rawJSON, alias(e))
}
