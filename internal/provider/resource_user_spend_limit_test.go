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

func userSpendLimitPlan(t *testing.T, model userSpendLimitModel) tfsdk.Plan {
	t.Helper()
	response := resource.SchemaResponse{}
	(&userSpendLimitResource{}).Schema(context.Background(), resource.SchemaRequest{}, &response)
	plan := tfsdk.Plan{Schema: response.Schema}
	if diags := plan.Set(context.Background(), &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}
	return plan
}

func userSpendLimitState(t *testing.T, model userSpendLimitModel) tfsdk.State {
	t.Helper()
	plan := userSpendLimitPlan(t, model)
	state := tfsdk.State{Schema: plan.Schema}
	if diags := state.Set(context.Background(), &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}
	return state
}

func TestUserSpendLimitLifecycle(t *testing.T) {
	var limit *int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		switch r.URL.Path {
		case "/teams/user-spend-limit":
			var body setUserSpendLimitAPIRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UserEmail != "Developer@example.com" {
				t.Errorf("unexpected body: %+v, %v", body, err)
			}
			limit = body.SpendLimitDollars
			w.Write([]byte(`{"outcome":"success"}`))
		case "/teams/spend":
			json.NewEncoder(w).Encode(teamSpendAPIResponse{TeamMemberSpend: []teamMemberSpendAPIModel{{UserID: "user_dev", Email: "developer@example.com", MonthlyLimitDollars: limit}}, TotalPages: 1})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	r := &userSpendLimitResource{client: &apiClient{teamAdmin: newRESTClient(server.Client(), server.URL, "key_test", "test")}}
	model := userSpendLimitModel{Email: types.StringValue("Developer@example.com"), SpendLimitDollars: types.Int64Value(100)}
	plan := userSpendLimitPlan(t, model)
	create := resource.CreateResponse{State: tfsdk.State{Schema: plan.Schema}}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &create)
	if create.Diagnostics.HasError() {
		t.Fatalf("create: %v", create.Diagnostics)
	}
	var state userSpendLimitModel
	if diags := create.State.Get(context.Background(), &state); diags.HasError() || !state.Email.Equal(model.Email) || state.SpendLimitDollars.ValueInt64() != 100 {
		t.Fatalf("state=%+v, diagnostics=%v", state, diags)
	}
	model.SpendLimitDollars = types.Int64Value(0)
	plan = userSpendLimitPlan(t, model)
	update := resource.UpdateResponse{State: create.State}
	r.Update(context.Background(), resource.UpdateRequest{Plan: plan, State: create.State}, &update)
	if update.Diagnostics.HasError() || limit == nil || *limit != 0 {
		t.Fatalf("update zero: %v, limit=%v", update.Diagnostics, limit)
	}
	deleted := resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{State: update.State}, &deleted)
	if deleted.Diagnostics.HasError() || limit != nil {
		t.Fatalf("delete must clear, not set zero: %v, limit=%v", deleted.Diagnostics, limit)
	}
	read := resource.ReadResponse{State: update.State}
	r.Read(context.Background(), resource.ReadRequest{State: update.State}, &read)
	if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
		t.Fatalf("cleared override should leave state: %v", read.Diagnostics)
	}
}

func TestUserSpendLimitReadFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"message":"invalid key"}`},
		{name: "forbidden", status: http.StatusForbidden, body: `{"message":"not an admin"}`},
		{name: "rate_limited", status: http.StatusTooManyRequests, body: `{"message":"slow down"}`},
		{name: "server_error", status: http.StatusInternalServerError, body: `{"message":"unavailable"}`},
		{name: "malformed_success", status: http.StatusOK, body: `{"teamMemberSpend":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()

			model := userSpendLimitModel{Email: types.StringValue("dev@example.com"), SpendLimitDollars: types.Int64Value(100)}
			prior := userSpendLimitState(t, model)
			r := &userSpendLimitResource{client: &apiClient{teamAdmin: newRESTClient(server.Client(), server.URL, "key_test", "test")}}
			response := resource.ReadResponse{State: prior}
			r.Read(context.Background(), resource.ReadRequest{State: prior}, &response)
			if !response.Diagnostics.HasError() {
				t.Fatal("expected refresh error")
			}
			var got userSpendLimitModel
			if diags := response.State.Get(context.Background(), &got); diags.HasError() || got.Email.ValueString() != "dev@example.com" || got.SpendLimitDollars.ValueInt64() != 100 {
				t.Fatalf("refresh must retain prior state: %+v, %v", got, diags)
			}
		})
	}
}

func TestUserSpendLimitRejectsUnverifiedMutation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/teams/user-spend-limit" {
			w.Write([]byte(`{"outcome":"success"}`))
			return
		}
		w.Write([]byte(`{"teamMemberSpend":[{"userId":"user_dev","email":"dev@example.com","monthlyLimitDollars":50}],"totalPages":1}`))
	}))
	defer server.Close()

	model := userSpendLimitModel{Email: types.StringValue("dev@example.com"), SpendLimitDollars: types.Int64Value(100)}
	plan := userSpendLimitPlan(t, model)
	r := &userSpendLimitResource{client: &apiClient{teamAdmin: newRESTClient(server.Client(), server.URL, "key_test", "test")}}
	response := resource.CreateResponse{State: tfsdk.State{Schema: plan.Schema}}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected verification error")
	}
	var got userSpendLimitModel
	if diags := response.State.Get(context.Background(), &got); diags.HasError() || got.Email.ValueString() != "dev@example.com" || got.SpendLimitDollars.ValueInt64() != 100 {
		t.Fatalf("failed verification must retain requested identity: %+v, %v", got, diags)
	}
}

func TestUserSpendLimitDeleteRequiresObservedClear(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/teams/user-spend-limit" {
			w.Write([]byte(`{"outcome":"success"}`))
			return
		}
		w.Write([]byte(`{"teamMemberSpend":[{"userId":"user_dev","email":"dev@example.com","monthlyLimitDollars":100}],"totalPages":1}`))
	}))
	defer server.Close()

	state := userSpendLimitState(t, userSpendLimitModel{Email: types.StringValue("dev@example.com"), SpendLimitDollars: types.Int64Value(100)})
	r := &userSpendLimitResource{client: &apiClient{teamAdmin: newRESTClient(server.Client(), server.URL, "key_test", "test")}}
	response := resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected clear verification error")
	}
}

func TestUserSpendLimitRetainsIdentityWhenReadAfterWriteFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/teams/user-spend-limit" {
			w.Write([]byte(`{"outcome":"success"}`))
			return
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	r := &userSpendLimitResource{client: &apiClient{teamAdmin: newRESTClient(server.Client(), server.URL, "key_test", "test")}}
	plan := userSpendLimitPlan(t, userSpendLimitModel{Email: types.StringValue("dev@example.com"), SpendLimitDollars: types.Int64Value(100)})
	response := resource.CreateResponse{State: tfsdk.State{Schema: plan.Schema}}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected read failure")
	}
	var state userSpendLimitModel
	if diags := response.State.Get(context.Background(), &state); diags.HasError() || state.Email.ValueString() != "dev@example.com" {
		t.Fatalf("lost identity: %+v, %v", state, diags)
	}
}

func TestUserSpendLimitImportAndValidation(t *testing.T) {
	r := &userSpendLimitResource{}
	plan := userSpendLimitPlan(t, userSpendLimitModel{})
	for _, id := range []string{"dev@example.com", " "} {
		response := resource.ImportStateResponse{State: tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}}
		r.ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &response)
		if response.Diagnostics.HasError() != (id == " ") {
			t.Fatalf("import %q: %v", id, response.Diagnostics)
		}
	}
	for _, limit := range []types.Int64{types.Int64Null(), types.Int64Unknown(), types.Int64Value(0), types.Int64Value(-1)} {
		plan := userSpendLimitPlan(t, userSpendLimitModel{Email: types.StringUnknown(), SpendLimitDollars: limit})
		response := resource.ValidateConfigResponse{}
		r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: plan.Schema, Raw: plan.Raw}}, &response)
		if response.Diagnostics.HasError() != (limit.ValueInt64() < 0) {
			t.Fatalf("limit %v: %v", limit, response.Diagnostics)
		}
	}
	for _, email := range []string{"dev@example.com", " dev@example.com", "dev@example.com ", " "} {
		plan := userSpendLimitPlan(t, userSpendLimitModel{Email: types.StringValue(email), SpendLimitDollars: types.Int64Value(0)})
		response := resource.ValidateConfigResponse{}
		r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: plan.Schema, Raw: plan.Raw}}, &response)
		if response.Diagnostics.HasError() != (email != "dev@example.com") {
			t.Fatalf("email %q: %v", email, response.Diagnostics)
		}
	}
}

func TestUserSpendLimitImportRefreshesLiveLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"teamMemberSpend":[{"userId":"user_dev","email":"dev@example.com","name":"Dev","monthlyLimitDollars":75,"effectivePerUserLimitDollars":90}],"totalPages":1}`))
	}))
	defer server.Close()

	r := &userSpendLimitResource{client: &apiClient{teamAdmin: newRESTClient(server.Client(), server.URL, "key_test", "test")}}
	empty := userSpendLimitPlan(t, userSpendLimitModel{})
	importResponse := resource.ImportStateResponse{State: tfsdk.State{Schema: empty.Schema, Raw: empty.Raw}}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: " dev@example.com "}, &importResponse)
	if importResponse.Diagnostics.HasError() {
		t.Fatalf("import: %v", importResponse.Diagnostics)
	}
	readResponse := resource.ReadResponse{State: importResponse.State}
	r.Read(context.Background(), resource.ReadRequest{State: importResponse.State}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read imported limit: %v", readResponse.Diagnostics)
	}
	var got userSpendLimitModel
	if diags := readResponse.State.Get(context.Background(), &got); diags.HasError() || got.Email.ValueString() != "dev@example.com" || got.SpendLimitDollars.ValueInt64() != 75 {
		t.Fatalf("imported state: %+v, %v", got, diags)
	}
}
