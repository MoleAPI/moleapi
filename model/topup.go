package model

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id                    int     `json:"id"`
	UserId                int     `json:"user_id" gorm:"index"`
	Amount                int64   `json:"amount"`
	Money                 float64 `json:"money"`
	TradeNo               string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	GatewayTradeNo        string  `json:"gateway_trade_no" gorm:"type:varchar(255);index;default:''"`
	PaymentProductId      string  `json:"payment_product_id" gorm:"type:varchar(255);default:''"`
	PaymentMode           string  `json:"payment_mode" gorm:"type:varchar(16);default:''"`
	PromisedQuota         int     `json:"promised_quota" gorm:"type:int;default:0"`
	CreditedQuota         int     `json:"credited_quota" gorm:"type:int;default:0"`
	InviteRebateInviterId int     `json:"invite_rebate_inviter_id" gorm:"type:int;default:0;column:invite_rebate_inviter_id;index"`
	InviteRebateRatio     int     `json:"invite_rebate_ratio" gorm:"type:int;default:0;column:invite_rebate_ratio"`
	InviteRebateQuota     int     `json:"invite_rebate_quota" gorm:"type:int;default:0;column:invite_rebate_quota"`
	PaymentCurrency       string  `json:"payment_currency" gorm:"type:varchar(8);default:''"`
	PaymentMethod         string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider       string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	CreateTime            int64   `json:"create_time"`
	CompleteTime          int64   `json:"complete_time"`
	Status                string  `json:"status"`
}

type InviteRebateTopUp struct {
	Id           int    `json:"id"`
	Source       string `json:"source"`
	Quota        int    `json:"quota"`
	RelatedUser  string `json:"related_user,omitempty"`
	CompleteTime int64  `json:"complete_time"`
}

type TopUpSearchParams struct {
	Keyword        string
	UserKeyword    string
	StartTimestamp int64
	EndTimestamp   int64
}

type InviteRewardHistoryParams struct {
	StartTimestamp int64
	EndTimestamp   int64
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodLanTu        = "lantu"
	PaymentMethodNowPayments  = "nowpayments"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderLanTu        = "lantu"
	PaymentProviderNowPayments  = "nowpayments"
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
	if topUp.Status == common.TopUpStatusPending {
		if topUp.PromisedQuota < 0 || topUp.PromisedQuota > common.MaxQuota {
			return ErrTopUpQuotaInvalid
		}
		if topUp.PromisedQuota == 0 {
			baseQuota, err := topUpBaseQuota(topUp)
			if err != nil {
				return err
			}
			topUp.PromisedQuota, err = calculateTopUpPromisedQuota(topUp, baseQuota, true)
			if err != nil {
				return err
			}
		}
	}
	return DB.Create(topUp).Error
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
	topUp, _ := FindTopUpByTradeNo(tradeNo)
	return topUp
}

func FindTopUpByTradeNo(tradeNo string) (*TopUp, error) {
	var topUp TopUp
	if err := DB.Where("trade_no = ?", tradeNo).First(&topUp).Error; err != nil {
		return nil, err
	}
	return &topUp, nil
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
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			if expectedPaymentProvider != PaymentProviderLanTu || topUp.PaymentProvider != "" || topUp.PaymentMethod != PaymentMethodLanTu {
				return ErrPaymentMethodMismatch
			}
			topUp.PaymentProvider = PaymentProviderLanTu
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

func BindPendingTopUpPaymentFacts(tradeNo string, expectedPaymentProvider string, gatewayTradeNo string, paymentCurrency string, paymentProductId string, paymentMode string) error {
	if tradeNo == "" || expectedPaymentProvider == "" || (gatewayTradeNo == "" && paymentProductId == "") {
		return errors.New("invalid payment facts")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		if gatewayTradeNo != "" && topUp.GatewayTradeNo != "" && topUp.GatewayTradeNo != gatewayTradeNo {
			return ErrPaymentMethodMismatch
		}
		if paymentCurrency != "" && topUp.PaymentCurrency != "" && topUp.PaymentCurrency != paymentCurrency {
			return ErrPaymentMethodMismatch
		}
		if paymentProductId != "" && topUp.PaymentProductId != "" && topUp.PaymentProductId != paymentProductId {
			return ErrPaymentMethodMismatch
		}
		if paymentMode != "" && topUp.PaymentMode != "" && topUp.PaymentMode != paymentMode {
			return ErrPaymentMethodMismatch
		}
		if gatewayTradeNo != "" {
			topUp.GatewayTradeNo = gatewayTradeNo
		}
		if paymentCurrency != "" {
			topUp.PaymentCurrency = paymentCurrency
		}
		if paymentProductId != "" {
			topUp.PaymentProductId = paymentProductId
		}
		if paymentMode != "" {
			topUp.PaymentMode = paymentMode
		}
		return tx.Save(topUp).Error
	})
}

// applyInviteRebateTx snapshots and credits the inviter's per-user top-up rebate.
// Invalid inviter data is skipped so a paid order is never delayed by rewards.
func applyInviteRebateTx(tx *gorm.DB, topUp *TopUp, invitee *User, creditedQuota int) (granted bool, err error) {
	if tx == nil || topUp == nil || invitee == nil {
		return false, errors.New("invalid invite rebate transaction")
	}
	if invitee.InviterId == 0 {
		return false, nil
	}

	topUp.InviteRebateInviterId = invitee.InviterId
	if invitee.InviterId == invitee.Id || creditedQuota <= 0 {
		common.SysError(fmt.Sprintf("skipped invalid invite rebate: user_id=%d inviter_id=%d quota=%d", invitee.Id, invitee.InviterId, creditedQuota))
		return false, nil
	}

	inviter := &User{}
	if err = lockForUpdate(tx).Select("id", "aff_quota", "aff_history", "invite_rebate_ratio").Where("id = ?", invitee.InviterId).First(inviter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.SysError(fmt.Sprintf("skipped invite rebate because inviter is missing: user_id=%d inviter_id=%d", invitee.Id, invitee.InviterId))
			return false, nil
		}
		return false, err
	}

	topUp.InviteRebateRatio = inviter.InviteRebateRatio
	if inviter.InviteRebateRatio <= 0 {
		return false, nil
	}
	if inviter.InviteRebateRatio > MaxInviteRebateRatio {
		common.SysError(fmt.Sprintf("skipped invalid invite rebate ratio: user_id=%d inviter_id=%d ratio=%d", invitee.Id, invitee.InviterId, inviter.InviteRebateRatio))
		return false, nil
	}

	rebateQuota := int64(creditedQuota) * int64(inviter.InviteRebateRatio) / int64(MaxInviteRebateRatio)
	if rebateQuota <= 0 {
		return false, nil
	}
	affQuota := int64(inviter.AffQuota) + rebateQuota
	affHistory := int64(inviter.AffHistoryQuota) + rebateQuota
	if inviter.AffQuota < 0 || inviter.AffHistoryQuota < 0 || affQuota > int64(common.MaxQuota) || affHistory > int64(common.MaxQuota) {
		common.SysError(fmt.Sprintf("skipped overflowing invite rebate: user_id=%d inviter_id=%d quota=%d", invitee.Id, invitee.InviterId, rebateQuota))
		return false, nil
	}

	topUp.InviteRebateQuota = int(rebateQuota)
	if err = tx.Model(inviter).Updates(map[string]interface{}{
		"aff_quota":   int(affQuota),
		"aff_history": int(affHistory),
	}).Error; err != nil {
		return false, err
	}
	return true, nil
}

func maskedIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) == 1 {
		return "*"
	}
	if len(runes) == 2 {
		return string(runes[:1]) + "*"
	}
	if len(runes) <= 4 {
		return string(runes[:1]) + strings.Repeat("*", len(runes)-2) + string(runes[len(runes)-1:])
	}
	return string(runes[:2]) + strings.Repeat("*", len(runes)-4) + string(runes[len(runes)-2:])
}

func maskedRelatedUser(user *User) string {
	if user == nil {
		return ""
	}
	if masked := maskedIdentifier(user.Username); masked != "" {
		return masked
	}
	return "#" + maskedIdentifier(strconv.Itoa(user.Id))
}

func settleTopUp(tradeNo string, expectedPaymentProvider string, quotaValue func(*TopUp) decimal.Decimal, applyDetails func(*TopUp, *User) map[string]interface{}) (topUp *TopUp, creditedQuota int, settled bool, err error) {
	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	topUp = &TopUp{}
	var inviteRebateGranted bool
	var inviteRebateRelatedUser string
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			if expectedPaymentProvider != PaymentProviderLanTu || topUp.PaymentProvider != "" || topUp.PaymentMethod != PaymentMethodLanTu {
				return ErrPaymentMethodMismatch
			}
			topUp.PaymentProvider = PaymentProviderLanTu
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		if topUp.PromisedQuota > 0 && topUp.PromisedQuota <= common.MaxQuota {
			creditedQuota = topUp.PromisedQuota
		} else {
			creditedQuota, err = calculateTopUpPromisedQuota(topUp, quotaValue(topUp), false)
			if err != nil {
				return err
			}
		}

		user := &User{}
		if err := lockForUpdate(tx).Select("id", "username", "quota", "email", "stripe_customer", "inviter_id", "inviter_topup_rewarded").Where("id = ?", topUp.UserId).First(user).Error; err != nil {
			return err
		}
		balanceLimit := common.MaxQuota
		if user.Quota > common.MaxQuota || (topUp.PromisedQuota > 0 && topUp.PromisedQuota <= common.MaxQuota) {
			// ponytail: honor bounded paid-order snapshots near the legacy balance cap; native int stays the ceiling until quota columns share one width.
			balanceLimit = math.MaxInt
		}
		if user.Quota > balanceLimit-creditedQuota {
			return ErrTopUpQuotaInvalid
		}
		quotaAfterTopUp := user.Quota + creditedQuota

		userUpdates := map[string]interface{}{}
		if applyDetails != nil {
			if details := applyDetails(topUp, user); details != nil {
				userUpdates = details
			}
		}
		userUpdates["quota"] = quotaAfterTopUp
		inviteRebateGranted, err = applyInviteRebateTx(tx, topUp, user, creditedQuota)
		if err != nil {
			return err
		}
		if inviteRebateGranted {
			inviteRebateRelatedUser = maskedRelatedUser(user)
		}

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
		if inviteRebateGranted {
			other := map[string]interface{}{
				"related_user": inviteRebateRelatedUser,
				"op": buildOpField("user.topup_rebate", map[string]interface{}{
					"quota_raw":    topUp.InviteRebateQuota,
					"related_user": inviteRebateRelatedUser,
				}),
			}
			recordLogWithQuota(
				topUp.InviteRebateInviterId,
				LogTypeSystem,
				fmt.Sprintf("邀请好友充值返利 %s，受邀用户 %s", logger.LogQuota(topUp.InviteRebateQuota), inviteRebateRelatedUser),
				topUp.InviteRebateQuota,
				common.MapToJsonStr(other),
			)
			if err := invalidateUserCache(topUp.InviteRebateInviterId); err != nil {
				common.SysError("failed to invalidate inviter cache after topup: " + err.Error())
			}
		}
	}
	return topUp, creditedQuota, settled, err
}

func topUpBonusBasis(topUp *TopUp) (int64, error) {
	if topUp.PaymentProvider != PaymentProviderCreem && topUp.PaymentMethod != PaymentMethodCreem {
		if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
			if topUp.Amount <= 0 || common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
				return 0, ErrTopUpQuotaInvalid
			}
			basis := decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
			if basis.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
				return math.MaxInt64, nil
			}
			return basis.IntPart(), nil
		}
		return topUp.Amount, nil
	}
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		return topUp.Amount, nil
	}
	if topUp.Amount <= 0 || common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return 0, ErrTopUpQuotaInvalid
	}
	basis := decimal.NewFromInt(topUp.Amount).Div(decimal.NewFromFloat(common.QuotaPerUnit))
	if basis.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return math.MaxInt64, nil
	}
	return basis.IntPart(), nil
}

func topUpBaseQuota(topUp *TopUp) (decimal.Decimal, error) {
	if topUp == nil || topUp.Amount <= 0 {
		return decimal.Zero, ErrTopUpQuotaInvalid
	}
	if topUp.PaymentProvider == PaymentProviderCreem || topUp.PaymentMethod == PaymentMethodCreem {
		return decimal.NewFromInt(topUp.Amount), nil
	}
	if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return decimal.Zero, ErrTopUpQuotaInvalid
	}
	if topUp.PaymentProvider == PaymentProviderStripe || topUp.PaymentMethod == PaymentMethodStripe {
		if topUp.PaymentCurrency == "" {
			if topUp.Money <= 0 || math.IsNaN(topUp.Money) || math.IsInf(topUp.Money, 0) {
				return decimal.Zero, ErrTopUpQuotaInvalid
			}
			return decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)), nil
		}
	}
	return decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)), nil
}

func calculateTopUpPromisedQuota(topUp *TopUp, baseQuota decimal.Decimal, checkedBonus bool) (int, error) {
	basis, err := topUpBonusBasis(topUp)
	if err != nil {
		return 0, err
	}
	bonusRate := operation_setting.GetTopupBonusRate(basis)
	if checkedBonus {
		bonusRate, err = operation_setting.GetTopupBonusRateChecked(basis)
		if err != nil {
			return 0, ErrTopUpQuotaInvalid
		}
	}
	quota, clamp := common.QuotaFromDecimalChecked(baseQuota.Mul(decimal.NewFromInt(1).Add(decimal.NewFromFloat(bonusRate))))
	if clamp != nil || quota <= 0 {
		return 0, ErrTopUpQuotaInvalid
	}
	return quota, nil
}

func topUpQuotaFromAmount(topUp *TopUp) decimal.Decimal {
	return decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
}

func stripeTopUpQuota(topUp *TopUp) decimal.Decimal {
	if topUp.PaymentCurrency == "" {
		return decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	return topUpQuotaFromAmount(topUp)
}

func Recharge(referenceId string, customerId string, callerIp string) (err error) {
	return RechargeStripeWithPaymentDetails(referenceId, customerId, "", "", callerIp)
}

func RechargeStripeWithPaymentDetails(referenceId string, customerId string, gatewayTradeNo string, paymentCurrency string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	topUp, quotaToAdd, settled, err := settleTopUp(referenceId, PaymentProviderStripe, stripeTopUpQuota, func(topUp *TopUp, _ *User) map[string]interface{} {
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
		RecordTopupLogWithOperation(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(quotaToAdd), topUp.Amount), callerIp, topUp.PaymentMethod, PaymentMethodStripe, "topup.completed", map[string]interface{}{
			"provider":  "Stripe",
			"quota_raw": quotaToAdd,
			"money":     fmt.Sprintf("%d", topUp.Amount),
		})
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

func GetInviteRebateTopUps(inviterId int, pageInfo *common.PageInfo, params InviteRewardHistoryParams) (topups []*InviteRebateTopUp, total int64, err error) {
	start := pageInfo.GetStartIdx()
	logs, total, err := getInviteRewardSystemLogs(inviterId, pageInfo, params)
	if err != nil {
		return nil, 0, err
	}

	for _, log := range logs {
		topups = append(topups, inviteRewardRecordFromLog(log))
	}
	for i := range topups {
		topups[i].Id = start + i + 1
	}
	return topups, total, nil
}

func getInviteRewardSystemLogs(userId int, pageInfo *common.PageInfo, params InviteRewardHistoryParams) (logs []*Log, total int64, err error) {
	contentQuery := "(content LIKE ? OR content LIKE ? OR content LIKE ? OR content LIKE ? OR content LIKE ? OR content LIKE ?)"
	contentArgs := []interface{}{
		"邀请好友充值返利 %",
		"邀请用户赠送 %",
		"使用邀请码赠送 %",
		"新用户注册赠送 %",
		"转移邀请奖励 %",
		"管理员调整额度 %",
	}
	query := LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", userId, LogTypeSystem).Where(
		contentQuery, contentArgs...,
	)
	limitedQuery := LOG_DB.Model(&Log{}).
		Where("user_id = ? AND type = ?", userId, LogTypeSystem).
		Where(contentQuery, contentArgs...).
		Select("id").Limit(inviteRewardHistoryHardLimit)
	if params.StartTimestamp > 0 {
		query = query.Where("created_at >= ?", params.StartTimestamp)
		limitedQuery = limitedQuery.Where("created_at >= ?", params.StartTimestamp)
	}
	if params.EndTimestamp > 0 {
		query = query.Where("created_at <= ?", params.EndTimestamp)
		limitedQuery = limitedQuery.Where("created_at <= ?", params.EndTimestamp)
	}
	if err = LOG_DB.Table("(?) AS invite_reward_logs", limitedQuery).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	start := pageInfo.GetStartIdx()
	if start >= inviteRewardHistoryHardLimit {
		return []*Log{}, total, nil
	}
	limit := pageInfo.GetPageSize()
	if start+limit > inviteRewardHistoryHardLimit {
		limit = inviteRewardHistoryHardLimit - start
	}
	if limit <= 0 {
		return []*Log{}, total, nil
	}
	err = query.Order("created_at desc, id desc").Limit(limit).Offset(start).Find(&logs).Error
	return logs, total, err
}

func inviteRewardRecordFromLog(log *Log) *InviteRebateTopUp {
	source := "system_reward"
	if strings.HasPrefix(log.Content, "邀请好友充值返利 ") {
		source = "topup_rebate"
	} else if strings.HasPrefix(log.Content, "管理员调整额度 ") {
		source = "admin_adjustment"
	} else if strings.HasPrefix(log.Content, "邀请用户赠送 ") {
		source = "invite_register"
	} else if strings.HasPrefix(log.Content, "使用邀请码赠送 ") {
		source = "invitee_register"
	} else if strings.HasPrefix(log.Content, "新用户注册赠送 ") {
		source = "new_user_reward"
	} else if strings.HasPrefix(log.Content, "转移邀请奖励 ") {
		source = "reward_transfer"
	}
	return &InviteRebateTopUp{
		Id:           log.Id,
		Source:       source,
		Quota:        log.Quota,
		RelatedUser:  inviteRewardRelatedUser(log),
		CompleteTime: log.CreatedAt,
	}
}

func inviteRewardRelatedUser(log *Log) string {
	other, _ := common.StrToMap(log.Other)
	if value, ok := other["related_user"].(string); ok {
		return value
	}
	return ""
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

// ponytail: cap reward log pagination; add a dedicated reward ledger if more history must be pageable.
const inviteRewardHistoryHardLimit = 1000

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
	return SearchAllTopUpsWithParams(TopUpSearchParams{Keyword: keyword}, pageInfo)
}

// SearchAllTopUpsWithParams 按订单、用户和时间搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUpsWithParams(params TopUpSearchParams, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
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
	if params.Keyword != "" {
		pattern, perr := sanitizeLikePattern(params.Keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("(trade_no LIKE ? ESCAPE '!' OR gateway_trade_no LIKE ? ESCAPE '!')", pattern, pattern)
	}
	if params.UserKeyword != "" {
		if userID, parseErr := strconv.Atoi(params.UserKeyword); parseErr == nil {
			query = query.Where("user_id = ?", userID)
		} else {
			pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(params.UserKeyword) + "%"
			var userIDs []int
			if err = tx.Unscoped().Model(&User{}).
				Where("(username LIKE ? ESCAPE '!' OR email LIKE ? ESCAPE '!' OR display_name LIKE ? ESCAPE '!')", pattern, pattern, pattern).
				Limit(1000).
				Pluck("id", &userIDs).Error; err != nil {
				tx.Rollback()
				common.SysError("failed to search topup users: " + err.Error())
				return nil, 0, errors.New("搜索充值记录失败")
			}
			query = query.Where("user_id IN ?", userIDs)
		}
	}
	if params.StartTimestamp > 0 {
		query = query.Where("create_time >= ?", params.StartTimestamp)
	}
	if params.EndTimestamp > 0 {
		query = query.Where("create_time <= ?", params.EndTimestamp)
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
		if topUp.PaymentProvider == PaymentProviderStripe || topUp.PaymentMethod == PaymentMethodStripe {
			return stripeTopUpQuota(topUp)
		}
		if topUp.PaymentProvider == PaymentProviderCreem || topUp.PaymentMethod == PaymentMethodCreem {
			return decimal.NewFromInt(topUp.Amount)
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
		RecordTopupLogWithOperation(topUp.UserId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, "admin", "topup.admin_complete", map[string]interface{}{
			"quota_raw": quotaToAdd,
			"money":     fmt.Sprintf("%.2f", topUp.Money),
		})
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
		RecordTopupLogWithOperation(topUp.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quotaToAdd, topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodCreem, "topup.completed", map[string]interface{}{
			"provider":  "Creem",
			"quota_raw": quotaToAdd,
			"money":     fmt.Sprintf("%.2f", topUp.Money),
		})
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
		RecordTopupLogWithOperation(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentProviderEpay, "topup.completed", map[string]interface{}{
			"provider":  "Epay",
			"quota_raw": quotaToAdd,
			"money":     fmt.Sprintf("%.2f", topUp.Money),
		})
	}
	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) (err error) {
	return RechargeWaffoWithPaymentDetails(tradeNo, "", "", callerIp)
}

func RechargeLanTuWithPaymentDetails(tradeNo string, gatewayTradeNo string, paymentCurrency string, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	topUp, quotaToAdd, settled, err := settleTopUp(tradeNo, PaymentProviderLanTu, topUpQuotaFromAmount, func(topUp *TopUp, _ *User) map[string]interface{} {
		if gatewayTradeNo != "" {
			topUp.GatewayTradeNo = gatewayTradeNo
		}
		if paymentCurrency != "" {
			topUp.PaymentCurrency = paymentCurrency
		}
		return nil
	})
	if err != nil {
		common.SysError("lantu topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if settled {
		RecordTopupLogWithOperation(topUp.UserId, fmt.Sprintf("蓝兔支付充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodLanTu, "topup.completed", map[string]interface{}{
			"provider":  "LanTu",
			"quota_raw": quotaToAdd,
			"money":     fmt.Sprintf("%.2f", topUp.Money),
		})
	}
	return nil
}

func RechargeNowPaymentsWithPaymentDetails(tradeNo string, gatewayTradeNo string, paymentCurrency string, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	topUp, quotaToAdd, settled, err := settleTopUp(tradeNo, PaymentProviderNowPayments, topUpQuotaFromAmount, func(topUp *TopUp, _ *User) map[string]interface{} {
		if gatewayTradeNo != "" {
			topUp.GatewayTradeNo = gatewayTradeNo
		}
		if paymentCurrency != "" {
			topUp.PaymentCurrency = paymentCurrency
		}
		return nil
	})
	if err != nil {
		common.SysError("nowpayments topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if settled {
		RecordTopupLogWithOperation(topUp.UserId, fmt.Sprintf("NOWPayments充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodNowPayments, "topup.completed", map[string]interface{}{
			"provider":  "NOWPayments",
			"quota_raw": quotaToAdd,
			"money":     fmt.Sprintf("%.2f", topUp.Money),
		})
	}
	return nil
}

func RechargeWaffoWithPaymentDetails(tradeNo string, gatewayTradeNo string, paymentCurrency string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	topUp, quotaToAdd, settled, err := settleTopUp(tradeNo, PaymentProviderWaffo, topUpQuotaFromAmount, func(topUp *TopUp, _ *User) map[string]interface{} {
		if gatewayTradeNo != "" {
			topUp.GatewayTradeNo = gatewayTradeNo
		}
		if paymentCurrency != "" {
			topUp.PaymentCurrency = paymentCurrency
		}
		return nil
	})
	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if settled {
		RecordTopupLogWithOperation(topUp.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodWaffo, "topup.completed", map[string]interface{}{
			"provider":  "Waffo",
			"quota_raw": quotaToAdd,
			"money":     fmt.Sprintf("%.2f", topUp.Money),
		})
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string) (err error) {
	return RechargeWaffoPancakeWithPaymentDetails(tradeNo, "", "", "")
}

func RechargeWaffoPancakeWithPaymentDetails(tradeNo string, gatewayTradeNo string, paymentCurrency string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	topUp, quotaToAdd, settled, err := settleTopUp(tradeNo, PaymentProviderWaffoPancake, topUpQuotaFromAmount, func(topUp *TopUp, _ *User) map[string]interface{} {
		if gatewayTradeNo != "" {
			topUp.GatewayTradeNo = gatewayTradeNo
		}
		if paymentCurrency != "" {
			topUp.PaymentCurrency = paymentCurrency
		}
		return nil
	})
	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if settled {
		RecordTopupLogWithOperation(topUp.UserId, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodWaffoPancake, "topup.completed", map[string]interface{}{
			"provider":  "Waffo Pancake",
			"quota_raw": quotaToAdd,
			"money":     fmt.Sprintf("%.2f", topUp.Money),
		})
	}

	return nil
}
