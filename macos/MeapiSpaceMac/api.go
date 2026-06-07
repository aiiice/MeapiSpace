package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type UsageResponse struct {
	Mode         string        `json:"mode"`
	IsValid      bool          `json:"isValid"`
	Status       string        `json:"status"`
	PlanName     string        `json:"planName"`
	GroupName    string        `json:"group_name"`
	GroupID      *int64        `json:"group_id"`
	Group        *GroupSummary `json:"group"`
	Remaining    *float64      `json:"remaining"`
	Unit         string        `json:"unit"`
	Balance      *float64      `json:"balance"`
	Quota        *QuotaSummary `json:"quota"`
	Usage        *UsageSummary `json:"usage"`
	Subscription *Subscription `json:"subscription"`
}

type GroupSummary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type QuotaSummary struct {
	Limit     float64 `json:"limit"`
	Used      float64 `json:"used"`
	Remaining float64 `json:"remaining"`
	Unit      string  `json:"unit"`
}

type Subscription struct {
	DailyUsageUSD   float64  `json:"daily_usage_usd"`
	WeeklyUsageUSD  float64  `json:"weekly_usage_usd"`
	MonthlyUsageUSD float64  `json:"monthly_usage_usd"`
	DailyLimitUSD   *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD  *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD *float64 `json:"monthly_limit_usd"`
	ExpiresAt       string   `json:"expires_at"`
}

type UsageSummary struct {
	Today UsagePeriod `json:"today"`
	Total UsagePeriod `json:"total"`
	RPM   float64     `json:"rpm"`
	TPM   float64     `json:"tpm"`
}

type UsagePeriod struct {
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	Cost        float64 `json:"cost"`
	ActualCost  float64 `json:"actual_cost"`
}

type APIClient struct {
	httpClient *http.Client
}

func NewAPIClient() *APIClient {
	return &APIClient{
		httpClient: &http.Client{Timeout: 12 * time.Second},
	}
}

func (c *APIClient) FetchUsage(ctx context.Context, baseURL, apiKey string) (*UsageResponse, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultConfig().BaseURL
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("访问密钥为空")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/usage?days=1", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "meapispace-mac/0.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, summarizeErrorBody(body))
	}

	var usage UsageResponse
	if err := json.Unmarshal(body, &usage); err != nil {
		return nil, err
	}
	if usage.Unit == "" {
		usage.Unit = "USD"
	}
	return &usage, nil
}

func summarizeErrorBody(body []byte) string {
	var parsed struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		switch {
		case parsed.Message != "":
			return parsed.Message
		case parsed.Error.Message != "":
			return parsed.Error.Message
		case parsed.Code != "":
			return parsed.Code
		}
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 180 {
		return s[:180] + "..."
	}
	if s == "" {
		return "empty response"
	}
	return s
}
