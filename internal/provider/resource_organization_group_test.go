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

func organizationGroupPlan(t *testing.T, model organizationGroupResourceModel) tfsdk.Plan {
	t.Helper()
	response := resource.SchemaResponse{}
	(&organizationGroupResource{}).Schema(context.Background(), resource.SchemaRequest{}, &response)
	plan := tfsdk.Plan{Schema: response.Schema}
	if diags := plan.Set(context.Background(), &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}
	return plan
}

func TestOrganizationGroupCreatePreservesConfiguredName(t *testing.T) {
	for _, failLimit := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "partial_failure"}[failLimit], func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "POST /organizations/groups":
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["name"] != "Engineering" {
						t.Errorf("unexpected create body: %v, error: %v", body, err)
					}
					w.WriteHeader(http.StatusCreated)
					w.Write([]byte(`{"group":{"id":"g_eng","name":"Engineering","memberCount":0,"monthlySpendingLimitDollars":null}}`))
				case "PATCH /organizations/groups/g_eng":
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["monthlySpendingLimitDollars"] != float64(0) {
						t.Errorf("zero limit omitted or incorrect: %v, error: %v", body, err)
					}
					if failLimit {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					w.Write([]byte(`{"group":{"id":"g_eng","name":"Engineering","memberCount":0,"monthlySpendingLimitDollars":0}}`))
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			r := &organizationGroupResource{client: &apiClient{organizationAdmin: newRESTClient(server.Client(), server.URL, "key_test", "test")}}
			plan := organizationGroupPlan(t, organizationGroupResourceModel{Name: types.StringValue(" Engineering "), MonthlySpendingLimitDollars: types.Int64Value(0)})
			response := resource.CreateResponse{State: tfsdk.State{Schema: plan.Schema}}
			r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
			if response.Diagnostics.HasError() != failLimit {
				t.Fatalf("diagnostics: %v", response.Diagnostics)
			}
			var state organizationGroupResourceModel
			if diags := response.State.Get(context.Background(), &state); diags.HasError() {
				t.Fatalf("read state: %v", diags)
			}
			if requests != 2 || state.ID.ValueString() != "g_eng" || state.Name.ValueString() != " Engineering " {
				t.Fatalf("requests=%d, state=%+v", requests, state)
			}
			if state.MonthlySpendingLimitDollars.IsNull() != failLimit {
				t.Fatalf("unexpected limit after write: %v", state.MonthlySpendingLimitDollars)
			}
		})
	}
}

func TestOrganizationGroupUpdateClearsLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		if r.Method != http.MethodPatch || body["clearMonthlySpendingLimitDollars"] != true || len(body) != 1 {
			t.Errorf("unexpected clear request: %s %v", r.Method, body)
		}
		w.Write([]byte(`{"group":{"id":"g_eng","name":"Engineering","memberCount":2,"monthlySpendingLimitDollars":null}}`))
	}))
	defer server.Close()
	r := &organizationGroupResource{client: &apiClient{organizationAdmin: newRESTClient(server.Client(), server.URL, "key_test", "test")}}
	model := organizationGroupResourceModel{ID: types.StringValue("g_eng"), Name: types.StringValue("Engineering"), MonthlySpendingLimitDollars: types.Int64Value(100)}
	oldPlan := organizationGroupPlan(t, model)
	model.MonthlySpendingLimitDollars = types.Int64Null()
	plan := organizationGroupPlan(t, model)
	response := resource.UpdateResponse{State: tfsdk.State{Schema: plan.Schema}}
	r.Update(context.Background(), resource.UpdateRequest{Plan: plan, State: tfsdk.State{Schema: oldPlan.Schema, Raw: oldPlan.Raw}}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("update: %v", response.Diagnostics)
	}
	var state organizationGroupResourceModel
	if diags := response.State.Get(context.Background(), &state); diags.HasError() || !state.MonthlySpendingLimitDollars.IsNull() {
		t.Fatalf("limit was not cleared: %+v, %v", state, diags)
	}
}

func TestOrganizationGroupReadDistinguishesMissingFromForbidden(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		r := &organizationGroupResource{client: &apiClient{organizationAdmin: newRESTClient(server.Client(), server.URL, "key_test", "test")}}
		plan := organizationGroupPlan(t, organizationGroupResourceModel{ID: types.StringValue("g_eng"), Name: types.StringValue("Engineering")})
		state := tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}
		response := resource.ReadResponse{State: state}
		r.Read(context.Background(), resource.ReadRequest{State: state}, &response)
		server.Close()
		if response.Diagnostics.HasError() != (status == http.StatusForbidden) || response.State.Raw.IsNull() != (status == http.StatusNotFound) {
			t.Fatalf("status %d: diagnostics=%v, state=%v", status, response.Diagnostics, response.State.Raw)
		}
	}
}

func TestOrganizationGroupValidateConfig(t *testing.T) {
	for _, tc := range []struct {
		name      string
		limit     types.Int64
		wantError bool
	}{
		{"null", types.Int64Null(), false},
		{"unknown", types.Int64Unknown(), false},
		{"zero", types.Int64Value(0), false},
		{"maximum", types.Int64Value(2147483647), false},
		{"negative", types.Int64Value(-1), true},
		{"overflow", types.Int64Value(2147483648), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := organizationGroupPlan(t, organizationGroupResourceModel{Name: types.StringValue("Engineering"), MonthlySpendingLimitDollars: tc.limit})
			response := resource.ValidateConfigResponse{}
			(&organizationGroupResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: plan.Schema, Raw: plan.Raw}}, &response)
			if response.Diagnostics.HasError() != tc.wantError {
				t.Fatalf("diagnostics: %v", response.Diagnostics)
			}
		})
	}
}
