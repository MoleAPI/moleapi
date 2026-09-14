package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupModelPricingOptionTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := modelManagementDB(t, "sqlite", "")
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{
		billing_setting.PluginBillingExprOption: `{"test-plugin::image-model":"tier(\"images\", u(\"image_count\") * 0.2)"}`,
		"ModelPrice":                            "{}",
		"ModelRatio":                            `{"old-model":1.25}`,
		"CompletionRatio":                       "{}",
		"CacheRatio":                            "{}",
		"CreateCacheRatio":                      "{}",
		"ImageRatio":                            `{"old-image-model":15}`,
		"ImageOutputRatio":                      `{"old-image-model":60}`,
		"AudioRatio":                            `{"old-audio-model":3}`,
		"AudioCompletionRatio":                  "{}",
		"billing_setting.billing_mode":          `{"old-tiered-model":"tiered_expr"}`,
		"billing_setting.billing_expr":          `{"old-tiered-model":"tier(\"base\", p * 1 + c * 6)"}`,
	}
	common.OptionMapRWMutex.Unlock()
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
	_, err := jsplugin.DefaultRegistry.Register(`
export const meta = {apiVersion:1,key:"test-plugin",name:"Pricing import fixture",version:"1.0.0",author:{name:"Test"},models:["image-model"],fetchMode:"per_task",usageSchema:{image_count:{type:"number",unit:"count"}}};
export function buildSubmitRequest(){return {};}
export function parseSubmitResponse(){return {};}
export function buildQueryRequest(){return {};}
export function parseTaskResult(){return {};}
`, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { jsplugin.DefaultRegistry.Unregister("test-plugin") })

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
	assert.Contains(t, pricingExportMap[string](t, exported, billing_setting.PluginBillingExprOption)["test-plugin::image-model"], "image_count")
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
				"billing_setting.plugin_billing_expr": {"test-plugin::image-model": "tier(\"images\", u(\"image_count\") * 0.4)"},
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
	assert.Equal(t, 8, imported.Data.UpdatedOptions)
	expr, ok := billing_setting.GetPluginBillingExpr("test-plugin", "image-model")
	require.True(t, ok)
	assert.Contains(t, expr, "0.4")
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
	expr, ok = billing_setting.GetBillingExpr("glm-5-turbo")
	require.True(t, ok)
	assert.Contains(t, expr, "p * 1")
}

func TestModelPricingImportRejectsInvalidBackupAtomically(t *testing.T) {
	for _, invalid := range []string{
		`"ModelRatio":{"invalid":-1}`,
		`"billing_setting.billing_expr":{"invalid":"tier("}`,
		`"billing_setting.plugin_billing_expr":{"missing-plugin::image-model":"tier(\"images\", u(\"image_count\") * 0.2)"}`,
	} {
		t.Run(invalid, func(t *testing.T) {
			setupModelPricingOptionTest(t)
			require.NoError(t, model.DB.Create(&model.Option{Key: "ModelPrice", Value: `{"keep":0.75}`}).Error)
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/option/model_pricing/import", strings.NewReader(`{"version":1,"pricing":{"ModelPrice":{"keep":5},`+invalid+`}}`))
			ImportModelPricing(context)
			var result struct{ Success bool }
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.False(t, result.Success)
			var saved model.Option
			require.NoError(t, model.DB.First(&saved, "key = ?", "ModelPrice").Error)
			assert.JSONEq(t, `{"keep":0.75}`, saved.Value)
		})
	}
}
