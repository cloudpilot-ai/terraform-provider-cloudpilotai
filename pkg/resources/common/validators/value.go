package validators

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

func StringOneOf(values ...string) []validator.String {
	return []validator.String{stringvalidator.OneOf(values...)}
}

func StringMatches(expression, message string) []validator.String {
	return []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(expression), message)}
}

func StringListOneOf(values ...string) []validator.List {
	return []validator.List{
		listvalidator.ValueStringsAre(stringvalidator.OneOf(values...)),
	}
}

func StringListMatches(expression, message string) []validator.List {
	return []validator.List{
		listvalidator.ValueStringsAre(stringvalidator.RegexMatches(regexp.MustCompile(expression), message)),
	}
}

func StringListOneOfWithSize(minSize, maxSize int, values ...string) []validator.List {
	return []validator.List{
		listvalidator.SizeBetween(minSize, maxSize),
		listvalidator.ValueStringsAre(stringvalidator.OneOf(values...)),
	}
}

func ListSizeBetween(minSize, maxSize int) []validator.List {
	return []validator.List{listvalidator.SizeBetween(minSize, maxSize)}
}

func ListSizeAtMost(maxSize int) []validator.List {
	return []validator.List{listvalidator.SizeAtMost(maxSize)}
}

func Int32Between(minValue, maxValue int32) []validator.Int32 {
	return []validator.Int32{int32BetweenValidator{minValue: minValue, maxValue: maxValue}}
}

func Int64AtLeast(minValue int64) []validator.Int64 {
	return []validator.Int64{int64AtLeastValidator{minValue: minValue}}
}

func Int64Between(minValue, maxValue int64) []validator.Int64 {
	return []validator.Int64{int64BetweenValidator{minValue: minValue, maxValue: maxValue}}
}

type int32BetweenValidator struct {
	minValue int32
	maxValue int32
}

func (v int32BetweenValidator) Description(ctx context.Context) string {
	return v.MarkdownDescription(ctx)
}

func (v int32BetweenValidator) MarkdownDescription(_ context.Context) string {
	return fmt.Sprintf("value must be between %d and %d, inclusive", v.minValue, v.maxValue)
}

func (v int32BetweenValidator) ValidateInt32(ctx context.Context, req validator.Int32Request, resp *validator.Int32Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if value := req.ConfigValue.ValueInt32(); value < v.minValue || value > v.maxValue {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid numeric value", v.Description(ctx))
	}
}

type int64AtLeastValidator struct {
	minValue int64
}

type int64BetweenValidator struct {
	minValue int64
	maxValue int64
}

func (v int64BetweenValidator) Description(ctx context.Context) string {
	return v.MarkdownDescription(ctx)
}

func (v int64BetweenValidator) MarkdownDescription(_ context.Context) string {
	return fmt.Sprintf("value must be between %d and %d, inclusive", v.minValue, v.maxValue)
}

func (v int64BetweenValidator) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if value := req.ConfigValue.ValueInt64(); value < v.minValue || value > v.maxValue {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid numeric value", v.Description(ctx))
	}
}

func (v int64AtLeastValidator) Description(ctx context.Context) string {
	return v.MarkdownDescription(ctx)
}

func (v int64AtLeastValidator) MarkdownDescription(_ context.Context) string {
	return fmt.Sprintf("value must be at least %d", v.minValue)
}

func (v int64AtLeastValidator) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueInt64() < v.minValue {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid numeric value", v.Description(ctx))
	}
}
