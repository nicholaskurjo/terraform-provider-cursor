package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type teamMemberDataSource struct {
	client *apiClient
}

type teamMemberDataSourceModel struct {
	Email                        types.String  `tfsdk:"email"`
	UserID                       types.String  `tfsdk:"user_id"`
	Name                         types.String  `tfsdk:"name"`
	Role                         types.String  `tfsdk:"role"`
	IsRemoved                    types.Bool    `tfsdk:"is_removed"`
	MonthlySpendingLimitDollars  types.Int64   `tfsdk:"monthly_spending_limit_dollars"`
	EffectivePerUserLimitDollars types.Int64   `tfsdk:"effective_per_user_limit_dollars"`
	SpendCents                   types.Float64 `tfsdk:"spend_cents"`
	OverallSpendCents            types.Float64 `tfsdk:"overall_spend_cents"`
}

func NewTeamMemberDataSource() datasource.DataSource {
	return &teamMemberDataSource{}
}

func (d *teamMemberDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_member"
}

func (d *teamMemberDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up one Cursor team member and their current spending-limit state by email.",
		Attributes: map[string]schema.Attribute{
			"email": schema.StringAttribute{
				Required:    true,
				Description: "Exact team-member email address.",
			},
			"user_id": schema.StringAttribute{
				Computed:    true,
				Description: "Cursor public user ID, suitable for organization-group memberships.",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Display name.",
			},
			"role": schema.StringAttribute{
				Computed:    true,
				Description: "Role within the team.",
			},
			"is_removed": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether Cursor marks the member as removed.",
			},
			"monthly_spending_limit_dollars": schema.Int64Attribute{
				Computed:    true,
				Description: "Explicit monthly user limit, or null when the member has no override.",
			},
			"effective_per_user_limit_dollars": schema.Int64Attribute{
				Computed:    true,
				Description: "Effective per-user limit after Cursor combines team, group, and user settings.",
			},
			"spend_cents": schema.Float64Attribute{
				Computed:    true,
				Description: "On-demand spend in cents for the current billing cycle.",
			},
			"overall_spend_cents": schema.Float64Attribute{
				Computed:    true,
				Description: "Total spend in cents for the current billing cycle, including included usage.",
			},
		},
	}
}

func (d *teamMemberDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*apiClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *apiClient, got %T", req.ProviderData))
		return
	}
	d.client = client
}

func (d *teamMemberDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config teamMemberDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil || d.client.teamAdmin == nil {
		resp.Diagnostics.AddError("Team Admin API not configured", "Set team_api_key or CURSOR_TEAM_API_KEY. A raw key_ or crsr_ token is also used as a fallback.")
		return
	}

	email := strings.TrimSpace(config.Email.ValueString())
	if email == "" {
		resp.Diagnostics.AddError("Invalid member email", "email must not be empty.")
		return
	}
	member, err := getTeamMemberByEmail(ctx, d.client.teamAdmin, email)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read team member", err.Error())
		return
	}
	if member == nil {
		resp.Diagnostics.AddError("Team member not found", fmt.Sprintf("No Cursor team member has email %q.", email))
		return
	}
	spend, err := getTeamMemberSpendByEmail(ctx, d.client.teamAdmin, email)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read team member spending", err.Error())
		return
	}

	state := teamMemberDataSourceModel{
		Email:                        config.Email,
		UserID:                       types.StringValue(string(member.ID)),
		Name:                         types.StringValue(member.Name),
		Role:                         types.StringValue(member.Role),
		IsRemoved:                    types.BoolValue(member.IsRemoved),
		MonthlySpendingLimitDollars:  types.Int64Null(),
		EffectivePerUserLimitDollars: types.Int64Null(),
		SpendCents:                   types.Float64Null(),
		OverallSpendCents:            types.Float64Null(),
	}
	if spend != nil {
		state.UserID = types.StringValue(string(spend.UserID))
		state.EffectivePerUserLimitDollars = types.Int64Value(spend.EffectivePerUserLimitDollars)
		state.SpendCents = types.Float64Value(spend.SpendCents)
		state.OverallSpendCents = types.Float64Value(spend.OverallSpendCents)
		if spend.MonthlyLimitDollars != nil {
			state.MonthlySpendingLimitDollars = types.Int64Value(*spend.MonthlyLimitDollars)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
