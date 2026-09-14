package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type publicID string

func (id *publicID) UnmarshalJSON(value []byte) error {
	var stringValue string
	if err := json.Unmarshal(value, &stringValue); err == nil {
		*id = publicID(stringValue)
		return nil
	}

	var numberValue json.Number
	if err := json.Unmarshal(value, &numberValue); err != nil {
		return fmt.Errorf("Cursor ID must be a string or number: %w", err)
	}
	*id = publicID(numberValue.String())
	return nil
}

type teamMemberAPIModel struct {
	ID        publicID `json:"id"`
	Email     string   `json:"email"`
	Name      string   `json:"name"`
	Role      string   `json:"role"`
	IsRemoved bool     `json:"isRemoved"`
}

type teamMembersAPIResponse struct {
	TeamMembers []teamMemberAPIModel `json:"teamMembers"`
}

type teamMemberSpendAPIModel struct {
	UserID                       publicID `json:"userId"`
	Email                        string   `json:"email"`
	Name                         string   `json:"name"`
	Role                         string   `json:"role"`
	SpendCents                   float64  `json:"spendCents"`
	OverallSpendCents            float64  `json:"overallSpendCents"`
	MonthlyLimitDollars          *int64   `json:"monthlyLimitDollars"`
	EffectivePerUserLimitDollars int64    `json:"effectivePerUserLimitDollars"`
}

type teamSpendAPIResponse struct {
	TeamMemberSpend []teamMemberSpendAPIModel `json:"teamMemberSpend"`
	TotalPages      int                       `json:"totalPages"`
}

type setUserSpendLimitAPIRequest struct {
	UserEmail         string `json:"userEmail"`
	SpendLimitDollars *int64 `json:"spendLimitDollars"`
}

type mutationAPIResponse struct {
	Outcome string `json:"outcome"`
	Message string `json:"message"`
}

func getTeamMemberByEmail(ctx context.Context, client *restClient, email string) (*teamMemberAPIModel, error) {
	var response teamMembersAPIResponse
	if err := client.do(ctx, http.MethodGet, "/teams/members", nil, &response); err != nil {
		return nil, err
	}
	for i := range response.TeamMembers {
		if strings.EqualFold(strings.TrimSpace(response.TeamMembers[i].Email), strings.TrimSpace(email)) {
			return &response.TeamMembers[i], nil
		}
	}
	return nil, nil
}

func getTeamMemberSpendByEmail(ctx context.Context, client *restClient, email string) (*teamMemberSpendAPIModel, error) {
	const pageSize = 100
	for page := 1; ; page++ {
		request := map[string]any{
			"searchTerm": email,
			"page":       page,
			"pageSize":   pageSize,
		}
		var response teamSpendAPIResponse
		if err := client.do(ctx, http.MethodPost, "/teams/spend", request, &response); err != nil {
			return nil, err
		}
		for i := range response.TeamMemberSpend {
			if strings.EqualFold(strings.TrimSpace(response.TeamMemberSpend[i].Email), strings.TrimSpace(email)) {
				return &response.TeamMemberSpend[i], nil
			}
		}
		if response.TotalPages <= page {
			return nil, nil
		}
	}
}

func setUserSpendLimit(ctx context.Context, client *restClient, email string, limit *int64) error {
	var response mutationAPIResponse
	if err := client.do(ctx, http.MethodPost, "/teams/user-spend-limit", setUserSpendLimitAPIRequest{
		UserEmail:         email,
		SpendLimitDollars: limit,
	}, &response); err != nil {
		return err
	}
	if response.Outcome != "" && response.Outcome != "success" {
		if response.Message != "" {
			return fmt.Errorf("Cursor rejected the spend-limit update: %s", response.Message)
		}
		return fmt.Errorf("Cursor rejected the spend-limit update with outcome %q", response.Outcome)
	}
	return nil
}

type organizationGroupAPIModel struct {
	ID                          string `json:"id"`
	Name                        string `json:"name"`
	MemberCount                 int64  `json:"memberCount"`
	MonthlySpendingLimitDollars *int64 `json:"monthlySpendingLimitDollars"`
}

type organizationGroupsAPIResponse struct {
	Groups     []organizationGroupAPIModel `json:"groups"`
	Pagination paginationAPIModel          `json:"pagination"`
}

type organizationGroupAPIResponse struct {
	Group organizationGroupAPIModel `json:"group"`
}

type paginationAPIModel struct {
	Page        int  `json:"page"`
	TotalPages  int  `json:"totalPages"`
	HasNextPage bool `json:"hasNextPage"`
}

type createOrganizationGroupAPIRequest struct {
	Name string `json:"name"`
}

type updateOrganizationGroupAPIRequest struct {
	Name                             *string `json:"name,omitempty"`
	MonthlySpendingLimitDollars      *int64  `json:"monthlySpendingLimitDollars,omitempty"`
	ClearMonthlySpendingLimitDollars bool    `json:"clearMonthlySpendingLimitDollars,omitempty"`
}

func createOrganizationGroup(ctx context.Context, client *restClient, name string) (*organizationGroupAPIModel, error) {
	var response organizationGroupAPIResponse
	err := client.do(ctx, http.MethodPost, "/organizations/groups", createOrganizationGroupAPIRequest{Name: name}, &response)
	if err != nil {
		return nil, err
	}
	return &response.Group, nil
}

func getOrganizationGroup(ctx context.Context, client *restClient, groupID string) (*organizationGroupAPIModel, error) {
	var response organizationGroupAPIResponse
	err := client.do(ctx, http.MethodGet, "/organizations/groups/"+url.PathEscape(groupID), nil, &response)
	if err != nil {
		return nil, err
	}
	return &response.Group, nil
}

func getOrganizationGroupByName(ctx context.Context, client *restClient, name string) (*organizationGroupAPIModel, error) {
	const pageSize = 200
	for page := 1; ; page++ {
		path := "/organizations/groups?page=" + strconv.Itoa(page) + "&pageSize=" + strconv.Itoa(pageSize)
		var response organizationGroupsAPIResponse
		if err := client.do(ctx, http.MethodGet, path, nil, &response); err != nil {
			return nil, err
		}
		for i := range response.Groups {
			if response.Groups[i].Name == name {
				return &response.Groups[i], nil
			}
		}
		if !response.Pagination.HasNextPage && response.Pagination.TotalPages <= page {
			return nil, nil
		}
	}
}

func updateOrganizationGroup(ctx context.Context, client *restClient, groupID string, request updateOrganizationGroupAPIRequest) (*organizationGroupAPIModel, error) {
	var response organizationGroupAPIResponse
	err := client.do(ctx, http.MethodPatch, "/organizations/groups/"+url.PathEscape(groupID), request, &response)
	if err != nil {
		return nil, err
	}
	return &response.Group, nil
}

func deleteOrganizationGroup(ctx context.Context, client *restClient, groupID string) error {
	return client.do(ctx, http.MethodDelete, "/organizations/groups/"+url.PathEscape(groupID), nil, nil)
}

type organizationGroupMemberAPIModel struct {
	UserID publicID `json:"userId"`
	Name   string   `json:"name"`
	Email  string   `json:"email"`
}

type organizationGroupMembersAPIResponse struct {
	Members    []organizationGroupMemberAPIModel `json:"members"`
	Pagination paginationAPIModel                `json:"pagination"`
}

type updateOrganizationGroupMembersAPIRequest struct {
	UserIDs []string `json:"userIds"`
}

func getOrganizationGroupMember(ctx context.Context, client *restClient, groupID string, userID string) (*organizationGroupMemberAPIModel, error) {
	const pageSize = 200
	for page := 1; ; page++ {
		path := "/organizations/groups/" + url.PathEscape(groupID) + "/members?page=" + strconv.Itoa(page) + "&pageSize=" + strconv.Itoa(pageSize)
		var response organizationGroupMembersAPIResponse
		if err := client.do(ctx, http.MethodGet, path, nil, &response); err != nil {
			return nil, err
		}
		for i := range response.Members {
			if string(response.Members[i].UserID) == userID {
				return &response.Members[i], nil
			}
		}
		if !response.Pagination.HasNextPage && response.Pagination.TotalPages <= page {
			return nil, nil
		}
	}
}

func updateOrganizationGroupMember(ctx context.Context, client *restClient, groupID string, userID string, operation string) error {
	path := "/organizations/groups/" + url.PathEscape(groupID) + "/members/" + operation
	return client.do(ctx, http.MethodPost, path, updateOrganizationGroupMembersAPIRequest{UserIDs: []string{userID}}, nil)
}
