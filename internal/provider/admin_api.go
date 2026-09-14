package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	if response.Outcome != "success" {
		if response.Message != "" {
			return fmt.Errorf("Cursor rejected the spend-limit update: %s", response.Message)
		}
		if response.Outcome == "" {
			return fmt.Errorf("Cursor returned a spend-limit response without a success outcome")
		}
		return fmt.Errorf("Cursor rejected the spend-limit update with outcome %q", response.Outcome)
	}
	return nil
}
