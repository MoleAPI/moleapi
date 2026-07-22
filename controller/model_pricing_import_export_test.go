package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelPricingOptionTest(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousOptionMap := common.OptionMap
	savedPricing := map[string]string{
		"ModelPrice":           ratio_setting.ModelPrice2JSONString(),
		"ModelRatio":           ratio_setting.ModelRatio2JSONString(),
		"CompletionRatio":      ratio_setting.CompletionRatio2JSONString(),
		"CacheRatio":           ratio_setting.CacheRatio2JSONString(),
		"CreateCacheRatio":     ratio_setting.CreateCacheRatio2JSONString(),
		"ImageRatio":           ratio_setting.ImageRatio2JSONString(),
		"AudioRatio":           ratio_setting.AudioRatio2JSONString(),
		"AudioCompletionRatio": ratio_setting.AudioCompletionRatio2JSONString(),
	}

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.User{}, &model.Log{}))

	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{
		"ModelPrice":           "{}",
		"ModelRatio":           `{"old-model":1.25}`,
		"CompletionRatio":      "{}",
		"CacheRatio":           "{}",
		"CreateCacheRatio":     "{}",
		"ImageRatio":           "{}",
		"AudioRatio":           "{}",
		"AudioCompletionRatio": "{}",
	}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		_ = ratio_setting.UpdateModelPriceByJSONString(savedPricing["ModelPrice"])
		_ = ratio_setting.UpdateModelRatioByJSONString(savedPricing["ModelRatio"])
		_ = ratio_setting.UpdateCompletionRatioByJSONString(savedPricing["CompletionRatio"])
		_ = ratio_setting.UpdateCacheRatioByJSONString(savedPricing["CacheRatio"])
		_ = ratio_setting.UpdateCreateCacheRatioByJSONString(savedPricing["CreateCacheRatio"])
		_ = ratio_setting.UpdateImageRatioByJSONString(savedPricing["ImageRatio"])
		_ = ratio_setting.UpdateAudioRatioByJSONString(savedPricing["AudioRatio"])
		_ = ratio_setting.UpdateAudioCompletionRatioByJSONString(savedPricing["AudioCompletionRatio"])
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestModelPricingImportExport(t *testing.T) {
	setupModelPricingOptionTest(t)

	exportResponse := httptest.NewRecorder()
	exportContext, _ := gin.CreateTestContext(exportResponse)
	exportContext.Request = httptest.NewRequest(http.MethodGet, "/api/option/model_pricing/export", nil)

	ExportModelPricing(exportContext)

	assert.Equal(t, http.StatusOK, exportResponse.Code)
	assert.Contains(t, exportResponse.Header().Get("Content-Disposition"), "model-pricing.json")
	var exported modelPricingExportPayload
	require.NoError(t, common.Unmarshal(exportResponse.Body.Bytes(), &exported))
	assert.Equal(t, 1, exported.Version)
	require.Contains(t, exported.Pricing, "ModelRatio")
	assert.Equal(t, 1.25, exported.Pricing["ModelRatio"]["old-model"])

	importResponse := httptest.NewRecorder()
	importContext, _ := gin.CreateTestContext(importResponse)
	importContext.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/option/model_pricing/import",
		strings.NewReader(`{
			"version": 1,
			"pricing": {
				"ModelRatio": {"glm-5-turbo": 0.25},
				"CompletionRatio": "{\"glm-5-turbo\":2}",
				"Unknown": {"ignored": 1}
			}
		}`),
	)

	ImportModelPricing(importContext)

	assert.Equal(t, http.StatusOK, importResponse.Code)
	var imported struct {
		Success bool `json:"success"`
		Data    struct {
			UpdatedOptions int      `json:"updated_options"`
			SkippedOptions []string `json:"skipped_options"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(importResponse.Body.Bytes(), &imported))
	require.True(t, imported.Success)
	assert.Equal(t, 2, imported.Data.UpdatedOptions)
	assert.Equal(t, []string{"Unknown"}, imported.Data.SkippedOptions)

	var saved model.Option
	require.NoError(t, model.DB.First(&saved, "`key` = ?", "ModelRatio").Error)
	var savedRatio map[string]float64
	require.NoError(t, common.UnmarshalJsonStr(saved.Value, &savedRatio))
	assert.Equal(t, 0.25, savedRatio["glm-5-turbo"])
}
