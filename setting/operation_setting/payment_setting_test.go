package operation_setting

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTopupDiscountAndBonusTiers(t *testing.T) {
	originalDiscount := paymentSetting.AmountDiscount
	originalBonus := paymentSetting.AmountBonus
	t.Cleanup(func() {
		paymentSetting.AmountDiscount = originalDiscount
		paymentSetting.AmountBonus = originalBonus
	})

	paymentSetting.AmountDiscount = map[int]float64{
		10:  0.9,
		20:  0.1,
		50:  0.75,
		100: math.Inf(1),
	}
	paymentSetting.AmountBonus = map[int]float64{}

	assert.Equal(t, 1.0, GetTopupDiscountMultiplier(9))
	assert.Equal(t, 0.9, GetTopupDiscountMultiplier(10))
	assert.Equal(t, 0.9, GetTopupDiscountMultiplier(19))
	assert.Equal(t, 1.0, GetTopupDiscountMultiplier(20))
	assert.Equal(t, 0.75, GetTopupDiscountMultiplier(50))
	assert.Equal(t, 1.0, GetTopupDiscountMultiplier(100))
	assert.Equal(t, map[int]float64{10: 0.9, 50: 0.75}, GetTopupDiscountMapForAPI())
	assert.Equal(t, 0.1, GetTopupBonusRate(20))
	assert.Equal(t, 0.1, GetTopupBonusRate(99))

	paymentSetting.AmountBonus = map[int]float64{30: 0.2, 80: 0.3}
	assert.Equal(t, 0.1, GetTopupBonusRate(20))
	assert.Equal(t, 0.2, GetTopupBonusRate(30))
	assert.Equal(t, 0.3, GetTopupBonusRate(100))
	assert.Equal(t, paymentSetting.AmountBonus, GetTopupBonusMapForAPI())

	paymentSetting.AmountBonus = map[int]float64{1: 1.01}
	_, err := GetTopupBonusRateChecked(1)
	assert.Error(t, err)
	assert.Zero(t, GetTopupBonusRate(1))
}

func TestTopupBonusMapFallsBackToLegacyDiscountValues(t *testing.T) {
	originalDiscount := paymentSetting.AmountDiscount
	originalBonus := paymentSetting.AmountBonus
	t.Cleanup(func() {
		paymentSetting.AmountDiscount = originalDiscount
		paymentSetting.AmountBonus = originalBonus
	})

	paymentSetting.AmountDiscount = map[int]float64{10: 0.05, 20: 0.9, 30: -0.1}
	paymentSetting.AmountBonus = map[int]float64{}

	assert.Equal(t, map[int]float64{10: 0.05}, GetTopupBonusMapForAPI())
}
