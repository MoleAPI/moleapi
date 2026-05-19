package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id     int   `json:"id"`
	UserId int   `json:"user_id" gorm:"index"`
	Amount int64 `json:"amount"`
	// AmountDisplay is a computed field for UI display:
	// - pending/expired: base amount
	// - success: includes current topup bonus rate
	AmountDisplay   string  `json:"amount_display" gorm:"-"`
	Money           float64 `json:"money"`
	TradeNo         string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	GatewayTradeNo  string  `json:"gateway_trade_no" gorm:"type:varchar(255);index;default:''"`
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
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderLanTu        = "lantu"
)

var (
	ErrPaymentMethodMismatch = errors.New("payment method mismatch")
	ErrTopUpNotFound         = errors.New("topup not found")
	ErrTopUpStatusInvalid    = errors.New("topup status invalid")
)

func (topUp *TopUp) FillAmountDisplay() {
	if topUp == nil {
		return
	}
	if topUp.Amount == 0 {
		topUp.AmountDisplay = "0"
		return
	}
	d := decimal.NewFromInt(topUp.Amount)
	if topUp.Status == common.TopUpStatusSuccess {
		bonusRate := operation_setting.GetTopupBonusRate(topUp.Amount)
		d = d.Mul(decimal.NewFromFloat(1.0 + bonusRate))
	}
	s := d.StringFixed(2)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	topUp.AmountDisplay = s
}

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
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
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

func GenerateUniqueTopUpTradeNo(userId int) (string, error) {
	if userId <= 0 {
		return "", errors.New("invalid user id")
	}
	for i := 0; i < 8; i++ {
		tradeNo := fmt.Sprintf("USR%dNO%s%s", userId, common.GetTimeString(), common.GetRandomString(4))
		var count int64
		if err := DB.Model(&TopUp{}).Where("trade_no = ?", tradeNo).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return tradeNo, nil
		}
		time.Sleep(time.Millisecond)
	}
	return "", errors.New("failed to generate unique topup trade no")
}

// FormatLanTuTradeNo builds an out_trade_no compatible with LanTu (ltzf) constraints:
// - length: 6-32
// - recommended: alphanumeric
// We use a seconds timestamp (14 digits) plus a short random suffix to avoid collisions.
func FormatLanTuTradeNo(userId int, t time.Time, suffix string) (string, error) {
	if userId <= 0 {
		return "", errors.New("invalid user id")
	}
	// Keep the legacy prefix so logs/search are familiar: USR{user}NO{time}{rand}
	tradeNo := fmt.Sprintf("USR%06dNO%s%s", userId, t.UTC().Format("20060102150405"), suffix)
	if l := len(tradeNo); l < 6 || l > 32 {
		return "", errors.New("invalid lantu out_trade_no length")
	}
	return tradeNo, nil
}

func GenerateUniqueLanTuTradeNo(userId int) (string, error) {
	if userId <= 0 {
		return "", errors.New("invalid user id")
	}
	for i := 0; i < 8; i++ {
		tradeNo, err := FormatLanTuTradeNo(userId, time.Now(), common.GetRandomString(4))
		if err != nil {
			return "", err
		}
		var count int64
		if err := DB.Model(&TopUp{}).Where("trade_no = ?", tradeNo).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return tradeNo, nil
		}
		time.Sleep(time.Millisecond)
	}
	return "", errors.New("failed to generate unique lantu trade no")
}

func Recharge(referenceId string, customerId string, callerIp ...string) (err error) {
	return RechargeStripeWithGatewayTradeNo(referenceId, customerId, "", callerIp...)
}

func RechargeStripeWithGatewayTradeNo(referenceId string, customerId string, gatewayTradeNo string, callerIp ...string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	var inviterId int
	var inviterRewardQuota int
	var inviterRewardGranted bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		if gatewayTradeNo != "" {
			topUp.GatewayTradeNo = gatewayTradeNo
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		// Stripe 订单：Money 代表充值美元数量（可能已包含分组倍率换算）。
		// 额外加赠：按 Amount 对应的加赠比例计算。
		dBaseQuota := decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
		bonusRate := operation_setting.GetTopupBonusRate(topUp.Amount)
		dFinalQuota := dBaseQuota.Mul(decimal.NewFromFloat(1.0 + bonusRate))
		quotaToAdd = int(dFinalQuota.IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(map[string]interface{}{
			"stripe_customer": customerId,
			"quota":           gorm.Expr("quota + ?", quotaToAdd),
		}).Error
		if err != nil {
			return err
		}

		rewardErr := error(nil)
		inviterId, inviterRewardQuota, inviterRewardGranted, rewardErr = rewardInviterOnFirstTopupTx(tx, topUp.UserId)
		if rewardErr != nil {
			return rewardErr
		}

		return nil
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	logCallerIp := ""
	if len(callerIp) > 0 {
		logCallerIp = callerIp[0]
	}
	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(quotaToAdd), topUp.Amount), logCallerIp, topUp.PaymentMethod, PaymentMethodStripe)
	if inviterRewardGranted && inviterId > 0 && inviterRewardQuota > 0 {
		RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请好友首笔充值赠送 %s", logger.LogQuota(inviterRewardQuota)))
	}

	return nil
}

// topUpQueryWindowSeconds limits regular user top-up queries to a recent window.
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

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

	for _, t := range topups {
		t.FillAmountDisplay()
	}
	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用）
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

	for _, t := range topups {
		t.FillAmountDisplay()
	}
	return topups, total, nil
}

// searchTopUpCountHardLimit caps COUNT work for top-up searches.
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
		query = query.Where("(trade_no LIKE ? ESCAPE '!' OR gateway_trade_no LIKE ? ESCAPE '!')", pattern, pattern)
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
	for _, t := range topups {
		t.FillAmountDisplay()
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用）
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
		query = query.Where("(trade_no LIKE ? ESCAPE '!' OR gateway_trade_no LIKE ? ESCAPE '!')", pattern, pattern)
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
	for _, t := range topups {
		t.FillAmountDisplay()
	}
	return topups, total, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp ...string) error {
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	var userId int
	var quotaToAdd int
	var payMoney float64
	var inviterId int
	var inviterRewardQuota int
	var inviterRewardGranted bool
	var paymentMethod string

	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		// 行级锁，避免并发补单
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}

		// 幂等处理：已成功直接返回
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("订单状态不是待支付，无法补单")
		}

		// 计算应充值额度：
		// - Stripe 订单：Money 代表充值美元数量（可能已包含分组倍率换算）
		// - 其他订单（如易支付）：Amount 为美元数量
		// 额外加赠：按 Amount 对应的加赠比例计算。
		bonusRate := operation_setting.GetTopupBonusRate(topUp.Amount)
		dBonusMultiplier := decimal.NewFromFloat(1.0 + bonusRate)
		if topUp.PaymentProvider == PaymentProviderStripe {
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd = int(decimal.NewFromFloat(topUp.Money).Mul(dQuotaPerUnit).Mul(dBonusMultiplier).IntPart())
		} else {
			dAmount := decimal.NewFromInt(topUp.Amount)
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).Mul(dBonusMultiplier).IntPart())
		}
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		// 标记完成
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		// 增加用户额度（立即写库，保持一致性）
		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		rewardErr := error(nil)
		inviterId, inviterRewardQuota, inviterRewardGranted, rewardErr = rewardInviterOnFirstTopupTx(tx, topUp.UserId)
		if rewardErr != nil {
			return rewardErr
		}

		userId = topUp.UserId
		payMoney = topUp.Money
		paymentMethod = topUp.PaymentMethod
		return nil
	})

	if err != nil {
		return err
	}

	// 事务外记录日志，避免阻塞
	logCallerIp := ""
	if len(callerIp) > 0 {
		logCallerIp = callerIp[0]
	}
	RecordTopupLog(userId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), payMoney), logCallerIp, paymentMethod, "admin")
	if inviterRewardGranted && inviterId > 0 && inviterRewardQuota > 0 {
		RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请好友首笔充值赠送 %s", logger.LogQuota(inviterRewardQuota)))
	}
	return nil
}
func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp ...string) (err error) {
	return RechargeCreemWithGatewayTradeNo(referenceId, customerEmail, customerName, "", callerIp...)
}

func RechargeCreemWithGatewayTradeNo(referenceId string, customerEmail string, customerName string, gatewayTradeNo string, callerIp ...string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int64
	var inviterId int
	var inviterRewardQuota int
	var inviterRewardGranted bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderCreem {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		if gatewayTradeNo != "" {
			topUp.GatewayTradeNo = gatewayTradeNo
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		// Creem 直接使用 Amount 作为充值额度（整数）
		quota = topUp.Amount

		// 构建更新字段，优先使用邮箱，如果邮箱为空则使用用户名
		updateFields := map[string]interface{}{
			"quota": gorm.Expr("quota + ?", quota),
		}

		// 如果有客户邮箱，尝试更新用户邮箱（仅当用户邮箱为空时）
		if customerEmail != "" {
			// 先检查用户当前邮箱是否为空
			var user User
			err = tx.Where("id = ?", topUp.UserId).First(&user).Error
			if err != nil {
				return err
			}

			// 如果用户邮箱为空，则更新为支付时使用的邮箱
			if user.Email == "" {
				updateFields["email"] = customerEmail
			}
		}

		err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(updateFields).Error
		if err != nil {
			return err
		}

		inviterId, inviterRewardQuota, inviterRewardGranted, err = rewardInviterOnFirstTopupTx(tx, topUp.UserId)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	logCallerIp := ""
	if len(callerIp) > 0 {
		logCallerIp = callerIp[0]
	}
	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quota, topUp.Money), logCallerIp, topUp.PaymentMethod, PaymentMethodCreem)
	if inviterRewardGranted && inviterId > 0 && inviterRewardQuota > 0 {
		RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请好友首笔充值赠送 %s", logger.LogQuota(inviterRewardQuota)))
	}

	return nil
}

func RechargeWaffo(tradeNo string, callerIp ...string) (err error) {
	return RechargeWaffoWithGatewayTradeNo(tradeNo, "", callerIp...)
}

func RechargeWaffoWithGatewayTradeNo(tradeNo string, gatewayTradeNo string, callerIp ...string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	var inviterId int
	var inviterRewardQuota int
	var inviterRewardGranted bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffo {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil // 幂等：已成功直接返回
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		dAmount := decimal.NewFromInt(topUp.Amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		bonusRate := operation_setting.GetTopupBonusRate(topUp.Amount)
		dBonusMultiplier := decimal.NewFromFloat(1.0 + bonusRate)
		quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).Mul(dBonusMultiplier).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		if gatewayTradeNo != "" {
			topUp.GatewayTradeNo = gatewayTradeNo
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		inviterId, inviterRewardQuota, inviterRewardGranted, err = rewardInviterOnFirstTopupTx(tx, topUp.UserId)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if quotaToAdd > 0 {
		logCallerIp := ""
		if len(callerIp) > 0 {
			logCallerIp = callerIp[0]
		}
		RecordTopupLog(topUp.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), logCallerIp, topUp.PaymentMethod, PaymentMethodWaffo)
	}
	if inviterRewardGranted && inviterId > 0 && inviterRewardQuota > 0 {
		RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请好友首笔充值赠送 %s", logger.LogQuota(inviterRewardQuota)))
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string) (err error) {
	return RechargeWaffoPancakeWithAudit(tradeNo, "", "")
}

func RechargeWaffoPancakeWithAudit(tradeNo string, gatewayTradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	var inviterId int
	var inviterRewardQuota int
	var inviterRewardGranted bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffoPancake {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		dAmount := decimal.NewFromInt(topUp.Amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		bonusRate := operation_setting.GetTopupBonusRate(topUp.Amount)
		dBonusMultiplier := decimal.NewFromFloat(1.0 + bonusRate)
		quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).Mul(dBonusMultiplier).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		if gatewayTradeNo != "" {
			topUp.GatewayTradeNo = gatewayTradeNo
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		inviterId, inviterRewardQuota, inviterRewardGranted, err = rewardInviterOnFirstTopupTx(tx, topUp.UserId)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if quotaToAdd > 0 {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodWaffoPancake)
	}
	if inviterRewardGranted && inviterId > 0 && inviterRewardQuota > 0 {
		RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请好友首笔充值赠送 %s", logger.LogQuota(inviterRewardQuota)))
	}

	return nil
}
