package perfmetrics

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestQuerySummaryAllSuppliesHourlyDisplayWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Advance only the synthetic clock to cover the current partial hour.
		time.Sleep(17 * time.Minute)
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
		previousDB := model.DB
		model.DB = db
		hotBuckets.Clear()
		t.Cleanup(func() {
			model.DB = previousDB
			hotBuckets.Clear()
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Close())
		})
		now := time.Now().Unix()
		currentHour := now - 17*60
		require.NoError(t, db.Create(&model.PerfMetric{
			ModelName: "test-model", Group: "default", BucketTs: currentHour,
			RequestCount: 4, SuccessCount: 3,
		}).Error)
		for _, groups := range [][]string{nil, {}} {
			result, err := QuerySummaryAll(24, groups)
			require.NoError(t, err)
			encoded, err := common.Marshal(result)
			require.NoError(t, err)
			var response struct {
				WindowStart *int64         `json:"window_start"`
				WindowEnd   *int64         `json:"window_end"`
				Models      []ModelSummary `json:"models"`
			}
			require.NoError(t, common.Unmarshal(encoded, &response))
			require.NotNil(t, response.WindowStart, "the marketplace needs a server-provided hour for its first status bar")
			require.NotNil(t, response.WindowEnd)
			assert.Equal(t, now, *response.WindowEnd)
			assert.Equal(t, currentHour-23*3600, *response.WindowStart)
			if groups != nil {
				assert.Empty(t, response.Models)
				continue
			}
			require.Len(t, response.Models, 1)
			assert.Equal(t, []SuccessRatePoint{{Ts: *response.WindowStart + 23*3600, SuccessRate: 75}}, response.Models[0].RecentSuccessSeries)
		}
	})
}

func TestRecordTaskResultSamplesTerminalTasks(t *testing.T) {
	hotBuckets.Clear()
	t.Cleanup(func() { hotBuckets.Clear() })
	now := time.Now().Unix()
	RecordTaskResult(&model.Task{
		Status:     model.TaskStatusSuccess,
		Group:      "a",
		SubmitTime: now - 120,
		StartTime:  now - 100,
		FinishTime: now,
		Properties: model.Properties{OriginModelName: "video-model"},
	}, &relaycommon.TaskInfo{TotalTokens: 5000})
	RecordTaskResult(&model.Task{
		Status:     model.TaskStatusFailure,
		Group:      "a",
		SubmitTime: now - 60,
		FinishTime: now,
		Properties: model.Properties{OriginModelName: "video-model"},
	}, relaycommon.FailTaskInfo("boom"))
	RecordTaskResult(&model.Task{Status: model.TaskStatusSuccess, FinishTime: now}, nil)

	merged := map[bucketKey]counters{}
	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		require.Equal(t, "video-model", k.model)
		require.Equal(t, "a", k.group)
		k.bucketTs = 0
		mergeCounters(merged, k, value.(*atomicBucket).snapshot())
		return true
	})
	assert.Equal(t, counters{
		requestCount:   2,
		successCount:   1,
		totalLatencyMs: 180000,
		outputTokens:   5000,
		generationMs:   100000,
	}, merged[bucketKey{model: "video-model", group: "a"}])
}
