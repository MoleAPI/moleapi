package helper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeUpstreamStreamError(t *testing.T) {
	upstream := `{"type":"response.failed","error":{"message":"private provider error","code":"private_code"}}`

	public := sanitizeUpstreamStreamError(nil, upstream)

	assert.Contains(t, public, "The upstream service is temporarily unavailable")
	assert.NotContains(t, public, "private provider error")
	assert.NotContains(t, public, "private_code")
	assert.Equal(t, `{"type":"message","content":"normal"}`, sanitizeUpstreamStreamError(nil, `{"type":"message","content":"normal"}`))
	assert.Equal(t, `{"type":"response.completed","error":null}`, sanitizeUpstreamStreamError(nil, `{"type":"response.completed","error":null}`))
}
