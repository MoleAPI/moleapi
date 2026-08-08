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

func TestGeminiImageDefaultsSeparateTextAndImageOutputPricing(t *testing.T) {
	assert.Equal(t, 0.25, defaultModelRatio["gemini-3.1-flash-image-preview"])
	assert.Equal(t, 6.0, defaultCompletionRatio["gemini-3.1-flash-image-preview"])
	assert.Equal(t, 120.0, defaultImageOutputRatio["gemini-3.1-flash-image-preview"])
}

func TestGPTImageDefaultsPriceEachTokenModality(t *testing.T) {
	tests := []struct {
		model            string
		modelRatio       float64
		completionRatio  float64
		cacheRatio       float64
		imageRatio       float64
		imageOutputRatio float64
	}{
		{"gpt-image-1", 2.5, 8, 0.25, 2, 8},
		{"gpt-image-1-mini", 1, 4, 0.1, 1.25, 4},
		{"gpt-image-1.5", 2.5, 2, 0.25, 1.6, 6.4},
		{"chatgpt-image-latest", 2.5, 2, 0.25, 1.6, 6.4},
		{"gpt-image-2", 2.5, 6, 0.25, 1.6, 6},
		{"gpt-image-2-2026-04-21", 2.5, 6, 0.25, 1.6, 6},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.modelRatio, defaultModelRatio[tt.model])
			assert.Equal(t, tt.completionRatio, defaultCompletionRatio[tt.model])
			assert.Equal(t, tt.cacheRatio, defaultCacheRatio[tt.model])
			assert.Equal(t, tt.imageRatio, defaultImageRatio[tt.model])
			assert.Equal(t, tt.imageOutputRatio, defaultImageOutputRatio[tt.model])
		})
	}
}
