package controller

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

func GetTopUpInfo(c *gin.Context) {
	complianceConfirmed := operation_setting.IsPaymentComplianceConfirmed()

	payMethods := []map[string]string{}
	if complianceConfirmed && isEpayTopUpEnabled() {
		payMethods = append(payMethods, operation_setting.PayMethods...)
	}

	// 如果启用了 Stripe 支付，添加到支付方法列表
	if isStripeTopUpEnabled() {
		// 检查是否已经包含 Stripe
		hasStripe := false
		for _, method := range payMethods {
			if method["type"] == "stripe" {
				hasStripe = true
				break
			}
		}

		if !hasStripe {
			stripeMethod := map[string]string{
				"name":      "Stripe",
				"type":      "stripe",
				"color":     "#635BFF",
				"min_topup": strconv.FormatInt(getConfiguredMinTopUp(setting.StripeMinTopUp), 10),
			}
			payMethods = append(payMethods, stripeMethod)
		}
	}

	enableLanTu := isLanTuTopUpEnabled()
	if enableLanTu {
		hasLanTu := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodLanTu {
				hasLanTu = true
				break
			}
		}
		if !hasLanTu {
			payMethods = append(payMethods, map[string]string{
				"name":      "WeChat",
				"type":      model.PaymentMethodLanTu,
				"color":     "#07C160",
				"min_topup": strconv.FormatInt(getLanTuMinTopUp(), 10),
			})
		}
	}

	enableNowPayments := isNowPaymentsTopUpEnabled()
	if enableNowPayments {
		hasNowPayments := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodNowPayments {
				hasNowPayments = true
				break
			}
		}
		if !hasNowPayments {
			payMethods = append(payMethods, map[string]string{
				"name":      "Crypto Pay",
				"type":      model.PaymentMethodNowPayments,
				"color":     "#F7931A",
				"min_topup": strconv.FormatInt(getConfiguredMinTopUp(setting.NowPaymentsMinTopUp), 10),
			})
		}
	}

	// Waffo Pancake is displayed above the standard Waffo gateway.
	enableWaffoPancake := isWaffoPancakeTopUpEnabled()
	if enableWaffoPancake {
		hasWaffoPancake := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffoPancake {
				hasWaffoPancake = true
				break
			}
		}

		if !hasWaffoPancake {
			payMethods = append(payMethods, map[string]string{
				"name":      "Global Pay",
				"type":      model.PaymentMethodWaffoPancake,
				"color":     "#22C55E",
				"min_topup": strconv.FormatInt(getConfiguredMinTopUp(setting.WaffoPancakeMinTopUp), 10),
			})
		}
	}

	// 如果启用了 Waffo 支付，添加到支付方法列表
	enableWaffo := isWaffoTopUpEnabled()
	if enableWaffo {
		hasWaffo := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffo {
				hasWaffo = true
				break
			}
		}

		if !hasWaffo {
			waffoMethod := map[string]string{
				"name":      "Waffo (Global Payment)",
				"type":      model.PaymentMethodWaffo,
				"color":     "#3B82F6",
				"min_topup": strconv.FormatInt(getConfiguredMinTopUp(setting.WaffoMinTopUp), 10),
			}
			payMethods = append(payMethods, waffoMethod)
		}
	}

	topupGroupRatio := 1.0
	if userId := c.GetInt("id"); userId > 0 {
		if group, err := model.GetUserGroup(userId, true); err == nil {
			if ratio := common.GetTopupGroupRatio(group); ratio > 0 {
				topupGroupRatio = ratio
			}
		}
	}

	data := gin.H{
		"enable_online_topup":              isEpayTopUpEnabled(),
		"enable_stripe_topup":              isStripeTopUpEnabled(),
		"enable_creem_topup":               isCreemTopUpEnabled(),
		"enable_lantu_topup":               enableLanTu,
		"enable_nowpayments_topup":         enableNowPayments,
		"enable_waffo_topup":               enableWaffo,
		"enable_waffo_pancake_topup":       enableWaffoPancake,
		"enable_redemption":                complianceConfirmed,
		"payment_compliance_confirmed":     complianceConfirmed,
		"payment_compliance_terms_version": operation_setting.CurrentComplianceTermsVersion,
		"waffo_pay_methods": func() interface{} {
			if enableWaffo {
				return setting.GetWaffoPayMethods()
			}
			return nil
		}(),
		"creem_products":                   setting.CreemProducts,
		"pay_methods":                      payMethods,
		"min_topup":                        getConfiguredMinTopUp(operation_setting.MinTopUp),
		"stripe_min_topup":                 getConfiguredMinTopUp(setting.StripeMinTopUp),
		"lantu_min_topup":                  getLanTuMinTopUp(),
		"nowpayments_min_topup":            getConfiguredMinTopUp(setting.NowPaymentsMinTopUp),
		"waffo_min_topup":                  getConfiguredMinTopUp(setting.WaffoMinTopUp),
		"waffo_pancake_min_topup":          getConfiguredMinTopUp(setting.WaffoPancakeMinTopUp),
		"amount_options":                   operation_setting.GetPaymentSetting().AmountOptions,
		"bonus":                            operation_setting.GetTopupBonusMapForAPI(),
		"discount":                         operation_setting.GetTopupDiscountMapForAPI(),
		"topup_group_ratio":                topupGroupRatio,
		"topup_link":                       common.TopUpLink,
		"quota_for_inviter":                common.QuotaForInviter,
		"quota_for_invitee":                common.QuotaForInvitee,
		"quota_for_inviter_on_first_topup": common.QuotaForInviterOnFirstTopup,
	}
	common.ApiSuccess(c, data)
}

type EpayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

type AmountRequest struct {
	Amount int64 `json:"amount"`
}

func GetEpayClient() *epay.Client {
	if operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	payAddress := operation_setting.PayAddress
	if payAddress == "" {
		payAddress = "https://epay-webhook.invalid"
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, payAddress)
	if err != nil {
		return nil
	}
	return withUrl
}

func hasExpectedEpayMerchant(params map[string]string) bool {
	return params != nil && params["pid"] != "" && params["pid"] == operation_setting.EpayId
}

func getPayMoney(amount int64, group string) float64 {
	dAmount := decimal.NewFromInt(amount)
	// 充值金额以“展示类型”为准：
	// - USD/CNY: 前端传 amount 为金额单位；TOKENS: 前端传 tokens，需要换成 USD 金额
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		dAmount = dAmount.Div(dQuotaPerUnit)
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	dTopupGroupRatio := decimal.NewFromFloat(topupGroupRatio)
	dPrice := decimal.NewFromFloat(operation_setting.Price)
	discount := operation_setting.GetTopupDiscountMultiplier(amount)
	dDiscount := decimal.NewFromFloat(discount)

	payMoney := dAmount.Mul(dPrice).Mul(dTopupGroupRatio).Mul(dDiscount)

	return payMoney.InexactFloat64()
}

func getMinTopup() int64 {
	return getConfiguredMinTopUp(operation_setting.MinTopUp)
}

func getConfiguredMinTopUp(configured int) int64 {
	if configured < 1 {
		configured = 1
	}
	if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
		return int64(configured)
	}
	if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return math.MaxInt64
	}
	minimum := decimal.NewFromInt(int64(configured)).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Ceil()
	if minimum.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return math.MaxInt64
	}
	return minimum.IntPart()
}

func normalizeTopUpAmount(amount int64) (int64, error) {
	if amount <= 0 || common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return 0, errors.New("充值数量无效")
	}

	normalized := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		normalized = normalized.Div(decimal.NewFromFloat(common.QuotaPerUnit))
		if !normalized.Equal(normalized.Truncate(0)) {
			return 0, errors.New("Token 充值数量必须对应完整的美元额度")
		}
	}

	if normalized.LessThanOrEqual(decimal.Zero) || normalized.GreaterThan(decimal.NewFromInt(int64(common.MaxQuota))) {
		return 0, errors.New("充值数量无效")
	}
	if err := validatePromisedTopUpQuota(normalized.IntPart(), amount); err != nil {
		return 0, err
	}
	return normalized.IntPart(), nil
}

func validatePromisedTopUpQuota(amount int64, bonusBasis int64) error {
	if amount <= 0 || bonusBasis <= 0 || common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return errors.New("充值数量无效")
	}
	bonusRate, err := operation_setting.GetTopupBonusRateChecked(bonusBasis)
	if err != nil {
		return errors.New("充值赠额配置无效")
	}
	quota := decimal.NewFromInt(amount).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Mul(decimal.NewFromInt(1).Add(decimal.NewFromFloat(bonusRate)))
	if quota.GreaterThan(decimal.NewFromInt(int64(common.MaxQuota))) {
		return errors.New("充值数量过高")
	}
	return nil
}

func RequestEpay(c *gin.Context) {
	if !isEpayTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "在线支付未启用"})
		return
	}

	var req EpayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}
	amount, err := normalizeTopUpAmount(req.Amount)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 || math.IsNaN(payMoney) || math.IsInf(payMoney, 0) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	payMoneyText := decimal.NewFromFloat(payMoney).StringFixed(2)
	payMoney, err = strconv.ParseFloat(payMoneyText, 64)
	if err != nil || payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额无效"})
		return
	}

	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(paymentReturnPath("/usage-logs"))
	notifyUrl, _ := url.Parse(callBackAddress + "/api/user/epay/notify")
	tradeNo, err := model.NewTopUpTradeNo(model.PaymentProviderEpay, id)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 生成充值订单号失败 user_id=%d error=%q", id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	client := GetEpayClient()
	if client == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
		return
	}
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentCurrency: "CNY",
		PaymentMethod:   req.PaymentMethod,
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 创建充值订单失败 user_id=%d trade_no=%s payment_method=%s amount=%d error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("TUC%d", req.Amount),
		Money:          payMoneyText,
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderEpay, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 拉起支付失败 user_id=%d trade_no=%s payment_method=%s amount=%d error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值订单创建成功 user_id=%d trade_no=%s payment_method=%s amount=%d money=%.2f", id, tradeNo, req.PaymentMethod, req.Amount, payMoney))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

// tradeNo lock
var orderLocks sync.Map
var createLock sync.Mutex

// refCountedMutex 带引用计数的互斥锁，确保最后一个使用者才从 map 中删除
type refCountedMutex struct {
	mu       sync.Mutex
	refCount int
}

// LockOrder 尝试对给定订单号加锁
func LockOrder(tradeNo string) {
	createLock.Lock()
	var rcm *refCountedMutex
	if v, ok := orderLocks.Load(tradeNo); ok {
		rcm = v.(*refCountedMutex)
	} else {
		rcm = &refCountedMutex{}
		orderLocks.Store(tradeNo, rcm)
	}
	rcm.refCount++
	createLock.Unlock()
	rcm.mu.Lock()
}

// UnlockOrder 释放给定订单号的锁
func UnlockOrder(tradeNo string) {
	v, ok := orderLocks.Load(tradeNo)
	if !ok {
		return
	}
	rcm := v.(*refCountedMutex)
	rcm.mu.Unlock()

	createLock.Lock()
	rcm.refCount--
	if rcm.refCount == 0 {
		orderLocks.Delete(tradeNo)
	}
	createLock.Unlock()
}

func EpayNotify(c *gin.Context) {
	if !isEpayWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook POST 表单解析失败 path=%q client_ip=%s error=%q", c.Request.URL.Path, c.ClientIP(), err.Error()))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		params = lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	} else {
		// GET 请求：从 URL Query 解析参数
		params = lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.URL.Query().Get(t)
			return r
		}, map[string]string{})
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 收到请求 path=%q client_ip=%s method=%s", c.Request.URL.Path, c.ClientIP(), c.Request.Method))

	if len(params) == 0 {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 参数为空 path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	client := GetEpayClient()
	if client == nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 client 未初始化 path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.URL.Path, c.ClientIP(), err.Error()))
		}
		return
	}
	if !hasExpectedEpayMerchant(params) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 商户号不匹配 path=%q client_ip=%s callback_pid=%q", c.Request.URL.Path, c.ClientIP(), params["pid"]))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	verifyInfo, verifyErr := client.Verify(params)
	if verifyErr != nil || verifyInfo == nil || !verifyInfo.VerifyStatus {
		if _, err := c.Writer.Write([]byte("fail")); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.URL.Path, c.ClientIP(), err.Error()))
		}
		if verifyErr != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_error=%q", c.Request.URL.Path, c.ClientIP(), verifyErr.Error()))
		} else {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_status=false", c.Request.URL.Path, c.ClientIP()))
		}
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签成功 trade_no=%s gateway_trade_no=%s callback_type=%s trade_status=%s money=%s client_ip=%s", verifyInfo.ServiceTradeNo, verifyInfo.TradeNo, verifyInfo.Type, verifyInfo.TradeStatus, verifyInfo.Money, c.ClientIP()))
	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 忽略事件 trade_no=%s callback_type=%s trade_status=%s client_ip=%s", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP()))
		if _, err := c.Writer.Write([]byte("success")); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 trade_no=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, c.ClientIP(), err.Error()))
		}
		return
	}

	LockOrder(verifyInfo.ServiceTradeNo)
	defer UnlockOrder(verifyInfo.ServiceTradeNo)
	topUp := model.GetTopUpByTradeNo(verifyInfo.ServiceTradeNo)
	if topUp == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调订单不存在 trade_no=%s callback_type=%s client_ip=%s", verifyInfo.ServiceTradeNo, verifyInfo.Type, c.ClientIP()))
		if _, err := c.Writer.Write([]byte("fail")); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 trade_no=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, c.ClientIP(), err.Error()))
		}
		return
	}
	if topUp.PaymentProvider != model.PaymentProviderEpay {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 订单支付网关不匹配 trade_no=%s order_provider=%s callback_type=%s client_ip=%s", verifyInfo.ServiceTradeNo, topUp.PaymentProvider, verifyInfo.Type, c.ClientIP()))
		if _, err := c.Writer.Write([]byte("fail")); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 trade_no=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, c.ClientIP(), err.Error()))
		}
		return
	}
	callbackMoney, moneyErr := decimal.NewFromString(verifyInfo.Money)
	expectedMoneyText := strconv.FormatFloat(topUp.Money, 'f', 2, 64)
	expectedMoney, expectedMoneyErr := decimal.NewFromString(expectedMoneyText)
	if moneyErr != nil || expectedMoneyErr != nil || !callbackMoney.Equal(expectedMoney) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调金额与订单不匹配 trade_no=%s expected_money=%s callback_money=%q client_ip=%s", verifyInfo.ServiceTradeNo, expectedMoneyText, verifyInfo.Money, c.ClientIP()))
		if _, err := c.Writer.Write([]byte("fail")); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 trade_no=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, c.ClientIP(), err.Error()))
		}
		return
	}
	if topUp.PaymentMethod != verifyInfo.Type {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 实际支付方式与订单不同 trade_no=%s order_payment_method=%s actual_type=%s client_ip=%s", verifyInfo.ServiceTradeNo, topUp.PaymentMethod, verifyInfo.Type, c.ClientIP()))
	}

	alreadySettled := topUp.Status == common.TopUpStatusSuccess
	if err := model.RechargeEpay(verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeNo, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 充值处理失败 trade_no=%s gateway_trade_no=%s user_id=%d client_ip=%s error=%q", verifyInfo.ServiceTradeNo, verifyInfo.TradeNo, topUp.UserId, c.ClientIP(), err.Error()))
		if _, writeErr := c.Writer.Write([]byte("fail")); writeErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 trade_no=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, c.ClientIP(), writeErr.Error()))
		}
		return
	}

	if alreadySettled {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 订单已处理，忽略重复回调 trade_no=%s status=%s client_ip=%s", verifyInfo.ServiceTradeNo, topUp.Status, c.ClientIP()))
	} else {
		settledTopUp := model.GetTopUpByTradeNo(verifyInfo.ServiceTradeNo)
		if settledTopUp == nil {
			settledTopUp = topUp
		}
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值成功 trade_no=%s gateway_trade_no=%s user_id=%d client_ip=%s quota_to_add=%d money=%.2f", settledTopUp.TradeNo, settledTopUp.GatewayTradeNo, settledTopUp.UserId, c.ClientIP(), settledTopUp.CreditedQuota, settledTopUp.Money))
	}
	if _, err := c.Writer.Write([]byte("success")); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 trade_no=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, c.ClientIP(), err.Error()))
	}
}

func RequestAmount(c *gin.Context) {
	var req AmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < getMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}
	if _, err := normalizeTopUpAmount(req.Amount); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney <= 0.01 || math.IsNaN(payMoney) || math.IsInf(payMoney, 0) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
}

func GetUserTopUps(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchUserTopUps(userId, keyword, pageInfo)
	} else {
		topups, total, err = model.GetUserTopUps(userId, pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

// GetAllTopUps 管理员获取全平台充值记录
func GetAllTopUps(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	params := model.TopUpSearchParams{
		Keyword:     strings.TrimSpace(c.Query("keyword")),
		UserKeyword: strings.TrimSpace(c.Query("user_keyword")),
	}
	var err error
	if value := c.Query("start_timestamp"); value != "" {
		params.StartTimestamp, err = strconv.ParseInt(value, 10, 64)
		if err != nil || params.StartTimestamp < 0 {
			common.ApiErrorMsg(c, "开始时间无效")
			return
		}
	}
	if value := c.Query("end_timestamp"); value != "" {
		params.EndTimestamp, err = strconv.ParseInt(value, 10, 64)
		if err != nil || params.EndTimestamp < 0 {
			common.ApiErrorMsg(c, "结束时间无效")
			return
		}
	}
	if params.StartTimestamp > 0 && params.EndTimestamp > 0 && params.StartTimestamp > params.EndTimestamp {
		common.ApiErrorMsg(c, "开始时间不能晚于结束时间")
		return
	}

	var (
		topups []*model.TopUp
		total  int64
	)
	if params.Keyword != "" || params.UserKeyword != "" || params.StartTimestamp > 0 || params.EndTimestamp > 0 {
		topups, total, err = model.SearchAllTopUpsWithParams(params, pageInfo)
	} else {
		topups, total, err = model.GetAllTopUps(pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

type AdminCompleteTopupRequest struct {
	TradeNo string `json:"trade_no"`
}

// AdminCompleteTopUp 管理员补单接口
func AdminCompleteTopUp(c *gin.Context) {
	var req AdminCompleteTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 订单级互斥，防止并发补单
	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	if err := model.ManualCompleteTopUp(req.TradeNo, c.ClientIP()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
