package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func membershipPlan(t *testing.T) tfsdk.Plan {
	t.Helper()
	response := resource.SchemaResponse{}
	(&organizationGroupMembershipResource{}).Schema(context.Background(), resource.SchemaRequest{}, &response)
	plan := tfsdk.Plan{Schema: response.Schema}
	model := organizationGroupMembershipModel{GroupID: types.StringValue("g_eng"), UserID: types.StringValue("user_dev")}
	if diags := plan.Set(context.Background(), &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}
	return plan
}

func TestOrganizationGroupMembershipLifecycle(t *testing.T) {
	memberPresent := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/organizations/groups/g_eng/members/bulk-add", "/organizations/groups/g_eng/members/bulk-remove":
			var body updateOrganizationGroupMembersAPIRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.UserIDs) != 1 || body.UserIDs[0] != "user_dev" || r.Method != http.MethodPost {
				t.Errorf("unexpected membership request: %+v, %v", body, err)
			}
			memberPresent = r.URL.Path == "/organizations/groups/g_eng/members/bulk-add"
			w.Write([]byte(`{}`))
		case "/organizations/groups/g_eng/members":
			members := []organizationGroupMemberAPIModel{}
			if memberPresent {
				members = append(members, organizationGroupMemberAPIModel{UserID: "user_dev", Name: "Dev", Email: "dev@example.com"})
			}
			json.NewEncoder(w).Encode(organizationGroupMembersAPIResponse{Members: members})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	r := &organizationGroupMembershipResource{client: &apiClient{organizationAdmin: newRESTClient(server.Client(), server.URL, "key_test", "test")}}
	plan := membershipPlan(t)
	create := resource.CreateResponse{State: tfsdk.State{Schema: plan.Schema}}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &create)
	if create.Diagnostics.HasError() || !memberPresent {
		t.Fatalf("create: %v", create.Diagnostics)
	}
	var state organizationGroupMembershipModel
	if diags := create.State.Get(context.Background(), &state); diags.HasError() || state.ID.ValueString() != "g_eng/user_dev" {
		t.Fatalf("state=%+v, diagnostics=%v", state, diags)
	}
	deleted := resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{State: create.State}, &deleted)
	if deleted.Diagnostics.HasError() || memberPresent {
		t.Fatalf("delete: %v", deleted.Diagnostics)
	}
	read := resource.ReadResponse{State: create.State}
	r.Read(context.Background(), resource.ReadRequest{State: create.State}, &read)
	if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
		t.Fatalf("removed membership should leave state: %v", read.Diagnostics)
	}
}

func TestOrganizationGroupMembershipImport(t *testing.T) {
	r := &organizationGroupMembershipResource{}
	plan := membershipPlan(t)
	for _, id := range []string{"g_eng/user_dev", "g_eng", "g_eng/", "/user_dev", "g_eng/user_dev/extra"} {
		response := resource.ImportStateResponse{State: tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}}
		r.ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &response)
		if response.Diagnostics.HasError() != (id != "g_eng/user_dev") {
			t.Fatalf("import %q: %v", id, response.Diagnostics)
		}
	}
}
