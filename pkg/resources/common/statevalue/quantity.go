package statevalue

import (
	"math"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"k8s.io/apimachinery/pkg/api/resource"
)

// PreserveQuantity retains the configured spelling when two known quantities
// have the same value. Callers retain responsibility for null and clear semantics.
func PreserveQuantity(configured, remote types.String) types.String {
	if configured.IsNull() || configured.IsUnknown() || remote.IsNull() || remote.IsUnknown() {
		return remote
	}
	configuredQuantity, configuredErr := resource.ParseQuantity(configured.ValueString())
	remoteQuantity, remoteErr := resource.ParseQuantity(remote.ValueString())
	if configuredErr == nil && remoteErr == nil && !saturatedBinaryQuantity(configuredQuantity) && !saturatedBinaryQuantity(remoteQuantity) && configuredQuantity.Cmp(remoteQuantity) == 0 {
		return configured
	}
	return remote
}

func saturatedBinaryQuantity(quantity resource.Quantity) bool {
	// ParseQuantity caps oversized BinarySI input; those spellings are not equivalent.
	return quantity.Format == resource.BinarySI && quantity.CmpInt64(math.MaxInt64) == 0
}
