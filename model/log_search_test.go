package model

import (
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
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

func TestLogReadsHideDatabaseErrors(t *testing.T) {
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
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("fail_admin_log_read", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*[]*Log); ok {
			tx.AddError(errors.New("database host and table details"))
		}
	}))

	_, _, err = GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", "", "")
	assert.EqualError(t, err, "查询日志失败")

	_, err = GetLogByTokenId(42)
	assert.EqualError(t, err, "查询日志失败")
}

func TestGetUserLogsCapsCount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	testGetUserLogsCapsCount(t, db, common.DatabaseTypeSQLite)
}

func TestGetUserLogsCapsCountDatabaseMatrix(t *testing.T) {
	tests := []struct {
		name         string
		environment  string
		databaseType common.DatabaseType
		open         func(string) gorm.Dialector
	}{
		{name: "mysql", environment: "TEST_MYSQL_DSN", databaseType: common.DatabaseTypeMySQL, open: mysql.Open},
		{name: "postgres", environment: "TEST_POSTGRES_DSN", databaseType: common.DatabaseTypePostgreSQL, open: func(dsn string) gorm.Dialector {
			return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.environment))
			if dsn == "" {
				t.Skip(test.environment + " is not configured")
			}
			db, err := gorm.Open(test.open(dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			testGetUserLogsCapsCount(t, db, test.databaseType)
		})
	}
}

func testGetUserLogsCapsCount(t *testing.T, db *gorm.DB, databaseType common.DatabaseType) {
	t.Helper()
	previousLogDB := LOG_DB
	previousLogDatabaseType := common.LogDatabaseType()
	LOG_DB = db
	common.SetLogDatabaseType(databaseType)
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogDatabaseType)
	})

	require.NoError(t, db.Migrator().DropTable(&Log{}))
	require.NoError(t, db.AutoMigrate(&Log{}))
	t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&Log{})) })
	logs := make([]Log, logSearchCountLimit+1)
	for i := range logs {
		logs[i] = Log{UserId: 303, CreatedAt: int64(i + 1), RequestId: "request-" + strconv.Itoa(i)}
	}
	require.NoError(t, db.CreateInBatches(logs, 500).Error)

	result, total, err := GetUserLogs(303, LogTypeUnknown, 0, 0, "", "", 0, 1, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(logSearchCountLimit), total)
	require.Len(t, result, 1)
}
