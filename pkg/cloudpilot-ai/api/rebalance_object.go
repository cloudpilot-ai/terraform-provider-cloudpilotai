package api

import customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"

// IsKnownEmptyObject reports an explicit empty object, including a typed object
// whose children are all known null. Unknown values defer changes instead.
func IsKnownEmptyObject[T any](value customfield.NestedObject[T]) bool {
	if value.IsNull() || value.IsUnknown() {
		return false
	}
	for _, child := range value.Attributes() {
		if child.IsUnknown() || !child.IsNull() {
			return false
		}
	}
	return true
}
