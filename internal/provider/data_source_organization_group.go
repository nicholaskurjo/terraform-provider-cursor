package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type organizationGroupDataSource struct {
	client *apiClient
}

type organizationGroupDataSourceModel struct {
	ID                          types.String `tfsdk:"id"`
	Name                        types.String `tfsdk:"name"`
	MonthlySpendingLimitDollars types.Int64  `tfsdk:"monthly_spending_limit_dollars"`
	MemberCount                 types.Int64  `tfsdk:"member_count"`
}

func NewOrganizationGroupDataSource() datasource.DataSource {
	return &organizationGroupDataSource{}
}

func (d *organizationGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_group"
}

func (d *organizationGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up one Cursor Enterprise Organization Group by ID or exact name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Organization group ID with the g_ prefix. Set exactly one of id or name.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Exact organization group name. Set exactly one of id or name.",
			},
			"monthly_spending_limit_dollars": schema.Int64Attribute{
				Computed:    true,
				Description: "Monthly spending limit in whole dollars for each group member, or null when no override is set.",
			},
			"member_count": schema.Int64Attribute{
				Computed:    true,
				Description: "Current number of group members.",
			},
		},
	}
}

func (d *organizationGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *organizationGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config organizationGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil || d.client.organizationAdmin == nil {
		resp.Diagnostics.AddError("Organization API not configured", "Set organization_api_key or CURSOR_ORGANIZATION_API_KEY. Organization-group routes require members:* or admin:* scope.")
		return
	}

	idSet := !config.ID.IsNull() && !config.ID.IsUnknown() && strings.TrimSpace(config.ID.ValueString()) != ""
	nameSet := !config.Name.IsNull() && !config.Name.IsUnknown() && strings.TrimSpace(config.Name.ValueString()) != ""
	if idSet == nameSet {
		resp.Diagnostics.AddError("Invalid organization group lookup", "Set exactly one of id or name.")
		return
	}

	var group *organizationGroupAPIModel
	var err error
	if idSet {
		group, err = getOrganizationGroup(ctx, d.client.organizationAdmin, strings.TrimSpace(config.ID.ValueString()))
	} else {
		group, err = getOrganizationGroupByName(ctx, d.client.organizationAdmin, strings.TrimSpace(config.Name.ValueString()))
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read organization group", err.Error())
		return
	}
	if group == nil {
		lookup := config.ID.ValueString()
		if nameSet {
			lookup = config.Name.ValueString()
		}
		resp.Diagnostics.AddError("Organization group not found", fmt.Sprintf("No Cursor organization group matched %q.", lookup))
		return
	}

	state := organizationGroupDataSourceModel{
		ID:                          types.StringValue(group.ID),
		Name:                        types.StringValue(group.Name),
		MonthlySpendingLimitDollars: types.Int64Null(),
		MemberCount:                 types.Int64Value(group.MemberCount),
	}
	if group.MonthlySpendingLimitDollars != nil {
		state.MonthlySpendingLimitDollars = types.Int64Value(*group.MonthlySpendingLimitDollars)
	}
	if idSet && group.ID == strings.TrimSpace(config.ID.ValueString()) {
		state.ID = config.ID
	}
	if nameSet && group.Name == strings.TrimSpace(config.Name.ValueString()) {
		state.Name = config.Name
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
