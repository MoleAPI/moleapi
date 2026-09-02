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

	// General format error (OpenAI, Anthropic, Gemini, DashScope, etc.).
	// Some providers put code/message at the top level instead of under error.
	if oaiError := errResponse.TryToOpenAIError(); oaiError != nil {
		newApiErr = types.WithOpenAIError(*oaiError, resp.StatusCode)
		if showBodyWhenFail {
			newApiErr.Err = buildErrWithBody(newApiErr.Error())
		}
		return
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
	rawError := err.Error()
	switch relayError := err.RelayError.(type) {
	case types.OpenAIError:
		rawError += " " + relayError.Type
	case types.ClaudeError:
		rawError += " " + relayError.Type
	}
	statusCode, publicError := publicUpstreamError(err.StatusCode, rawError, string(err.GetErrorCode()))
	err.SetPublicError(statusCode, publicError)
}

func PublicUpstreamError(statusCode int, rawError string) (int, types.OpenAIError) {
	return publicUpstreamError(statusCode, rawError, "")
}

func publicUpstreamError(statusCode int, rawError string, upstreamCode string) (int, types.OpenAIError) {
	message := strings.ToLower(rawError + " " + upstreamCode)
	upstreamCode = strings.ToLower(strings.TrimSpace(upstreamCode))
	publicError := types.OpenAIError{Type: "invalid_request_error", Code: "invalid_request"}
	clientRejected := statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity || statusCode == http.StatusUnavailableForLegalReasons
	policyRejected := clientRejected || statusCode == http.StatusForbidden
	requestParam := ""
	for _, param := range []string{
		"max_completion_tokens", "max_tokens", "thinking_budget", "enable_thinking", "incremental_output", "enable_search",
		"tool_choice", "response_format", "reasoning_effort", "temperature", "top_p", "top_k", "messages", "prompt",
		"stop", "tools", "model", "file_id", "voice_id", "purpose",
	} {
		if strings.Contains(message, param) {
			requestParam = param
			break
		}
	}

	switch {
	case strings.Contains(message, "authentication_error") || strings.Contains(message, "invalid_api_key") || strings.Contains(message, "api key invalid") ||
		strings.Contains(message, "api_key_invalid") || strings.Contains(message, "api key not valid") || strings.Contains(message, "billing_error") ||
		strings.Contains(message, "failed_precondition") || strings.Contains(message, "spend limit") || strings.Contains(message, "usage limit") ||
		strings.Contains(message, "quota_exceeded") || strings.Contains(message, "insufficient balance") || strings.Contains(message, "credit balance") || strings.Contains(message, "account balance") ||
		strings.Contains(message, "arrearage") || strings.Contains(message, "account_overdue") || strings.Contains(message, "exceeded_current_quota_error") ||
		strings.Contains(message, "authfailure.") || strings.Contains(message, "workspace.accessdenied") || strings.Contains(message, "invalid_iam_token") ||
		strings.Contains(message, "coding_plan_api_key_") || strings.Contains(message, "coding_plan_not_subscribed") || strings.Contains(message, "coding_plan_subscription_expired") ||
		strings.Contains(message, "欠费") || strings.Contains(message, "余额不足") || strings.Contains(message, "套餐已到期") || strings.Contains(message, "无效的 api key"):
		publicError.Message = "The upstream service is temporarily unavailable. Please try again later."
		publicError.Type = "server_error"
		publicError.Code = "upstream_unavailable"
		return http.StatusServiceUnavailable, publicError
	case clientRejected && strings.Contains(message, "enable_thinking") && strings.Contains(message, "non-streaming"):
		publicError.Message = "Set enable_thinking=false for a non-streaming request, or set stream=true."
		publicError.Code = "invalid_parameter_value"
		publicError.Param = "enable_thinking"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "thinking_budget") && (strings.Contains(message, "positive integer") || strings.Contains(message, "not greater than")):
		publicError.Message = "The thinking_budget parameter must be a positive integer within the model's supported limit."
		publicError.Code = "invalid_parameter_value"
		publicError.Param = "thinking_budget"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "incremental_output") && strings.Contains(message, "true"):
		publicError.Message = "Set incremental_output=true when enable_thinking is enabled."
		publicError.Code = "invalid_parameter_value"
		publicError.Param = "incremental_output"
		return http.StatusBadRequest, publicError
	case clientRejected && (strings.Contains(message, "only support stream mode") || strings.Contains(message, "only supports stream mode") || strings.Contains(message, "does not support synchronous calls") || strings.Contains(message, "does not support http call")):
		publicError.Message = "This model only supports streaming requests. Set stream=true."
		publicError.Code = "unsupported_parameter"
		publicError.Param = "stream"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "enable_search") && strings.Contains(message, "not support"):
		publicError.Message = "This model does not support enable_search. Remove it or use a compatible model."
		publicError.Code = "unsupported_parameter"
		publicError.Param = "enable_search"
		return http.StatusBadRequest, publicError
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
	case clientRejected && (strings.Contains(message, "invalid service_tier argument") || strings.Contains(message, "invalid_service_tier")):
		publicError.Message = "The selected service_tier is unavailable. Choose a supported tier or remove service_tier."
		publicError.Code = "unsupported_parameter"
		publicError.Param = "service_tier"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "parameter_unknown"):
		publicError.Message = "The request contains an unsupported parameter. Remove the unrecognized parameter and try again."
		publicError.Code = "unknown_parameter"
		return http.StatusBadRequest, publicError
	case (clientRejected || statusCode == http.StatusRequestedRangeNotSatisfiable) && strings.Contains(message, "out_of_range"):
		publicError.Message = "A request parameter is outside the supported range. Check the parameter values and limits."
		publicError.Code = "invalid_parameter_value"
		return http.StatusBadRequest, publicError
	case clientRejected && (strings.Contains(message, "previous_response_not_found") ||
		(strings.Contains(message, "previous_response_id") && strings.Contains(message, "cannot be resolved"))):
		publicError.Message = "The previous_response_id could not be resolved. Send the full input context and remove previous_response_id."
		publicError.Code = "previous_response_not_found"
		publicError.Param = "previous_response_id"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "websocket_connection_limit_reached"):
		publicError.Message = "The upstream WebSocket session expired. Open a new connection and continue."
		publicError.Code = "websocket_connection_limit_reached"
		return http.StatusBadRequest, publicError
	case clientRejected && (strings.Contains(message, "does not support assistant message prefill") || strings.Contains(message, "conversation must end with a user message")):
		publicError.Message = "This model does not support assistant message prefill. End the conversation with a user message."
		publicError.Code = "unsupported_input"
		publicError.Param = "messages"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "thinking") && strings.Contains(message, "blocks") && strings.Contains(message, "cannot be modified"):
		publicError.Message = "Thinking blocks in the latest assistant message must be returned exactly as received."
		publicError.Code = "invalid_thinking_block"
		publicError.Param = "messages"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "thinking.type.enabled") && strings.Contains(message, "not supported"):
		publicError.Message = "This model does not support thinking.type=enabled. Use adaptive thinking with output_config.effort."
		publicError.Code = "unsupported_parameter"
		publicError.Param = "thinking.type"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "adaptive thinking is not supported"):
		publicError.Message = "This model does not support adaptive thinking. Use thinking.type=enabled with budget_tokens."
		publicError.Code = "unsupported_parameter"
		publicError.Param = "thinking.type"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "thinking.type.disabled") && strings.Contains(message, "not supported"):
		publicError.Message = "This model does not support disabling thinking. Remove the thinking parameter."
		publicError.Code = "unsupported_parameter"
		publicError.Param = "thinking.type"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "tool_choice") && strings.Contains(message, "not supported for this model"):
		publicError.Message = "This model does not support forced tool choice. Use tool_choice=auto or tool_choice=none."
		publicError.Code = "unsupported_parameter"
		publicError.Param = "tool_choice"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "signature") && strings.Contains(message, "bound to a different conversation"):
		publicError.Message = "A thinking block belongs to a different conversation. Remove it or resend the original unchanged conversation history."
		publicError.Code = "invalid_thinking_block"
		publicError.Param = "messages"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "block_binding") && strings.Contains(message, "extra inputs are not permitted"):
		publicError.Message = "thinking.block_binding requires the provider's matching beta feature. Remove block_binding when that feature is unavailable."
		publicError.Code = "unsupported_parameter"
		publicError.Param = "thinking.block_binding"
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
	case clientRejected && (upstreamCode == "1261" || upstreamCode == "1039" || upstreamCode == "336007" || upstreamCode == "336103" ||
		upstreamCode == "characters_too_long" || upstreamCode == "tokens_too_long" ||
		strings.Contains(message, "input token length too long") || strings.Contains(message, "exceeded model token limit") || strings.Contains(message, "total message size") ||
		strings.Contains(message, "range of input length")):
		publicError.Message = "The request exceeds the model's context length. Reduce the input or maximum output tokens."
		publicError.Code = "context_length_exceeded"
		publicError.Param = "messages"
		return http.StatusBadRequest, publicError
	case clientRejected && (upstreamCode == "1301" || upstreamCode == "1026" || upstreamCode == "1027" || upstreamCode == "content_filter" ||
		upstreamCode == "image_url_unsafe" || upstreamCode == "system_unsafe" || upstreamCode == "user_setting_unsafe" || upstreamCode == "functions_unsafe" ||
		upstreamCode == "taskpromptpolicyviolation" || upstreamCode == "auditsubmitillegal" || upstreamCode == "creationpolicyviolation" ||
		strings.Contains(message, "content_filter") || strings.Contains(message, "considered high risk") || strings.Contains(message, "涉敏")):
		publicError.Message = "The request was blocked by the provider's safety policy. Revise the content and try again."
		publicError.Code = "content_policy_violation"
		return http.StatusUnprocessableEntity, publicError
	case clientRejected && (upstreamCode == "modelunavailable" || upstreamCode == "model_offline"):
		publicError.Message = "The requested model is temporarily unavailable. Try another model or try again later."
		publicError.Type = "server_error"
		publicError.Code = "model_unavailable"
		publicError.Param = "model"
		return http.StatusServiceUnavailable, publicError
	case clientRejected && (upstreamCode == "1211" || upstreamCode == "invalid_model" || upstreamCode == "modelnotfound" || upstreamCode == "model_not_found" ||
		strings.Contains(message, "模型不存在")):
		publicError.Message = "The requested model is unavailable or is not supported by this endpoint."
		publicError.Code = "model_not_found"
		publicError.Param = "model"
		return http.StatusNotFound, publicError
	case clientRejected && (upstreamCode == "1212" || upstreamCode == "unsupportedoperation" || upstreamCode == "method_not_supported" ||
		upstreamCode == "actionoffline" || upstreamCode == "1221" || upstreamCode == "1222"):
		publicError.Message = "The requested operation is not supported by this model or endpoint."
		publicError.Code = "unsupported_operation"
		return http.StatusBadRequest, publicError
	case clientRejected && (upstreamCode == "malformed_json" || upstreamCode == "336002"):
		publicError.Message = "The request body is not valid JSON. Check its syntax and field types."
		publicError.Code = "invalid_json"
		return http.StatusBadRequest, publicError
	case clientRejected && (upstreamCode == "1213" || upstreamCode == "336006" || strings.HasPrefix(upstreamCode, "missingparameter") || strings.HasPrefix(upstreamCode, "fieldlacking")):
		publicError.Message = "The request is missing a required parameter."
		publicError.Code = "missing_required_parameter"
		publicError.Param = requestParam
		return http.StatusBadRequest, publicError
	case clientRejected && (upstreamCode == "1210" || upstreamCode == "1214" || upstreamCode == "1215" || upstreamCode == "2013" || upstreamCode == "336001" ||
		strings.HasPrefix(upstreamCode, "invalidparameter") || upstreamCode == "invalid_request_argument" ||
		upstreamCode == "fieldinvalid" || upstreamCode == "fieldunwanted"):
		publicError.Message = "A request parameter is invalid or unsupported. Check its value and model compatibility."
		publicError.Code = "invalid_parameter_value"
		publicError.Param = requestParam
		if requestParam != "" {
			publicError.Message = fmt.Sprintf("The %s parameter is invalid or unsupported. Check its value and model compatibility.", requestParam)
		}
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "file size is too large"):
		publicError.Message = "The uploaded file is too large for the upstream service."
		publicError.Code = "request_too_large"
		publicError.Param = "file"
		return http.StatusRequestEntityTooLarge, publicError
	case clientRejected && strings.Contains(message, "file size is zero"):
		publicError.Message = "The uploaded file is empty."
		publicError.Code = "invalid_file"
		publicError.Param = "file"
		return http.StatusBadRequest, publicError
	case clientRejected && requestParam != "" && (strings.Contains(message, "invalid") || strings.Contains(message, "must be") || strings.Contains(message, "not supported") ||
		strings.Contains(message, "out of range") || strings.Contains(message, "range of")):
		publicError.Message = fmt.Sprintf("The %s parameter is invalid or unsupported. Check its value and model compatibility.", requestParam)
		publicError.Code = "invalid_parameter_value"
		publicError.Param = requestParam
		return http.StatusBadRequest, publicError
	case clientRejected && (upstreamCode == "invalid_image_url" || upstreamCode == "invalid_image_generation_refer_image" || upstreamCode == "imageformatinvalid"):
		publicError.Message = "The image input is invalid or uses an unsupported format."
		publicError.Code = "invalid_image"
		publicError.Param = "image"
		return http.StatusBadRequest, publicError
	case clientRejected && upstreamCode == "1042":
		publicError.Message = "The input contains too many invalid or invisible characters. Clean the input and try again."
		publicError.Code = "invalid_input"
		publicError.Param = "input"
		return http.StatusBadRequest, publicError
	case clientRejected && (upstreamCode == "2037" || upstreamCode == "2048"):
		publicError.Message = "The audio duration is outside the supported range."
		publicError.Code = "invalid_audio_duration"
		return http.StatusBadRequest, publicError
	case clientRejected && (upstreamCode == "20132" || upstreamCode == "2039"):
		publicError.Message = "The voice cloning parameters are invalid. Check file_id and voice_id."
		publicError.Code = "invalid_parameter_value"
		publicError.Param = "voice_id"
		return http.StatusBadRequest, publicError
	case clientRejected && (strings.Contains(message, "malformed_function_call") || strings.Contains(message, "malformed_tool_call")):
		publicError.Message = "The model produced a malformed tool call. Retry the request or simplify the tool definitions."
		publicError.Code = "malformed_tool_call"
		publicError.Param = "tools"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "unexpected_tool_call"):
		publicError.Message = "The model called a tool that was not declared. Add the tool to the request or retry without tool use."
		publicError.Code = "unexpected_tool_call"
		publicError.Param = "tools"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "too_many_tool_calls"):
		publicError.Message = "The model produced too many tool calls. Retry with fewer or simpler tools."
		publicError.Code = "too_many_tool_calls"
		publicError.Param = "tools"
		return http.StatusBadRequest, publicError
	case clientRejected && strings.Contains(message, "no_image"):
		publicError.Message = "The model could not generate an image. Revise the prompt and try again."
		publicError.Code = "no_image"
		return http.StatusBadRequest, publicError
	case policyRejected && (strings.Contains(message, "content_policy") || strings.Contains(message, "content policy") || strings.Contains(message, "safety") || strings.Contains(message, "moderation") ||
		strings.Contains(message, "image_unsafe") || strings.Contains(message, "recitation") || strings.Contains(message, "prohibited_content") ||
		strings.Contains(message, "content_blocked") || strings.Contains(message, "blocklist") || strings.Contains(message, "spii") ||
		strings.Contains(message, "image_other") || message == "language" || strings.HasSuffix(message, " language")):
		publicError.Message = "The request was blocked by the provider's safety policy. Revise the content and try again."
		publicError.Code = "content_policy_violation"
		return http.StatusUnprocessableEntity, publicError
	case statusCode == http.StatusNotImplemented && strings.Contains(message, "unimplemented"):
		publicError.Message = "The requested operation is not supported by this model."
		publicError.Code = "unsupported_operation"
		return http.StatusBadRequest, publicError
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
