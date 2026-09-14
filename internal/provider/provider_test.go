package provider

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestProviderRegistersUserSpendLimitResource(t *testing.T) {
	p := New("test")().(*cursorProvider)

	var resourceNames []string
	for _, factory := range p.Resources(context.Background()) {
		response := resource.MetadataResponse{}
		factory().Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "cursor"}, &response)
		resourceNames = append(resourceNames, response.TypeName)
	}
	sort.Strings(resourceNames)
	wantResources := []string{
		"cursor_platform_workflow",
		"cursor_user_spend_limit",
	}
	if !reflect.DeepEqual(resourceNames, wantResources) {
		t.Fatalf("resources = %#v, want %#v", resourceNames, wantResources)
	}

	var dataSourceNames []string
	for _, factory := range p.DataSources(context.Background()) {
		response := datasource.MetadataResponse{}
		factory().Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "cursor"}, &response)
		dataSourceNames = append(dataSourceNames, response.TypeName)
	}
	sort.Strings(dataSourceNames)
	wantDataSources := []string{"cursor_platform_workflow"}
	if !reflect.DeepEqual(dataSourceNames, wantDataSources) {
		t.Fatalf("data sources = %#v, want %#v", dataSourceNames, wantDataSources)
	}
}

func TestProviderSchemaIncludesTeamAdminCredentials(t *testing.T) {
	p := New("test")().(*cursorProvider)
	response := provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, &response)

	for _, name := range []string{"token", "endpoint", "team_api_key", "team_api_endpoint"} {
		if _, ok := response.Schema.Attributes[name]; !ok {
			t.Errorf("provider schema is missing %q", name)
		}
	}
	for _, name := range []string{"team_api_key"} {
		attribute, ok := response.Schema.Attributes[name].(providerschema.StringAttribute)
		if !ok {
			t.Fatalf("provider attribute %q has type %T", name, response.Schema.Attributes[name])
		}
		if !attribute.Sensitive {
			t.Errorf("provider attribute %q should be sensitive", name)
		}
	}
}
