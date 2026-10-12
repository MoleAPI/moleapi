package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/bytedance/gopkg/util/gopool"
)

const CodingPlanQuotaReasonPrefix = "coding_plan_quota:"

var ErrCodingPlanQuotaLogDBUnavailable = errors.New("coding plan quota log database is unavailable")

type CodingPlanQuotaStatus struct {
	Blocked bool
	Reason  string
	ResetAt *time.Time
}

// EvaluateCodingPlanQuota reads existing consume logs instead of adding a
// second usage ledger.
func EvaluateCodingPlanQuota(channel *model.Channel, now time.Time) (CodingPlanQuotaStatus, error) {
	if channel == nil || channel.Type != constant.ChannelTypeAdvancedCustom {
		return CodingPlanQuotaStatus{}, nil
	}
	config := channel.GetOtherSettings().CodingPlanQuota
	if config == nil {
		return CodingPlanQuotaStatus{}, nil
	}
	if err := config.Validate(); err != nil {
		return CodingPlanQuotaStatus{}, err
	}
	unit := strings.TrimSpace(config.Unit)
	if model.LOG_DB == nil {
		return CodingPlanQuotaStatus{}, ErrCodingPlanQuotaLogDBUnavailable
	}

	status := CodingPlanQuotaStatus{}
	for _, window := range config.Windows {
		cutoff := now.Add(-time.Duration(window.DurationSeconds) * time.Second).Unix()
		usage, err := codingPlanWindowUsage(channel.Id, unit, cutoff)
		if err != nil {
			return CodingPlanQuotaStatus{}, err
		}
		if usage >= window.Limit {
			resetAt, err := codingPlanWindowResetAt(channel.Id, unit, cutoff, window.DurationSeconds, usage, window.Limit)
			if err != nil {
				return CodingPlanQuotaStatus{}, err
			}
			if !status.Blocked {
				status.Blocked = true
				status.Reason = fmt.Sprintf(
					"%s%s window reached (%d/%d)",
					CodingPlanQuotaReasonPrefix,
					unit,
					usage,
					window.Limit,
				)
			}
			if resetAt != nil && (status.ResetAt == nil || resetAt.After(*status.ResetAt)) {
				status.ResetAt = resetAt
			}
		}
	}
	if status.ResetAt != nil {
		status.Reason += fmt.Sprintf("; reset_at=%d", status.ResetAt.Unix())
	}
	return status, nil
}

func codingPlanWindowUsage(channelID int, unit string, cutoff int64) (int64, error) {
	query := model.LOG_DB.Model(&model.Log{}).
		Where("channel_id = ? AND type = ? AND created_at >= ?", channelID, model.LogTypeConsume, cutoff)
	if unit == dto.CodingPlanQuotaUnitRequests {
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return 0, err
		}
		return count, nil
	}

	var tokens int64
	if err := query.Select("COALESCE(SUM(prompt_tokens + completion_tokens), 0)").Scan(&tokens).Error; err != nil {
		return 0, err
	}
	return tokens, nil
}

func codingPlanWindowResetAt(channelID int, unit string, cutoff, durationSeconds, usage, limit int64) (*time.Time, error) {
	query := model.LOG_DB.Model(&model.Log{}).
		Where("channel_id = ? AND type = ? AND created_at >= ?", channelID, model.LogTypeConsume, cutoff).
		Order("created_at ASC")
	if unit == dto.CodingPlanQuotaUnitRequests {
		var log model.Log
		if err := query.Select("created_at").Offset(int(usage - limit)).Limit(1).Take(&log).Error; err != nil {
			return nil, err
		}
		resetAt := time.Unix(log.CreatedAt+durationSeconds, 0)
		return &resetAt, nil
	}

	var logs []model.Log
	if err := query.Select("created_at", "prompt_tokens", "completion_tokens").Find(&logs).Error; err != nil {
		return nil, err
	}
	needToExpire := usage - limit
	var expired int64
	for _, log := range logs {
		expired += int64(log.PromptTokens + log.CompletionTokens)
		if expired > needToExpire {
			resetAt := time.Unix(log.CreatedAt+durationSeconds, 0)
			return &resetAt, nil
		}
	}
	return nil, nil
}

// CheckCodingPlanQuotaAfterConsume makes local plan limits react to the request
// that reaches the cap. The fast checks keep ordinary channels off this path.
// ponytail: concurrent in-flight requests may exceed the cap briefly; use a
// distributed pre-request reservation only if strict admission becomes necessary.
func CheckCodingPlanQuotaAfterConsume(channelID, channelType int, config *dto.CodingPlanQuotaConfig) {
	if channelType != constant.ChannelTypeAdvancedCustom || config == nil {
		return
	}
	gopool.Go(func() {
		channel, err := model.GetChannelById(channelID, true)
		if err != nil || channel.Status != common.ChannelStatusEnabled || channel.ChannelInfo.IsMultiKey {
			return
		}
		status, err := EvaluateCodingPlanQuota(channel, time.Now())
		if err != nil {
			common.SysLog(fmt.Sprintf("coding plan quota check failed: channel_id=%d, error=%v", channelID, err))
			return
		}
		if status.Blocked {
			DisableChannel(*types.NewChannelError(channel.Id, channel.Type, channel.Name, false, "", channel.GetAutoBan()), status.Reason)
		}
	})
}

func IsCodingPlanQuotaAutoDisabled(channel *model.Channel) bool {
	if channel == nil || channel.Status != common.ChannelStatusAutoDisabled {
		return false
	}
	reason, _ := channel.GetOtherInfo()["status_reason"].(string)
	return strings.HasPrefix(reason, CodingPlanQuotaReasonPrefix)
}

// UpdateCodingPlanQuotaChannels applies local quota state to existing channel
// status handling. Unknown or invalid usage is fail-open: a probe failure must
// not disable a working upstream channel.
func UpdateCodingPlanQuotaChannels(channels []*model.Channel, now time.Time) {
	for _, channel := range channels {
		if channel == nil || channel.ChannelInfo.IsMultiKey {
			continue
		}
		settings := channel.GetOtherSettings()
		if settings.CodingPlanQuota == nil {
			continue
		}

		status, err := EvaluateCodingPlanQuota(channel, now)
		if err != nil {
			common.SysLog(fmt.Sprintf("coding plan quota check failed: channel_id=%d, error=%v", channel.Id, err))
			continue
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
			continue
		}
		if IsCodingPlanQuotaAutoDisabled(channel) {
			EnableChannel(channel.Id, "", channel.Name)
		}
	}
}
