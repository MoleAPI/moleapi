package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResetStatusCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		statusCode       int
		statusCodeConfig string
		expectedCode     int
	}{
		{
			name:             "map string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"503"}`,
			expectedCode:     503,
		},
		{
			name:             "map int value",
			statusCode:       429,
			statusCodeConfig: `{"429":503}`,
			expectedCode:     503,
		},
		{
			name:             "skip invalid string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"bad-code"}`,
			expectedCode:     429,
		},
		{
			name:             "skip status code 200",
			statusCode:       200,
			statusCodeConfig: `{"200":503}`,
			expectedCode:     200,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			newAPIError := &types.NewAPIError{
				StatusCode: tc.statusCode,
			}
			ResetStatusCode(newAPIError, tc.statusCodeConfig)
			require.Equal(t, tc.expectedCode, newAPIError.StatusCode)
		})
	}
}

func TestRelayErrorHandlerTruncatesInvalidJSONBodyInLog(t *testing.T) {
	withDebugEnabled(t, false)

	body := strings.Repeat("b", common.LocalLogContentLimit+256)
	var logBuffer bytes.Buffer

	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logBuffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, "bad response status code 500", newAPIError.Error())
	require.Contains(t, logBuffer.String(), "[truncated")
	require.Contains(t, logBuffer.String(), fmt.Sprintf("original_length=%d", len(body)))
	require.NotContains(t, logBuffer.String(), strings.Repeat("b", common.LocalLogContentLimit+1))
}

func TestRelayErrorHandlerKeepsStructuredErrorMessage(t *testing.T) {
	message := strings.Repeat("c", common.LocalLogContentLimit+256)
	body := `{"message":"` + message + `"}`
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, message, newAPIError.Error())
	require.Equal(t, "The upstream service is temporarily unavailable. Please try again later.", newAPIError.ToOpenAIError().Message)
}

func TestRelayErrorHandlerKeepsOpenAIErrorMessage(t *testing.T) {
	message := strings.Repeat("d", common.LocalLogContentLimit+256)
	body := `{"error":{"message":"` + message + `","type":"server_error","code":"server_error"}}`
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, message, newAPIError.Error())
	publicError := newAPIError.ToOpenAIError()
	require.Equal(t, "The upstream service is temporarily unavailable. Please try again later.", publicError.Message)
	require.Equal(t, "server_error", publicError.Type)
	require.Equal(t, "upstream_unavailable", publicError.Code)
	require.Empty(t, publicError.Metadata)
}

func TestRelayErrorHandlerMasksUpstreamDistributorMessageForUsers(t *testing.T) {
	message := "No available channel for model claude-opus-5 under group claude max满血号池 (distributor) (request id: upstream)"
	body := `{"error":{"message":"` + message + `","type":"upstream_error","code":"bad_response_status_code"}}`
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, message, newAPIError.Error())
	require.Equal(t, "The upstream service is temporarily unavailable. Please try again later.", newAPIError.ToOpenAIError().Message)
	require.Equal(t, "status_code=503, The upstream service is temporarily unavailable. Please try again later.", newAPIError.MaskSensitiveErrorWithStatusCode())

	newAPIError.SetMessage(message + " (request id: local)")
	require.Equal(t, "The upstream service is temporarily unavailable. Please try again later. (request id: local)", newAPIError.ToOpenAIError().Message)
}

func TestRelayErrorHandlerMasksUpstreamImageResultMessageForUsers(t *testing.T) {
	message := "GPT Image response did not contain a valid b64_json result"
	body := `{"message":"` + message + `"}`
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, message, newAPIError.Error())
	require.Equal(t, "The upstream service is temporarily unavailable. Please try again later.", newAPIError.ToOpenAIError().Message)
	require.Equal(t, "status_code=503, The upstream service is temporarily unavailable. Please try again later.", newAPIError.MaskSensitiveErrorWithStatusCode())
}

func TestPublicUpstreamErrorClassifiesActionableAndPrivateFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		rawError   string
		wantStatus int
		wantType   string
		wantCode   string
		wantParam  string
	}{
		{name: "reasoning content", statusCode: 400, rawError: "reasoning_content is required for assistant tool call", wantStatus: 400, wantType: "invalid_request_error", wantCode: "invalid_reasoning_content", wantParam: "messages"},
		{name: "thought signature", statusCode: 400, rawError: "missing_thought_signature", wantStatus: 400, wantType: "invalid_request_error", wantCode: "missing_thought_signature", wantParam: "messages"},
		{name: "token parameter", statusCode: 400, rawError: "Unsupported parameter: max_tokens; use max_completion_tokens", wantStatus: 400, wantType: "invalid_request_error", wantCode: "unsupported_parameter", wantParam: "max_tokens"},
		{name: "context length", statusCode: 400, rawError: "context_length_exceeded", wantStatus: 400, wantType: "invalid_request_error", wantCode: "context_length_exceeded", wantParam: "messages"},
		{name: "provider credentials", statusCode: 401, rawError: "invalid provider key sk-secret", wantStatus: 503, wantType: "server_error", wantCode: "upstream_unavailable"},
		{name: "rate limit", statusCode: 429, rawError: "account org-secret exhausted", wantStatus: 429, wantType: "rate_limit_error", wantCode: "rate_limit_exceeded"},
		{name: "cloudflare timeout", statusCode: 524, rawError: "provider.example timed out", wantStatus: 504, wantType: "server_error", wantCode: "upstream_timeout"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statusCode, publicError := PublicUpstreamError(test.statusCode, test.rawError)
			require.Equal(t, test.wantStatus, statusCode)
			require.Equal(t, test.wantType, publicError.Type)
			require.Equal(t, test.wantCode, publicError.Code)
			require.Equal(t, test.wantParam, publicError.Param)
			require.NotContains(t, publicError.Message, "secret")
			require.NotContains(t, publicError.Message, "provider.example")
		})
	}
}

func TestSetPublicUpstreamErrorKeepsRawAdminDetail(t *testing.T) {
	upstreamError := types.NewOpenAIError(errors.New("provider key sk-secret was rejected"), types.ErrorCodeBadResponseStatusCode, http.StatusUnauthorized)

	SetPublicUpstreamError(context.Background(), upstreamError)

	require.Equal(t, "status_code=401, provider key sk-secret was rejected", upstreamError.ErrorWithStatusCode())
	require.Equal(t, http.StatusServiceUnavailable, upstreamError.PublicStatusCode())
	require.Equal(t, "upstream_unavailable", upstreamError.ToOpenAIError().Code)
	require.NotContains(t, upstreamError.ToOpenAIError().Message, "sk-secret")
	require.Equal(t, "overloaded_error", upstreamError.ToClaudeError().Type)
}

func TestRelayErrorHandlerKeepsInvalidJSONBodyInDebugLog(t *testing.T) {
	withDebugEnabled(t, true)

	body := strings.Repeat("e", common.LocalLogContentLimit+256)
	var logBuffer bytes.Buffer

	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logBuffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.NotContains(t, logBuffer.String(), "[truncated")
	require.Contains(t, logBuffer.String(), body)
}

func withDebugEnabled(t *testing.T, enabled bool) {
	t.Helper()

	oldDebug := common.DebugEnabled
	common.DebugEnabled = enabled
	t.Cleanup(func() {
		common.DebugEnabled = oldDebug
	})
}
