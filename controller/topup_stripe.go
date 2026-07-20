package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	stripeprice "github.com/stripe/stripe-go/v81/price"
	"github.com/stripe/stripe-go/v81/webhook"
)

var stripeAdaptor = &StripeAdaptor{}

const (
	maxStripeTopUpUSD      int64 = 10_000
	maxStripeUnitAmount    int64 = 99_999_999
	stripePaymentMode            = "payment"
	stripePaymentPromoMode       = "payment+promo"
)

type stripeTopUpQuote struct {
	creditAmount int64
	money        float64
	unitAmount   int64
	currency     stripe.Currency
}

type stripeTopUpPriceConfig struct {
	priceID   string
	productID string
	currency  stripe.Currency
}

var stripeTopUpPriceCache struct {
	sync.RWMutex
	apiSecret string
	config    stripeTopUpPriceConfig
}

// StripePayRequest represents a payment request for Stripe checkout.
type StripePayRequest struct {
	// Amount is the quantity of units to purchase.
	Amount int64 `json:"amount"`
	// PaymentMethod specifies the payment method (e.g., "stripe").
	PaymentMethod string `json:"payment_method"`
	// SuccessURL is the optional custom URL to redirect after successful payment.
	// If empty, defaults to the server's console log page.
	SuccessURL string `json:"success_url,omitempty"`
	// CancelURL is the optional custom URL to redirect when payment is canceled.
	// If empty, defaults to the server's console topup page.
	CancelURL string `json:"cancel_url,omitempty"`
}

type StripeAdaptor struct {
}

func (*StripeAdaptor) RequestAmount(c *gin.Context, req *StripePayRequest) {
	if !isStripeTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Stripe 支付未启用"})
		return
	}
	if err := validateStripeTopUpAmount(req.Amount); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	priceConfig, err := getStripeTopUpPriceConfig()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 读取价格配置失败 user_id=%d error=%q", id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Stripe 支付配置错误"})
		return
	}
	quote, err := calculateStripeTopUpQuote(req.Amount, group, priceConfig.currency)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 计算充值金额失败 user_id=%d amount=%d error=%q", id, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额无效"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": formatStripeMoney(quote)})
}

func (*StripeAdaptor) RequestPay(c *gin.Context, req *StripePayRequest) {
	if !isStripeTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Stripe 支付未启用"})
		return
	}
	if req.PaymentMethod != model.PaymentMethodStripe {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}
	if err := validateStripeTopUpAmount(req.Amount); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": err.Error(), "data": 10})
		return
	}

	if req.SuccessURL != "" && common.ValidateRedirectURL(req.SuccessURL) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "支付成功重定向URL不在可信任域名列表中", "data": ""})
		return
	}

	if req.CancelURL != "" && common.ValidateRedirectURL(req.CancelURL) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "支付取消重定向URL不在可信任域名列表中", "data": ""})
		return
	}

	id := c.GetInt("id")
	user, err := model.GetUserById(id, false)
	if err != nil || user == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "用户不存在"})
		return
	}
	priceConfig, err := getStripeTopUpPriceConfig()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 读取价格配置失败 user_id=%d error=%q", id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Stripe 支付配置错误"})
		return
	}
	quote, err := calculateStripeTopUpQuote(req.Amount, user.Group, priceConfig.currency)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 计算充值金额失败 user_id=%d amount=%d error=%q", id, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额无效"})
		return
	}

	referenceId, err := model.NewTopUpTradeNo(model.PaymentProviderStripe)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 生成充值订单号失败 user_id=%d error=%q", id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	topUp := &model.TopUp{
		UserId:           id,
		Amount:           quote.creditAmount,
		Money:            quote.money,
		TradeNo:          referenceId,
		PaymentProductId: priceConfig.productID,
		PaymentCurrency:  strings.ToUpper(string(quote.currency)),
		PaymentMode:      stripePaymentMode,
		PaymentMethod:    model.PaymentMethodStripe,
		PaymentProvider:  model.PaymentProviderStripe,
		CreateTime:       time.Now().Unix(),
		Status:           common.TopUpStatusPending,
	}
	if setting.StripePromotionCodesEnabled {
		topUp.PaymentMode = stripePaymentPromoMode
	}
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 创建充值订单失败 user_id=%d trade_no=%s amount=%d error=%q", id, referenceId, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	checkoutSession, err := genStripeLink(referenceId, user.StripeCustomer, user.Email, priceConfig, quote.unitAmount, req.SuccessURL, req.CancelURL)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 创建 Checkout Session 失败 user_id=%d trade_no=%s amount=%d error=%q", id, referenceId, req.Amount, err.Error()))
		_ = model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusFailed)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	if checkoutSession == nil || checkoutSession.ID == "" || checkoutSession.ClientReferenceID != referenceId ||
		!strings.EqualFold(string(checkoutSession.Currency), string(quote.currency)) || checkoutSession.AmountSubtotal != quote.unitAmount {
		_ = model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe Checkout Session 事实不匹配 user_id=%d trade_no=%s", id, referenceId))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	if err := model.BindPendingTopUpPaymentFacts(referenceId, model.PaymentProviderStripe, checkoutSession.ID, strings.ToUpper(string(quote.currency)), priceConfig.productID, topUp.PaymentMode); err != nil {
		_ = model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 保存 Checkout Session 失败 user_id=%d trade_no=%s error=%q", id, referenceId, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Stripe 充值订单创建成功 user_id=%d trade_no=%s amount=%d credit_amount=%d money=%.2f currency=%s", id, referenceId, req.Amount, quote.creditAmount, quote.money, strings.ToUpper(string(quote.currency))))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": checkoutSession.URL,
		},
	})
}

func RequestStripeAmount(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestAmount(c, &req)
}

func RequestStripePay(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestPay(c, &req)
}

func StripeWebhook(c *gin.Context) {
	ctx := c.Request.Context()
	if !isStripeWebhookEnabled() {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe webhook 读取请求体失败 path=%q client_ip=%s error=%q", c.Request.URL.Path, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	signature := c.GetHeader("Stripe-Signature")
	logger.LogInfo(ctx, fmt.Sprintf("Stripe webhook 收到请求 path=%q client_ip=%s body_size=%d", c.Request.URL.Path, c.ClientIP(), len(payload)))
	event, err := webhook.ConstructEventWithOptions(payload, signature, setting.StripeWebhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})

	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe webhook 验签失败 path=%q client_ip=%s error=%q", c.Request.URL.Path, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	callerIp := c.ClientIP()
	logger.LogInfo(ctx, fmt.Sprintf("Stripe webhook 验签成功 event_type=%s client_ip=%s path=%q", string(event.Type), callerIp, c.Request.URL.Path))
	var handleErr error
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted:
		handleErr = sessionCompleted(ctx, event, callerIp)
	case stripe.EventTypeCheckoutSessionExpired:
		handleErr = sessionExpired(ctx, event)
	case stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded:
		handleErr = sessionAsyncPaymentSucceeded(ctx, event, callerIp)
	case stripe.EventTypeCheckoutSessionAsyncPaymentFailed:
		handleErr = sessionAsyncPaymentFailed(ctx, event, callerIp)
	default:
		logger.LogInfo(ctx, fmt.Sprintf("Stripe webhook 忽略事件 event_type=%s client_ip=%s", string(event.Type), callerIp))
	}
	if handleErr != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe webhook 处理失败 event_type=%s client_ip=%s error=%q", string(event.Type), callerIp, handleErr.Error()))
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	c.Status(http.StatusOK)
}

func sessionCompleted(ctx context.Context, event stripe.Event, callerIp string) error {
	customerId := event.GetObjectValue("customer")
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "complete" != status {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout.completed 状态异常，忽略处理 trade_no=%s status=%s client_ip=%s", referenceId, status, callerIp))
		return nil
	}

	paymentStatus := event.GetObjectValue("payment_status")
	if paymentStatus != "paid" {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe Checkout 支付未完成，等待异步结果 trade_no=%s payment_status=%s client_ip=%s", referenceId, paymentStatus, callerIp))
		return nil
	}

	return fulfillOrder(ctx, event, referenceId, customerId, callerIp)
}

// sessionAsyncPaymentSucceeded handles delayed payment methods (bank transfer, SEPA, etc.)
// that confirm payment after the checkout session completes.
func sessionAsyncPaymentSucceeded(ctx context.Context, event stripe.Event, callerIp string) error {
	customerId := event.GetObjectValue("customer")
	referenceId := event.GetObjectValue("client_reference_id")
	logger.LogInfo(ctx, fmt.Sprintf("Stripe 异步支付成功 trade_no=%s client_ip=%s", referenceId, callerIp))

	return fulfillOrder(ctx, event, referenceId, customerId, callerIp)
}

// sessionAsyncPaymentFailed marks orders as failed when delayed payment methods
// ultimately fail (e.g. bank transfer not received, SEPA rejected).
func sessionAsyncPaymentFailed(ctx context.Context, event stripe.Event, callerIp string) error {
	referenceId := event.GetObjectValue("client_reference_id")
	logger.LogWarn(ctx, fmt.Sprintf("Stripe 异步支付失败 trade_no=%s client_ip=%s", referenceId, callerIp))

	if len(referenceId) == 0 {
		return errors.New("Stripe 异步支付失败事件缺少订单号")
	}

	LockOrder(referenceId)
	defer UnlockOrder(referenceId)

	if err := model.UpdatePendingSubscriptionOrderStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusFailed); err == nil {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 订阅订单已标记为失败 trade_no=%s client_ip=%s", referenceId, callerIp))
		return nil
	} else if !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		return fmt.Errorf("Stripe 标记订阅订单失败状态失败 trade_no=%s: %w", referenceId, err)
	}

	err := model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusFailed)
	if errors.Is(err, model.ErrTopUpStatusInvalid) {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 异步支付失败但订单已处于终态，忽略处理 trade_no=%s client_ip=%s", referenceId, callerIp))
		return nil
	}
	if err != nil {
		return fmt.Errorf("Stripe 标记充值订单失败状态失败 trade_no=%s: %w", referenceId, err)
	}
	logger.LogInfo(ctx, fmt.Sprintf("Stripe 充值订单已标记为失败 trade_no=%s client_ip=%s", referenceId, callerIp))
	return nil
}

// fulfillOrder is the shared logic for crediting quota after payment is confirmed.
func validateStripePaymentFacts(tradeNo string, gatewayTradeNo string, paymentCurrency string, paymentMode string, money float64, event stripe.Event) (string, error) {
	if event.GetObjectValue("client_reference_id") != tradeNo {
		return "", errors.New("Stripe 订单号不匹配")
	}
	if event.GetObjectValue("status") != "complete" || event.GetObjectValue("payment_status") != "paid" {
		return "", errors.New("Stripe Checkout 尚未完成支付")
	}
	legacyOrder := paymentMode == ""
	if !legacyOrder && (gatewayTradeNo == "" || paymentCurrency == "" || (paymentMode != stripePaymentMode && paymentMode != stripePaymentPromoMode)) {
		return "", errors.New("Stripe 本地支付事实不完整")
	}
	if gatewayTradeNo != "" && event.GetObjectValue("id") != gatewayTradeNo {
		return "", errors.New("Stripe Checkout Session 不匹配")
	}
	if event.GetObjectValue("mode") != string(stripe.CheckoutSessionModePayment) {
		return "", errors.New("Stripe Checkout 模式不匹配")
	}
	currency := strings.ToUpper(event.GetObjectValue("currency"))
	if currency == "" || (paymentCurrency != "" && !strings.EqualFold(currency, paymentCurrency)) {
		return "", errors.New("Stripe 支付币种不匹配")
	}
	minorUnit := stripeCurrencyMinorUnit(stripe.Currency(strings.ToLower(currency)))
	if minorUnit <= 0 {
		return "", errors.New("Stripe 支付币种无效")
	}
	expectedSubtotal := decimal.NewFromFloat(money).Mul(decimal.NewFromInt(minorUnit)).Round(0).IntPart()
	subtotal, err := strconv.ParseInt(event.GetObjectValue("amount_subtotal"), 10, 64)
	if err != nil || subtotal <= 0 || (!legacyOrder && subtotal != expectedSubtotal) || (legacyOrder && subtotal < expectedSubtotal) {
		return "", errors.New("Stripe 原始小计不匹配")
	}
	total, err := strconv.ParseInt(event.GetObjectValue("amount_total"), 10, 64)
	if err != nil || total <= 0 || total > subtotal {
		return "", errors.New("Stripe 实付金额无效")
	}
	if paymentMode == stripePaymentMode && total != subtotal {
		return "", errors.New("Stripe 实付金额不匹配")
	}
	return currency, nil
}

func fulfillOrder(ctx context.Context, event stripe.Event, referenceId string, customerId string, callerIp string) error {
	if len(referenceId) == 0 {
		return errors.New("Stripe 完成订单时缺少订单号")
	}

	LockOrder(referenceId)
	defer UnlockOrder(referenceId)
	payload := map[string]any{
		"checkout_session": event.GetObjectValue("id"),
		"customer":         customerId,
		"amount_subtotal":  event.GetObjectValue("amount_subtotal"),
		"amount_total":     event.GetObjectValue("amount_total"),
		"currency":         strings.ToUpper(event.GetObjectValue("currency")),
		"mode":             event.GetObjectValue("mode"),
		"event_type":       string(event.Type),
	}
	if order := model.GetSubscriptionOrderByTradeNo(referenceId); order != nil {
		if order.PaymentProvider != model.PaymentProviderStripe {
			return errors.New("Stripe 订阅订单支付网关不匹配")
		}
		currency, err := validateStripePaymentFacts(order.TradeNo, order.GatewayTradeNo, order.PaymentCurrency, order.PaymentMode, order.Money, event)
		if err != nil {
			return err
		}
		if err := model.CompleteSubscriptionOrderWithPaymentDetails(referenceId, common.GetJsonString(payload), model.PaymentProviderStripe, "", event.GetObjectValue("id"), currency); err != nil {
			return fmt.Errorf("Stripe 订阅订单处理失败 trade_no=%s event_type=%s: %w", referenceId, string(event.Type), err)
		}
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 订阅订单处理成功 trade_no=%s event_type=%s client_ip=%s", referenceId, string(event.Type), callerIp))
		return nil
	}

	topUp := model.GetTopUpByTradeNo(referenceId)
	if topUp == nil {
		return model.ErrTopUpNotFound
	}
	if topUp.PaymentProvider != model.PaymentProviderStripe {
		return errors.New("Stripe 充值订单支付网关不匹配")
	}
	currency, err := validateStripePaymentFacts(topUp.TradeNo, topUp.GatewayTradeNo, topUp.PaymentCurrency, topUp.PaymentMode, topUp.Money, event)
	if err != nil {
		return err
	}
	err = model.RechargeStripeWithPaymentDetails(referenceId, customerId, event.GetObjectValue("id"), currency, callerIp)
	if err != nil {
		return fmt.Errorf("Stripe 充值处理失败 trade_no=%s event_type=%s: %w", referenceId, string(event.Type), err)
	}

	total, _ := strconv.ParseFloat(event.GetObjectValue("amount_total"), 64)
	logger.LogInfo(ctx, fmt.Sprintf("Stripe 充值成功 trade_no=%s amount_total=%.2f currency=%s event_type=%s client_ip=%s", referenceId, total/100, currency, string(event.Type), callerIp))
	return nil
}

func sessionExpired(ctx context.Context, event stripe.Event) error {
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "expired" != status {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout.expired 状态异常，忽略处理 trade_no=%s status=%s", referenceId, status))
		return nil
	}

	if len(referenceId) == 0 {
		logger.LogWarn(ctx, "Stripe checkout.expired 缺少订单号")
		return nil
	}

	// Subscription order expiration
	LockOrder(referenceId)
	defer UnlockOrder(referenceId)
	if err := model.ExpireSubscriptionOrder(referenceId, model.PaymentProviderStripe); err == nil {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 订阅订单已过期 trade_no=%s", referenceId))
		return nil
	} else if !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		return fmt.Errorf("Stripe 订阅订单过期处理失败 trade_no=%s: %w", referenceId, err)
	}

	err := model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusExpired)
	if errors.Is(err, model.ErrTopUpNotFound) {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 充值订单不存在，无法标记过期 trade_no=%s", referenceId))
		return nil
	}
	if errors.Is(err, model.ErrTopUpStatusInvalid) {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 充值订单已处于终态，忽略过期事件 trade_no=%s", referenceId))
		return nil
	}
	if errors.Is(err, model.ErrPaymentMethodMismatch) {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 过期事件支付网关不匹配，忽略处理 trade_no=%s", referenceId))
		return nil
	}
	if err != nil {
		return fmt.Errorf("Stripe 充值订单过期处理失败 trade_no=%s: %w", referenceId, err)
	}

	logger.LogInfo(ctx, fmt.Sprintf("Stripe 充值订单已过期 trade_no=%s", referenceId))
	return nil
}

func genStripeLink(referenceId string, customerId string, email string, priceConfig stripeTopUpPriceConfig, unitAmount int64, successURL string, cancelURL string) (*stripe.CheckoutSession, error) {
	if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
		return nil, fmt.Errorf("无效的Stripe API密钥")
	}

	stripe.Key = setting.StripeApiSecret

	// Use custom URLs if provided, otherwise use defaults
	if successURL == "" {
		successURL = paymentReturnPath("/usage-logs")
	}
	if cancelURL == "" {
		cancelURL = paymentReturnPath("/wallet")
	}

	params := newStripeTopUpCheckoutParams(referenceId, customerId, email, priceConfig, unitAmount, successURL, cancelURL)
	result, err := session.New(params)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func newStripeTopUpCheckoutParams(referenceId string, customerId string, email string, priceConfig stripeTopUpPriceConfig, unitAmount int64, successURL string, cancelURL string) *stripe.CheckoutSessionParams {
	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(successURL),
		CancelURL:         stripe.String(cancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency:   stripe.String(string(priceConfig.currency)),
					Product:    stripe.String(priceConfig.productID),
					UnitAmount: stripe.Int64(unitAmount),
				},
				Quantity: stripe.Int64(1),
			},
		},
		Mode:                stripe.String(string(stripe.CheckoutSessionModePayment)),
		AllowPromotionCodes: stripe.Bool(setting.StripePromotionCodesEnabled),
	}

	if "" == customerId {
		if "" != email {
			params.CustomerEmail = stripe.String(email)
		}

		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}

	return params
}

func getStripeTopUpPriceConfig() (stripeTopUpPriceConfig, error) {
	priceID := strings.TrimSpace(setting.StripePriceId)
	apiSecret := strings.TrimSpace(setting.StripeApiSecret)
	stripeTopUpPriceCache.RLock()
	cached := stripeTopUpPriceCache.config
	cachedAPISecret := stripeTopUpPriceCache.apiSecret
	stripeTopUpPriceCache.RUnlock()
	if cached.priceID == priceID && cachedAPISecret == apiSecret && cached.productID != "" {
		return cached, nil
	}

	stripe.Key = apiSecret
	configuredPrice, err := stripeprice.Get(priceID, nil)
	if err != nil {
		return stripeTopUpPriceConfig{}, err
	}
	if configuredPrice.Deleted || configuredPrice.Type != stripe.PriceTypeOneTime {
		return stripeTopUpPriceConfig{}, errors.New("Stripe Price 必须是一次性价格")
	}
	if configuredPrice.Product == nil || strings.TrimSpace(configuredPrice.Product.ID) == "" {
		return stripeTopUpPriceConfig{}, errors.New("Stripe Price 缺少产品")
	}
	currency := stripe.Currency(strings.ToLower(strings.TrimSpace(string(configuredPrice.Currency))))
	if !isStripeCurrencyCode(currency) {
		return stripeTopUpPriceConfig{}, errors.New("Stripe Price 货币无效")
	}

	config := stripeTopUpPriceConfig{
		priceID:   priceID,
		productID: configuredPrice.Product.ID,
		currency:  currency,
	}
	stripeTopUpPriceCache.Lock()
	stripeTopUpPriceCache.apiSecret = apiSecret
	stripeTopUpPriceCache.config = config
	stripeTopUpPriceCache.Unlock()
	return config, nil
}

func calculateStripeTopUpQuote(amount int64, group string, currency stripe.Currency) (stripeTopUpQuote, error) {
	if err := validateStripeTopUpAmount(amount); err != nil {
		return stripeTopUpQuote{}, err
	}

	creditAmount := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		creditAmount = creditAmount.Div(decimal.NewFromFloat(common.QuotaPerUnit))
		if !creditAmount.Equal(creditAmount.Truncate(0)) {
			return stripeTopUpQuote{}, errors.New("Token 充值数量必须对应完整的美元额度")
		}
	}
	if err := validatePromisedTopUpQuota(creditAmount.IntPart(), amount); err != nil {
		return stripeTopUpQuote{}, err
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}
	if topupGroupRatio < 0 || math.IsNaN(topupGroupRatio) || math.IsInf(topupGroupRatio, 0) {
		return stripeTopUpQuote{}, errors.New("充值分组倍率无效")
	}
	if setting.StripeUnitPrice <= 0 || math.IsNaN(setting.StripeUnitPrice) || math.IsInf(setting.StripeUnitPrice, 0) {
		return stripeTopUpQuote{}, errors.New("Stripe 单价无效")
	}

	discount := operation_setting.GetTopupDiscountMultiplier(amount)

	minorUnit := stripeCurrencyMinorUnit(currency)
	if minorUnit == 0 {
		return stripeTopUpQuote{}, errors.New("Stripe 货币无效")
	}
	paymentAmount := creditAmount.
		Mul(decimal.NewFromFloat(setting.StripeUnitPrice)).
		Mul(decimal.NewFromFloat(topupGroupRatio)).
		Mul(decimal.NewFromFloat(discount))
	if stripeCurrencyRequiresWholeAmount(currency) {
		paymentAmount = paymentAmount.Round(0)
	}
	roundedUnitAmount := paymentAmount.Mul(decimal.NewFromInt(minorUnit)).Round(0)
	if roundedUnitAmount.LessThan(decimal.NewFromInt(1)) {
		return stripeTopUpQuote{}, errors.New("Stripe 充值金额过低")
	}
	if roundedUnitAmount.GreaterThan(decimal.NewFromInt(maxStripeUnitAmount)) {
		return stripeTopUpQuote{}, errors.New("Stripe 充值金额过高")
	}
	unitAmount := roundedUnitAmount.IntPart()

	money := decimal.NewFromInt(unitAmount).Div(decimal.NewFromInt(minorUnit))
	return stripeTopUpQuote{
		creditAmount: creditAmount.IntPart(),
		money:        money.InexactFloat64(),
		unitAmount:   unitAmount,
		currency:     currency,
	}, nil
}

func getStripeMinTopup() int64 {
	return getConfiguredMinTopUp(setting.StripeMinTopUp)
}

func getStripeMaxTopup() int64 {
	maxTopup := decimal.NewFromInt(maxStripeTopUpUSD)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		maxTopup = maxTopup.Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	return maxTopup.IntPart()
}

func validateStripeTopUpAmount(amount int64) error {
	if amount < getStripeMinTopup() {
		return fmt.Errorf("充值数量不能小于 %d", getStripeMinTopup())
	}
	if amount > getStripeMaxTopup() {
		return fmt.Errorf("充值数量不能大于 %d", getStripeMaxTopup())
	}
	return nil
}

func formatStripeMoney(quote stripeTopUpQuote) string {
	minorUnit := stripeCurrencyMinorUnit(quote.currency)
	digits := int32(2)
	if minorUnit == 1 || stripeCurrencyRequiresWholeAmount(quote.currency) {
		digits = 0
	}
	return decimal.NewFromInt(quote.unitAmount).Div(decimal.NewFromInt(minorUnit)).StringFixed(digits)
}

func isStripeCurrencyCode(currency stripe.Currency) bool {
	code := string(currency)
	if len(code) != 3 {
		return false
	}
	for _, char := range code {
		if char < 'a' || char > 'z' {
			return false
		}
	}
	return true
}

func stripeCurrencyMinorUnit(currency stripe.Currency) int64 {
	if !isStripeCurrencyCode(currency) {
		return 0
	}
	switch currency {
	case "bif", "clp", "djf", "gnf", "jpy", "kmf", "krw", "mga", "pyg", "rwf", "vnd", "vuv", "xaf", "xof", "xpf":
		return 1
	default:
		return 100
	}
}

func stripeCurrencyRequiresWholeAmount(currency stripe.Currency) bool {
	return currency == "isk" || currency == "ugx"
}
