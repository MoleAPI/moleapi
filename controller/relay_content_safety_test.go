package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModerationScanFlaggedAllowsCleanResults(t *testing.T) {
	flagged, categories, err := moderationScanFlagged([]byte(`{"results":[{"flagged":false,"categories":{"hate":false}}]}`))

	require.NoError(t, err)
	assert.False(t, flagged)
	assert.Empty(t, categories)
}

func TestModerationScanFlaggedReturnsMatchedCategories(t *testing.T) {
	flagged, categories, err := moderationScanFlagged([]byte(`{"results":[{"flagged":true,"categories":{"violence":true,"hate":true,"self-harm":false}},{"flagged":true,"categories":{"hate":true}}]}`))

	require.NoError(t, err)
	assert.True(t, flagged)
	assert.Equal(t, []string{"hate", "violence"}, categories)
}

func TestModerationUpstreamModelUsesChannelMapping(t *testing.T) {
	modelMapping := `{"text-moderation-stable":"omni-moderation-latest"}`

	modelName, err := moderationUpstreamModel(&model.Channel{ModelMapping: &modelMapping})

	require.NoError(t, err)
	assert.Equal(t, "omni-moderation-latest", modelName)
}
