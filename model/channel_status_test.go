package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/channelprobe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupChannelStatusTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	memoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = memoryCacheEnabled
	})
}

func TestUpdateChannelStatusPersistsMultiKeyState(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:   "multi-key-status",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         2,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 1,
		},
	}
	require.NoError(t, DB.Create(&channel).Error)

	changed := UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "provider rejected key")
	require.True(t, changed)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, "provider rejected key", stored.ChannelInfo.MultiKeyDisabledReason[0])
	assert.NotZero(t, stored.ChannelInfo.MultiKeyDisabledTime[0])
	assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)
}

func TestSaveStatusStateFromSingleKeySnapshotPreservesUnownedColumns(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:        "single-key-status",
		Key:         "original-key",
		Status:      common.ChannelStatusEnabled,
		Models:      "original-model",
		Group:       "default",
		UsedQuota:   100,
		ChannelInfo: ChannelInfo{},
	}
	require.NoError(t, DB.Create(&channel).Error)

	stale, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)

	concurrentChannelInfo := ChannelInfo{
		IsMultiKey:           true,
		MultiKeySize:         2,
		MultiKeyMode:         constant.MultiKeyModePolling,
		MultiKeyPollingIndex: 1,
	}
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"key":          "rotated-key",
		"used_quota":   gorm.Expr("used_quota + ?", 250),
		"models":       "concurrent-model",
		"channel_info": concurrentChannelInfo,
	}).Error)

	stale.Status = common.ChannelStatusManuallyDisabled
	stale.SetOtherInfo(map[string]any{
		"status_reason": "manual operation",
		"status_time":   int64(1234),
	})
	require.NoError(t, stale.saveStatusState())

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	assert.Equal(t, "rotated-key", stored.Key)
	assert.Equal(t, int64(350), stored.UsedQuota)
	assert.Equal(t, "concurrent-model", stored.Models)
	assert.Equal(t, concurrentChannelInfo, stored.ChannelInfo)

	otherInfo := stored.GetOtherInfo()
	assert.Equal(t, "manual operation", otherInfo["status_reason"])
	assert.Equal(t, float64(1234), otherInfo["status_time"])
}

func TestChannelProbeStateUsesExistingOtherInfoWithoutLoadingKeys(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{Name: "probe-state", Key: "secret", OtherInfo: `{"status_reason":"kept"}`}
	require.NoError(t, DB.Create(&channel).Error)
	channel.OtherInfo = `{"status_reason":"kept","channel_probe":{"next_model_index":1}}`
	require.NoError(t, channel.SaveOtherInfo())

	channels, err := GetAllChannelsWithoutKey()
	require.NoError(t, err)
	require.Len(t, channels, 1)
	assert.Empty(t, channels[0].Key)
	assert.Contains(t, channels[0].OtherInfo, "channel_probe")
}

func TestAutoPausedModelIsExcludedWithDatabaseAndMemoryRouting(t *testing.T) {
	setupChannelStatusTest(t)
	testAutoPausedModelRouting(t)
}

func TestAutoPausedModelDatabaseMatrix(t *testing.T) {
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
			defer func() { require.NoError(t, sqlDB.Close()) }()

			require.NoError(t, db.Migrator().DropTable(&Ability{}, &Channel{}))
			require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

			originalDB, originalLogDB := DB, LOG_DB
			originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
			originalMemoryCache := common.MemoryCacheEnabled
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(test.databaseType, test.databaseType)
			common.MemoryCacheEnabled = false
			initCol()
			defer func() {
				DB, LOG_DB = originalDB, originalLogDB
				common.SetDatabaseTypes(originalMainType, originalLogType)
				common.MemoryCacheEnabled = originalMemoryCache
				initCol()
			}()

			testAutoPausedModelRouting(t)
		})
	}
}

func testAutoPausedModelRouting(t *testing.T) {
	t.Helper()
	versionQuery := "SELECT version()"
	if DB.Dialector.Name() == "sqlite" {
		versionQuery = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, DB.Raw(versionQuery).Scan(&version).Error)
	t.Logf("%s version: %s", DB.Dialector.Name(), version)

	state := channelprobe.State{}
	state.Pause("model-a", "confirmed model failure", 10, 1810, channelprobe.Failure{OccurredAt: 10, StatusCode: 404, Requests: 3})
	otherInfo, err := channelprobe.StateIntoOtherInfo("", state)
	require.NoError(t, err)
	tag := "model-health"
	channel := Channel{
		Name: "model-health", Key: "test-key", Status: common.ChannelStatusEnabled,
		Models: "model-a,model-b", Group: "default", OtherInfo: otherInfo, Tag: &tag,
	}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, channel.UpdateAbilities(nil))

	assertRouting := func(t *testing.T) {
		t.Helper()
		selected, err := GetRandomSatisfiedChannel("default", "model-a", 0, nil)
		require.NoError(t, err)
		assert.Nil(t, selected)
		selected, err = GetRandomSatisfiedChannel("default", "model-b", 0, nil)
		require.NoError(t, err)
		require.NotNil(t, selected)
		assert.Equal(t, channel.Id, selected.Id)
	}

	common.MemoryCacheEnabled = false
	assertRouting(t)
	common.MemoryCacheEnabled = true
	InitChannelCache()
	assertRouting(t)
	require.NoError(t, channel.UpdateAbilities(nil))
	var pausedAbility Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "model-a").First(&pausedAbility).Error)
	assert.False(t, pausedAbility.Enabled)
	succeeded, failed, err := FixAbility()
	require.NoError(t, err)
	assert.Equal(t, 1, succeeded)
	assert.Zero(t, failed)
	assertRouting(t)

	require.NoError(t, DisableChannelByTag(tag))
	selected, err := GetRandomSatisfiedChannel("default", "model-b", 0, nil)
	require.NoError(t, err)
	assert.Nil(t, selected)
	require.NoError(t, EnableChannelByTag(tag))
	assertRouting(t)

	common.MemoryCacheEnabled = false
	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual"))
	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusEnabled, ""))
	assertRouting(t)

	fresh, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	state = channelprobe.StateFromOtherInfo(fresh.OtherInfo)
	state.RecoverNow("model-a", 20)
	require.NoError(t, fresh.SaveProbeState(state))
	selected, err = GetRandomSatisfiedChannel("default", "model-a", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, channel.Id, selected.Id)

	state.Pause("model-a", "confirmed again", 30, 1830, channelprobe.Failure{OccurredAt: 30, StatusCode: 500, Requests: 3})
	require.NoError(t, fresh.SaveProbeState(state))
	fresh.Models = "model-b"
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", fresh.Id).Update("models", fresh.Models).Error)
	require.NoError(t, fresh.UpdateAbilities(nil))
	state.RecoverNow("model-a", 40)
	require.NoError(t, fresh.SaveProbeState(state))
	var removedCount int64
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ? AND model = ?", fresh.Id, "model-a").Count(&removedCount).Error)
	assert.Zero(t, removedCount)
}
