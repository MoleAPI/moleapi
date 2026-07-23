package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelDescriptionImportExportRoundTrip(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Vendor{
		Name:        "OpenAI",
		Description: "Original vendor",
		Icon:        "OpenAI",
		Status:      1,
	}).Error)
	var openAI model.Vendor
	require.NoError(t, db.Where("name = ?", "OpenAI").First(&openAI).Error)
	require.NoError(t, db.Create(&model.Model{
		ModelName:       "round-trip-model",
		Description:     "English description",
		DescriptionI18N: model.JSONValue(`{"fr":"Ancien","zh":"中文介绍"}`),
		Tags:            "对话,推理",
		VendorID:        openAI.Id,
		Status:          1,
	}).Error)

	exportRecorder := httptest.NewRecorder()
	exportContext, _ := gin.CreateTestContext(exportRecorder)
	exportContext.Request = httptest.NewRequest(http.MethodGet, "/api/models/descriptions/export", nil)
	ExportModelDescriptions(exportContext)

	require.Equal(t, http.StatusOK, exportRecorder.Code)
	var exported modelDescriptionExport
	require.NoError(t, common.Unmarshal(exportRecorder.Body.Bytes(), &exported))
	require.Len(t, exported.Models, 1)
	assert.Equal(t, "English description", exported.Models[0].Description)
	assert.Equal(t, "中文介绍", exported.Models[0].DescriptionI18N["zh"])
	assert.Equal(t, "对话,推理", exported.Models[0].Tags)
	assert.Equal(t, "OpenAI", exported.Models[0].VendorName)
	require.Len(t, exported.Vendors, 1)
	assert.Equal(t, "OpenAI", exported.Vendors[0].Name)

	body := `{"vendors":[{"name":"Anthropic","description":"Claude vendor","icon":"Claude","status":1}],"models":[{"model_name":"round-trip-model","description":"Updated English","description_i18n":{"zh":"更新中文","ja":"日本語"},"tags":"缓存,长上下文","vendor_name":"Anthropic","status":1,"sync_official":0,"name_rule":1},{"model_name":"missing-model","description":"created model","description_i18n":{"zh":"新增中文"},"tags":"对话","vendor_name":"Anthropic","status":1,"sync_official":1,"name_rule":0}]}`
	importRecorder := httptest.NewRecorder()
	importContext, _ := gin.CreateTestContext(importRecorder)
	importContext.Request = httptest.NewRequest(http.MethodPost, "/api/models/descriptions/import", strings.NewReader(body))
	ImportModelDescriptions(importContext)

	require.Equal(t, http.StatusOK, importRecorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			UpdatedModels  int      `json:"updated_models"`
			CreatedModels  int      `json:"created_models"`
			CreatedVendors int      `json:"created_vendors"`
			SkippedModels  []string `json:"skipped_models"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(importRecorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, 1, response.Data.UpdatedModels)
	assert.Equal(t, 1, response.Data.CreatedModels)
	assert.Equal(t, 1, response.Data.CreatedVendors)
	assert.Empty(t, response.Data.SkippedModels)

	var stored model.Model
	require.NoError(t, db.Where("model_name = ?", "round-trip-model").First(&stored).Error)
	assert.Equal(t, "Updated English", stored.Description)
	assert.Equal(t, "缓存,长上下文", stored.Tags)
	assert.Equal(t, 0, stored.SyncOfficial)
	assert.Equal(t, model.NameRulePrefix, stored.NameRule)
	translations := modelDescriptionTranslations(stored.DescriptionI18N)
	assert.Equal(t, "Ancien", translations["fr"])
	assert.Equal(t, "更新中文", translations["zh"])
	assert.Equal(t, "日本語", translations["ja"])

	var anthropic model.Vendor
	require.NoError(t, db.Where("name = ?", "Anthropic").First(&anthropic).Error)
	assert.Equal(t, "Claude vendor", anthropic.Description)
	assert.Equal(t, "Claude", anthropic.Icon)
	assert.Equal(t, anthropic.Id, stored.VendorID)

	var created model.Model
	require.NoError(t, db.Where("model_name = ?", "missing-model").First(&created).Error)
	assert.Equal(t, "created model", created.Description)
	assert.Equal(t, "对话", created.Tags)
	assert.Equal(t, anthropic.Id, created.VendorID)
	assert.Equal(t, "新增中文", modelDescriptionTranslations(created.DescriptionI18N)["zh"])
}

func TestLocalizedModelDescriptionHelperPreservesBaseDescription(t *testing.T) {
	locale, ok := normalizeLocale("zh-CN")
	require.True(t, ok)
	assert.Equal(t, "zh", locale)

	m := model.Model{
		Description:     "English description",
		DescriptionI18N: model.JSONValue(`{"ja":"日本語"}`),
	}
	require.NoError(t, applyLocalizedModelDescription(&m, locale, "中文介绍"))

	assert.Equal(t, "English description", m.Description)
	translations := modelDescriptionTranslations(m.DescriptionI18N)
	assert.Equal(t, "中文介绍", translations["zh"])
	assert.Equal(t, "日本語", translations["ja"])
}
