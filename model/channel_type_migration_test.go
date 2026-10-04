package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func testLegacyCodingPlanChannelMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	versionQuery := "SELECT version()"
	if db.Dialector.Name() == "sqlite" {
		versionQuery = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("%s version: %s", db.Dialector.Name(), version)

	for _, target := range []any{&Option{}, &Channel{}} {
		_ = db.Migrator().DropTable(target)
	}
	t.Cleanup(func() {
		_ = db.Migrator().DropTable(&Option{}, &Channel{})
	})

	// Fresh databases receive the marker and repeated startup is a no-op.
	require.NoError(t, db.AutoMigrate(&Channel{}, &Option{}))
	require.NoError(t, migrateLegacyCodingPlanChannelTypes(db))
	require.NoError(t, migrateLegacyCodingPlanChannelTypes(db))
	var freshMarkerCount int64
	require.NoError(t, db.Model(&Option{}).Where(&Option{Key: legacyCodingPlanChannelMigrationKey}).Count(&freshMarkerCount).Error)
	assert.Equal(t, int64(1), freshMarkerCount)

	require.NoError(t, db.Migrator().DropTable(&Option{}, &Channel{}))
	require.NoError(t, db.AutoMigrate(&Channel{}, &Option{}))

	legacySettings, err := common.Marshal(dto.ChannelOtherSettings{CodingPlanProvider: dto.CodingPlanProviderGLMChina})
	require.NoError(t, err)
	editedSettings, err := common.Marshal(dto.ChannelOtherSettings{
		CodingPlanProvider: dto.CodingPlanProviderKimi,
		AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
			IncomingPath: "/v1/chat/completions",
			UpstreamPath: "https://edited.example/v1/chat/completions",
			Converter:    "none",
		}}},
	})
	require.NoError(t, err)
	providerBase := dto.CodingPlanProviderGLMChina
	vllmSetting := `{"proxy":"https://proxy.example"}`
	taskSetting := `{"task_plugin_key":"legacy-task"}`
	channels := []Channel{
		{Name: "legacy-generated", Type: 59, Key: "key", BaseURL: &providerBase, OtherSettings: string(legacySettings)},
		{Name: "legacy-edited", Type: 59, Key: "key", OtherSettings: string(editedSettings)},
		{Name: "current-sub2api", Type: 59, Key: "key", OtherSettings: "{}"},
		{Name: "legacy-sub2api", Type: 60, Key: "key"},
		{Name: "legacy-new-api", Type: 61, Key: "key"},
		{Name: "legacy-task-plugin", Type: 62, Key: "key", Setting: &taskSetting},
		{Name: "current-vllm", Type: 62, Key: "key", Setting: &vllmSetting},
		{Name: "current-sglang", Type: 63, Key: "key"},
	}
	require.NoError(t, db.Create(&channels).Error)

	require.NoError(t, migrateLegacyCodingPlanChannelTypes(db))
	require.NoError(t, migrateLegacyCodingPlanChannelTypes(db))

	var migrated []Channel
	require.NoError(t, db.Order("id").Find(&migrated).Error)
	require.Len(t, migrated, len(channels))
	byName := make(map[string]Channel, len(migrated))
	for _, channel := range migrated {
		byName[channel.Name] = channel
	}

	assert.Equal(t, constant.ChannelTypeAdvancedCustom, byName["legacy-generated"].Type)
	generatedChannel := byName["legacy-generated"]
	generated := generatedChannel.GetOtherSettings()
	assert.Empty(t, generated.CodingPlanProvider)
	require.NotNil(t, generated.AdvancedCustom)
	assert.True(t, generated.AdvancedCustom.SupportsPathForModel("/v1/messages", "glm-5"))

	assert.Equal(t, constant.ChannelTypeAdvancedCustom, byName["legacy-edited"].Type)
	editedChannel := byName["legacy-edited"]
	edited := editedChannel.GetOtherSettings()
	assert.Empty(t, edited.CodingPlanProvider)
	route, ok := edited.AdvancedCustom.MatchPath("/v1/chat/completions")
	require.True(t, ok)
	assert.Equal(t, "https://edited.example/v1/chat/completions", route.UpstreamPath)

	assert.Equal(t, constant.ChannelTypeSub2API, byName["current-sub2api"].Type)
	assert.Equal(t, constant.ChannelTypeSub2API, byName["legacy-sub2api"].Type)
	assert.Equal(t, constant.ChannelTypeNewAPI, byName["legacy-new-api"].Type)
	assert.Equal(t, constant.ChannelTypeTaskPlugin, byName["legacy-task-plugin"].Type)
	assert.Equal(t, constant.ChannelTypeVLLM, byName["current-vllm"].Type)
	assert.Equal(t, constant.ChannelTypeSGLang, byName["current-sglang"].Type)
	assert.True(t, db.Migrator().HasIndex(&Channel{}, "Name"))

	var markerCount int64
	require.NoError(t, db.Model(&Option{}).Where(&Option{Key: legacyCodingPlanChannelMigrationKey}).Count(&markerCount).Error)
	assert.Equal(t, int64(1), markerCount)
}

func TestLegacyCodingPlanChannelMigrationSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	testLegacyCodingPlanChannelMigration(t, db)
}

func TestLegacyCodingPlanChannelMigrationMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	testLegacyCodingPlanChannelMigration(t, db)
}

func TestLegacyCodingPlanChannelMigrationPostgreSQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{})
	require.NoError(t, err)
	testLegacyCodingPlanChannelMigration(t, db)
}
