package provider_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"

	cloudpilotprovider "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/provider"
)

type providerWithRegistrations interface {
	DataSources(context.Context) []func() datasource.DataSource
	Resources(context.Context) []func() resource.Resource
}

func TestAllPublishedSchemaAttributesHaveDescriptions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	publishedProvider := cloudpilotprovider.NewProvider("test")()

	var metadataResponse frameworkprovider.MetadataResponse
	publishedProvider.Metadata(ctx, frameworkprovider.MetadataRequest{}, &metadataResponse)
	if metadataResponse.TypeName == "" {
		t.Fatal("provider metadata has no type name")
	}

	var providerSchemaResponse frameworkprovider.SchemaResponse
	publishedProvider.Schema(ctx, frameworkprovider.SchemaRequest{}, &providerSchemaResponse)
	if providerSchemaResponse.Diagnostics.HasError() {
		t.Fatalf("provider schema diagnostics: %v", providerSchemaResponse.Diagnostics)
	}
	providerSchema := providerSchemaResponse.Schema
	assertDescription(t, "provider", providerSchema.Description, providerSchema.MarkdownDescription)
	assertProviderAttributesDocumented(t, "provider", providerSchema.Attributes)

	registrations, ok := publishedProvider.(providerWithRegistrations)
	if !ok {
		t.Fatalf("provider %T does not expose resource and data source registrations", publishedProvider)
	}

	for _, factory := range registrations.Resources(ctx) {
		publishedResource := factory()
		var resourceMetadataResponse resource.MetadataResponse
		publishedResource.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: metadataResponse.TypeName}, &resourceMetadataResponse)
		name := "resource." + resourceMetadataResponse.TypeName
		if resourceMetadataResponse.TypeName == "" {
			t.Fatal("registered resource has no type name")
		}
		var resourceSchemaResponse resource.SchemaResponse
		publishedResource.Schema(ctx, resource.SchemaRequest{}, &resourceSchemaResponse)
		if resourceSchemaResponse.Diagnostics.HasError() {
			t.Fatalf("%s schema diagnostics: %v", name, resourceSchemaResponse.Diagnostics)
		}
		resourceSchema := resourceSchemaResponse.Schema
		assertDescription(t, name, resourceSchema.Description, resourceSchema.MarkdownDescription)
		assertResourceAttributesDocumented(t, name, resourceSchema.Attributes)
	}

	for _, factory := range registrations.DataSources(ctx) {
		publishedDataSource := factory()
		var dataSourceMetadataResponse datasource.MetadataResponse
		publishedDataSource.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: metadataResponse.TypeName}, &dataSourceMetadataResponse)
		name := "data_source." + dataSourceMetadataResponse.TypeName
		if dataSourceMetadataResponse.TypeName == "" {
			t.Fatal("registered data source has no type name")
		}
		var dataSourceSchemaResponse datasource.SchemaResponse
		publishedDataSource.Schema(ctx, datasource.SchemaRequest{}, &dataSourceSchemaResponse)
		if dataSourceSchemaResponse.Diagnostics.HasError() {
			t.Fatalf("%s schema diagnostics: %v", name, dataSourceSchemaResponse.Diagnostics)
		}
		datasourceSchema := dataSourceSchemaResponse.Schema
		assertDescription(t, name, datasourceSchema.Description, datasourceSchema.MarkdownDescription)
		assertDatasourceAttributesDocumented(t, name, datasourceSchema.Attributes)
	}
}

func assertProviderAttributesDocumented(t *testing.T, prefix string, attributes map[string]providerschema.Attribute) {
	t.Helper()
	for _, name := range sortedKeys(attributes) {
		attribute := attributes[name]
		path := prefix + "." + name
		assertDescription(t, path, attribute.GetDescription(), attribute.GetMarkdownDescription())
		switch nested := attribute.(type) {
		case providerschema.ListNestedAttribute:
			assertProviderAttributesDocumented(t, path, nested.NestedObject.Attributes)
		case providerschema.MapNestedAttribute:
			assertProviderAttributesDocumented(t, path, nested.NestedObject.Attributes)
		case providerschema.SetNestedAttribute:
			assertProviderAttributesDocumented(t, path, nested.NestedObject.Attributes)
		case providerschema.SingleNestedAttribute:
			assertProviderAttributesDocumented(t, path, nested.Attributes)
		}
	}
}

func assertResourceAttributesDocumented(t *testing.T, prefix string, attributes map[string]resourceschema.Attribute) {
	t.Helper()
	for _, name := range sortedKeys(attributes) {
		attribute := attributes[name]
		path := prefix + "." + name
		assertDescription(t, path, attribute.GetDescription(), attribute.GetMarkdownDescription())
		switch nested := attribute.(type) {
		case resourceschema.ListNestedAttribute:
			assertResourceAttributesDocumented(t, path, nested.NestedObject.Attributes)
		case resourceschema.MapNestedAttribute:
			assertResourceAttributesDocumented(t, path, nested.NestedObject.Attributes)
		case resourceschema.SetNestedAttribute:
			assertResourceAttributesDocumented(t, path, nested.NestedObject.Attributes)
		case resourceschema.SingleNestedAttribute:
			assertResourceAttributesDocumented(t, path, nested.Attributes)
		}
	}
}

func assertDatasourceAttributesDocumented(t *testing.T, prefix string, attributes map[string]datasourceschema.Attribute) {
	t.Helper()
	for _, name := range sortedKeys(attributes) {
		attribute := attributes[name]
		path := prefix + "." + name
		assertDescription(t, path, attribute.GetDescription(), attribute.GetMarkdownDescription())
		switch nested := attribute.(type) {
		case datasourceschema.ListNestedAttribute:
			assertDatasourceAttributesDocumented(t, path, nested.NestedObject.Attributes)
		case datasourceschema.MapNestedAttribute:
			assertDatasourceAttributesDocumented(t, path, nested.NestedObject.Attributes)
		case datasourceschema.SetNestedAttribute:
			assertDatasourceAttributesDocumented(t, path, nested.NestedObject.Attributes)
		case datasourceschema.SingleNestedAttribute:
			assertDatasourceAttributesDocumented(t, path, nested.Attributes)
		}
	}
}

func assertDescription(t *testing.T, path, description, markdownDescription string) {
	t.Helper()
	if strings.TrimSpace(description) == "" && strings.TrimSpace(markdownDescription) == "" {
		t.Errorf("%s has no description", path)
	}
}

func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
