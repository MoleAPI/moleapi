package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id              int     `json:"id"`
	UserId          int     `json:"user_id" gorm:"index"`
	Amount          int64   `json:"amount"`
	Money           float64 `json:"money"`
	TradeNo         string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	GatewayTradeNo  string  `json:"gateway_trade_no" gorm:"type:varchar(255);index;default:''"`
	CreditedQuota   int     `json:"credited_quota" gorm:"type:int;default:0"`
	PaymentCurrency string  `json:"payment_currency" gorm:"type:varchar(8);default:''"`
	PaymentMethod   string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	CreateTime      int64   `json:"create_time"`
	CompleteTime    int64   `json:"complete_time"`
	Status          string  `json:"status"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderBalance      = "balance"
)

var (
	ErrPaymentMethodMismatch = errors.New("payment method mismatch")
	ErrTopUpNotFound         = errors.New("topup not found")
	ErrTopUpStatusInvalid    = errors.New("topup status invalid")
	ErrTopUpQuotaInvalid     = errors.New("topup quota invalid")
)

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

func settleTopUp(tradeNo string, expectedPaymentProvider string, quotaValue func(*TopUp) decimal.Decimal, applyDetails func(*TopUp, *User) map[string]interface{}) (topUp *TopUp, creditedQuota int, settled bool, err error) {
	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	topUp = &TopUp{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		var clamp *common.QuotaClamp
		creditedQuota, clamp = common.QuotaFromDecimalChecked(quotaValue(topUp))
		if clamp != nil || creditedQuota <= 0 {
			return ErrTopUpQuotaInvalid
		}

		user := &User{}
		if err := lockForUpdate(tx).Select("id", "quota", "email", "stripe_customer").Where("id = ?", topUp.UserId).First(user).Error; err != nil {
			return err
		}
		quotaAfterTopUp := int64(user.Quota) + int64(creditedQuota)
		if quotaAfterTopUp > int64(common.MaxQuota) {
			return ErrTopUpQuotaInvalid
		}

		userUpdates := map[string]interface{}{}
		if applyDetails != nil {
			if details := applyDetails(topUp, user); details != nil {
				userUpdates = details
			}
		}
		userUpdates["quota"] = quotaAfterTopUp

		topUp.CreditedQuota = creditedQuota
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}
		if err := tx.Model(user).Updates(userUpdates).Error; err != nil {
			return err
		}

		settled = true
		return nil
	})
	if err == nil && settled {
		if err := invalidateUserCache(topUp.UserId); err != nil {
			common.SysError("failed to invalidate user cache after topup: " + err.Error())
		}
	}
	return topUp, creditedQuota, settled, err
}

func topUpQuotaFromAmount(topUp *TopUp) decimal.Decimal {
	return decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
}

func Recharge(referenceId string, customerId string, callerIp string) (err error) {
	return RechargeStripeWithPaymentDetails(referenceId, customerId, "", "", callerIp)
}

func RechargeStripeWithPaymentDetails(referenceId string, customerId string, gatewayTradeNo string, paymentCurrency string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	topUp, quotaToAdd, settled, err := settleTopUp(referenceId, PaymentProviderStripe, func(topUp *TopUp) decimal.Decimal {
		return decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	}, func(topUp *TopUp, _ *User) map[string]interface{} {
		if gatewayTradeNo != "" {
			topUp.GatewayTradeNo = gatewayTradeNo
		}
		if paymentCurrency != "" {
			topUp.PaymentCurrency = paymentCurrency
		}
		userUpdates := map[string]interface{}{}
		if customerId != "" {
			userUpdates["stripe_customer"] = customerId
		}
		return userUpdates
	})
	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if settled {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(quotaToAdd), topUp.Amount), callerIp, topUp.PaymentMethod, PaymentMethodStripe)
	}

	return nil
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cutoff := topUpQueryCutoff()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, cutoff).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ? AND create_time >= ?", userId, cutoff).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&TopUp{}).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, topUpQueryCutoff())
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	topUp, quotaToAdd, settled, err := settleTopUp(tradeNo, "", func(topUp *TopUp) decimal.Decimal {
		if topUp.PaymentProvider == PaymentProviderStripe {
			return decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
		}
		return topUpQuotaFromAmount(topUp)
	}, nil)
	if err != nil {
		switch {
		case errors.Is(err, ErrTopUpNotFound):
			return errors.New("充值订单不存在")
		case errors.Is(err, ErrTopUpStatusInvalid):
			return errors.New("订单状态不是待支付，无法补单")
		case errors.Is(err, ErrTopUpQuotaInvalid):
			return errors.New("无效的充值额度")
		default:
			return err
		}
	}

	if settled {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, "admin")
	}
	return nil
}
func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {
	return RechargeCreemWithPaymentDetails(referenceId, customerEmail, customerName, "", "", callerIp)
}

func RechargeCreemWithPaymentDetails(referenceId string, customerEmail string, customerName string, gatewayTradeNo string, paymentCurrency string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	topUp, quotaToAdd, settled, err := settleTopUp(referenceId, PaymentProviderCreem, func(topUp *TopUp) decimal.Decimal {
		return decimal.NewFromInt(topUp.Amount)
	}, func(topUp *TopUp, user *User) map[string]interface{} {
		if gatewayTradeNo != "" {
			topUp.GatewayTradeNo = gatewayTradeNo
		}
		if paymentCurrency != "" {
			topUp.PaymentCurrency = paymentCurrency
		}
		userUpdates := map[string]interface{}{}
		if customerEmail != "" && user.Email == "" {
			userUpdates["email"] = customerEmail
		}
		return userUpdates
	})
	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if settled {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quotaToAdd, topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodCreem)
	}

	return nil
}

func RechargeEpay(tradeNo string, paymentMethod string, gatewayTradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	topUp, quotaToAdd, settled, err := settleTopUp(tradeNo, PaymentProviderEpay, topUpQuotaFromAmount, func(topUp *TopUp, _ *User) map[string]interface{} {
		if paymentMethod != "" {
			topUp.PaymentMethod = paymentMethod
		}
		if gatewayTradeNo != "" {
			topUp.GatewayTradeNo = gatewayTradeNo
		}
		return nil
	})
	if err != nil {
		common.SysError("epay topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if settled {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentProviderEpay)
	}
	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	topUp, quotaToAdd, settled, err := settleTopUp(tradeNo, PaymentProviderWaffo, topUpQuotaFromAmount, nil)
	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if settled {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodWaffo)
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	topUp, quotaToAdd, settled, err := settleTopUp(tradeNo, PaymentProviderWaffoPancake, topUpQuotaFromAmount, nil)
	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if settled {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), "", topUp.PaymentMethod, PaymentMethodWaffoPancake)
	}

	return nil
}
