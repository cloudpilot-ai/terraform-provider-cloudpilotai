package eks

import (
	"context"
	"fmt"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
)

func validateMemoryExtensionReservations(ctx context.Context, data ClusterModel) error {
	if data.NodeClasses.IsNullOrUnknown() {
		return nil
	}
	templates := map[string]*api.EC2NodeClassTemplateModel{}
	if !data.NodeClassTemplates.IsNullOrUnknown() {
		values, diags := data.NodeClassTemplates.AsStructSliceT(ctx)
		if diags.HasError() {
			return fmt.Errorf("nodeclass_templates: %v", diags)
		}
		for _, value := range values {
			templates[value.TemplateName.ValueString()] = &value
		}
	}
	classes, diags := data.NodeClasses.AsStructSliceT(ctx)
	if diags.HasError() {
		return fmt.Errorf("nodeclasses: %v", diags)
	}
	for _, class := range classes {
		if class.OriginNodeClassJSON.IsUnknown() || (!class.OriginNodeClassJSON.IsNull() && class.OriginNodeClassJSON.ValueString() != "") {
			continue
		}
		if err := api.ValidateMemoryExtensionReservations(ctx, nil, templates[class.TemplateName.ValueString()], class.ToEC2NodeClassTemplateModel()); err != nil {
			return fmt.Errorf("nodeclass %q: %w", class.Name.ValueString(), err)
		}
	}
	return nil
}
