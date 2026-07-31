package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPerplexityLargeDefaultRatiosAreNotFree(t *testing.T) {
	expected := 1.0 / 1000 * USD
	assert.Equal(t, expected, defaultModelRatio["llama-3-sonar-large-32k-chat"])
	assert.Equal(t, expected, defaultModelRatio["llama-3-sonar-large-32k-online"])
}
