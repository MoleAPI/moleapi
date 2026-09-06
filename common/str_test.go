package common

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestLogDetailPreviewLimitsPersistedContent(t *testing.T) {
	content := strings.Repeat("x", LogDetailContentLimit+20)

	preview := LogDetailPreview(content)

	assert.LessOrEqual(t, len(preview), LogDetailContentLimit+64)
	assert.Contains(t, preview, "[truncated")
	assert.Equal(t, content[:LogDetailContentLimit], preview[:LogDetailContentLimit])

	unicodeContent := strings.Repeat("界", LogDetailContentLimit/3+2)
	unicodePreview := LogDetailPreview(unicodeContent)
	assert.True(t, utf8.ValidString(unicodePreview))
}
