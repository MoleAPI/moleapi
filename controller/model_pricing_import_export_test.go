package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
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
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	savedBillingMode, err := common.Marshal(billing_setting.GetBillingModeCopy())
	require.NoError(t, err)
	savedBillingExpr, err := common.Marshal(billing_setting.GetBillingExprCopy())
	require.NoError(t, err)
	savedPricing := map[string]string{
		"ModelPrice":                   ratio_setting.ModelPrice2JSONString(),
		"ModelRatio":                   ratio_setting.ModelRatio2JSONString(),
		"CompletionRatio":              ratio_setting.CompletionRatio2JSONString(),
		"CacheRatio":                   ratio_setting.CacheRatio2JSONString(),
		"CreateCacheRatio":             ratio_setting.CreateCacheRatio2JSONString(),
		"ImageRatio":                   ratio_setting.ImageRatio2JSONString(),
		"ImageOutputRatio":             ratio_setting.ImageOutputRatio2JSONString(),
		"AudioRatio":                   ratio_setting.AudioRatio2JSONString(),
		"AudioCompletionRatio":         ratio_setting.AudioCompletionRatio2JSONString(),
		"billing_setting.billing_mode": string(savedBillingMode),
		"billing_setting.billing_expr": string(savedBillingExpr),
	}

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.User{}, &model.Log{}))

	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{
		"ModelPrice":                   "{}",
		"ModelRatio":                   `{"old-model":1.25}`,
		"CompletionRatio":              "{}",
		"CacheRatio":                   "{}",
		"CreateCacheRatio":             "{}",
		"ImageRatio":                   `{"old-image-model":15}`,
		"ImageOutputRatio":             `{"old-image-model":60}`,
		"AudioRatio":                   `{"old-audio-model":3}`,
		"AudioCompletionRatio":         "{}",
		"billing_setting.billing_mode": `{"old-tiered-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"old-tiered-model":"tier(\"base\", p * 1 + c * 6)"}`,
	}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		_ = ratio_setting.UpdateModelPriceByJSONString(savedPricing["ModelPrice"])
		_ = ratio_setting.UpdateModelRatioByJSONString(savedPricing["ModelRatio"])
		_ = ratio_setting.UpdateCompletionRatioByJSONString(savedPricing["CompletionRatio"])
		_ = ratio_setting.UpdateCacheRatioByJSONString(savedPricing["CacheRatio"])
		_ = ratio_setting.UpdateCreateCacheRatioByJSONString(savedPricing["CreateCacheRatio"])
		_ = ratio_setting.UpdateImageRatioByJSONString(savedPricing["ImageRatio"])
		_ = ratio_setting.UpdateImageOutputRatioByJSONString(savedPricing["ImageOutputRatio"])
		_ = ratio_setting.UpdateAudioRatioByJSONString(savedPricing["AudioRatio"])
		_ = ratio_setting.UpdateAudioCompletionRatioByJSONString(savedPricing["AudioCompletionRatio"])
		_ = model.UpdateOptionsBulk(map[string]string{
			"billing_setting.billing_mode": savedPricing["billing_setting.billing_mode"],
			"billing_setting.billing_expr": savedPricing["billing_setting.billing_expr"],
		})
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.RedisEnabled = previousRedisEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}

func pricingExportMap[T any](t *testing.T, payload modelPricingExportPayload, key string) map[string]T {
	t.Helper()
	raw, err := common.Marshal(payload.Pricing[key])
	require.NoError(t, err)
	values := make(map[string]T)
	require.NoError(t, common.Unmarshal(raw, &values))
	return values
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
	assert.Equal(t, 1.25, pricingExportMap[float64](t, exported, "ModelRatio")["old-model"])
	assert.Equal(t, 15.0, pricingExportMap[float64](t, exported, "ImageRatio")["old-image-model"])
	assert.Equal(t, 60.0, pricingExportMap[float64](t, exported, "ImageOutputRatio")["old-image-model"])
	assert.Equal(t, 3.0, pricingExportMap[float64](t, exported, "AudioRatio")["old-audio-model"])
	assert.Equal(t, billing_setting.BillingModeTieredExpr, pricingExportMap[string](t, exported, "billing_setting.billing_mode")["old-tiered-model"])
	assert.Contains(t, pricingExportMap[string](t, exported, "billing_setting.billing_expr")["old-tiered-model"], "p * 1")

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
				"ImageRatio": {"glm-image": 15},
				"ImageOutputRatio": {"glm-image": 60},
				"AudioRatio": {"glm-audio": 3},
				"billing_setting.billing_mode": {"glm-5-turbo": "tiered_expr"},
				"billing_setting.billing_expr": {"glm-5-turbo": "tier(\"base\", p * 1 + c * 6)"},
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
	assert.Equal(t, 7, imported.Data.UpdatedOptions)
	assert.Equal(t, []string{"Unknown"}, imported.Data.SkippedOptions)

	var saved model.Option
	require.NoError(t, model.DB.First(&saved, "`key` = ?", "ModelRatio").Error)
	var savedRatio map[string]float64
	require.NoError(t, common.UnmarshalJsonStr(saved.Value, &savedRatio))
	assert.Equal(t, 0.25, savedRatio["glm-5-turbo"])
	saved = model.Option{}
	require.NoError(t, model.DB.First(&saved, "`key` = ?", "ImageRatio").Error)
	require.NoError(t, common.UnmarshalJsonStr(saved.Value, &savedRatio))
	assert.Equal(t, 15.0, savedRatio["glm-image"])
	saved = model.Option{}
	require.NoError(t, model.DB.First(&saved, "`key` = ?", "ImageOutputRatio").Error)
	require.NoError(t, common.UnmarshalJsonStr(saved.Value, &savedRatio))
	assert.Equal(t, 60.0, savedRatio["glm-image"])
	saved = model.Option{}
	require.NoError(t, model.DB.First(&saved, "`key` = ?", "AudioRatio").Error)
	require.NoError(t, common.UnmarshalJsonStr(saved.Value, &savedRatio))
	assert.Equal(t, 3.0, savedRatio["glm-audio"])
	assert.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode("glm-5-turbo"))
	expr, ok := billing_setting.GetBillingExpr("glm-5-turbo")
	require.True(t, ok)
	assert.Contains(t, expr, "p * 1")
}
