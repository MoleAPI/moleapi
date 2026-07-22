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
	require.NoError(t, db.Create(&model.Model{
		ModelName:       "round-trip-model",
		Description:     "English description",
		DescriptionI18N: model.JSONValue(`{"zh":"中文介绍"}`),
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

	body := `{"models":[{"model_name":"round-trip-model","description":"Updated English","description_i18n":{"zh":"更新中文","ja":"日本語"}},{"model_name":"missing-model","description":"skip me"}]}`
	importRecorder := httptest.NewRecorder()
	importContext, _ := gin.CreateTestContext(importRecorder)
	importContext.Request = httptest.NewRequest(http.MethodPost, "/api/models/descriptions/import", strings.NewReader(body))
	ImportModelDescriptions(importContext)

	require.Equal(t, http.StatusOK, importRecorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			UpdatedModels int      `json:"updated_models"`
			SkippedModels []string `json:"skipped_models"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(importRecorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, 1, response.Data.UpdatedModels)
	assert.Equal(t, []string{"missing-model"}, response.Data.SkippedModels)

	var stored model.Model
	require.NoError(t, db.Where("model_name = ?", "round-trip-model").First(&stored).Error)
	assert.Equal(t, "Updated English", stored.Description)
	translations := modelDescriptionTranslations(stored.DescriptionI18N)
	assert.Equal(t, "更新中文", translations["zh"])
	assert.Equal(t, "日本語", translations["ja"])
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
