package perfmetrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChannelSuccessResultAggregatesAndSortsChannels(t *testing.T) {
	result := buildChannelSuccessResult(map[channelBucketKey]channelCounters{
		{channelId: 1, bucketTs: 200}: {requestCount: 5, successCount: 4},
		{channelId: 1, bucketTs: 100}: {requestCount: 10, successCount: 9},
		{channelId: 2, bucketTs: 100}: {requestCount: 4, successCount: 4},
	}, map[int]string{1: "primary"})

	require.Len(t, result.Channels, 2)
	assert.Equal(t, 1, result.Channels[0].ChannelId)
	assert.Equal(t, "primary", result.Channels[0].ChannelName)
	assert.Equal(t, int64(15), result.Channels[0].RequestCount)
	assert.Equal(t, int64(13), result.Channels[0].SuccessCount)
	assert.Equal(t, 86.67, result.Channels[0].SuccessRate)
	require.Len(t, result.Channels[0].Series, 2)
	assert.Equal(t, int64(100), result.Channels[0].Series[0].Ts)
	assert.Equal(t, int64(200), result.Channels[0].Series[1].Ts)

	assert.Equal(t, 2, result.Channels[1].ChannelId)
	assert.Equal(t, "channel-2", result.Channels[1].ChannelName)
	assert.Equal(t, 100.0, result.Channels[1].SuccessRate)
}
