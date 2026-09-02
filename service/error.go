package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func MidjourneyErrorWrapper(code int, desc string) *taskdto.MidjourneyResponse {
	return &taskdto.MidjourneyResponse{
		Code:        code,
		Description: desc,
	}
}

func MidjourneyErrorWithStatusCodeWrapper(code int, desc string, statusCode int) *taskdto.MidjourneyResponseWithStatusCode {
	return &taskdto.MidjourneyResponseWithStatusCode{
		StatusCode: statusCode,
		Response:   *MidjourneyErrorWrapper(code, desc),
	}
}

//// OpenAIErrorWrapper wraps an error into an OpenAIErrorWithStatusCode
//func OpenAIErrorWrapper(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	text := err.Error()
//	lowerText := strings.ToLower(text)
//	if !strings.HasPrefix(lowerText, "get file base64 from url") && !strings.HasPrefix(lowerText, "mime type is not supported") {
//		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
//			common.SysLog(fmt.Sprintf("error: %s", text))
//			text = "请求上游地址失败"
//		}
//	}
//	openAIError := dto.OpenAIError{
//		Message: text,
//		Type:    "new_api_error",
//		Code:    code,
//	}
//	return &dto.OpenAIErrorWithStatusCode{
//		Error:      openAIError,
//		StatusCode: statusCode,
//	}
//}
//
//func OpenAIErrorWrapperLocal(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	openaiErr := OpenAIErrorWrapper(err, code, statusCode)
//	openaiErr.LocalError = true
//	return openaiErr
//}

func ClaudeErrorWrapper(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if !strings.HasPrefix(lowerText, "get file base64 from url") {
		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
			common.SysLog(fmt.Sprintf("error: %s", text))
			text = "请求上游地址失败"
		}
	}
	claudeError := types.ClaudeError{
		Message: text,
		Type:    "new_api_error",
	}
	return &dto.ClaudeErrorWithStatusCode{
		Error:      claudeError,
		StatusCode: statusCode,
	}
}

func ClaudeErrorWrapperLocal(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	claudeErr := ClaudeErrorWrapper(err, code, statusCode)
	claudeErr.LocalError = true
	return claudeErr
}

func RelayErrorHandler(ctx context.Context, resp *http.Response, showBodyWhenFail bool) (newApiErr *types.NewAPIError) {
	defer func() {
		SetPublicUpstreamError(ctx, newApiErr)
	}()
	newApiErr = types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, resp.StatusCode)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	CloseResponseBodyGracefully(resp)
	var errResponse dto.GeneralErrorResponse
	responseBodyText := string(responseBody)
	responseBodyPreview := common.LocalLogPreview(responseBodyText)
	buildErrWithBody := func(message string) error {
		if message == "" {
			return fmt.Errorf("bad response status code %d, body: %s", resp.StatusCode, responseBodyText)
		}
		return fmt.Errorf("bad response status code %d, message: %s, body: %s", resp.StatusCode, message, responseBodyText)
	}

	err = common.Unmarshal(responseBody, &errResponse)
	if err != nil {
		if showBodyWhenFail {
			newApiErr.Err = buildErrWithBody("")
		} else {
			logger.LogError(ctx, fmt.Sprintf("bad response status code %d, body: %s", resp.StatusCode, responseBodyPreview))
			newApiErr.Err = fmt.Errorf("bad response status code %d", resp.StatusCode)
		}
		return
	}

	if common.GetJsonType(errResponse.Error) == "object" {
		// General format error (OpenAI, Anthropic, Gemini, etc.)
		oaiError := errResponse.TryToOpenAIError()
		if oaiError != nil {
			newApiErr = types.WithOpenAIError(*oaiError, resp.StatusCode)
			if showBodyWhenFail {
				newApiErr.Err = buildErrWithBody(newApiErr.Error())
			}
			return
		}
	}
	message := errResponse.ToMessage()
	if message == "" {
		// The body parsed as JSON but carried no usable error message; log the
		// raw body so the upstream failure remains diagnosable.
		logger.LogError(ctx, fmt.Sprintf("bad response status code %d with empty error message, body: %s", resp.StatusCode, responseBodyPreview))
	}
	newApiErr = types.NewOpenAIError(errors.New(message), types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
	if showBodyWhenFail {
		newApiErr.Err = buildErrWithBody(newApiErr.Error())
	}
	return
}

func SetPublicUpstreamError(ctx context.Context, err *types.NewAPIError) {
	if err == nil {
		return
	}
	logger.LogWarn(ctx, fmt.Sprintf("masked upstream error from user response: %s", common.LocalLogPreview(err.Error())))
	statusCode, publicError := PublicUpstreamError(err.StatusCode, err.Error()+" "+string(err.GetErrorCode()))
	err.SetPublicError(statusCode, publicError)
}

func PublicUpstreamError(statusCode int, rawError string) (int, types.OpenAIError) {
	message := strings.ToLower(rawError)
	publicError := types.OpenAIError{Type: "invalid_request_error", Code: "invalid_request"}
	clientRejected := statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity || statusCode == http.StatusUnavailableForLegalReasons
	policyRejected := clientRejected || statusCode == http.StatusForbidden

	switch {
	case strings.Contains(message, "authentication_error") || strings.Contains(message, "invalid_api_key") || strings.Contains(message, "api key invalid") ||
		strings.Contains(message, "insufficient balance") || strings.Contains(message, "credit balance") || strings.Contains(message, "account balance"):
		publicError.Message = "The upstream service is temporarily unavailable. Please try again later."
		publicError.Type = "server_error"
		publicError.Code = "upstream_unavailable"
		return http.StatusServiceUnavailable, publicError
	case clientRejected && strings.Contains(message, "prompt_is_required"):
		publicError.Message = "The prompt parameter is required."
		publicError.Code = "missing_required_parameter"
		publicError.Param = "prompt"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "task_id_is_required"):
		publicError.Message = "The task_id parameter is required."
		publicError.Code = "missing_required_parameter"
		publicError.Param = "task_id"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "action_is_required"):
		publicError.Message = "The action parameter is required."
		publicError.Code = "missing_required_parameter"
		publicError.Param = "action"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "index_is_required"):
		publicError.Message = "The index parameter is required."
		publicError.Code = "missing_required_parameter"
		publicError.Param = "index"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "tools cannot be empty") &&
		(strings.Contains(message, "tool choice") || strings.Contains(message, "tool_choice")) && strings.Contains(message, "required"):
		publicError.Message = "The tools parameter cannot be empty when tool_choice is set to required."
		publicError.Code = "invalid_parameter_value"
		publicError.Param = "tools"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "chatcompletionrequest[\"stop\"]") &&
		strings.Contains(message, "arraylist") && strings.Contains(message, "deserialize from string value"):
		publicError.Message = "The upstream service requires stop to be an array of strings for this model. Send stop as an array or remove it."
		publicError.Code = "invalid_parameter_value"
		publicError.Param = "stop"
		return http.StatusBadRequest, publicError
	case clientRejected && (strings.Contains(message, "missing_thought_signature") || strings.Contains(message, "thought_signature") || strings.Contains(message, "thought signature")):
		publicError.Message = "The request is missing a required thought signature. Return the model's thought signature unchanged with the related tool call."
		publicError.Code = "missing_thought_signature"
		publicError.Param = "messages"
		return http.StatusBadRequest, publicError
	case clientRejected && (strings.Contains(message, "reasoning_content") || strings.Contains(message, "reasoning content") || strings.Contains(message, "thinking block")):
		publicError.Message = "Invalid reasoning_content. Preserve the assistant reasoning_content returned with the previous tool call, or omit it when the model does not support it."
		publicError.Code = "invalid_reasoning_content"
		publicError.Param = "messages"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "max_completion_tokens") && strings.Contains(message, "max_tokens"),
		clientRejected && strings.Contains(message, "unsupported parameter") && strings.Contains(message, "max_tokens"),
		clientRejected && strings.Contains(message, "max_tokens") && strings.Contains(message, "not support"):
		publicError.Message = "This model does not support max_tokens. Use max_completion_tokens instead."
		publicError.Code = "unsupported_parameter"
		publicError.Param = "max_tokens"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "enable_thinking"):
		publicError.Message = "This request does not support enable_thinking. Remove it or use a compatible model and mode."
		publicError.Code = "unsupported_parameter"
		publicError.Param = "enable_thinking"
		return http.StatusBadRequest, publicError
	case clientRejected && (strings.Contains(message, "only n=1") || strings.Contains(message, "only supports n = 1") || strings.Contains(message, "only supports n=1") || strings.Contains(message, "n must be 1")):
		publicError.Message = "This model only supports n=1."
		publicError.Code = "invalid_parameter_value"
		publicError.Param = "n"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "size") && (strings.Contains(message, "required") || strings.Contains(message, "missing")):
		publicError.Message = "The size parameter is required."
		publicError.Code = "missing_required_parameter"
		publicError.Param = "size"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "image") && (strings.Contains(message, "not support") || strings.Contains(message, "unsupported")):
		publicError.Message = "This model does not support image input. Remove the image or use a vision-capable model."
		publicError.Code = "unsupported_input"
		publicError.Param = "messages"
		return http.StatusBadRequest, publicError
	case clientRejected && (strings.Contains(message, "context_length_exceeded") || strings.Contains(message, "context length") || strings.Contains(message, "maximum context")):
		publicError.Message = "The request exceeds the model's context length. Reduce the input or maximum output tokens."
		publicError.Code = "context_length_exceeded"
		publicError.Param = "messages"
		return http.StatusBadRequest, publicError
	case policyRejected && (strings.Contains(message, "content_policy") || strings.Contains(message, "content policy") || strings.Contains(message, "safety") || strings.Contains(message, "moderation") || strings.Contains(message, "image_unsafe")):
		publicError.Message = "The request was blocked by the provider's safety policy. Revise the content and try again."
		publicError.Code = "content_policy_violation"
		return http.StatusUnprocessableEntity, publicError
	}

	switch statusCode {
	case http.StatusBadRequest:
		publicError.Message = "The upstream service rejected the request. Check the request parameters and model compatibility."
		return statusCode, publicError
	case http.StatusNotFound, http.StatusGone:
		publicError.Message = "The requested model or resource is unavailable."
		publicError.Code = "resource_not_found"
		if strings.Contains(message, "model") {
			publicError.Code = "model_not_found"
			publicError.Param = "model"
		}
		return http.StatusNotFound, publicError
	case http.StatusMethodNotAllowed:
		publicError.Message = "The requested operation is not supported by this model."
		publicError.Code = "unsupported_operation"
		return http.StatusBadRequest, publicError
	case http.StatusRequestTimeout, http.StatusGatewayTimeout, 524:
		publicError.Message = "The upstream service timed out. Please try again later."
		publicError.Type = "server_error"
		publicError.Code = "upstream_timeout"
		return http.StatusGatewayTimeout, publicError
	case http.StatusConflict:
		publicError.Message = "The upstream service reported a conflict. Please try again."
		publicError.Code = "upstream_conflict"
		return statusCode, publicError
	case http.StatusRequestEntityTooLarge:
		publicError.Message = "The request is too large for the upstream service."
		publicError.Code = "request_too_large"
		return statusCode, publicError
	case http.StatusUnprocessableEntity:
		publicError.Message = "The upstream service could not process the request. Check the input format and parameters."
		publicError.Code = "unprocessable_entity"
		return statusCode, publicError
	case http.StatusTooManyRequests:
		publicError.Message = "The upstream service is busy. Please try again later."
		publicError.Type = "rate_limit_error"
		publicError.Code = "rate_limit_exceeded"
		return statusCode, publicError
	case http.StatusUnavailableForLegalReasons:
		publicError.Message = "The request was blocked by the provider's safety policy. Revise the content and try again."
		publicError.Code = "content_policy_violation"
		return http.StatusUnprocessableEntity, publicError
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden:
		publicError.Message = "The upstream service is temporarily unavailable. Please try again later."
		publicError.Type = "server_error"
		publicError.Code = "upstream_unavailable"
		return http.StatusServiceUnavailable, publicError
	}
	if statusCode >= http.StatusInternalServerError && statusCode <= 599 {
		publicError.Message = "The upstream service is temporarily unavailable. Please try again later."
		publicError.Type = "server_error"
		publicError.Code = "upstream_unavailable"
		return http.StatusServiceUnavailable, publicError
	}
	publicError.Message = "The upstream request failed. Please try again later."
	publicError.Type = "server_error"
	publicError.Code = "upstream_error"
	return http.StatusBadGateway, publicError
}

func PublicTaskFailureMessage(rawError string) string {
	_, publicError := PublicUpstreamError(http.StatusBadRequest, rawError)
	if publicError.Code == "invalid_request" {
		return "The task failed. Please check the request and try again."
	}
	return publicError.Message
}

func ResetStatusCode(newApiErr *types.NewAPIError, statusCodeMappingStr string) {
	if newApiErr == nil {
		return
	}
	if statusCodeMappingStr == "" || statusCodeMappingStr == "{}" {
		return
	}
	statusCodeMapping := make(map[string]any)
	err := common.Unmarshal([]byte(statusCodeMappingStr), &statusCodeMapping)
	if err != nil {
		return
	}
	if newApiErr.StatusCode == http.StatusOK {
		return
	}
	codeStr := strconv.Itoa(newApiErr.StatusCode)
	if value, ok := statusCodeMapping[codeStr]; ok {
		intCode, ok := parseStatusCodeMappingValue(value)
		if !ok {
			return
		}
		newApiErr.StatusCode = intCode
	}
}

func parseStatusCodeMappingValue(value any) (int, bool) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return 0, false
		}
		statusCode, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return statusCode, true
	case float64:
		if v != math.Trunc(v) {
			return 0, false
		}
		return int(v), true
	case int:
		return v, true
	case json.Number:
		statusCode, err := strconv.Atoi(v.String())
		if err != nil {
			return 0, false
		}
		return statusCode, true
	default:
		return 0, false
	}
}

func TaskErrorWrapperLocal(err error, code string, statusCode int) *taskdto.TaskError {
	openaiErr := TaskErrorWrapper(err, code, statusCode)
	openaiErr.LocalError = true
	return openaiErr
}

func TaskErrorWrapper(err error, code string, statusCode int) *taskdto.TaskError {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
		common.SysLog(fmt.Sprintf("error: %s", text))
		//text = "请求上游地址失败"
		text = common.MaskSensitiveInfo(text)
	}
	//避免暴露内部错误
	taskError := &taskdto.TaskError{
		Code:       code,
		Message:    text,
		StatusCode: statusCode,
		Error:      err,
	}

	return taskError
}

// TaskErrorFromAPIError 将 PreConsumeBilling 返回的 NewAPIError 转换为 TaskError。
func TaskErrorFromAPIError(apiErr *types.NewAPIError) *taskdto.TaskError {
	if apiErr == nil {
		return nil
	}
	return &taskdto.TaskError{
		Code:       string(apiErr.GetErrorCode()),
		Message:    apiErr.Err.Error(),
		StatusCode: apiErr.StatusCode,
		Error:      apiErr.Err,
	}
}
