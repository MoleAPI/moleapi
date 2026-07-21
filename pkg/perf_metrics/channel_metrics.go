package perfmetrics

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

const (
	channelMetricBucketSeconds = int64(3600)
	channelMetricMaxHours      = 24 * 7
)

var channelHotBuckets sync.Map

type channelBucketKey struct {
	channelId int
	bucketTs  int64
}

type channelCounters struct {
	requestCount int64
	successCount int64
}

type atomicChannelBucket struct {
	requestCount atomic.Int64
	successCount atomic.Int64
}

type ChannelBucketPoint struct {
	Ts           int64   `json:"ts"`
	RequestCount int64   `json:"request_count"`
	SuccessCount int64   `json:"success_count"`
	SuccessRate  float64 `json:"success_rate"`
}

type ChannelSummary struct {
	ChannelId    int                  `json:"channel_id"`
	ChannelName  string               `json:"channel_name"`
	RequestCount int64                `json:"request_count"`
	SuccessCount int64                `json:"success_count"`
	SuccessRate  float64              `json:"success_rate"`
	Series       []ChannelBucketPoint `json:"series"`
}

type ChannelSuccessResult struct {
	Channels []ChannelSummary `json:"channels"`
}

func RecordChannelAttempt(channelId int, success bool) {
	if channelId <= 0 || !perf_metrics_setting.GetSetting().Enabled {
		return
	}
	key := channelBucketKey{
		channelId: channelId,
		bucketTs:  channelBucketStart(time.Now().Unix()),
	}
	actual, _ := channelHotBuckets.LoadOrStore(key, &atomicChannelBucket{})
	bucket := actual.(*atomicChannelBucket)
	bucket.requestCount.Add(1)
	if success {
		bucket.successCount.Add(1)
	}
}

func QueryChannelSuccess(hours int) (ChannelSuccessResult, error) {
	if hours <= 0 {
		hours = 24
	}
	if hours > channelMetricMaxHours {
		hours = channelMetricMaxHours
	}
	endTs := time.Now().Unix()
	startTs := channelBucketStart(endTs) - int64(hours-1)*channelMetricBucketSeconds
	merged := make(map[channelBucketKey]channelCounters)

	rows, err := model.GetChannelPerfMetrics(startTs, endTs)
	if err != nil {
		return ChannelSuccessResult{}, err
	}
	for _, row := range rows {
		mergeChannelCounters(merged, channelBucketKey{
			channelId: row.ChannelId,
			bucketTs:  row.BucketTs,
		}, channelCounters{
			requestCount: row.RequestCount,
			successCount: row.SuccessCount,
		})
	}

	// ponytail: the active hour is process-local; add Redis merging only if
	// live cross-node accuracy becomes necessary.
	channelHotBuckets.Range(func(key, value any) bool {
		k := key.(channelBucketKey)
		if k.bucketTs < startTs || k.bucketTs > endTs {
			return true
		}
		mergeChannelCounters(merged, k, value.(*atomicChannelBucket).snapshot())
		return true
	})

	channelIds := make([]int, 0)
	seen := make(map[int]struct{})
	for key, value := range merged {
		if value.requestCount == 0 {
			continue
		}
		if _, ok := seen[key.channelId]; ok {
			continue
		}
		seen[key.channelId] = struct{}{}
		channelIds = append(channelIds, key.channelId)
	}
	names, err := model.GetChannelNamesByIDs(channelIds)
	if err != nil {
		return ChannelSuccessResult{}, err
	}
	return buildChannelSuccessResult(merged, names), nil
}

func buildChannelSuccessResult(merged map[channelBucketKey]channelCounters, names map[int]string) ChannelSuccessResult {
	bucketsByChannel := make(map[int]map[int64]channelCounters)
	for key, value := range merged {
		if value.requestCount == 0 {
			continue
		}
		if _, ok := bucketsByChannel[key.channelId]; !ok {
			bucketsByChannel[key.channelId] = make(map[int64]channelCounters)
		}
		bucketsByChannel[key.channelId][key.bucketTs] = value
	}

	channels := make([]ChannelSummary, 0, len(bucketsByChannel))
	for channelId, buckets := range bucketsByChannel {
		timestamps := make([]int64, 0, len(buckets))
		for ts := range buckets {
			timestamps = append(timestamps, ts)
		}
		sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

		var total channelCounters
		series := make([]ChannelBucketPoint, 0, len(timestamps))
		for _, ts := range timestamps {
			value := buckets[ts]
			total.requestCount += value.requestCount
			total.successCount += value.successCount
			series = append(series, ChannelBucketPoint{
				Ts:           ts,
				RequestCount: value.requestCount,
				SuccessCount: value.successCount,
				SuccessRate:  channelSuccessRate(value),
			})
		}
		name := names[channelId]
		if name == "" {
			name = fmt.Sprintf("channel-%d", channelId)
		}
		channels = append(channels, ChannelSummary{
			ChannelId:    channelId,
			ChannelName:  name,
			RequestCount: total.requestCount,
			SuccessCount: total.successCount,
			SuccessRate:  channelSuccessRate(total),
			Series:       series,
		})
	}
	sort.Slice(channels, func(i, j int) bool {
		if channels[i].RequestCount == channels[j].RequestCount {
			return channels[i].ChannelId < channels[j].ChannelId
		}
		return channels[i].RequestCount > channels[j].RequestCount
	})
	return ChannelSuccessResult{Channels: channels}
}

func channelBucketStart(ts int64) int64 {
	return ts - ts%channelMetricBucketSeconds
}

func channelSuccessRate(value channelCounters) float64 {
	if value.requestCount <= 0 {
		return 0
	}
	rate := float64(value.successCount) / float64(value.requestCount) * 100
	return math.Round(rate*100) / 100
}

func mergeChannelCounters(merged map[channelBucketKey]channelCounters, key channelBucketKey, value channelCounters) {
	if value.requestCount == 0 {
		return
	}
	current := merged[key]
	current.requestCount += value.requestCount
	current.successCount += value.successCount
	merged[key] = current
}

func flushCompletedChannelBuckets() {
	currentBucket := channelBucketStart(time.Now().Unix())
	channelHotBuckets.Range(func(key, value any) bool {
		k := key.(channelBucketKey)
		if k.bucketTs >= currentBucket {
			return true
		}
		bucket := value.(*atomicChannelBucket)
		drained := bucket.drain()
		if drained.requestCount == 0 {
			deleteOldChannelBucket(k, key)
			return true
		}
		if err := model.UpsertChannelPerfMetric(&model.ChannelPerfMetric{
			ChannelId:    k.channelId,
			BucketTs:     k.bucketTs,
			RequestCount: drained.requestCount,
			SuccessCount: drained.successCount,
		}); err != nil {
			bucket.addCounters(drained)
			common.SysError(fmt.Sprintf("failed to flush channel metric bucket channel=%d bucket=%d: %s", k.channelId, k.bucketTs, err.Error()))
			return true
		}
		deleteOldChannelBucket(k, key)
		return true
	})
}

func deleteOldChannelBucket(k channelBucketKey, rawKey any) {
	if k.bucketTs < channelBucketStart(time.Now().Add(-24*time.Hour).Unix()) {
		channelHotBuckets.Delete(rawKey)
	}
}

func (b *atomicChannelBucket) snapshot() channelCounters {
	return channelCounters{
		requestCount: b.requestCount.Load(),
		successCount: b.successCount.Load(),
	}
}

func (b *atomicChannelBucket) drain() channelCounters {
	return channelCounters{
		requestCount: b.requestCount.Swap(0),
		successCount: b.successCount.Swap(0),
	}
}

func (b *atomicChannelBucket) addCounters(value channelCounters) {
	b.requestCount.Add(value.requestCount)
	b.successCount.Add(value.successCount)
}
