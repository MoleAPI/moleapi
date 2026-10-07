package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

const (
	CodingPlanOfficialReasonPrefix  = "coding_plan_official:"
	codingPlanOfficialCacheTTL      = time.Minute
	codingPlanOfficialTimeout       = 15 * time.Second
	maxCodingPlanQuotaResponseBytes = 1 << 20
)

type codingPlanProviderWindow struct {
	Name     string
	Used     float64
	ResetsAt *time.Time
	Aside    bool
}

type codingPlanProviderQuota struct {
	Provider string
	Plan     string
	Windows  []codingPlanProviderWindow
}

type codingPlanProviderSource struct {
	URL    string
	Bearer bool
	Parse  func([]byte) (string, []codingPlanProviderWindow, error)
}

type codingPlanOfficialCacheEntry struct {
	Key       [32]byte
	FetchedAt time.Time
	Quota     codingPlanProviderQuota
	Err       error
}

var codingPlanOfficialCache = struct {
	sync.Mutex
	entries map[int]codingPlanOfficialCacheEntry
}{entries: make(map[int]codingPlanOfficialCacheEntry)}

func UpdateOfficialCodingPlanQuotaChannels(channels []*model.Channel, now time.Time) {
	var wait sync.WaitGroup
	for _, channel := range channels {
		if channel == nil || channel.ChannelInfo.IsMultiKey {
			continue
		}
		if !hasOfficialCodingPlanQuota(channel) {
			continue
		}
		wait.Add(1)
		go func(channel *model.Channel) {
			defer wait.Done()
			status, err := EvaluateOfficialCodingPlanQuota(channel, now)
			if err != nil {
				common.SysLog(fmt.Sprintf("official coding plan quota check failed: channel_id=%d, error=%v", channel.Id, err))
				return
			}
			if status.Blocked {
				if channel.Status == common.ChannelStatusEnabled {
					DisableChannel(*types.NewChannelError(
						channel.Id,
						channel.Type,
						channel.Name,
						channel.ChannelInfo.IsMultiKey,
						"",
						channel.GetAutoBan(),
					), status.Reason)
				}
				return
			}
			if status.Available && IsOfficialCodingPlanQuotaAutoDisabled(channel) {
				EnableChannel(channel.Id, "", channel.Name)
			}
		}(channel)
	}
	wait.Wait()
}

type officialCodingPlanQuotaStatus struct {
	Available bool
	Blocked   bool
	Reason    string
}

func EvaluateOfficialCodingPlanQuota(channel *model.Channel, now time.Time) (officialCodingPlanQuotaStatus, error) {
	if channel == nil {
		return officialCodingPlanQuotaStatus{}, nil
	}
	if channel.Type == constant.ChannelTypeCodex {
		return evaluateCodexQuota(channel, now)
	}
	if channel.Type != constant.ChannelTypeAdvancedCustom {
		return officialCodingPlanQuotaStatus{}, nil
	}
	provider := strings.TrimSpace(channel.GetOtherSettings().CodingPlanProvider)
	if provider == dto.CodingPlanProviderDoubao || provider == dto.CodingPlanProviderVolcengineAgent {
		if _, ok := volcengineCredentialsFromEnv(); !ok {
			return officialCodingPlanQuotaStatus{}, nil
		}
		quota, err := fetchVolcengineQuota(channel, provider)
		if err != nil {
			return officialCodingPlanQuotaStatus{}, err
		}
		return quotaStatus(quota, now), nil
	}
	if provider == dto.CodingPlanProviderCommandCode {
		quota, err := fetchCommandCodeQuota(channel)
		if err != nil {
			return officialCodingPlanQuotaStatus{}, err
		}
		return quotaStatus(quota, now), nil
	}
	source, ok := codingPlanQuotaSource(provider)
	if !ok {
		return officialCodingPlanQuotaStatus{}, nil
	}
	key := strings.TrimSpace(channel.Key)
	if key == "" {
		return officialCodingPlanQuotaStatus{}, nil
	}
	quota, err := fetchCodingPlanProviderQuota(channel, source, key)
	if err != nil {
		return officialCodingPlanQuotaStatus{}, err
	}
	return quotaStatus(quota, now), nil
}

func hasOfficialCodingPlanQuota(channel *model.Channel) bool {
	if channel.Type == constant.ChannelTypeCodex {
		return true
	}
	if channel.Type != constant.ChannelTypeAdvancedCustom {
		return false
	}
	switch strings.TrimSpace(channel.GetOtherSettings().CodingPlanProvider) {
	case dto.CodingPlanProviderDoubao, dto.CodingPlanProviderVolcengineAgent, dto.CodingPlanProviderCommandCode:
		return true
	default:
		_, ok := codingPlanQuotaSource(strings.TrimSpace(channel.GetOtherSettings().CodingPlanProvider))
		return ok
	}
}

func IsOfficialCodingPlanQuotaAutoDisabled(channel *model.Channel) bool {
	if channel == nil || channel.Status != common.ChannelStatusAutoDisabled {
		return false
	}
	reason, _ := channel.GetOtherInfo()["status_reason"].(string)
	return strings.HasPrefix(reason, CodingPlanOfficialReasonPrefix)
}

func codingPlanQuotaSource(provider string) (codingPlanProviderSource, bool) {
	switch provider {
	case dto.CodingPlanProviderGLMChina:
		return codingPlanProviderSource{"https://open.bigmodel.cn/api/monitor/usage/quota/limit", false, parseGLMCodingPlanQuota}, true
	case dto.CodingPlanProviderGLMGlobal:
		return codingPlanProviderSource{"https://api.z.ai/api/monitor/usage/quota/limit", false, parseGLMCodingPlanQuota}, true
	case dto.CodingPlanProviderKimi:
		return codingPlanProviderSource{"https://api.kimi.com/coding/v1/usages", true, parseKimiCodingPlanQuota}, true
	case dto.CodingPlanProviderMiniMax:
		return codingPlanProviderSource{"https://api.minimax.io/v1/token_plan/remains", true, parseMiniMaxCodingPlanQuota}, true
	case dto.CodingPlanProviderOpenCodeGo:
		return codingPlanProviderSource{"https://opencode.ai/zen/go/v1/usage", true, parseOpenCodeCodingPlanQuota}, true
	default:
		return codingPlanProviderSource{}, false
	}
}

func fetchCodingPlanProviderQuota(channel *model.Channel, source codingPlanProviderSource, key string) (codingPlanProviderQuota, error) {
	return fetchCachedCodingPlanQuota(channel, source.URL+"\x00"+key, func() (codingPlanProviderQuota, error) {
		client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
		if err != nil {
			return codingPlanProviderQuota{}, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), codingPlanOfficialTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
		if err != nil {
			return codingPlanProviderQuota{}, err
		}
		if source.Bearer {
			req.Header.Set("Authorization", "Bearer "+key)
		} else {
			req.Header.Set("Authorization", key)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Accept-Language", "en-US,en")
		resp, err := client.Do(req)
		if err != nil {
			return codingPlanProviderQuota{}, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxCodingPlanQuotaResponseBytes+1))
		if err != nil {
			return codingPlanProviderQuota{}, err
		}
		if len(body) > maxCodingPlanQuotaResponseBytes {
			return codingPlanProviderQuota{}, fmt.Errorf("coding plan quota response exceeds %d bytes", maxCodingPlanQuotaResponseBytes)
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return codingPlanProviderQuota{}, fmt.Errorf("coding plan quota endpoint returned %s", resp.Status)
		}
		plan, windows, err := source.Parse(body)
		quota := codingPlanProviderQuota{Provider: channel.GetOtherSettings().CodingPlanProvider, Plan: plan, Windows: windows}
		if err == nil && len(windows) == 0 {
			err = fmt.Errorf("coding plan quota endpoint returned no windows")
		}
		return quota, err
	})
}

func fetchCachedCodingPlanQuota(channel *model.Channel, cacheMaterial string, fetch func() (codingPlanProviderQuota, error)) (codingPlanProviderQuota, error) {
	hash := sha256.Sum256([]byte(cacheMaterial))
	codingPlanOfficialCache.Lock()
	if cached, ok := codingPlanOfficialCache.entries[channel.Id]; ok && cached.Key == hash && time.Since(cached.FetchedAt) < codingPlanOfficialCacheTTL {
		codingPlanOfficialCache.Unlock()
		return cached.Quota, cached.Err
	}
	codingPlanOfficialCache.Unlock()

	quota, err := fetch()
	codingPlanOfficialCache.Lock()
	codingPlanOfficialCache.entries[channel.Id] = codingPlanOfficialCacheEntry{Key: hash, FetchedAt: time.Now(), Quota: quota, Err: err}
	codingPlanOfficialCache.Unlock()
	return quota, err
}

type volcengineCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
}

func volcengineCredentialsFromEnv() (volcengineCredentials, bool) {
	accessKeyID := firstNonEmptyEnv("VOLC_ACCESS_KEY_ID", "VOLC_ACCESSKEY", "VOLC_ACCESS_KEY")
	secretAccessKey := firstNonEmptyEnv("VOLC_SECRET_ACCESS_KEY", "VOLC_SECRETKEY", "VOLC_SECRET_KEY")
	if accessKeyID == "" || secretAccessKey == "" {
		return volcengineCredentials{}, false
	}
	return volcengineCredentials{AccessKeyID: accessKeyID, SecretAccessKey: secretAccessKey}, true
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

const (
	volcengineOpenAPIHost      = "open.volcengineapi.com"
	volcengineAPIService       = "ark"
	volcengineAPIVersion       = "2024-01-01"
	volcengineSignedHeaders    = "host;x-date;x-content-sha256;content-type"
	volcengineContentType      = "application/json; charset=utf-8"
	volcengineDefaultRegion    = "cn-beijing"
	volcengineCodingPlanAction = "GetCodingPlanUsage"
	volcengineAgentPlanAction  = "GetAFPUsage"
)

func volcengineRegion(baseURL string) string {
	withoutScheme := strings.TrimSpace(baseURL)
	if parts := strings.SplitN(withoutScheme, "://", 2); len(parts) == 2 {
		withoutScheme = parts[1]
	}
	host := strings.SplitN(withoutScheme, "/", 2)[0]
	for _, part := range strings.Split(host, ".") {
		if strings.HasPrefix(part, "cn-") || strings.HasPrefix(part, "ap-") {
			return part
		}
	}
	return volcengineDefaultRegion
}

func volcHMAC(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}

func volcSHA256Hex(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func volcURIEncode(value string) string {
	const hex = "0123456789ABCDEF"
	var builder strings.Builder
	for i := 0; i < len(value); i++ {
		b := value[i]
		if b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' || b == '_' || b == '.' || b == '~' {
			builder.WriteByte(b)
			continue
		}
		builder.WriteByte('%')
		builder.WriteByte(hex[b>>4])
		builder.WriteByte(hex[b&15])
	}
	return builder.String()
}

func volcengineCanonicalQuery(action, region string) string {
	return "Action=" + volcURIEncode(action) + "&Region=" + volcURIEncode(region) + "&Version=" + volcURIEncode(volcengineAPIVersion)
}

func volcengineSign(accessKeyID, secretAccessKey, region, canonicalQuery string, body []byte, now time.Time) (string, string, string) {
	xDate := now.UTC().Format("20060102T150405Z")
	shortDate := now.UTC().Format("20060102")
	bodyHash := volcSHA256Hex(body)
	canonicalHeaders := fmt.Sprintf("host:%s\nx-date:%s\nx-content-sha256:%s\ncontent-type:%s\n", volcengineOpenAPIHost, xDate, bodyHash, volcengineContentType)
	canonicalRequest := fmt.Sprintf("POST\n/\n%s\n%s\n%s\n%s", canonicalQuery, canonicalHeaders, volcengineSignedHeaders, bodyHash)
	scope := shortDate + "/" + region + "/" + volcengineAPIService + "/request"
	stringToSign := "HMAC-SHA256\n" + xDate + "\n" + scope + "\n" + volcSHA256Hex([]byte(canonicalRequest))
	kDate := volcHMAC([]byte(secretAccessKey), []byte(shortDate))
	kRegion := volcHMAC(kDate, []byte(region))
	kService := volcHMAC(kRegion, []byte(volcengineAPIService))
	kSigning := volcHMAC(kService, []byte("request"))
	signature := fmt.Sprintf("%x", volcHMAC(kSigning, []byte(stringToSign)))
	authorization := fmt.Sprintf("HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", accessKeyID, scope, volcengineSignedHeaders, signature)
	return authorization, xDate, bodyHash
}

func fetchVolcengineQuota(channel *model.Channel, provider string) (codingPlanProviderQuota, error) {
	credentials, ok := volcengineCredentialsFromEnv()
	if !ok {
		return codingPlanProviderQuota{}, nil
	}
	action := volcengineCodingPlanAction
	if provider == dto.CodingPlanProviderVolcengineAgent {
		action = volcengineAgentPlanAction
	}
	cacheMaterial := "volcengine:" + provider + ":" + credentials.AccessKeyID + "\x00" + credentials.SecretAccessKey
	return fetchCachedCodingPlanQuota(channel, cacheMaterial, func() (codingPlanProviderQuota, error) {
		body, err := fetchVolcengineOpenAPI(channel, credentials, action)
		if err != nil {
			return codingPlanProviderQuota{}, err
		}
		var plan string
		var windows []codingPlanProviderWindow
		if action == volcengineAgentPlanAction {
			plan, windows, err = parseVolcengineAgentQuota(body)
		} else {
			plan, windows, err = parseVolcengineCodingQuota(body)
		}
		if err == nil && len(windows) == 0 {
			err = fmt.Errorf("Volcengine %s returned no quota windows", action)
		}
		return codingPlanProviderQuota{Provider: provider, Plan: plan, Windows: windows}, err
	})
}

func fetchVolcengineOpenAPI(channel *model.Channel, credentials volcengineCredentials, action string) ([]byte, error) {
	region := volcengineRegion(channel.GetBaseURL())
	query := volcengineCanonicalQuery(action, region)
	body := []byte{}
	authorization, xDate, bodyHash := volcengineSign(credentials.AccessKeyID, credentials.SecretAccessKey, region, query, body, time.Now())
	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), codingPlanOfficialTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+volcengineOpenAPIHost+"/?"+query, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Date", xDate)
	req.Header.Set("X-Content-Sha256", bodyHash)
	req.Header.Set("Content-Type", volcengineContentType)
	req.Header.Set("Authorization", authorization)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxCodingPlanQuotaResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(responseBody) > maxCodingPlanQuotaResponseBytes {
		return nil, fmt.Errorf("Volcengine quota response exceeds %d bytes", maxCodingPlanQuotaResponseBytes)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Volcengine %s returned %s", action, resp.Status)
	}
	var envelope struct {
		ResponseMetadata struct {
			Error *struct {
				Code string `json:"Code"`
			} `json:"Error"`
		} `json:"ResponseMetadata"`
	}
	if err := common.Unmarshal(responseBody, &envelope); err == nil && envelope.ResponseMetadata.Error != nil {
		code := strings.TrimSpace(envelope.ResponseMetadata.Error.Code)
		if code == "" {
			code = "unknown error"
		}
		return nil, fmt.Errorf("Volcengine %s failed: %s", action, code)
	}
	return responseBody, nil
}

func parseVolcengineAgentQuota(body []byte) (string, []codingPlanProviderWindow, error) {
	var envelope map[string]any
	if err := common.UnmarshalUseNumber(body, &envelope); err != nil {
		return "", nil, err
	}
	result := mapValue(envelope, "Result")
	windows := make([]codingPlanProviderWindow, 0, 3)
	for _, item := range []struct {
		key  string
		name string
	}{{"AFPFiveHour", "5 hours"}, {"AFPWeekly", "7 days"}, {"AFPMonthly", "Month"}} {
		window := mapValue(result, item.key)
		quota, ok := quotaNumber(window["Quota"])
		if !ok || quota <= 0 {
			continue
		}
		used, _ := quotaNumber(window["Used"])
		parsed := codingPlanProviderWindow{Name: item.name, Used: clampPercent(used / quota * 100)}
		if reset := quotaResetTime(window["ResetTime"]); !reset.IsZero() {
			parsed.ResetsAt = &reset
		}
		windows = append(windows, parsed)
	}
	plan, _ := result["PlanType"].(string)
	return strings.TrimSpace(plan), windows, nil
}

func parseVolcengineCodingQuota(body []byte) (string, []codingPlanProviderWindow, error) {
	var envelope map[string]any
	if err := common.UnmarshalUseNumber(body, &envelope); err != nil {
		return "", nil, err
	}
	result := mapValue(envelope, "Result")
	items, _ := result["QuotaUsage"].([]any)
	windows := make([]codingPlanProviderWindow, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		level, _ := item["Level"].(string)
		name := ""
		switch strings.ToLower(strings.TrimSpace(level)) {
		case "session", "5h", "fivehour", "five_hour", "rolling_5h":
			name = "5 hours"
		case "weekly", "week", "7d":
			name = "7 days"
		case "monthly", "month":
			name = "Month"
		default:
			continue
		}
		percent, _ := quotaNumber(item["Percent"])
		parsed := codingPlanProviderWindow{Name: name, Used: clampPercent(percent)}
		if reset := quotaResetTime(item["ResetTimestamp"]); !reset.IsZero() {
			parsed.ResetsAt = &reset
		}
		windows = append(windows, parsed)
	}
	return "Coding Plan", windows, nil
}

func mapValue(values map[string]any, key string) map[string]any {
	value, _ := values[key].(map[string]any)
	return value
}

func quotaResetTime(value any) time.Time {
	if number, ok := quotaNumber(value); ok && number > 0 {
		if number >= 1e12 {
			return time.UnixMilli(int64(number))
		}
		return time.Unix(int64(number), 0)
	}
	if text, ok := value.(string); ok {
		parsed, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(text))
		return parsed
	}
	return time.Time{}
}

func fetchCommandCodeQuota(channel *model.Channel) (codingPlanProviderQuota, error) {
	// ponytail: the official Provider API does not expose quota metadata; reuse
	// the CLI's alpha endpoints and fail open if they change.
	key := strings.TrimSpace(channel.Key)
	if key == "" {
		return codingPlanProviderQuota{}, nil
	}
	return fetchCachedCodingPlanQuota(channel, "command-code:\x00"+key, func() (codingPlanProviderQuota, error) {
		client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
		if err != nil {
			return codingPlanProviderQuota{}, err
		}
		fetch := func(path string, params url.Values) (map[string]any, error) {
			endpoint := "https://api.commandcode.ai" + path
			if len(params) > 0 {
				endpoint += "?" + params.Encode()
			}
			ctx, cancel := context.WithTimeout(context.Background(), codingPlanOfficialTimeout)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("Accept", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(io.LimitReader(resp.Body, maxCodingPlanQuotaResponseBytes+1))
			if err != nil {
				return nil, err
			}
			if len(body) > maxCodingPlanQuotaResponseBytes {
				return nil, fmt.Errorf("Command Code response exceeds %d bytes", maxCodingPlanQuotaResponseBytes)
			}
			if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
				return nil, fmt.Errorf("Command Code endpoint returned %s", resp.Status)
			}
			var parsed map[string]any
			if err := common.UnmarshalUseNumber(body, &parsed); err != nil {
				return nil, err
			}
			return parsed, nil
		}

		whoami, err := fetch("/alpha/whoami", url.Values{"limits": {"1"}})
		if err != nil {
			return codingPlanProviderQuota{}, err
		}
		orgID := ""
		if org, ok := whoami["org"].(map[string]any); ok {
			orgID, _ = org["id"].(string)
		}
		orgParams := url.Values{}
		if orgID != "" {
			orgParams.Set("orgId", orgID)
		}
		credits, err := fetch("/alpha/billing/credits", orgParams)
		if err != nil {
			return codingPlanProviderQuota{}, err
		}
		subscription, err := fetch("/alpha/billing/subscriptions", orgParams)
		if err != nil {
			return codingPlanProviderQuota{}, err
		}
		summaryParams := url.Values{}
		if data, ok := subscription["data"].(map[string]any); ok {
			if periodStart, ok := data["currentPeriodStart"].(string); ok && periodStart != "" {
				summaryParams.Set("since", periodStart)
			}
		}
		if orgID != "" {
			summaryParams.Set("orgId", orgID)
		}
		summary, err := fetch("/alpha/usage/summary", summaryParams)
		if err != nil {
			return codingPlanProviderQuota{}, err
		}
		plan, windows, err := parseCommandCodeQuota(credits, subscription, summary)
		return codingPlanProviderQuota{Provider: dto.CodingPlanProviderCommandCode, Plan: plan, Windows: windows}, err
	})
}

func parseCommandCodeQuota(credits, subscription, summary map[string]any) (string, []codingPlanProviderWindow, error) {
	creditRoot := mapValue(credits, "credits")
	if len(creditRoot) == 0 {
		return "", nil, fmt.Errorf("Command Code response missing credits")
	}
	remaining := 0.0
	for _, key := range []string{"monthlyCredits", "purchasedCredits", "freeCredits"} {
		value, _ := quotaNumber(creditRoot[key])
		if value > 0 {
			remaining += value
		}
	}
	totalSpent, ok := quotaNumber(summary["totalCost"])
	if !ok || totalSpent < 0 {
		return "", nil, fmt.Errorf("Command Code response missing totalCost")
	}
	windows := make([]codingPlanProviderWindow, 0, 3)
	windowLimits := mapValue(credits, "windowLimits")
	if len(windowLimits) == 0 {
		windowLimits = mapValue(creditRoot, "windowLimits")
	}
	if limited, _ := windowLimits["limited"].(bool); limited {
		for _, item := range []struct {
			key  string
			name string
		}{{"fiveHour", "5 hours"}, {"weekly", "7 days"}} {
			window := mapValue(windowLimits, item.key)
			used, usedOK := quotaNumber(window["used"])
			cap, capOK := quotaNumber(window["cap"])
			if !usedOK || !capOK || cap <= 0 {
				continue
			}
			parsed := codingPlanProviderWindow{Name: item.name, Used: clampPercent(used / cap * 100)}
			if reset := quotaResetTime(window["resetAt"]); !reset.IsZero() {
				parsed.ResetsAt = &reset
			}
			windows = append(windows, parsed)
		}
	}
	periodEnd := time.Time{}
	if data, ok := subscription["data"].(map[string]any); ok {
		periodEnd = quotaResetTime(data["currentPeriodEnd"])
	}
	monthlyPercent := 0.0
	if total := totalSpent + remaining; total > 0 {
		monthlyPercent = clampPercent(totalSpent / total * 100)
	}
	windows = append(windows, codingPlanProviderWindow{Name: "Month", Used: monthlyPercent, ResetsAt: timePtr(periodEnd)})
	plan, _ := creditRoot["planId"].(string)
	if plan == "" {
		if data, ok := subscription["data"].(map[string]any); ok {
			plan, _ = data["planId"].(string)
		}
	}
	return plan, windows, nil
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func evaluateCodexQuota(channel *model.Channel, now time.Time) (officialCodingPlanQuotaStatus, error) {
	hash := sha256.Sum256([]byte("codex:" + channel.GetBaseURL() + "\x00" + channel.Key))
	codingPlanOfficialCache.Lock()
	if cached, ok := codingPlanOfficialCache.entries[channel.Id]; ok && cached.Key == hash && time.Since(cached.FetchedAt) < codingPlanOfficialCacheTTL {
		codingPlanOfficialCache.Unlock()
		return quotaStatus(cached.Quota, now), cached.Err
	}
	codingPlanOfficialCache.Unlock()

	var oauthKey map[string]any
	err := common.Unmarshal([]byte(strings.TrimSpace(channel.Key)), &oauthKey)
	if err != nil {
		return officialCodingPlanQuotaStatus{}, err
	}
	accessToken, _ := oauthKey["access_token"].(string)
	accountID, _ := oauthKey["account_id"].(string)
	refreshToken, _ := oauthKey["refresh_token"].(string)
	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return officialCodingPlanQuotaStatus{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), codingPlanOfficialTimeout)
	defer cancel()
	statusCode, body, err := FetchCodexWhamUsage(ctx, client, channel.GetBaseURL(), accessToken, accountID)
	if err != nil {
		return officialCodingPlanQuotaStatus{}, err
	}
	if (statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden) && strings.TrimSpace(refreshToken) != "" {
		refreshCtx, refreshCancel := context.WithTimeout(context.Background(), 10*time.Second)
		refreshed, refreshErr := RefreshCodexOAuthTokenWithProxy(refreshCtx, refreshToken, channel.GetSetting().Proxy)
		refreshCancel()
		if refreshErr == nil {
			oauthKey["access_token"] = refreshed.AccessToken
			oauthKey["refresh_token"] = refreshed.RefreshToken
			if encoded, encodeErr := common.Marshal(oauthKey); encodeErr == nil {
				if model.DB != nil {
					_ = model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("key", string(encoded)).Error
					model.InitChannelCache()
				}
			}
			statusCode, body, err = FetchCodexWhamUsage(ctx, client, channel.GetBaseURL(), refreshed.AccessToken, accountID)
			if err != nil {
				return officialCodingPlanQuotaStatus{}, err
			}
		}
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return officialCodingPlanQuotaStatus{}, fmt.Errorf("codex quota endpoint returned status %d", statusCode)
	}
	plan, windows, err := parseCodexQuota(body)
	if err != nil {
		return officialCodingPlanQuotaStatus{}, err
	}
	quota := codingPlanProviderQuota{Provider: "codex", Plan: plan, Windows: windows}
	codingPlanOfficialCache.Lock()
	codingPlanOfficialCache.entries[channel.Id] = codingPlanOfficialCacheEntry{Key: hash, FetchedAt: time.Now(), Quota: quota}
	codingPlanOfficialCache.Unlock()
	return quotaStatus(quota, now), nil
}

func quotaStatus(quota codingPlanProviderQuota, now time.Time) officialCodingPlanQuotaStatus {
	for _, window := range quota.Windows {
		if window.ResetsAt != nil && !now.Before(*window.ResetsAt) {
			continue
		}
		if !window.Aside && window.Used >= 100 {
			return officialCodingPlanQuotaStatus{
				Available: true,
				Blocked:   true,
				Reason: fmt.Sprintf(
					"%s%s window reached (%.0f%%)",
					CodingPlanOfficialReasonPrefix,
					quota.Provider,
					window.Used,
				),
			}
		}
	}
	return officialCodingPlanQuotaStatus{Available: true}
}

func parseGLMCodingPlanQuota(body []byte) (string, []codingPlanProviderWindow, error) {
	var response struct {
		Success *bool  `json:"success"`
		Message string `json:"msg"`
		Data    *struct {
			Level  string `json:"level"`
			Limits []struct {
				Type          string   `json:"type"`
				Unit          int      `json:"unit"`
				Number        int      `json:"number"`
				Percentage    *float64 `json:"percentage"`
				NextResetTime int64    `json:"nextResetTime"`
			} `json:"limits"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return "", nil, err
	}
	if response.Success != nil && !*response.Success || response.Data == nil {
		if response.Message != "" {
			return "", nil, fmt.Errorf("%s", response.Message)
		}
		return "", nil, fmt.Errorf("no coding plan in response")
	}
	windows := make([]codingPlanProviderWindow, 0, len(response.Data.Limits))
	for _, limit := range response.Data.Limits {
		used := 0.0
		if limit.Percentage != nil {
			used = clampPercent(*limit.Percentage)
		}
		window := codingPlanProviderWindow{Used: used}
		if limit.NextResetTime > 0 {
			reset := time.UnixMilli(limit.NextResetTime)
			window.ResetsAt = &reset
		}
		switch {
		case strings.EqualFold(limit.Type, "TIME_LIMIT"):
			window.Name, window.Aside = "MCP · Month", true
		case limit.Unit == 3:
			hours := limit.Number
			if hours <= 0 {
				hours = 5
			}
			window.Name = fmt.Sprintf("%d hours", hours)
		case limit.Unit == 6:
			window.Name = "7 days"
		default:
			window.Name = "Allowance"
		}
		windows = append(windows, window)
	}
	return response.Data.Level, windows, nil
}

func parseKimiCodingPlanQuota(body []byte) (string, []codingPlanProviderWindow, error) {
	var response struct {
		Usage  map[string]any `json:"usage"`
		Limits []struct {
			Window map[string]any `json:"window"`
			Detail map[string]any `json:"detail"`
		} `json:"limits"`
	}
	if err := common.UnmarshalUseNumber(body, &response); err != nil {
		return "", nil, err
	}
	windows := make([]codingPlanProviderWindow, 0, len(response.Limits)+1)
	for _, limit := range response.Limits {
		detail := limit.Detail
		if len(detail) == 0 {
			detail = limit.Window
		}
		window, ok := kimiWindow(detail, limit.Window)
		if ok {
			windows = append(windows, window)
		}
	}
	if window, ok := kimiWindow(response.Usage, nil); ok {
		window.Name = "7 days"
		windows = append(windows, window)
	}
	if len(windows) == 0 {
		return "", nil, fmt.Errorf("no Kimi coding plan usage in response")
	}
	return "", windows, nil
}

func kimiWindow(detail, descriptor map[string]any) (codingPlanProviderWindow, bool) {
	limit, ok := quotaNumber(detail["limit"])
	if !ok || limit <= 0 {
		return codingPlanProviderWindow{}, false
	}
	used, ok := quotaNumber(detail["used"])
	if !ok {
		remaining, remainingOK := quotaNumber(detail["remaining"])
		if !remainingOK {
			return codingPlanProviderWindow{}, false
		}
		used = limit - remaining
	}
	window := codingPlanProviderWindow{Used: clampPercent(used / limit * 100)}
	if descriptor != nil {
		duration, _ := quotaNumber(descriptor["duration"])
		unit, _ := descriptor["timeUnit"].(string)
		window.Name = "Allowance"
		switch {
		case strings.Contains(unit, "MINUTE"):
			window.Name = fmt.Sprintf("%d minutes", int64(duration))
		case strings.Contains(unit, "HOUR"):
			window.Name = fmt.Sprintf("%d hours", int64(duration))
		case strings.Contains(unit, "DAY"):
			window.Name = fmt.Sprintf("%d days", int64(duration))
		}
	}
	if reset := quotaTime(detail); !reset.IsZero() {
		window.ResetsAt = &reset
	}
	return window, true
}

func parseMiniMaxCodingPlanQuota(body []byte) (string, []codingPlanProviderWindow, error) {
	var response struct {
		Remains []struct {
			Model      string   `json:"model_name"`
			Start      int64    `json:"start_time"`
			End        int64    `json:"end_time"`
			Left       *float64 `json:"current_interval_remaining_percent"`
			Status     int      `json:"current_interval_status"`
			Total      *float64 `json:"current_interval_total_count"`
			WeekStart  int64    `json:"weekly_start_time"`
			WeekEnd    int64    `json:"weekly_end_time"`
			WeekLeft   *float64 `json:"current_weekly_remaining_percent"`
			WeekStatus int      `json:"current_weekly_status"`
			WeekTotal  *float64 `json:"current_weekly_total_count"`
		} `json:"model_remains"`
		Base *struct {
			Code int    `json:"status_code"`
			Msg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return "", nil, err
	}
	if response.Base == nil {
		return "", nil, fmt.Errorf("no MiniMax coding plan in response")
	}
	if response.Base.Code != 0 {
		if response.Base.Msg != "" {
			return "", nil, fmt.Errorf("%s", response.Base.Msg)
		}
		return "", nil, fmt.Errorf("MiniMax returned status %d", response.Base.Code)
	}
	windows := []codingPlanProviderWindow{}
	for _, remain := range response.Remains {
		name := strings.TrimSpace(remain.Model)
		if name == "" || remain.Status == 3 && remain.WeekStatus == 3 && zeroQuota(remain.Total) && zeroQuota(remain.WeekTotal) {
			continue
		}
		general := strings.EqualFold(name, "general")
		for _, item := range []struct {
			left       *float64
			status     int
			start, end int64
			weekly     bool
		}{{remain.Left, remain.Status, remain.Start, remain.End, false}, {remain.WeekLeft, remain.WeekStatus, remain.WeekStart, remain.WeekEnd, true}} {
			if item.status == 3 || item.left == nil && item.status != 2 {
				continue
			}
			used := 100.0
			if item.status != 2 {
				used = clampPercent(100 - *item.left)
			}
			window := codingPlanProviderWindow{Used: used, Aside: !general}
			if item.weekly {
				window.Name = "7 days"
			} else {
				window.Name = "Allowance"
			}
			if item.end > 0 {
				reset := quotaUnixTime(item.end)
				window.ResetsAt = &reset
			}
			windows = append(windows, window)
		}
	}
	return "", windows, nil
}

func parseOpenCodeCodingPlanQuota(body []byte) (string, []codingPlanProviderWindow, error) {
	var response struct {
		Usage map[string]struct {
			Percent  *float64 `json:"percent"`
			ResetsAt string   `json:"resetsAt"`
		} `json:"usage"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return "", nil, err
	}
	windows := []codingPlanProviderWindow{}
	for _, item := range []struct {
		key, name string
		aside     bool
	}{{"rolling", "5 hours", false}, {"weekly", "7 days", false}, {"monthly", "Month", false}} {
		usage, ok := response.Usage[item.key]
		if !ok || usage.Percent == nil {
			continue
		}
		window := codingPlanProviderWindow{Name: item.name, Used: clampPercent(*usage.Percent), Aside: item.aside}
		if reset, err := time.Parse(time.RFC3339, usage.ResetsAt); err == nil && *usage.Percent > 0 {
			window.ResetsAt = &reset
		}
		windows = append(windows, window)
	}
	if len(windows) == 0 {
		return "", nil, fmt.Errorf("no OpenCode Go usage in response")
	}
	return "", windows, nil
}

func parseCodexQuota(body []byte) (string, []codingPlanProviderWindow, error) {
	type codexWindow struct {
		UsedPercent float64 `json:"used_percent"`
		ResetAt     int64   `json:"reset_at"`
	}
	var response struct {
		PlanType  string `json:"plan_type"`
		RateLimit struct {
			Primary   *codexWindow `json:"primary_window"`
			Secondary *codexWindow `json:"secondary_window"`
		} `json:"rate_limit"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return "", nil, err
	}
	windows := []codingPlanProviderWindow{}
	for _, item := range []struct {
		name   string
		window *codexWindow
	}{{"primary", response.RateLimit.Primary}, {"secondary", response.RateLimit.Secondary}} {
		if item.window == nil {
			continue
		}
		window := codingPlanProviderWindow{Name: item.name, Used: clampPercent(item.window.UsedPercent)}
		if item.window.ResetAt > 0 {
			reset := time.Unix(item.window.ResetAt, 0)
			window.ResetsAt = &reset
		}
		windows = append(windows, window)
	}
	if len(windows) == 0 {
		return "", nil, fmt.Errorf("no Codex rate-limit windows in response")
	}
	return response.PlanType, windows, nil
}
func quotaNumber(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint64:
		return float64(value), true
	case json.Number:
		result, err := value.Float64()
		return result, err == nil
	case string:
		result, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return result, err == nil
	default:
		return 0, false
	}
}

func quotaTime(values map[string]any) time.Time {
	for _, key := range []string{"resetTime", "resetAt", "reset_at", "reset_time"} {
		if value, ok := values[key].(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func quotaUnixTime(value int64) time.Time {
	if value >= 1e12 {
		return time.UnixMilli(value)
	}
	return time.Unix(value, 0)
}

func zeroQuota(value *float64) bool { return value != nil && *value == 0 }

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
