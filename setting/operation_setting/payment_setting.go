package operation_setting

import (
	"errors"
	"math"

	"github.com/QuantumNous/new-api/setting/config"
)

type PaymentSetting struct {
	AmountOptions  []int           `json:"amount_options"`
	AmountDiscount map[int]float64 `json:"amount_discount"` // 兼容旧配置：大于 0.5 表示支付折扣，小于等于 0.5 表示赠额比例
	AmountBonus    map[int]float64 `json:"amount_bonus"`    // 充值金额对应的赠额比例，例如 100 元 0.05 表示额外赠送 5%

	ComplianceConfirmed    bool   `json:"compliance_confirmed"`
	ComplianceTermsVersion string `json:"compliance_terms_version"`
	ComplianceConfirmedAt  int64  `json:"compliance_confirmed_at"`
	ComplianceConfirmedBy  int    `json:"compliance_confirmed_by"`
	ComplianceConfirmedIP  string `json:"compliance_confirmed_ip"`
}

const CurrentComplianceTermsVersion = "v1"

// 默认配置
var paymentSetting = PaymentSetting{
	AmountOptions:  []int{10, 20, 50, 100, 200, 500},
	AmountDiscount: map[int]float64{},
	AmountBonus:    map[int]float64{},
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
}

func GetPaymentSetting() *PaymentSetting {
	return &paymentSetting
}

// GetTopupDiscountMultiplier returns the payment multiplier for the highest
// configured tier at or below amount. Legacy values at or below 0.5 are bonus
// rates and therefore must not reduce the payment price.
func GetTopupDiscountMultiplier(amount int64) float64 {
	bestTier := -1
	multiplier := 1.0
	for tier, value := range paymentSetting.AmountDiscount {
		if tier >= 0 && int64(tier) <= amount && tier > bestTier {
			bestTier = tier
			multiplier = value
		}
	}
	if bestTier < 0 || multiplier <= 0.5 || multiplier > 1 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return 1
	}
	return multiplier
}

func GetTopupDiscountMapForAPI() map[int]float64 {
	discount := make(map[int]float64)
	for tier, value := range paymentSetting.AmountDiscount {
		if tier >= 0 && value > 0.5 && value <= 1 && !math.IsNaN(value) && !math.IsInf(value, 0) {
			discount[tier] = value
		}
	}
	return discount
}

// GetTopupBonusRate prefers the explicit bonus tiers and falls back to legacy
// amount_discount entries at or below 0.5.
func GetTopupBonusRateChecked(amount int64) (float64, error) {
	bestTier := -1
	bonus := 0.0
	for tier, value := range paymentSetting.AmountBonus {
		if tier >= 0 && int64(tier) <= amount && tier > bestTier {
			bestTier = tier
			bonus = value
		}
	}
	if bestTier >= 0 {
		if bonus < 0 || bonus > 1 || math.IsNaN(bonus) || math.IsInf(bonus, 0) {
			return 0, errors.New("invalid topup bonus rate")
		}
		if bonus > 0 {
			return bonus, nil
		}
	}

	bestTier = -1
	bonus = 0
	for tier, value := range paymentSetting.AmountDiscount {
		if tier >= 0 && int64(tier) <= amount && tier > bestTier && value > 0 && value <= 0.5 && !math.IsNaN(value) && !math.IsInf(value, 0) {
			bestTier = tier
			bonus = value
		}
	}
	return bonus, nil
}

func GetTopupBonusRate(amount int64) float64 {
	bonus, _ := GetTopupBonusRateChecked(amount)
	return bonus
}

func GetTopupBonusMapForAPI() map[int]float64 {
	if len(paymentSetting.AmountBonus) > 0 {
		return paymentSetting.AmountBonus
	}
	bonus := make(map[int]float64)
	for tier, value := range paymentSetting.AmountDiscount {
		if tier >= 0 && value > 0 && value <= 0.5 && !math.IsNaN(value) && !math.IsInf(value, 0) {
			bonus[tier] = value
		}
	}
	return bonus
}

func IsPaymentComplianceConfirmed() bool {
	return paymentSetting.ComplianceConfirmed &&
		paymentSetting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}
