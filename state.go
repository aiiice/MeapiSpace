package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

type DisplayState struct {
	ModeLabel      string
	StatusLabel    string
	RemainingText  string
	UsageText      string
	DetailText     string
	UpdatedText    string
	Tooltip        string
	TodayCostText  string
	TotalCostText  string
	TodayTokenText string
	SourceText     string
	Progress       float64
	RemainingUSD   *float64
	LowBalance     bool
	Unlimited      bool
	HasData        bool
	Error          string
}

func DisplayFromUsage(resp *UsageResponse, cfg Config, fetchedAt time.Time) DisplayState {
	if resp == nil {
		return DisplayState{Error: "no usage data"}
	}

	remaining, hasRemaining := resolveRemaining(resp)
	modeLabel := resolveModeLabel(resp)
	status := strings.TrimSpace(resp.Status)
	if status == "" {
		if resp.IsValid {
			status = "有效"
		} else {
			status = "未知"
		}
	} else {
		status = localizedStatus(status)
	}

	var progress float64
	unlimited := hasRemaining && remaining < 0
	switch {
	case resp.Quota != nil && resp.Quota.Limit > 0:
		progress = clamp01(resp.Quota.Remaining / resp.Quota.Limit)
	case hasRemaining && remaining >= 0:
		progress = clamp01(remaining / cfg.WalletFullReferenceUSD)
	case unlimited:
		progress = 1
	default:
		progress = 0
	}

	low := hasRemaining && remaining >= 0 && remaining < cfg.LowBalanceThresholdUSD
	remainingText := "剩余额度：未知"
	if hasRemaining {
		if unlimited {
			remainingText = "剩余额度：无限制"
		} else {
			remainingText = "剩余额度：" + formatUSD(remaining)
		}
	}

	detailText := "额度来源：" + modeLabel
	if resp.Quota != nil && resp.Quota.Limit > 0 {
		detailText = fmt.Sprintf(
			"密钥额度：%s / %s",
			formatUSD(resp.Quota.Remaining),
			formatUSD(resp.Quota.Limit),
		)
	}

	usageText := "今日消耗：--    累计：--"
	todayCostText := "今日消耗 --"
	totalCostText := "累计消耗 --"
	todayTokenText := "今日令牌 --"
	if resp.Usage != nil {
		usageText = fmt.Sprintf(
			"今日消耗：%s    累计：%s",
			formatUSD(resp.Usage.Today.ActualCost),
			formatUSD(resp.Usage.Total.ActualCost),
		)
		todayCostText = "今日消耗 " + formatUSD(resp.Usage.Today.ActualCost)
		totalCostText = "累计消耗 " + formatUSD(resp.Usage.Total.ActualCost)
		todayTokenText = "今日令牌 " + formatCompactInt(resp.Usage.Today.TotalTokens)
	}

	updatedText := "更新时间：" + fetchedAt.Format("15:04:05")
	statusLabel := "状态：" + status
	tooltip := appName + " 额度"
	if hasRemaining {
		if unlimited {
			tooltip += "：无限制"
		} else {
			tooltip += "：" + formatUSD(remaining)
		}
	}
	if low {
		tooltip += "（低余额）"
	}

	var remainingPtr *float64
	if hasRemaining {
		v := remaining
		remainingPtr = &v
	}

	return DisplayState{
		ModeLabel:      modeLabel,
		StatusLabel:    statusLabel,
		RemainingText:  remainingText,
		UsageText:      usageText,
		DetailText:     detailText,
		UpdatedText:    updatedText,
		Tooltip:        tooltip,
		TodayCostText:  todayCostText,
		TotalCostText:  totalCostText,
		TodayTokenText: todayTokenText,
		SourceText:     "来源 " + modeLabel,
		Progress:       progress,
		RemainingUSD:   remainingPtr,
		LowBalance:     low,
		Unlimited:      unlimited,
		HasData:        true,
	}
}

func DisplayForError(err error) DisplayState {
	msg := "未知错误"
	if err != nil {
		msg = err.Error()
	}
	return DisplayState{
		ModeLabel:      "未连接",
		StatusLabel:    "状态：错误",
		RemainingText:  "剩余额度：--",
		UsageText:      "今日消耗：--    累计：--",
		DetailText:     "错误：" + msg,
		UpdatedText:    "更新时间：--",
		Tooltip:        appName + " 额度：请求失败",
		TodayCostText:  "今日消耗 --",
		TotalCostText:  "累计消耗 --",
		TodayTokenText: "今日令牌 --",
		SourceText:     "请求失败",
		Error:          msg,
	}
}

func DisplayNoAPIKey() DisplayState {
	return DisplayState{
		ModeLabel:      "未设置",
		StatusLabel:    "状态：等待访问密钥",
		RemainingText:  "剩余额度：--",
		UsageText:      "今日消耗：--    累计：--",
		DetailText:     "右键浮窗或托盘图标，选择“设置访问密钥”。",
		UpdatedText:    "更新时间：--",
		Tooltip:        appName + " 额度：未设置访问密钥",
		TodayCostText:  "今日消耗 --",
		TotalCostText:  "累计消耗 --",
		TodayTokenText: "今日令牌 --",
		SourceText:     "等待设置",
	}
}

func resolveRemaining(resp *UsageResponse) (float64, bool) {
	if resp == nil {
		return 0, false
	}
	if resp.Quota != nil {
		return resp.Quota.Remaining, true
	}
	if resp.Remaining != nil {
		return *resp.Remaining, true
	}
	if resp.Balance != nil {
		return *resp.Balance, true
	}
	return 0, false
}

func resolveModeLabel(resp *UsageResponse) string {
	if resp == nil {
		return "未知"
	}
	if resp.Quota != nil {
		return "密钥额度"
	}
	if resp.Subscription != nil || strings.Contains(strings.ToLower(resp.Mode), "subscription") {
		return "订阅额度"
	}
	if strings.TrimSpace(resp.PlanName) != "" {
		return resp.PlanName
	}
	if strings.TrimSpace(resp.Mode) != "" {
		return resp.Mode
	}
	return "钱包余额"
}

func formatUSD(v float64) string {
	if math.Abs(v) >= 100 {
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

func formatCompactInt(v int64) string {
	abs := math.Abs(float64(v))
	switch {
	case abs >= 100_000_000:
		return fmt.Sprintf("%.2f亿", float64(v)/100_000_000)
	default:
		return fmt.Sprintf("%.1f万", float64(v)/10_000)
	}
}

func localizedStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return "有效"
	case "inactive", "disabled":
		return "停用"
	case "expired":
		return "已过期"
	case "quota_exhausted":
		return "额度用尽"
	case "unknown":
		return "未知"
	default:
		return status
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
