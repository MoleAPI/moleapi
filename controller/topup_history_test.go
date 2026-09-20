package controller

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUserBillingDateRange(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	start := time.Now().AddDate(0, -3, 0).Unix()
	for i, row := range []model.TopUp{
		{UserId: 7, CreateTime: start - 1}, {UserId: 7, CreateTime: start},
		{UserId: 7, CreateTime: start + 60}, {UserId: 7, CreateTime: start + 61},
		{UserId: 8, CreateTime: start},
	} {
		row.TradeNo = fmt.Sprintf("range-%d", i)
		require.NoError(t, db.Create(&row).Error)
	}
	for _, query := range []string{
		fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", start, start+60),
		fmt.Sprintf("keyword=range-%%25&start_timestamp=%d&end_timestamp=%d", start, start+60),
	} {
		r := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(r)
		c.Set("id", 7)
		c.Request = httptest.NewRequest("GET", "/api/user/topup/self?"+query, nil)
		GetUserTopUps(c)
		var response struct {
			Success bool
			Data    struct {
				Total int
				Items []model.TopUp
			}
		}
		require.NoError(t, common.Unmarshal(r.Body.Bytes(), &response))
		require.True(t, response.Success, r.Body.String())
		assert.Equal(t, 2, response.Data.Total)
		require.Len(t, response.Data.Items, 2)
		for _, row := range response.Data.Items {
			assert.Equal(t, 7, row.UserId)
			assert.GreaterOrEqual(t, row.CreateTime, start)
			assert.LessOrEqual(t, row.CreateTime, start+60)
		}
	}
}

func TestGetInviteRebateTopUpsHidesDatabaseErrors(t *testing.T) {
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
	})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/api/user/aff/history?p=1&page_size=10", nil)
	context.Request.Header.Set("Accept-Language", "en")
	context.Set("id", 7)

	GetInviteRebateTopUps(context)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, common.TranslateMessage(context, i18n.MsgDatabaseError), response.Message)
	assert.NotContains(t, recorder.Body.String(), "database is closed")
}
