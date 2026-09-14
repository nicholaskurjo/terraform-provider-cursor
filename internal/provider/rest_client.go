package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxRESTResponseBytes = 4 << 20

type restClient struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
	userAgent  string
}

type restError struct {
	StatusCode int
	Body       string
}

func (e *restError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("Cursor API returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("Cursor API returned HTTP %d: %s", e.StatusCode, e.Body)
}

func newRESTClient(httpClient *http.Client, endpoint string, apiKey string, version string) *restClient {
	return &restClient{
		httpClient: httpClient,
		endpoint:   strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		apiKey:     strings.TrimSpace(apiKey),
		userAgent:  userAgentPrefix + strings.TrimSpace(version),
	}
}

func (c *restClient) do(ctx context.Context, method string, path string, requestBody any, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, body)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.SetBasicAuth(c.apiKey, "")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling Cursor API: %w", err)
	}
	defer resp.Body.Close()

	responseBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxRESTResponseBytes))
	if err != nil {
		return fmt.Errorf("reading Cursor API response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &restError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(responseBytes))}
	}
	if responseBody == nil || len(responseBytes) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBytes, responseBody); err != nil {
		return fmt.Errorf("decoding Cursor API response: %w", err)
	}
	return nil
}

func isRESTStatus(err error, statusCode int) bool {
	var apiErr *restError
	return errors.As(err, &apiErr) && apiErr.StatusCode == statusCode
}
