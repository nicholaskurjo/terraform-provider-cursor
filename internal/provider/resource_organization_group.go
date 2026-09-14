package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type organizationGroupResource struct {
	client *apiClient
}

type organizationGroupResourceModel struct {
	ID                          types.String `tfsdk:"id"`
	Name                        types.String `tfsdk:"name"`
	MonthlySpendingLimitDollars types.Int64  `tfsdk:"monthly_spending_limit_dollars"`
	MemberCount                 types.Int64  `tfsdk:"member_count"`
}

var (
	_ resource.ResourceWithImportState    = (*organizationGroupResource)(nil)
	_ resource.ResourceWithValidateConfig = (*organizationGroupResource)(nil)
)

func NewOrganizationGroupResource() resource.Resource {
	return &organizationGroupResource{}
}

func (r *organizationGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_group"
}

func (r *organizationGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Cursor Enterprise Organization Group and its flat per-member monthly spending limit.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Organization group ID with the g_ prefix.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Unique organization group name.",
			},
			"monthly_spending_limit_dollars": schema.Int64Attribute{
				Optional:    true,
				Description: "Monthly spending limit in whole dollars for each group member. Omit to leave the group without a spending-limit override.",
			},
			"member_count": schema.Int64Attribute{
				Computed:    true,
				Description: "Current number of group members.",
			},
		},
	}
}

func (r *organizationGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *organizationGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan organizationGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.configured(&resp.Diagnostics) {
		return
	}

	group, err := createOrganizationGroup(ctx, r.client.organizationAdmin, strings.TrimSpace(plan.Name.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create organization group", err.Error())
		return
	}
	state := organizationGroupModelFromAPI(group)
	preserveEquivalentOrganizationGroupName(&state, plan.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() || plan.MonthlySpendingLimitDollars.IsNull() || plan.MonthlySpendingLimitDollars.IsUnknown() {
		return
	}

	limit := plan.MonthlySpendingLimitDollars.ValueInt64()
	groupID := group.ID
	group, err = updateOrganizationGroup(ctx, r.client.organizationAdmin, groupID, updateOrganizationGroupAPIRequest{
		MonthlySpendingLimitDollars: &limit,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Organization group created but spending limit failed",
			fmt.Sprintf("Cursor created group %s, but its spending limit could not be set: %s. The group ID is retained in state; run terraform apply again to retry.", groupID, err),
		)
		return
	}
	state = organizationGroupModelFromAPI(group)
	preserveEquivalentOrganizationGroupName(&state, plan.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *organizationGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state organizationGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.configured(&resp.Diagnostics) {
		return
	}

	group, err := getOrganizationGroup(ctx, r.client.organizationAdmin, state.ID.ValueString())
	if err != nil {
		if isRESTStatus(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read organization group", err.Error())
		return
	}
	updated := organizationGroupModelFromAPI(group)
	preserveEquivalentOrganizationGroupName(&updated, state.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &updated)...)
}

func (r *organizationGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan organizationGroupResourceModel
	var state organizationGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.configured(&resp.Diagnostics) {
		return
	}

	request := updateOrganizationGroupAPIRequest{}
	changed := false
	if !plan.Name.Equal(state.Name) {
		name := strings.TrimSpace(plan.Name.ValueString())
		request.Name = &name
		changed = true
	}
	if !plan.MonthlySpendingLimitDollars.Equal(state.MonthlySpendingLimitDollars) {
		changed = true
		if plan.MonthlySpendingLimitDollars.IsNull() {
			request.ClearMonthlySpendingLimitDollars = true
		} else {
			limit := plan.MonthlySpendingLimitDollars.ValueInt64()
			request.MonthlySpendingLimitDollars = &limit
		}
	}

	var group *organizationGroupAPIModel
	var err error
	if changed {
		group, err = updateOrganizationGroup(ctx, r.client.organizationAdmin, state.ID.ValueString(), request)
	} else {
		group, err = getOrganizationGroup(ctx, r.client.organizationAdmin, state.ID.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to update organization group", err.Error())
		return
	}
	updated := organizationGroupModelFromAPI(group)
	preserveEquivalentOrganizationGroupName(&updated, plan.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &updated)...)
}

func (r *organizationGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state organizationGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.configured(&resp.Diagnostics) {
		return
	}
	if err := deleteOrganizationGroup(ctx, r.client.organizationAdmin, state.ID.ValueString()); err != nil && !isRESTStatus(err, http.StatusNotFound) {
		resp.Diagnostics.AddError("Failed to delete organization group", err.Error())
	}
}

func (r *organizationGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *organizationGroupResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var name types.String
	var limit types.Int64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("name"), &name)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("monthly_spending_limit_dollars"), &limit)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !name.IsNull() && !name.IsUnknown() && strings.TrimSpace(name.ValueString()) == "" {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid organization group name", "name must not be empty.")
	}
	if !limit.IsNull() && !limit.IsUnknown() && (limit.ValueInt64() < 0 || limit.ValueInt64() > 2147483647) {
		resp.Diagnostics.AddAttributeError(path.Root("monthly_spending_limit_dollars"), "Invalid spending limit", "monthly_spending_limit_dollars must be between 0 and 2147483647.")
	}
}

// Cursor trims group names. Preserve equivalent configured whitespace to avoid
// an inconsistent result after apply, while still reporting actual name drift.
func preserveEquivalentOrganizationGroupName(state *organizationGroupResourceModel, reference types.String) {
	if reference.IsNull() || reference.IsUnknown() {
		return
	}
	if state.Name.ValueString() == strings.TrimSpace(reference.ValueString()) {
		state.Name = reference
	}
}

func (r *organizationGroupResource) configured(diagnostics *diag.Diagnostics) bool {
	if r.client != nil && r.client.organizationAdmin != nil {
		return true
	}
	diagnostics.AddError("Organization API not configured", "Set organization_api_key or CURSOR_ORGANIZATION_API_KEY. Organization-group routes require members:* or admin:* scope.")
	return false
}

func organizationGroupModelFromAPI(group *organizationGroupAPIModel) organizationGroupResourceModel {
	model := organizationGroupResourceModel{
		ID:                          types.StringValue(group.ID),
		Name:                        types.StringValue(group.Name),
		MonthlySpendingLimitDollars: types.Int64Null(),
		MemberCount:                 types.Int64Value(group.MemberCount),
	}
	if group.MonthlySpendingLimitDollars != nil {
		model.MonthlySpendingLimitDollars = types.Int64Value(*group.MonthlySpendingLimitDollars)
	}
	return model
}
