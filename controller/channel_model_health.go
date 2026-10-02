package controller

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelprobe"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const (
	modelFailureWindow    = 15 * time.Minute
	modelRecoveryDelay    = 30 * time.Minute
	modelFailureMinimum   = 3
	modelFailureScanLimit = 5000
)

type modelFailureCandidate struct {
	Model   string
	Failure channelprobe.Failure
}

func structuredModelFailure(log *model.Log) (channelprobe.Failure, bool) {
	if log == nil || log.ChannelId == 0 || strings.TrimSpace(log.ModelName) == "" || log.TokenName == "模型测试" {
		return channelprobe.Failure{}, false
	}
	var other map[string]any
	if common.UnmarshalJsonStr(log.Other, &other) != nil {
		return channelprobe.Failure{}, false
	}
	statusCode := 0
	switch value := other["status_code"].(type) {
	case float64:
		statusCode = int(value)
	case int:
		statusCode = value
	}
	errorCode := strings.TrimSpace(fmt.Sprintf("%v", other["error_code"]))
	errorType := strings.TrimSpace(fmt.Sprintf("%v", other["error_type"]))
	if errorCode == "<nil>" {
		errorCode = ""
	}
	if errorType == "<nil>" {
		errorType = ""
	}

	if errorCode == string(types.ErrorCodeModelNotFound) || statusCode == http.StatusNotFound {
		return channelprobe.Failure{OccurredAt: log.CreatedAt, StatusCode: statusCode, ErrorCode: errorCode, ErrorType: errorType}, true
	}
	if statusCode < 500 {
		return channelprobe.Failure{}, false
	}
	switch types.ErrorCode(errorCode) {
	case types.ErrorCodeBadResponseStatusCode,
		types.ErrorCodeBadResponse,
		types.ErrorCodeBadResponseBody,
		types.ErrorCodeEmptyResponse,
		types.ErrorCodeReadResponseBodyFailed:
		return channelprobe.Failure{OccurredAt: log.CreatedAt, StatusCode: statusCode, ErrorCode: errorCode, ErrorType: errorType}, true
	default:
		return channelprobe.Failure{}, false
	}
}

func collectModelFailureCandidates(channels []*model.Channel, now int64) map[int][]modelFailureCandidate {
	result := make(map[int][]modelFailureCandidate)
	if !common.AutomaticDisableChannelEnabled || model.LOG_DB == nil {
		return result
	}
	eligible := make(map[int]*model.Channel)
	for _, channel := range channels {
		if channel != nil && channel.Status == common.ChannelStatusEnabled && channel.GetAutoBan() {
			eligible[channel.Id] = channel
		}
	}
	if len(eligible) == 0 {
		return result
	}

	var logs []*model.Log
	// ponytail: scan only the newest 5,000 structured errors; a busier install can move this aggregation into its log backend when this conservative scan becomes a measured limit.
	err := model.LOG_DB.Select("id", "created_at", "channel_id", "model_name", "token_name", "request_id", "other").
		Where("type = ? AND created_at >= ?", model.LogTypeError, now-int64(modelFailureWindow/time.Second)).
		Order("created_at DESC").Limit(modelFailureScanLimit).Find(&logs).Error
	if err != nil {
		common.SysError("failed to scan model health errors: " + err.Error())
		return result
	}

	type bucket struct {
		requests map[string]struct{}
		failure  channelprobe.Failure
	}
	buckets := make(map[int]map[string]*bucket)
	for _, log := range logs {
		channel := eligible[log.ChannelId]
		if channel == nil || strings.TrimSpace(log.RequestId) == "" {
			continue
		}
		modelName := strings.TrimSpace(log.ModelName)
		state := channelprobe.StateFromOtherInfo(channel.OtherInfo)
		modelState := state.Models[modelName]
		if state.IsAutoPaused(modelName) || log.CreatedAt <= modelState.LastAutoProbeAt || !channelDeclaresModel(channel, modelName) {
			continue
		}
		failure, ok := structuredModelFailure(log)
		if !ok {
			continue
		}
		if buckets[channel.Id] == nil {
			buckets[channel.Id] = make(map[string]*bucket)
		}
		item := buckets[channel.Id][modelName]
		if item == nil {
			item = &bucket{requests: make(map[string]struct{}), failure: failure}
			buckets[channel.Id][modelName] = item
		}
		item.requests[log.RequestId] = struct{}{}
		if failure.OccurredAt > item.failure.OccurredAt {
			item.failure = failure
		}
	}
	for channelID, models := range buckets {
		for modelName, item := range models {
			if len(item.requests) < modelFailureMinimum {
				continue
			}
			item.failure.Requests = len(item.requests)
			result[channelID] = append(result[channelID], modelFailureCandidate{Model: modelName, Failure: item.failure})
		}
		sort.Slice(result[channelID], func(i, j int) bool { return result[channelID][i].Model < result[channelID][j].Model })
	}
	return result
}

func channelDeclaresModel(channel *model.Channel, modelName string) bool {
	for _, declared := range channel.GetModels() {
		if strings.TrimSpace(declared) == modelName {
			return true
		}
	}
	return false
}

func selectControlModel(channel *model.Channel, state channelprobe.State, target string) string {
	fallback := ""
	for _, modelName := range channel.GetModels() {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" || modelName == target || state.IsAutoPaused(modelName) {
			continue
		}
		if state.Models[modelName].Status == channelprobe.StatusHealthy {
			return modelName
		}
		if fallback == "" {
			fallback = modelName
		}
	}
	return fallback
}

func testModelAvailability(ctx context.Context, channel *model.Channel, testUserID int, modelName string) testResult {
	probe := newChannelProbeSpec(channelprobe.ModeHi, "model_health", "", "", "", time.Now().UnixNano())
	result := testChannel(ctx, channel, testUserID, modelName, "", shouldUseStreamForAutomaticChannelTest(channel), probe)
	if result.localErr != nil || result.newAPIError != nil {
		recordChannelTestFailure(channel, testUserID, result)
	}
	return result
}

func modelAvailabilityPassed(result testResult) bool {
	return result.localErr == nil && result.newAPIError == nil && result.evaluation != nil && result.evaluation.Passed()
}

func failureFromTest(result testResult, now int64, requests int) channelprobe.Failure {
	failure := channelprobe.Failure{OccurredAt: now, Requests: requests}
	if result.newAPIError != nil {
		failure.StatusCode = result.newAPIError.StatusCode
		failure.ErrorCode = string(result.newAPIError.GetErrorCode())
		failure.ErrorType = string(result.newAPIError.GetErrorType())
	}
	return failure
}

func recordAutomaticModelAudit(action string, channel *model.Channel, modelName string) {
	params := model.AuditFields{"channel_id": channel.Id, "channel_name": channel.Name, "model": modelName}
	model.RecordAuditLog(nil, model.AuditLog{
		ActorRole: common.RoleRootUser, AuthMethod: "system", Category: model.AuditCategoryOperation,
		Action: action, Content: auditContentEN(action, params), Success: true,
		Other: model.AuditOther{Op: &model.AuditOperation{Action: action, Params: params}},
	})
}

func notifyAutomaticModelState(channel *model.Channel, modelName string, paused bool) {
	action := "恢复"
	state := "已恢复流量"
	if paused {
		action = "暂停"
		state = "已自动暂停"
	}
	subject := fmt.Sprintf("渠道「%s」（#%d）的模型 %s 已%s", channel.Name, channel.Id, modelName, action)
	service.NotifyRootUser(fmt.Sprintf("channel_model_%d_%s_%t", channel.Id, modelName, paused), subject, subject+"，"+state)
}

func checkOneModelHealth(ctx context.Context, channel *model.Channel, testUserID int, candidate *modelFailureCandidate, now int64) channelTestSummary {
	summary := channelTestSummary{}
	state := channelprobe.StateFromOtherInfo(channel.OtherInfo)
	modelName := ""
	recovery := false
	for _, declared := range channel.GetModels() {
		declared = strings.TrimSpace(declared)
		modelState := state.Models[declared]
		if modelState.AutoPaused && modelState.NextProbeAt > 0 && modelState.NextProbeAt <= now {
			modelName = declared
			recovery = true
			break
		}
	}
	if modelName == "" && candidate != nil {
		modelName = candidate.Model
	}
	if modelName == "" || !channelDeclaresModel(channel, modelName) {
		return summary
	}
	previousProbeAt := state.Models[modelName].LastAutoProbeAt

	result := testModelAvailability(ctx, channel, testUserID, modelName)
	summary.ModelTested++
	nextProbeAt := now + int64(modelRecoveryDelay/time.Second)
	if recovery {
		freshChannel, err := model.GetChannelById(channel.Id, true)
		if err != nil {
			common.SysError(fmt.Sprintf("failed to reload model recovery state: channel_id=%d model=%s error=%v", channel.Id, modelName, err))
			return summary
		}
		freshState := channelprobe.StateFromOtherInfo(freshChannel.OtherInfo)
		if !freshState.IsAutoPaused(modelName) || freshState.Models[modelName].LastAutoProbeAt != previousProbeAt {
			return summary
		}
		channel, state = freshChannel, freshState
		if modelAvailabilityPassed(result) {
			change := state.RecordAutoProbe(modelName, true, "pass", now, nextProbeAt, nil)
			if change.Recovered {
				summary.ModelRecovered++
				recordAutomaticModelAudit("channel.model_auto_recover", channel, modelName)
				notifyAutomaticModelState(channel, modelName, false)
			}
		} else {
			failure := failureFromTest(result, now, 1)
			state.RecordAutoProbe(modelName, false, "failed", now, nextProbeAt, &failure)
		}
		if err := channel.SaveProbeState(state); err != nil {
			common.SysError(fmt.Sprintf("failed to save model recovery state: channel_id=%d model=%s error=%v", channel.Id, modelName, err))
		}
		return summary
	}

	if modelAvailabilityPassed(result) {
		state.RecordAutoProbe(modelName, true, "candidate_pass", now, 0, nil)
		if err := channel.SaveProbeState(state); err != nil {
			common.SysError(fmt.Sprintf("failed to save model probe state: channel_id=%d model=%s error=%v", channel.Id, modelName, err))
		}
		return summary
	}
	controlModel := selectControlModel(channel, state, modelName)
	if controlModel == "" {
		failure := failureFromTest(result, now, candidate.Failure.Requests)
		state.RecordAutoProbe(modelName, false, "inconclusive_no_control", now, 0, &failure)
		if err := channel.SaveProbeState(state); err != nil {
			common.SysError(fmt.Sprintf("failed to save inconclusive model probe: channel_id=%d model=%s error=%v", channel.Id, modelName, err))
		}
		return summary
	}
	controlResult := testModelAvailability(ctx, channel, testUserID, controlModel)
	summary.ModelTested++
	if modelAvailabilityPassed(controlResult) {
		failure := candidate.Failure
		state.Pause(modelName, "Production failures confirmed by target and control probes", now, nextProbeAt, failure)
		if err := channel.SaveProbeState(state); err != nil {
			common.SysError(fmt.Sprintf("failed to pause channel model: channel_id=%d model=%s error=%v", channel.Id, modelName, err))
			return summary
		}
		summary.ModelPaused++
		recordAutomaticModelAudit("channel.model_auto_pause", channel, modelName)
		notifyAutomaticModelState(channel, modelName, true)
		return summary
	}
	failure := failureFromTest(result, now, candidate.Failure.Requests)
	state.RecordAutoProbe(modelName, false, "inconclusive_both_failed", now, 0, &failure)
	if err := channel.SaveProbeState(state); err != nil {
		common.SysError(fmt.Sprintf("failed to save failed model probes: channel_id=%d model=%s error=%v", channel.Id, modelName, err))
	}
	if controlResult.newAPIError != nil && service.ShouldDisableChannel(controlResult.newAPIError) && channel.GetAutoBan() {
		reason := controlResult.newAPIError.MaskSensitiveErrorWithStatusCode()
		service.DisableChannel(*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(controlResult.context, constant.ContextKeyChannelKey), channel.GetAutoBan()), reason)
	}
	return summary
}

func runModelAutoHealthChecks(ctx context.Context, channels []*model.Channel, testUserID int) channelTestSummary {
	now := common.GetTimestamp()
	candidates := collectModelFailureCandidates(channels, now)
	selected := make([]*model.Channel, 0)
	selectedCandidates := make(map[int]*modelFailureCandidate)
	for _, channel := range channels {
		if channel == nil || channel.Status == common.ChannelStatusManuallyDisabled {
			continue
		}
		state := channelprobe.StateFromOtherInfo(channel.OtherInfo)
		due := false
		for _, modelName := range channel.GetModels() {
			modelState := state.Models[strings.TrimSpace(modelName)]
			if modelState.AutoPaused && modelState.NextProbeAt > 0 && modelState.NextProbeAt <= now {
				due = true
				break
			}
		}
		if !due && len(candidates[channel.Id]) == 0 {
			continue
		}
		selected = append(selected, channel)
		if !due {
			candidate := candidates[channel.Id][0]
			selectedCandidates[channel.Id] = &candidate
		}
	}
	return runChannelTestWorkers(ctx, selected, operation_setting.GetMonitorSetting().ChannelTestConcurrency, func(ctx context.Context, channel *model.Channel) channelTestSummary {
		return checkOneModelHealth(ctx, channel, testUserID, selectedCandidates[channel.Id], now)
	}, nil)
}

func RetestChannelModel(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var request struct {
		Model string `json:"model"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	request.Model = strings.TrimSpace(request.Model)
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if request.Model == "" || !channelDeclaresModel(channel, request.Model) {
		common.ApiErrorMsg(c, "model is not configured on this channel")
		return
	}
	testUserID, err := resolveChannelTestUserID(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := testModelAvailability(c.Request.Context(), channel, testUserID, request.Model)
	now := common.GetTimestamp()
	state := channelprobe.StateFromOtherInfo(channel.OtherInfo)
	wasPaused := state.IsAutoPaused(request.Model)
	if !modelAvailabilityPassed(result) {
		failure := failureFromTest(result, now, 1)
		state.RecordAutoProbe(request.Model, false, "manual_failed", now, now+int64(modelRecoveryDelay/time.Second), &failure)
		if err := channel.SaveProbeState(state); err != nil {
			common.ApiError(c, err)
			return
		}
		recordManageAudit(c, "channel.model_manual_retest", map[string]any{"id": channel.Id, "model": request.Model, "success": false})
		response := gin.H{"success": false, "message": "Model test failed"}
		if result.newAPIError != nil {
			response["error_code"] = result.newAPIError.GetErrorCode()
		}
		c.JSON(http.StatusOK, response)
		return
	}
	state.RecoverNow(request.Model, now)
	if err := channel.SaveProbeState(state); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.model_manual_retest", map[string]any{"id": channel.Id, "model": request.Model, "success": true})
	if wasPaused {
		notifyAutomaticModelState(channel, request.Model, false)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": state.Models[request.Model]})
}
