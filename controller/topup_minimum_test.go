package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
)

func TestConfiguredMinimumTopUpFollowsDisplayUnits(t *testing.T) {
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalQuotaPerUnit := common.QuotaPerUnit
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	common.QuotaPerUnit = 500_000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	assert.EqualValues(t, 10, getConfiguredMinTopUp(10))

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	assert.EqualValues(t, 5_000_000, getConfiguredMinTopUp(10))
}
