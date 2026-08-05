package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupManageUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.Log{}, &model.CasbinRule{}, &model.AuthzRole{},
	))

	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func performManageUserRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/manage", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 9999)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "root-operator")
	ManageUser(c)
	return recorder
}

func TestManageUserDisableAdvancesAuthVersionOnceAndRevokesSession(t *testing.T) {
	db := setupManageUserTestDB(t)
	now := time.Now().Unix()
	user := model.User{
		Username: "managed-disable-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.UserSession{
		SID: "managed-disable-session", UserID: user.Id, Version: 1, UserAuthVersion: 1,
		Status: model.UserSessionStatusActive, RefreshHash: "refresh-hash", LoginMethod: "password",
		LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"disable"}`, user.Id))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, updated.Status)
	assert.EqualValues(t, 2, updated.AuthVersion)
	var session model.UserSession
	require.NoError(t, db.First(&session, "sid = ?", "managed-disable-session").Error)
	assert.Equal(t, model.UserSessionStatusRevoked, session.Status)
}

func TestManageUserDemoteAdvancesAuthVersionAndRevokesSessionsOnce(t *testing.T) {
	db := setupManageUserTestDB(t)
	previousMaster := common.IsMasterNode
	common.IsMasterNode = false
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	require.NoError(t, authz.Init(db))

	now := time.Now().Unix()
	user := model.User{
		Username: "managed-demote-user", Password: "password", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	for _, sid := range []string{"managed-demote-session-one", "managed-demote-session-two"} {
		require.NoError(t, db.Create(&model.UserSession{
			SID: sid, UserID: user.Id, Version: 1, UserAuthVersion: 1,
			Status: model.UserSessionStatusActive, RefreshHash: "refresh-" + sid, LoginMethod: "password",
			LastActiveAt: now, ExpiresAt: now + 3600,
		}).Error)
	}

	sessionUpdateCount := 0
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:count_demote_session_updates", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "user_sessions" {
			sessionUpdateCount++
		}
	}))

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"demote"}`, user.Id))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, common.RoleCommonUser, updated.Role)
	assert.EqualValues(t, 2, updated.AuthVersion)
	var sessions []model.UserSession
	require.NoError(t, db.Where("user_id = ?", user.Id).Order("sid asc").Find(&sessions).Error)
	require.Len(t, sessions, 2)
	for _, session := range sessions {
		assert.Equal(t, model.UserSessionStatusRevoked, session.Status)
		assert.Equal(t, "admin_demote", session.RevokedReason)
	}
	assert.Equal(t, 1, sessionUpdateCount)
}

func TestManageUserDeleteReturnsImmediatelyAndUnknownActionFails(t *testing.T) {
	db := setupManageUserTestDB(t)
	deleted := model.User{
		Username: "managed-delete-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "delete-aff",
	}
	require.NoError(t, db.Create(&deleted).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"delete"}`, deleted.Id))
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var deletedCount int64
	require.NoError(t, db.Unscoped().Model(&model.User{}).Where("id = ? AND deleted_at IS NOT NULL", deleted.Id).Count(&deletedCount).Error)
	assert.EqualValues(t, 1, deletedCount)

	unchanged := model.User{
		Username: "managed-unknown-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "unknown-aff",
	}
	require.NoError(t, db.Create(&unchanged).Error)
	recorder = performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"unknown"}`, unchanged.Id))
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	require.NoError(t, db.First(&unchanged, unchanged.Id).Error)
	assert.EqualValues(t, 1, unchanged.AuthVersion)
	assert.Equal(t, common.UserStatusEnabled, unchanged.Status)
}

func TestManageUserQuotaAuditIncludesRawQuota(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{
		Username: "managed-quota-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", Quota: 100,
	}
	require.NoError(t, db.Create(&user).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"subtract","value":25}`, user.Id))
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var log model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeManage).First(&log).Error)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	op := other["op"].(map[string]interface{})
	params := op["params"].(map[string]interface{})
	assert.Equal(t, "user.quota_subtract", op["action"])
	assert.EqualValues(t, user.Id, params["target_user_id"])
	assert.EqualValues(t, 25, params["quota_raw"])

	var rewardLog model.Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", user.Id, model.LogTypeSystem).First(&rewardLog).Error)
	assert.Equal(t, -25, rewardLog.Quota)
	assert.Contains(t, rewardLog.Content, "管理员调整额度")
	require.NoError(t, common.UnmarshalJsonStr(rewardLog.Other, &other))
	op = other["op"].(map[string]interface{})
	assert.Equal(t, "user.quota_subtract", op["action"])
}

func TestManageUserQuotaSupportsLargeBalanceAdjustments(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("large account balances require a 64-bit server")
	}
	db := setupManageUserTestDB(t)
	const largeAdjustment int64 = 5_000_000_000
	user := model.User{
		Username: "managed-quota-limit", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", Quota: common.MaxQuota,
	}
	require.NoError(t, db.Create(&user).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"add","value":%d}`, user.Id, largeAdjustment))
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	recorder = performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"subtract","value":1000000000}`, user.Id))
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	recorder = performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"override","value":6000000000}`, user.Id))
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.EqualValues(t, int64(6_000_000_000), updated.Quota)

	var systemLogs []model.Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", user.Id, model.LogTypeSystem).Order("id asc").Find(&systemLogs).Error)
	require.Len(t, systemLogs, 3)
	assert.EqualValues(t, largeAdjustment, systemLogs[0].Quota)
	assert.EqualValues(t, -1_000_000_000, systemLogs[1].Quota)
}

func TestManageUserQuotaRejectsBalanceOutsideExactRange(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("large account balances require a 64-bit server")
	}
	db := setupManageUserTestDB(t)
	user := model.User{
		Username: "managed-quota-overflow", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", Quota: 100,
	}
	require.NoError(t, db.Create(&user).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"add","value":9007199254740992}`, user.Id))
	assert.Contains(t, recorder.Body.String(), `"success":false`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, 100, updated.Quota)
	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Count(&logCount).Error)
	assert.Zero(t, logCount)
}

func TestManageUserStopsWhenTargetLookupFails(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:fail_manage_user_lookup", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "users" {
			tx.AddError(errors.New("sensitive database detail"))
		}
	}))
	t.Cleanup(func() {
		db.Callback().Query().Remove("test:fail_manage_user_lookup")
	})

	recorder := performManageUserRequest(t, `{"id":1,"action":"disable"}`)

	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.NotContains(t, recorder.Body.String(), "sensitive database detail")
}
