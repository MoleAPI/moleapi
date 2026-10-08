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
)

const CodingPlanQuotaReasonPrefix = "coding_plan_quota:"

var ErrCodingPlanQuotaLogDBUnavailable = errors.New("coding plan quota log database is unavailable")

type CodingPlanQuotaStatus struct {
	Blocked bool
	Reason  string
}

// EvaluateCodingPlanQuota reads existing consume logs instead of adding a
// second usage ledger. The scheduler is intentionally the enforcement point;
// request admission remains unchanged until a distributed reservation path is
// needed.
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

	for _, window := range config.Windows {
		cutoff := now.Add(-time.Duration(window.DurationSeconds) * time.Second).Unix()
		usage, err := codingPlanWindowUsage(channel.Id, unit, cutoff)
		if err != nil {
			return CodingPlanQuotaStatus{}, err
		}
		if usage >= window.Limit {
			return CodingPlanQuotaStatus{
				Blocked: true,
				Reason: fmt.Sprintf(
					"%s%s window reached (%d/%d)",
					CodingPlanQuotaReasonPrefix,
					unit,
					usage,
					window.Limit,
				),
			}, nil
		}
	}
	return CodingPlanQuotaStatus{}, nil
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
