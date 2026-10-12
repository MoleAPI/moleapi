package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

func shouldCloseActiveWebSocketsAfterDisable(channelId int) bool {
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to check channel status before closing active websockets: channel_id=%d, error=%v", channelId, err))
		return true
	}
	return channel.Status != common.ChannelStatusEnabled
}

// disable & notify
func DisableChannel(channelError types.ChannelError, reason string) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, common.LocalLogPreview(reason)))

	// 检查是否启用自动禁用功能
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		if shouldCloseActiveWebSocketsAfterDisable(channelError.ChannelId) {
			CloseActiveWebSocketsForChannel(channelError.ChannelId, ChannelDisabledCloseReason)
		}
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
	}
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

func ShouldDisableChannel(err *types.NewAPIError) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	// A plain 429 is traffic throttling, not proof that a plan quota is spent.
	if err.StatusCode == 429 {
		return isQuotaExhaustedError(err)
	}
	if operation_setting.ShouldDisableByStatusCode(err.StatusCode) {
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	return search || isQuotaExhaustedError(err)
}

func isQuotaExhaustedError(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if isQuotaExhaustedMessage(strings.ToLower(err.Error())) || isQuotaExhaustedMessage(strings.ToLower(string(err.GetErrorCode()))) {
		return true
	}
	return len(err.Metadata) > 0 && isQuotaExhaustedMessage(strings.ToLower(string(err.Metadata)))
}

func isQuotaExhaustedMessage(message string) bool {
	for _, marker := range []string{
		"quota exceeded",
		"quota_exceeded",
		"quotaexceeded",
		"quota exhausted",
		"quota_exhausted",
		"quotaexhausted",
		"quota depleted",
		"quota_depleted",
		"call count exceeded quota",
		"insufficient quota",
		"insufficient_quota",
		"insufficientquota",
		"insufficient balance",
		"credit balance is too low",
		"no credits remaining",
		"usage quota exceeded",
		"usage quota exhausted",
		"usage limit exceeded",
		"usage limit reached",
		"daily usage limit reached",
		"weekly usage limit reached",
		"monthly usage limit reached",
		"reached your daily usage limit",
		"reached your weekly usage limit",
		"reached your monthly usage limit",
		"subscription expired",
		"not subscribed",
		"调用次数超出配额",
		"配额耗尽",
		"额度耗尽",
		"套餐额度用尽",
		"套餐额度已用尽",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func quotaResetAtFromError(err *types.NewAPIError, now time.Time) *time.Time {
	if err == nil || len(err.Metadata) == 0 {
		return nil
	}
	var metadata any
	if common.UnmarshalUseNumber(err.Metadata, &metadata) != nil {
		return nil
	}
	var walk func(any) *time.Time
	walk = func(value any) *time.Time {
		switch value := value.(type) {
		case map[string]any:
			for key, item := range value {
				normalized := strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.ToLower(key))
				if normalized == "resetat" || normalized == "resettime" || normalized == "resetsat" || normalized == "retryafter" {
					if parsed := parseQuotaResetTime(item, normalized == "retryafter", now); parsed != nil {
						return parsed
					}
				}
			}
			for _, item := range value {
				if parsed := walk(item); parsed != nil {
					return parsed
				}
			}
		case []any:
			for _, item := range value {
				if parsed := walk(item); parsed != nil {
					return parsed
				}
			}
		}
		return nil
	}
	return walk(metadata)
}

func parseQuotaResetTime(value any, retryAfter bool, now time.Time) *time.Time {
	var number float64
	switch value := value.(type) {
	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			return nil
		}
		number = parsed
	case float64:
		number = value
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value)); err == nil && parsed.After(now) {
			return &parsed
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return nil
		}
		number = parsed
	default:
		return nil
	}

	var parsed time.Time
	if retryAfter && number < 1e9 {
		parsed = now.Add(time.Duration(number * float64(time.Second)))
	} else if number >= 1e12 {
		parsed = time.UnixMilli(int64(number))
	} else {
		parsed = time.Unix(int64(number), 0)
	}
	if !parsed.After(now) {
		return nil
	}
	return &parsed
}

func ShouldConfirmModelFailure(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	switch err.GetErrorCode() {
	case types.ErrorCodeModelNotFound,
		types.ErrorCodeBadResponseStatusCode,
		types.ErrorCodeBadResponse,
		types.ErrorCodeBadResponseBody,
		types.ErrorCodeEmptyResponse,
		types.ErrorCodeReadResponseBodyFailed:
		return err.StatusCode == 404 || err.StatusCode >= 500
	default:
		return err.StatusCode == 404
	}
}

func ShouldEnableChannel(newAPIError *types.NewAPIError, status int) bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}
