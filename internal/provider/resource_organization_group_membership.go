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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type organizationGroupMembershipResource struct {
	client *apiClient
}

type organizationGroupMembershipModel struct {
	ID      types.String `tfsdk:"id"`
	GroupID types.String `tfsdk:"group_id"`
	UserID  types.String `tfsdk:"user_id"`
	Name    types.String `tfsdk:"name"`
	Email   types.String `tfsdk:"email"`
}

var _ resource.ResourceWithImportState = (*organizationGroupMembershipResource)(nil)

func NewOrganizationGroupMembershipResource() resource.Resource {
	return &organizationGroupMembershipResource{}
}

func (r *organizationGroupMembershipResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_group_membership"
}

func (r *organizationGroupMembershipResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages one member of a manually managed Cursor Enterprise Organization Group. Manage SCIM-backed membership in the identity provider instead.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Composite group_id/user_id membership ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				Required:    true,
				Description: "Organization group ID with the g_ prefix.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_id": schema.StringAttribute{
				Required:    true,
				Description: "Cursor public user ID with the user_ prefix.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Display name of the group member.",
			},
			"email": schema.StringAttribute{
				Computed:    true,
				Description: "Email address of the group member.",
			},
		},
	}
}

func (r *organizationGroupMembershipResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *organizationGroupMembershipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan organizationGroupMembershipModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.configured(&resp.Diagnostics) {
		return
	}
	groupID := plan.GroupID.ValueString()
	userID := plan.UserID.ValueString()
	if err := updateOrganizationGroupMember(ctx, r.client.organizationAdmin, strings.TrimSpace(groupID), strings.TrimSpace(userID), "bulk-add"); err != nil {
		resp.Diagnostics.AddError("Failed to add organization group member", err.Error())
		return
	}
	r.read(ctx, groupID, userID, &resp.State, &resp.Diagnostics, true)
}

func (r *organizationGroupMembershipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state organizationGroupMembershipModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.configured(&resp.Diagnostics) {
		return
	}
	r.read(ctx, state.GroupID.ValueString(), state.UserID.ValueString(), &resp.State, &resp.Diagnostics, false)
}

func (r *organizationGroupMembershipResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// group_id and user_id both require replacement, so Terraform never calls Update.
}

func (r *organizationGroupMembershipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state organizationGroupMembershipModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.configured(&resp.Diagnostics) {
		return
	}
	err := updateOrganizationGroupMember(ctx, r.client.organizationAdmin, strings.TrimSpace(state.GroupID.ValueString()), strings.TrimSpace(state.UserID.ValueString()), "bulk-remove")
	if err != nil && !isRESTStatus(err, http.StatusNotFound) {
		resp.Diagnostics.AddError("Failed to remove organization group member", err.Error())
	}
}

func (r *organizationGroupMembershipResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	groupID, userID, ok := strings.Cut(strings.TrimSpace(req.ID), "/")
	if !ok || strings.TrimSpace(groupID) == "" || strings.TrimSpace(userID) == "" || strings.Contains(userID, "/") {
		resp.Diagnostics.AddError("Invalid import ID", "Import organization group membership as group_id/user_id, for example g_abc/user_def.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), groupID+"/"+userID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), groupID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), userID)...)
}

func (r *organizationGroupMembershipResource) configured(diagnostics *diag.Diagnostics) bool {
	if r.client != nil && r.client.organizationAdmin != nil {
		return true
	}
	diagnostics.AddError("Organization API not configured", "Set organization_api_key or CURSOR_ORGANIZATION_API_KEY. Organization-group routes require members:* or admin:* scope.")
	return false
}

func (r *organizationGroupMembershipResource) read(ctx context.Context, groupID string, userID string, state *tfsdk.State, diagnostics *diag.Diagnostics, afterCreate bool) {
	if afterCreate {
		partial := organizationGroupMembershipModel{
			ID:      types.StringValue(groupID + "/" + userID),
			GroupID: types.StringValue(groupID),
			UserID:  types.StringValue(userID),
		}
		diagnostics.Append(state.Set(ctx, &partial)...)
		if diagnostics.HasError() {
			return
		}
	}
	member, err := getOrganizationGroupMember(ctx, r.client.organizationAdmin, strings.TrimSpace(groupID), strings.TrimSpace(userID))
	if err != nil {
		if isRESTStatus(err, http.StatusNotFound) && !afterCreate {
			state.RemoveResource(ctx)
			return
		}
		diagnostics.AddError("Failed to read organization group membership", err.Error())
		return
	}
	if member == nil {
		if afterCreate {
			diagnostics.AddError("Organization group membership was not created", "Cursor did not add the requested user. Confirm that the user belongs to the organization and that the group is manually managed rather than SCIM-backed.")
			return
		}
		state.RemoveResource(ctx)
		return
	}
	updated := organizationGroupMembershipModel{
		ID:      types.StringValue(groupID + "/" + userID),
		GroupID: types.StringValue(groupID),
		UserID:  types.StringValue(userID),
		Name:    types.StringValue(member.Name),
		Email:   types.StringValue(member.Email),
	}
	diagnostics.Append(state.Set(ctx, &updated)...)
}
