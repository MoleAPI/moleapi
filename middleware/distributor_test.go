package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelRequestNormalizesClientModelAliasesFromPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "mole gpt alias",
			path:     "/v1beta/models/mole-gpt5.6-sol:generateContent",
			expected: "gpt-5.6-sol",
		},
		{
			name:     "bare gpt alias",
			path:     "/v1beta/models/gpt5.6-sol:generateContent",
			expected: "gpt-5.6-sol",
		},
		{
			name:     "mole non gpt alias",
			path:     "/v1beta/models/mole-claude-opus-4-6:generateContent",
			expected: "claude-opus-4-6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)

			request, shouldSelectChannel, err := getModelRequest(ctx)

			require.NoError(t, err)
			require.NotNil(t, request)
			assert.True(t, shouldSelectChannel)
			assert.Equal(t, tt.expected, request.Model)
		})
	}
}

func TestRequestPathForChannelSelectionNormalizesPlaygroundChat(t *testing.T) {
	assert.Equal(t, "/v1/chat/completions", requestPathForChannelSelection("/pg/chat/completions", "gpt-4o"))
	assert.Equal(t, "/v1/images/generations", requestPathForChannelSelection("/pg/chat/completions", "gpt-image-2"))
	assert.Equal(t, "/v1/images/generations", requestPathForChannelSelection("/v1/chat/completions", "gpt-image-2"))
	assert.Equal(t, "/v1/images/generations", requestPathForChannelSelection("/v1/responses", "gpt-image-2"))
	assert.Equal(t, "/v1/responses", requestPathForChannelSelection("/v1/responses", "gpt-4o"))
}
