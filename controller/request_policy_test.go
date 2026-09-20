package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestPolicyAndChannelUsageDatabaseMatrix(t *testing.T) {
	for _, kind := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(kind, func(t *testing.T) {
			dsn := os.Getenv("TEST_" + strings.ToUpper(kind) + "_DSN")
			if kind != "sqlite" && dsn == "" {
				t.Skip("test database DSN is not configured")
			}
			db, _ := newAuditTestDatabase(t, kind, dsn)
			logDB, _ := newAuditTestDatabase(t, kind, dsn)
			previousDB, previousLogDB := model.DB, model.LOG_DB
			previousOptions := common.OptionMap
			previousMonitor := *operation_setting.GetMonitorSetting()
			previousAffinity := *operation_setting.GetChannelAffinitySetting()
			previousRetry := common.RetryTimes
			model.DB, model.LOG_DB = db, logDB
			common.OptionMap = make(map[string]string)
			t.Cleanup(func() {
				model.DB, model.LOG_DB = previousDB, previousLogDB
				common.OptionMap = previousOptions
				*operation_setting.GetMonitorSetting() = previousMonitor
				*operation_setting.GetChannelAffinitySetting() = previousAffinity
				common.RetryTimes = previousRetry
			})
			require.NoError(t, db.AutoMigrate(&model.Option{}))
			require.NoError(t, logDB.AutoMigrate(&model.Log{}))
			request := func(body string) *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPatch, "/api/option/request_policy", strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				UpdateRequestPolicy(c)
				return w
			}
			response := request(`{"options":{"RetryTimes":"2","channel_affinity_setting.session_mode":"strict","monitor_setting.channel_test_mode":"auto_detect","monitor_setting.channel_test_type":"intelligence"}}`)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Equal(t, 2, common.RetryTimes)
			assert.Equal(t, "strict", operation_setting.GetChannelAffinitySetting().SessionMode)
			assert.Equal(t, "intelligence", operation_setting.GetMonitorSetting().ChannelTestType)
			assert.Equal(t, "auto_detect", operation_setting.GetMonitorSetting().ChannelTestMode)
			var option model.Option
			require.NoError(t, db.Where(&model.Option{Key: "monitor_setting.channel_test_type"}).First(&option).Error)
			assert.Equal(t, "intelligence", option.Value)
			for _, body := range []string{
				`{"options":{"RetryTimes":"-1","monitor_setting.channel_test_type":"hi"}}`,
				`{"options":{"RetryTimes":"3","monitor_setting.channel_test_concurrency":"99"}}`,
				`{"options":{"RetryTimes":"3","monitor_setting.channel_test_type":"invalid"}}`,
				`{"options":{"channel_affinity_setting.session_mode":"invalid"}}`,
				`{"options":{"monitor_setting.channel_test_type":"custom"}}`,
				`{"options":{"ZohoDeskClientSecret":"forbidden"}}`,
			} {
				response = request(body)
				assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
				assert.Equal(t, 2, common.RetryTimes, "invalid patches must not partially apply")
			}
			logs := []model.Log{
				{Type: model.LogTypeConsume, ChannelId: 1, CreatedAt: 100, Quota: 10},
				{Type: model.LogTypeConsume, ChannelId: 1, CreatedAt: 101, Quota: 20},
				{Type: model.LogTypeConsume, ChannelId: 2, CreatedAt: 199, Quota: 40},
				{Type: model.LogTypeConsume, ChannelId: 1, CreatedAt: 99, Quota: 100},
				{Type: model.LogTypeConsume, ChannelId: 1, CreatedAt: 200, Quota: 100},
				{Type: model.LogTypeTopup, ChannelId: 1, CreatedAt: 150, Quota: 100},
			}
			require.NoError(t, logDB.Create(&logs).Error)
			usage, err := model.ChannelQuotaUsage(context.Background(), 100, 200)
			require.NoError(t, err)
			assert.Equal(t, map[int]int64{1: 30, 2: 40}, usage)
			var count int64
			require.NoError(t, logDB.Model(&model.Log{}).Count(&count).Error)
			assert.EqualValues(t, len(logs), count)
		})
	}
}
