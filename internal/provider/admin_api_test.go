package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRESTClientUsesBasicAuthAndUserAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "key_test" || password != "" {
			t.Fatalf("BasicAuth() = (%q, %q, %v), want key_test, empty, true", username, password, ok)
		}
		if got, want := r.Header.Get("User-Agent"), "terraform-provider-cursor/test"; got != want {
			t.Fatalf("User-Agent = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := newRESTClient(server.Client(), server.URL, " key_test ", "test")
	var response struct {
		OK bool `json:"ok"`
	}
	if err := client.do(context.Background(), http.MethodGet, "/test", nil, &response); err != nil {
		t.Fatalf("do() error: %v", err)
	}
	if !response.OK {
		t.Fatal("do() did not decode the response")
	}
}

func TestRESTClientReturnsStructuredError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"missing members:* scope"}`))
	}))
	defer server.Close()

	client := newRESTClient(server.Client(), server.URL, "key_test", "test")
	err := client.do(context.Background(), http.MethodGet, "/test", nil, nil)
	if err == nil {
		t.Fatal("do() expected error, got nil")
	}
	if !isRESTStatus(err, http.StatusForbidden) {
		t.Fatalf("isRESTStatus(%v, 403) = false", err)
	}
}

func TestGetTeamMemberSpendByEmailUsesExactMatchAndPagination(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if request["searchTerm"] != "developer@company.com" {
			t.Fatalf("searchTerm = %v", request["searchTerm"])
		}
		page := int(request["page"].(float64))
		w.Header().Set("Content-Type", "application/json")
		if page == 1 {
			w.Write([]byte(`{"teamMemberSpend":[{"userId":123,"email":"other@company.com"}],"totalPages":2}`))
			return
		}
		w.Write([]byte(`{"teamMemberSpend":[{"userId":"user_abc","email":"Developer@Company.com","name":"Dev","monthlyLimitDollars":125,"effectivePerUserLimitDollars":200,"spendCents":12.5,"overallSpendCents":34.75}],"totalPages":2}`))
	}))
	defer server.Close()

	client := newRESTClient(server.Client(), server.URL, "key_test", "test")
	member, err := getTeamMemberSpendByEmail(context.Background(), client, "developer@company.com")
	if err != nil {
		t.Fatalf("getTeamMemberSpendByEmail() error: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	if member == nil || string(member.UserID) != "user_abc" || member.MonthlyLimitDollars == nil || *member.MonthlyLimitDollars != 125 {
		t.Fatalf("unexpected member: %+v", member)
	}
}

func TestSetUserSpendLimitSupportsSetAndClear(t *testing.T) {
	var limits []*int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request setUserSpendLimitAPIRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		limits = append(limits, request.SpendLimitDollars)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"outcome":"success","message":"updated"}`))
	}))
	defer server.Close()

	client := newRESTClient(server.Client(), server.URL, "key_test", "test")
	limit := int64(100)
	if err := setUserSpendLimit(context.Background(), client, "developer@company.com", &limit); err != nil {
		t.Fatalf("setUserSpendLimit(set) error: %v", err)
	}
	if err := setUserSpendLimit(context.Background(), client, "developer@company.com", nil); err != nil {
		t.Fatalf("setUserSpendLimit(clear) error: %v", err)
	}
	if len(limits) != 2 || limits[0] == nil || *limits[0] != 100 || limits[1] != nil {
		t.Fatalf("limits = %#v, want [100, nil]", limits)
	}
}

func TestSetUserSpendLimitRejectsErrorOutcome(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"outcome":"error","message":"member not found"}`))
	}))
	defer server.Close()

	client := newRESTClient(server.Client(), server.URL, "key_test", "test")
	limit := int64(100)
	if err := setUserSpendLimit(context.Background(), client, "missing@company.com", &limit); err == nil {
		t.Fatal("setUserSpendLimit() expected error, got nil")
	}
}

func TestOrganizationGroupHelpers(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/organizations/groups":
			if r.URL.Query().Get("page") == "1" {
				w.Write([]byte(`{"groups":[{"id":"g_other","name":"Other","memberCount":1}],"pagination":{"page":1,"totalPages":2,"hasNextPage":true}}`))
				return
			}
			w.Write([]byte(`{"groups":[{"id":"g_engineering","name":"Engineering","memberCount":2,"monthlySpendingLimitDollars":300}],"pagination":{"page":2,"totalPages":2,"hasNextPage":false}}`))
		case "/organizations/groups/g_engineering/members":
			w.Write([]byte(`{"members":[{"userId":"user_abc","name":"Alex","email":"alex@company.com"}],"pagination":{"page":1,"totalPages":1,"hasNextPage":false}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newRESTClient(server.Client(), server.URL, "key_org", "test")
	group, err := getOrganizationGroupByName(context.Background(), client, "Engineering")
	if err != nil {
		t.Fatalf("getOrganizationGroupByName() error: %v", err)
	}
	if group == nil || group.ID != "g_engineering" || group.MonthlySpendingLimitDollars == nil || *group.MonthlySpendingLimitDollars != 300 {
		t.Fatalf("unexpected group: %+v", group)
	}
	member, err := getOrganizationGroupMember(context.Background(), client, group.ID, "user_abc")
	if err != nil {
		t.Fatalf("getOrganizationGroupMember() error: %v", err)
	}
	if member == nil || member.Email != "alex@company.com" {
		t.Fatalf("unexpected member: %+v", member)
	}
	wantPaths := []string{
		"/organizations/groups?page=1&pageSize=200",
		"/organizations/groups?page=2&pageSize=200",
		"/organizations/groups/g_engineering/members?page=1&pageSize=200",
	}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("paths = %#v, want %#v", paths, wantPaths)
	}
}

func TestPublicIDAcceptsStringAndNumber(t *testing.T) {
	for input, want := range map[string]string{`"user_abc"`: "user_abc", `12345`: "12345"} {
		var id publicID
		if err := json.Unmarshal([]byte(input), &id); err != nil {
			t.Fatalf("Unmarshal(%s) error: %v", input, err)
		}
		if string(id) != want {
			t.Fatalf("Unmarshal(%s) = %q, want %q", input, id, want)
		}
	}
}
