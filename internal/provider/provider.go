package provider

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	envToken           = "CURSOR_TOKEN"
	envEndpoint        = "CURSOR_ENDPOINT"
	envTeamAPIKey      = "CURSOR_TEAM_API_KEY"
	envTeamAPIEndpoint = "CURSOR_TEAM_API_ENDPOINT"

	defaultEndpoint        = "https://api2.cursor.sh"
	defaultTeamAPIEndpoint = "https://api.cursor.com"
)

type cursorProvider struct {
	version string
}

type cursorProviderModel struct {
	Token           types.String `tfsdk:"token"`
	Endpoint        types.String `tfsdk:"endpoint"`
	TeamAPIKey      types.String `tfsdk:"team_api_key"`
	TeamAPIEndpoint types.String `tfsdk:"team_api_endpoint"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &cursorProvider{version: version}
	}
}

func (p *cursorProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "cursor"
	resp.Version = p.version
}

func (p *cursorProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Cursor Automations and Enterprise team administration settings.",
		Attributes: map[string]schema.Attribute{
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Auth token for the Cursor API. Can also be set via CURSOR_TOKEN.",
			},
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: fmt.Sprintf("Cursor API base URL. Defaults to %s. Can also be set via CURSOR_ENDPOINT.", defaultEndpoint),
			},
			"team_api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Team Admin API key used for per-user spend limits. Can also be set via CURSOR_TEAM_API_KEY.",
			},
			"team_api_endpoint": schema.StringAttribute{
				Optional:    true,
				Description: fmt.Sprintf("Cursor Team Admin API base URL. Defaults to %s. Can also be set via CURSOR_TEAM_API_ENDPOINT.", defaultTeamAPIEndpoint),
			},
		},
	}
}

func (p *cursorProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config cursorProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token := getStringValue(config.Token, envToken)
	teamAPIKey := getStringValue(config.TeamAPIKey, envTeamAPIKey)
	if token == "" && teamAPIKey == "" {
		resp.Diagnostics.AddError(
			"Missing Cursor API credentials",
			fmt.Sprintf("Set at least one of token or team_api_key (or %s or %s).", envToken, envTeamAPIKey),
		)
		return
	}

	endpoint := getStringValue(config.Endpoint, envEndpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	endpoint = strings.TrimRight(endpoint, "/")
	teamAPIEndpoint := getStringValue(config.TeamAPIEndpoint, envTeamAPIEndpoint)
	if teamAPIEndpoint == "" {
		teamAPIEndpoint = defaultTeamAPIEndpoint
	}
	teamAPIEndpoint = strings.TrimRight(teamAPIEndpoint, "/")

	client, err := newAPIClient(endpoint, token, teamAPIEndpoint, teamAPIKey, p.version)
	if err != nil {
		resp.Diagnostics.AddError("Failed to configure Cursor client", err.Error())
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *cursorProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPlatformWorkflowResource,
		NewUserSpendLimitResource,
	}
}

func (p *cursorProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewPlatformWorkflowDataSource,
	}
}

func getStringValue(value types.String, envKey string) string {
	if !value.IsNull() && !value.IsUnknown() {
		return strings.TrimSpace(value.ValueString())
	}
	return strings.TrimSpace(os.Getenv(envKey))
}
