package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type userSpendLimitResource struct {
	client *apiClient
}

type userSpendLimitModel struct {
	Email                        types.String  `tfsdk:"email"`
	UserID                       types.String  `tfsdk:"user_id"`
	Name                         types.String  `tfsdk:"name"`
	SpendLimitDollars            types.Int64   `tfsdk:"spend_limit_dollars"`
	EffectivePerUserLimitDollars types.Int64   `tfsdk:"effective_per_user_limit_dollars"`
	SpendCents                   types.Float64 `tfsdk:"spend_cents"`
	OverallSpendCents            types.Float64 `tfsdk:"overall_spend_cents"`
}

var (
	_ resource.ResourceWithImportState    = (*userSpendLimitResource)(nil)
	_ resource.ResourceWithValidateConfig = (*userSpendLimitResource)(nil)
)

func NewUserSpendLimitResource() resource.Resource {
	return &userSpendLimitResource{}
}

func (r *userSpendLimitResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_spend_limit"
}

func (r *userSpendLimitResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Enterprise monthly spend limit for one Cursor team member.",
		Attributes: map[string]schema.Attribute{
			"email": schema.StringAttribute{
				Required:    true,
				Description: "Email address of an existing member of the team associated with the Team Admin API key.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_id": schema.StringAttribute{
				Computed:    true,
				Description: "Cursor public user ID.",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Display name of the team member.",
			},
			"spend_limit_dollars": schema.Int64Attribute{
				Required:    true,
				Description: "Monthly spending limit in whole dollars. Zero sets a $0 user limit; remove the resource to clear the override. Other applicable policies may affect the effective limit.",
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

func (r *userSpendLimitResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*apiClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *apiClient, got %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *userSpendLimitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userSpendLimitModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil || r.client.teamAdmin == nil {
		resp.Diagnostics.AddError("Team Admin API not configured", "Set team_api_key or CURSOR_TEAM_API_KEY.")
		return
	}

	limit := plan.SpendLimitDollars.ValueInt64()
	if err := setUserSpendLimit(ctx, r.client.teamAdmin, strings.TrimSpace(plan.Email.ValueString()), &limit); err != nil {
		resp.Diagnostics.AddError("Failed to set user spend limit", err.Error())
		return
	}
	r.read(ctx, plan, &resp.State, &resp.Diagnostics, true)
}

func (r *userSpendLimitResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userSpendLimitModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil || r.client.teamAdmin == nil {
		resp.Diagnostics.AddError("Team Admin API not configured", "Set team_api_key or CURSOR_TEAM_API_KEY.")
		return
	}
	r.read(ctx, state, &resp.State, &resp.Diagnostics, false)
}

func (r *userSpendLimitResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userSpendLimitModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil || r.client.teamAdmin == nil {
		resp.Diagnostics.AddError("Team Admin API not configured", "Set team_api_key or CURSOR_TEAM_API_KEY.")
		return
	}

	limit := plan.SpendLimitDollars.ValueInt64()
	if err := setUserSpendLimit(ctx, r.client.teamAdmin, strings.TrimSpace(plan.Email.ValueString()), &limit); err != nil {
		resp.Diagnostics.AddError("Failed to update user spend limit", err.Error())
		return
	}
	r.read(ctx, plan, &resp.State, &resp.Diagnostics, true)
}

func (r *userSpendLimitResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userSpendLimitModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil || r.client.teamAdmin == nil {
		resp.Diagnostics.AddError("Team Admin API not configured", "Set team_api_key or CURSOR_TEAM_API_KEY.")
		return
	}
	if err := setUserSpendLimit(ctx, r.client.teamAdmin, strings.TrimSpace(state.Email.ValueString()), nil); err != nil {
		resp.Diagnostics.AddError("Failed to clear user spend limit", err.Error())
		return
	}
	spend, err := getTeamMemberSpendByEmail(ctx, r.client.teamAdmin, strings.TrimSpace(state.Email.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to verify cleared user spend limit", err.Error())
		return
	}
	if spend != nil && spend.MonthlyLimitDollars != nil {
		resp.Diagnostics.AddError(
			"User spend limit could not be cleared",
			fmt.Sprintf("Cursor reported a successful clear, but the member still has a $%d monthly limit. The resource remains in state so the operation can be retried safely.", *spend.MonthlyLimitDollars),
		)
	}
}

func (r *userSpendLimitResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	email := strings.TrimSpace(req.ID)
	if email == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Import a user spend limit using the member email address.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("email"), email)...)
}

func (r *userSpendLimitResource) read(ctx context.Context, reference userSpendLimitModel, state *tfsdk.State, diagnostics *diag.Diagnostics, afterWrite bool) {
	// Keep the identity after a successful mutation even if the subsequent read
	// fails, so the next refresh can recover the existing override.
	if afterWrite {
		partial := userSpendLimitModel{Email: reference.Email, SpendLimitDollars: reference.SpendLimitDollars}
		diagnostics.Append(state.Set(ctx, &partial)...)
		if diagnostics.HasError() {
			return
		}
	}
	spend, err := getTeamMemberSpendByEmail(ctx, r.client.teamAdmin, strings.TrimSpace(reference.Email.ValueString()))
	if err != nil {
		diagnostics.AddError("Failed to read user spend limit", err.Error())
		return
	}
	if spend == nil || spend.MonthlyLimitDollars == nil {
		if afterWrite {
			diagnostics.AddError("User spend limit could not be verified", "Cursor accepted the update but did not return the user limit. The member email is retained in state; refresh before retrying.")
			return
		}
		state.RemoveResource(ctx)
		return
	}
	if afterWrite && !reference.SpendLimitDollars.IsNull() && !reference.SpendLimitDollars.IsUnknown() && *spend.MonthlyLimitDollars != reference.SpendLimitDollars.ValueInt64() {
		diagnostics.AddError(
			"User spend limit could not be verified",
			fmt.Sprintf("Cursor reported a successful update to $%d, but a follow-up read returned $%d. The member email and requested limit are retained in state; refresh before retrying.", reference.SpendLimitDollars.ValueInt64(), *spend.MonthlyLimitDollars),
		)
		return
	}

	updated := userSpendLimitModel{
		Email:                        reference.Email,
		UserID:                       types.StringValue(string(spend.UserID)),
		Name:                         types.StringValue(spend.Name),
		SpendLimitDollars:            types.Int64Value(*spend.MonthlyLimitDollars),
		EffectivePerUserLimitDollars: types.Int64Value(spend.EffectivePerUserLimitDollars),
		SpendCents:                   types.Float64Value(spend.SpendCents),
		OverallSpendCents:            types.Float64Value(spend.OverallSpendCents),
	}
	diagnostics.Append(state.Set(ctx, &updated)...)
}

func (r *userSpendLimitResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var email types.String
	var limit types.Int64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("email"), &email)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("spend_limit_dollars"), &limit)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !email.IsNull() && !email.IsUnknown() {
		if strings.TrimSpace(email.ValueString()) == "" {
			resp.Diagnostics.AddAttributeError(path.Root("email"), "Invalid member email", "email must not be empty.")
		} else if email.ValueString() != strings.TrimSpace(email.ValueString()) {
			resp.Diagnostics.AddAttributeError(path.Root("email"), "Invalid member email", "email must not contain leading or trailing whitespace.")
		}
	}
	if !limit.IsNull() && !limit.IsUnknown() && limit.ValueInt64() < 0 {
		resp.Diagnostics.AddAttributeError(path.Root("spend_limit_dollars"), "Invalid spend limit", "spend_limit_dollars must be zero or greater.")
	}
}
