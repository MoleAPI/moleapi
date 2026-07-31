package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const (
	nowPaymentsAPIBase           = "https://api.nowpayments.io"
	nowPaymentsSandboxAPIBase    = "https://api-sandbox.nowpayments.io"
	nowPaymentsInvoicePath       = "/v1/invoice"
	nowPaymentsResponseBodyLimit = 64 << 10
)

var nowPaymentsHTTPClient = &http.Client{Timeout: 10 * time.Second}

type nowPaymentsPayRequest struct {
	Amount int64 `json:"amount"`
}

type nowPaymentsInvoiceRequest struct {
	PriceAmount      float64 `json:"price_amount"`
	PriceCurrency    string  `json:"price_currency"`
	OrderID          string  `json:"order_id"`
	OrderDescription string  `json:"order_description,omitempty"`
	IPNCallbackURL   string  `json:"ipn_callback_url"`
	SuccessURL       string  `json:"success_url,omitempty"`
	CancelURL        string  `json:"cancel_url,omitempty"`
}

type nowPaymentsInvoiceResponse struct {
	ID            json.RawMessage `json:"id"`
	InvoiceURL    string          `json:"invoice_url"`
	OrderID       string          `json:"order_id"`
	PriceAmount   json.RawMessage `json:"price_amount"`
	PriceCurrency string          `json:"price_currency"`
}

type nowPaymentsIPN struct {
	PaymentID       json.RawMessage `json:"payment_id"`
	PaymentStatus   string          `json:"payment_status"`
	PriceAmount     json.RawMessage `json:"price_amount"`
	PriceCurrency   string          `json:"price_currency"`
	OrderID         string          `json:"order_id"`
	PayCurrency     string          `json:"pay_currency"`
	ActuallyPaid    json.RawMessage `json:"actually_paid"`
	OutcomeAmount   json.RawMessage `json:"outcome_amount"`
	OutcomeCurrency string          `json:"outcome_currency"`
}

func getNowPaymentsAPIBase() string {
	if setting.NowPaymentsSandbox {
		return nowPaymentsSandboxAPIBase
	}
	return nowPaymentsAPIBase
}

func getNowPaymentsCurrency() string {
	currency := strings.ToUpper(strings.TrimSpace(setting.NowPaymentsCurrency))
	if currency == "" {
		return "USD"
	}
	return currency
}

func getNowPaymentsMinTopUp() int64 {
	return getConfiguredMinTopUp(setting.NowPaymentsMinTopUp)
}

func getNowPaymentsPayMoney(amount int64, group string) float64 {
	dAmount := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount = dAmount.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}
	discount := operation_setting.GetTopupDiscountMultiplier(amount)
	payMoney := dAmount.
		Mul(decimal.NewFromFloat(setting.NowPaymentsUnitPrice)).
		Mul(decimal.NewFromFloat(topupGroupRatio)).
		Mul(decimal.NewFromFloat(discount))
	return payMoney.InexactFloat64()
}

func nowPaymentsExpectedMode() string {
	if setting.NowPaymentsSandbox {
		return "sandbox"
	}
	return "prod"
}

func validNowPaymentsURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.Host != "" &&
		parsed.User == nil
}

func validateNowPaymentsInvoiceData(data *nowPaymentsInvoiceResponse, tradeNo string, expectedMoney string, currency string) (string, string, error) {
	if data == nil {
		return "", "", errors.New("NOWPayments invoice response missing")
	}
	invoiceID := strings.TrimSpace(common.JsonRawMessageToString(data.ID))
	if invoiceID == "" {
		return "", "", errors.New("NOWPayments invoice id missing")
	}
	invoiceURL := strings.TrimSpace(data.InvoiceURL)
	if !validNowPaymentsURL(invoiceURL) {
		return "", "", errors.New("NOWPayments invoice URL invalid")
	}
	if strings.TrimSpace(data.OrderID) != "" && strings.TrimSpace(data.OrderID) != tradeNo {
		return "", "", errors.New("NOWPayments invoice order mismatch")
	}
	if strings.TrimSpace(data.PriceCurrency) != "" && !strings.EqualFold(strings.TrimSpace(data.PriceCurrency), currency) {
		return "", "", errors.New("NOWPayments invoice currency mismatch")
	}
	if len(bytes.TrimSpace(data.PriceAmount)) > 0 {
		actual, err := decimal.NewFromString(common.JsonRawMessageToString(data.PriceAmount))
		expected, expectedErr := decimal.NewFromString(expectedMoney)
		if err != nil || expectedErr != nil || !actual.Equal(expected) {
			return "", "", errors.New("NOWPayments invoice amount mismatch")
		}
	}
	return invoiceID, invoiceURL, nil
}

func createNowPaymentsInvoice(ctx context.Context, payload nowPaymentsInvoiceRequest) (*nowPaymentsInvoiceResponse, error) {
	body, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, getNowPaymentsAPIBase()+nowPaymentsInvoicePath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-api-key", strings.TrimSpace(setting.NowPaymentsApiKey))

	response, err := nowPaymentsHTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, nowPaymentsResponseBodyLimit+1))
	if err != nil {
		return nil, err
	}
	if len(responseBody) > nowPaymentsResponseBodyLimit {
		return nil, errors.New("NOWPayments response too large")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("NOWPayments http status %d", response.StatusCode)
	}

	var invoice nowPaymentsInvoiceResponse
	if err := common.Unmarshal(responseBody, &invoice); err != nil {
		return nil, errors.New("invalid NOWPayments response")
	}
	return &invoice, nil
}

func RequestNowPaymentsAmount(c *gin.Context) {
	if !isNowPaymentsTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "NOWPayments 支付未启用"})
		return
	}
	var req nowPaymentsPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount < getNowPaymentsMinTopUp() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getNowPaymentsMinTopUp())})
		return
	}
	if _, err := normalizeTopUpAmount(req.Amount); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}

	group := c.GetString("group")
	if group == "" {
		var err error
		group, err = model.GetUserGroup(c.GetInt("id"), true)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
			return
		}
	}
	payMoney := getNowPaymentsPayMoney(req.Amount, group)
	if payMoney <= 0.01 || math.IsNaN(payMoney) || math.IsInf(payMoney, 0) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": decimal.NewFromFloat(payMoney).StringFixed(2)})
}

func RequestNowPaymentsPay(c *gin.Context) {
	if !isNowPaymentsTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "NOWPayments 支付未启用"})
		return
	}
	var req nowPaymentsPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount < getNowPaymentsMinTopUp() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getNowPaymentsMinTopUp())})
		return
	}
	amount, err := normalizeTopUpAmount(req.Amount)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}

	userID := c.GetInt("id")
	group := c.GetString("group")
	if group == "" {
		var err error
		group, err = model.GetUserGroup(userID, true)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
			return
		}
	}
	payMoney := getNowPaymentsPayMoney(req.Amount, group)
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

	tradeNo, err := model.NewTopUpTradeNo(model.PaymentProviderNowPayments, userID)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments 生成充值订单号失败 user_id=%d error=%q", userID, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	currency := getNowPaymentsCurrency()
	topUp := &model.TopUp{
		UserId:          userID,
		Amount:          amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMode:     nowPaymentsExpectedMode(),
		PaymentCurrency: currency,
		PaymentMethod:   model.PaymentMethodNowPayments,
		PaymentProvider: model.PaymentProviderNowPayments,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments 创建充值订单失败 user_id=%d trade_no=%s amount=%d error=%q", userID, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	notifyURL := common.BuildURL(service.GetCallbackAddress(), "/api/nowpayments/webhook")
	if !validNowPaymentsURL(notifyURL) {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderNowPayments, common.TopUpStatusFailed)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "回调地址未配置"})
		return
	}
	successURL := paymentReturnPath("/wallet?show_history=true")
	if !validNowPaymentsURL(successURL) {
		successURL = ""
	}
	cancelURL := paymentReturnPath("/wallet")
	if !validNowPaymentsURL(cancelURL) {
		cancelURL = ""
	}

	invoice, err := createNowPaymentsInvoice(c.Request.Context(), nowPaymentsInvoiceRequest{
		PriceAmount:      payMoney,
		PriceCurrency:    strings.ToLower(currency),
		OrderID:          tradeNo,
		OrderDescription: fmt.Sprintf("Wallet top-up %d", req.Amount),
		IPNCallbackURL:   notifyURL,
		SuccessURL:       successURL,
		CancelURL:        cancelURL,
	})
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderNowPayments, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments 创建发票失败 user_id=%d trade_no=%s error=%q", userID, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	invoiceID, invoiceURL, err := validateNowPaymentsInvoiceData(invoice, tradeNo, payMoneyText, currency)
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderNowPayments, common.TopUpStatusFailed)
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("NOWPayments 发票响应不匹配 user_id=%d trade_no=%s error=%q", userID, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	if err := model.BindPendingTopUpPaymentFacts(tradeNo, model.PaymentProviderNowPayments, "", currency, invoiceID, topUp.PaymentMode); err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderNowPayments, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments 保存发票信息失败 user_id=%d trade_no=%s error=%q", userID, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("NOWPayments 充值发票创建成功 user_id=%d trade_no=%s invoice_id=%s amount=%d money=%.2f currency=%s", userID, tradeNo, invoiceID, req.Amount, payMoney, currency))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"checkout_url": invoiceURL,
			"order_id":     tradeNo,
		},
	})
}

func nowPaymentsSignaturePayload(body []byte) ([]byte, error) {
	var payload any
	if err := common.UnmarshalUseNumber(body, &payload); err != nil {
		return nil, err
	}
	return common.MarshalNoEscape(payload)
}

func signNowPaymentsPayload(body []byte, secret string) (string, error) {
	payload, err := nowPaymentsSignaturePayload(body)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha512.New, []byte(strings.TrimSpace(secret)))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func verifyNowPaymentsSignature(body []byte, received string, secret string) bool {
	if strings.TrimSpace(received) == "" || strings.TrimSpace(secret) == "" {
		return false
	}
	expected, err := signNowPaymentsPayload(body, secret)
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(expected), []byte(strings.ToLower(strings.TrimSpace(received))))
}

func classifyNowPaymentsStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "finished":
		return common.TopUpStatusSuccess
	case "failed", "refunded":
		return common.TopUpStatusFailed
	case "expired":
		return common.TopUpStatusExpired
	default:
		return common.TopUpStatusPending
	}
}

func validateNowPaymentsPaymentCallback(topUp *model.TopUp, payload *nowPaymentsIPN) (string, string, error) {
	if topUp == nil || payload == nil || topUp.PaymentProvider != model.PaymentProviderNowPayments {
		return "", "", errors.New("payment order mismatch")
	}
	if strings.TrimSpace(payload.OrderID) == "" || strings.TrimSpace(payload.OrderID) != topUp.TradeNo {
		return "", "", errors.New("payment order mismatch")
	}

	paymentID := strings.TrimSpace(common.JsonRawMessageToString(payload.PaymentID))
	if paymentID == "" {
		return "", "", errors.New("payment id missing")
	}
	if topUp.GatewayTradeNo != "" && topUp.GatewayTradeNo != paymentID {
		return "", "", errors.New("payment id mismatch")
	}

	currency := strings.ToUpper(strings.TrimSpace(payload.PriceCurrency))
	expectedCurrency := strings.ToUpper(strings.TrimSpace(topUp.PaymentCurrency))
	if currency == "" || expectedCurrency == "" || currency != expectedCurrency {
		return "", "", errors.New("payment currency mismatch")
	}

	actualAmount, amountErr := decimal.NewFromString(common.JsonRawMessageToString(payload.PriceAmount))
	expectedAmount, expectedErr := decimal.NewFromString(decimal.NewFromFloat(topUp.Money).StringFixed(2))
	if amountErr != nil || expectedErr != nil || !actualAmount.Equal(expectedAmount) {
		return "", "", errors.New("payment amount mismatch")
	}
	return currency, paymentID, nil
}

func markNowPaymentsOrderStatus(tradeNo string, status string) error {
	err := model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderNowPayments, status)
	if err == nil || errors.Is(err, model.ErrTopUpNotFound) || errors.Is(err, model.ErrTopUpStatusInvalid) || errors.Is(err, model.ErrPaymentMethodMismatch) {
		return nil
	}
	return err
}

func NowPaymentsWebhook(c *gin.Context) {
	if !isNowPaymentsWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("NOWPayments webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, nowPaymentsResponseBodyLimit+1))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if len(body) > nowPaymentsResponseBodyLimit {
		c.AbortWithStatus(http.StatusRequestEntityTooLarge)
		return
	}
	signature := c.GetHeader("x-nowpayments-sig")
	if !verifyNowPaymentsSignature(body, signature, setting.NowPaymentsIPNSecret) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("NOWPayments webhook 验签失败 path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	var payload nowPaymentsIPN
	if err := common.Unmarshal(body, &payload); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments webhook 解析失败 path=%q client_ip=%s error=%q", c.Request.URL.Path, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	tradeNo := strings.TrimSpace(payload.OrderID)
	status := classifyNowPaymentsStatus(payload.PaymentStatus)
	if status == common.TopUpStatusPending {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("NOWPayments 订单仍在处理中 trade_no=%s payment_status=%s client_ip=%s", tradeNo, payload.PaymentStatus, c.ClientIP()))
		c.String(http.StatusOK, "OK")
		return
	}

	topUp := model.GetTopUpByTradeNo(tradeNo)
	currency, paymentID, err := validateNowPaymentsPaymentCallback(topUp, &payload)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("NOWPayments 支付事实不匹配 trade_no=%s payment_status=%s client_ip=%s error=%q", tradeNo, payload.PaymentStatus, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	if status == common.TopUpStatusFailed || status == common.TopUpStatusExpired {
		if err := markNowPaymentsOrderStatus(tradeNo, status); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments 标记订单状态失败 trade_no=%s status=%s error=%q", tradeNo, status, err.Error()))
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("NOWPayments 订单结束但未入账 trade_no=%s payment_status=%s client_ip=%s", tradeNo, payload.PaymentStatus, c.ClientIP()))
		c.String(http.StatusOK, "OK")
		return
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	if err := model.RechargeNowPaymentsWithPaymentDetails(tradeNo, paymentID, currency, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments 充值处理失败 trade_no=%s payment_id=%s client_ip=%s error=%q", tradeNo, paymentID, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("NOWPayments 充值成功 trade_no=%s payment_id=%s client_ip=%s", tradeNo, paymentID, c.ClientIP()))
	c.String(http.StatusOK, "OK")
}
