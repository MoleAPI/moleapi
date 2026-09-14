package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// PrepareRequestBilling estimates and reserves one request's charge. Transports
// provide the current request body through BodyStorage or BillingRequestInput;
// channel retries retain the resulting billing session and pricing snapshot.
func PrepareRequestBilling(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needWaffoPancakeCheck := setting.ShouldCheckPromptWithWaffoPancake()
	needModerationCheck := setting.ShouldCheckPromptWithModeration()
	meta := &types.TokenCountMeta{TokenType: types.TokenTypeTokenizer}
	if info.Request != nil && (needSensitiveCheck || needWaffoPancakeCheck || needModerationCheck || constant.CountToken) {
		meta = info.Request.GetTokenCountMeta()
	} else {
		// Avoid building CombineText when only the pricing quantities are needed.
		switch request := info.Request.(type) {
		case *dto.GeneralOpenAIRequest:
			meta.MaxTokens = int(max(lo.FromPtr(request.MaxTokens), lo.FromPtr(request.MaxCompletionTokens)))
		case *dto.OpenAIResponsesRequest:
			meta.MaxTokens = int(lo.FromPtr(request.MaxOutputTokens))
		case *dto.ClaudeRequest:
			meta.MaxTokens = int(lo.FromPtr(request.MaxTokens))
		case *dto.ImageRequest:
			meta = request.GetTokenCountMeta()
		}
	}

	if safetyErr := checkPromptSafety(c, info, meta, needSensitiveCheck, needWaffoPancakeCheck, needModerationCheck); safetyErr != nil {
		return safetyErr
	}

	tokens, err := service.EstimateRequestToken(c, meta, info)
	if err != nil {
		return types.NewError(err, types.ErrorCodeCountTokenFailed)
	}
	info.SetEstimatePromptTokens(tokens)

	priceData, err := helper.ModelPriceHelper(c, info, tokens, meta)
	if err != nil {
		return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
	}
	if priceData.FreeModel {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", info.OriginModelName))
		return nil
	}
	return service.PreConsumeBilling(c, priceData.QuotaToPreConsume, info)
}

// RefundFailedRequestBilling applies the common final-failure policy after all
// eligible attempts have ended. A settled BillingSession never refunds again.
func RefundFailedRequestBilling(c *gin.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) *types.NewAPIError {
	if apiErr == nil {
		return nil
	}
	apiErr = service.NormalizeViolationFeeError(apiErr)
	if info.Billing != nil {
		info.Billing.Refund(c)
	}
	service.ChargeViolationFeeIfNeeded(c, info, apiErr)
	return apiErr
}

const promptModerationModel = "text-moderation-stable"

type moderationScanResponse struct {
	Error   *types.OpenAIError `json:"error,omitempty"`
	Results []struct {
		Flagged    bool            `json:"flagged"`
		Categories map[string]bool `json:"categories,omitempty"`
	} `json:"results"`
}

func checkPromptSafety(c *gin.Context, relayInfo *relaycommon.RelayInfo, meta *types.TokenCountMeta, needSensitiveCheck, needWaffoPancakeCheck, needModerationCheck bool) *types.NewAPIError {
	if meta == nil || strings.TrimSpace(meta.CombineText) == "" {
		return nil
	}
	prompt := meta.CombineText

	if needSensitiveCheck {
		contains, words := service.CheckSensitiveText(prompt)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			return sensitiveWordsDetectedError("prompt blocked by sensitive words")
		}
	}

	if needWaffoPancakeCheck && isImageRelayMode(relayInfo.RelayMode) {
		result, err := service.ScanWaffoPancakePrompt(c.Request.Context(), prompt)
		if err != nil {
			logger.LogWarn(c, fmt.Sprintf("Waffo Pancake prompt safety check failed: %s", err.Error()))
			return promptSafetyUnavailableError("Waffo Pancake", err)
		}
		if result != nil && result.Action != "" && result.Action != "allow" {
			detail := result.Action
			if result.ReasonCode != "" {
				detail += "/" + result.ReasonCode
			}
			if len(result.MatchedCategories) > 0 {
				detail += " categories=" + strings.Join(result.MatchedCategories, ",")
			}
			logger.LogWarn(c, fmt.Sprintf(
				"Waffo Pancake prompt blocked: action=%s reason=%s request_id=%s categories=%s",
				result.Action,
				result.ReasonCode,
				result.RequestID,
				strings.Join(result.MatchedCategories, ","),
			))
			return promptBlockedError("prompt blocked by Waffo Pancake content safety: " + detail)
		}
	}

	if needModerationCheck && relayInfo.RelayMode != relayconstant.RelayModeModerations {
		flagged, categories, err := scanPromptWithModeration(c, relayInfo, prompt)
		if err != nil {
			logger.LogWarn(c, fmt.Sprintf("prompt moderation check failed: %s", err.Error()))
			return promptSafetyUnavailableError("moderation", err)
		}
		if flagged {
			detail := "prompt blocked by moderation"
			if len(categories) > 0 {
				detail += ": " + strings.Join(categories, ",")
			}
			logger.LogWarn(c, fmt.Sprintf("prompt blocked by moderation: categories=%s", strings.Join(categories, ",")))
			return promptBlockedError(detail)
		}
	}

	return nil
}

func isImageRelayMode(relayMode int) bool {
	return relayMode == relayconstant.RelayModeImagesGenerations || relayMode == relayconstant.RelayModeImagesEdits
}

func scanPromptWithModeration(c *gin.Context, relayInfo *relaycommon.RelayInfo, prompt string) (bool, []string, error) {
	group := moderationGroup(c, relayInfo)
	channel, err := model.GetRandomSatisfiedChannel(group, promptModerationModel, 0, []taskdto.ChannelFilter{{
		Kind:        taskdto.FilterRequestPath,
		RequestPath: "/v1/moderations",
	}})
	if err != nil {
		return false, nil, err
	}
	if channel == nil {
		return false, nil, fmt.Errorf("no available moderation channel for group %s", group)
	}

	upstreamModel, err := moderationUpstreamModel(channel)
	if err != nil {
		return false, nil, err
	}

	key, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return false, nil, apiErr
	}
	bodyBytes, err := common.Marshal(map[string]any{
		"model": upstreamModel,
		"input": prompt,
	})
	if err != nil {
		return false, nil, err
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, moderationRequestURL(channel, upstreamModel), bytes.NewReader(bodyBytes))
	if err != nil {
		return false, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if channel.Type == constant.ChannelTypeAzure {
		req.Header.Set("api-key", key)
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if channel.OpenAIOrganization != nil && *channel.OpenAIOrganization != "" {
		req.Header.Set("OpenAI-Organization", *channel.OpenAIOrganization)
	}

	client := service.GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, nil, err
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return false, nil, fmt.Errorf("moderation upstream returned status %d: %s", resp.StatusCode, common.LocalLogPreview(string(responseBody)))
	}
	return moderationScanFlagged(responseBody)
}

func moderationGroup(c *gin.Context, relayInfo *relaycommon.RelayInfo) string {
	group := relayInfo.UsingGroup
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	}
	if group == "auto" {
		autoGroup := common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
		if autoGroup != "" {
			return autoGroup
		}
		groups := service.GetUserAutoGroup(relayInfo.UserGroup)
		if len(groups) > 0 {
			return groups[0]
		}
	}
	if group == "" {
		group = relayInfo.UserGroup
	}
	return group
}

func moderationRequestURL(channel *model.Channel, modelName string) string {
	baseURL := strings.TrimRight(channel.GetBaseURL(), "/")
	if channel.Type == constant.ChannelTypeAzure {
		apiVersion := strings.TrimSpace(channel.Other)
		if apiVersion == "" {
			apiVersion = constant.AzureDefaultAPIVersion
		}
		return fmt.Sprintf("%s/openai/deployments/%s/moderations?api-version=%s", baseURL, modelName, url.QueryEscape(apiVersion))
	}
	return baseURL + "/v1/moderations"
}

func moderationUpstreamModel(channel *model.Channel) (string, error) {
	modelName := promptModerationModel
	modelMapping := channel.GetModelMapping()
	if modelMapping == "" || modelMapping == "{}" {
		return modelName, nil
	}

	modelMap := map[string]string{}
	if err := common.Unmarshal([]byte(modelMapping), &modelMap); err != nil {
		return "", errors.New("unmarshal_model_mapping_failed")
	}

	visited := map[string]bool{modelName: true}
	for {
		nextModel := modelMap[modelName]
		if nextModel == "" || nextModel == modelName {
			return modelName, nil
		}
		if visited[nextModel] {
			return "", errors.New("model_mapping_contains_cycle")
		}
		visited[nextModel] = true
		modelName = nextModel
	}
}

func moderationScanFlagged(responseBody []byte) (bool, []string, error) {
	var response moderationScanResponse
	if err := common.Unmarshal(responseBody, &response); err != nil {
		return false, nil, err
	}
	if response.Error != nil && response.Error.Message != "" {
		return false, nil, errors.New(response.Error.Message)
	}

	matchedCategories := map[string]struct{}{}
	flagged := false
	for _, result := range response.Results {
		if !result.Flagged {
			continue
		}
		flagged = true
		for category, matched := range result.Categories {
			if matched {
				matchedCategories[category] = struct{}{}
			}
		}
	}

	categories := make([]string, 0, len(matchedCategories))
	for category := range matchedCategories {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	return flagged, categories, nil
}

func sensitiveWordsDetectedError(message string) *types.NewAPIError {
	err := types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeSensitiveWordsDetected, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	err.SetPublicMessage(message)
	return err
}

func promptBlockedError(message string) *types.NewAPIError {
	err := types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodePromptBlocked, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	err.SetPublicMessage(message)
	return err
}

func promptSafetyUnavailableError(source string, err error) *types.NewAPIError {
	message := source + " content safety check failed"
	apiErr := types.NewErrorWithStatusCode(fmt.Errorf("%s: %w", message, err), types.ErrorCodePromptBlocked, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	apiErr.SetPublicMessage(message)
	return apiErr
}
