package model

import (
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLogListSearchMatchesUserIdAndBothRequestIds(t *testing.T) {
	previousLogDB := LOG_DB
	previousLogDatabaseType := common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogDatabaseType)
		if sqlDB, err := db.DB(); err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	require.NoError(t, db.AutoMigrate(&Log{}))
	require.NoError(t, db.Create([]Log{
		{
			UserId:            101,
			Username:          "alice",
			CreatedAt:         10,
			RequestId:         "local-a",
			UpstreamRequestId: "upstream-a",
		},
		{
			UserId:            202,
			Username:          "bob",
			CreatedAt:         20,
			RequestId:         "local-b",
			UpstreamRequestId: "upstream-b",
		},
	}).Error)

	logs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", " 202 ", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, "bob", logs[0].Username)

	logs, total, err = GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", " upstream-b ", "")
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, 202, logs[0].UserId)

	logs, total, err = GetUserLogs(202, LogTypeUnknown, 0, 0, "", "", 0, 10, "", " upstream-b ", "")
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, "bob", logs[0].Username)
}
