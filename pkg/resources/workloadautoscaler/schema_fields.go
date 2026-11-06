package workloadautoscaler

import "github.com/hashicorp/terraform-plugin-framework/resource/schema"

func heapBoundAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{Description: description, Optional: true}
}
