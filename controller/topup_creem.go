package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"gorm.io/gorm"
)

const CreemSignatureHeader = "creem-signature"

const creemWebhookBodyLimit = 1 << 20

var creemAdaptor = &CreemAdaptor{}

var creemHTTPClient = &http.Client{Timeout: 30 * time.Second}

// 生成HMAC-SHA256签名
func generateCreemSignature(payload string, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// 验证Creem webhook签名
func verifyCreemSignature(payload string, signature string, secret string) bool {
	if secret == "" {
		return false
	}

	expectedSignature := generateCreemSignature(payload, secret)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

type CreemPayRequest struct {
	ProductId     string `json:"product_id"`
	PaymentMethod string `json:"payment_method"`
}

type CreemProduct struct {
	ProductId string  `json:"productId"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	Currency  string  `json:"currency"`
	Quota     int64   `json:"quota"`
}

type CreemAdaptor struct {
}

func creemExpectedMode() string {
	if setting.CreemTestMode {
		return "test"
	}
	return "prod"
}

func (*CreemAdaptor) RequestPay(c *gin.Context, req *CreemPayRequest) {
	if !isCreemTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Creem 支付未启用"})
		return
	}

	if req.PaymentMethod != model.PaymentMethodCreem {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}

	if req.ProductId == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "请选择产品"})
		return
	}

	// 解析产品列表
	var products []CreemProduct
	err := common.Unmarshal([]byte(setting.CreemProducts), &products)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 产品配置解析失败 user_id=%d error=%q", c.GetInt("id"), err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "产品配置错误"})
		return
	}

	// 查找对应的产品
	var selectedProduct *CreemProduct
	for _, product := range products {
		if product.ProductId == req.ProductId {
			selectedProduct = &product
			break
		}
	}

	if selectedProduct == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "产品不存在"})
		return
	}
	if selectedProduct.Price <= 0 || math.IsNaN(selectedProduct.Price) || math.IsInf(selectedProduct.Price, 0) ||
		selectedProduct.Quota <= 0 || selectedProduct.Quota > int64(common.MaxQuota) || strings.TrimSpace(selectedProduct.Currency) == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "产品配置错误"})
		return
	}

	id := c.GetInt("id")
	user, err := model.GetUserById(id, false)
	if err != nil || user == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "用户不存在"})
		return
	}

	referenceId, err := model.NewTopUpTradeNo(model.PaymentProviderCreem, id)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 生成充值订单号失败 user_id=%d error=%q", id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	// 先创建订单记录，使用产品配置的金额和充值额度
	topUp := &model.TopUp{
		UserId:           id,
		Amount:           selectedProduct.Quota, // 充值额度
		Money:            selectedProduct.Price, // 支付金额
		TradeNo:          referenceId,
		PaymentProductId: selectedProduct.ProductId,
		PaymentMode:      creemExpectedMode(),
		PaymentCurrency:  strings.ToUpper(strings.TrimSpace(selectedProduct.Currency)),
		PaymentMethod:    model.PaymentMethodCreem,
		PaymentProvider:  model.PaymentProviderCreem,
		CreateTime:       time.Now().Unix(),
		Status:           common.TopUpStatusPending,
	}
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 创建充值订单失败 user_id=%d trade_no=%s product_id=%s error=%q", id, referenceId, selectedProduct.ProductId, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	// 创建支付链接，传入用户邮箱
	checkout, err := genCreemLink(c.Request.Context(), referenceId, selectedProduct, user.Email)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 创建支付链接失败 user_id=%d trade_no=%s product_id=%s error=%q", id, referenceId, selectedProduct.ProductId, err.Error()))
		_ = model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderCreem, common.TopUpStatusFailed)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	if err := model.BindPendingTopUpPaymentFacts(referenceId, model.PaymentProviderCreem, checkout.Id, topUp.PaymentCurrency, topUp.PaymentProductId, topUp.PaymentMode); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 固化充值订单响应失败 user_id=%d trade_no=%s checkout_id=%s error=%q", id, referenceId, checkout.Id, err.Error()))
		_ = model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderCreem, common.TopUpStatusFailed)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 充值订单创建成功 user_id=%d trade_no=%s product_id=%s product_name=%q quota=%d money=%.2f", id, referenceId, selectedProduct.ProductId, selectedProduct.Name, selectedProduct.Quota, selectedProduct.Price))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"checkout_url": checkout.CheckoutUrl,
			"order_id":     referenceId,
		},
	})
}

func RequestCreemPay(c *gin.Context) {
	var req CreemPayRequest

	// 读取body内容用于打印，同时保留原始数据供后续使用
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 支付请求读取失败 error=%q", err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "read query error"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 支付请求已收到 user_id=%d body_size=%d", c.GetInt("id"), len(bodyBytes)))

	// 重新设置body供后续的ShouldBindJSON使用
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	err = c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	creemAdaptor.RequestPay(c, &req)
}

// 新的Creem Webhook结构体，匹配实际的webhook数据格式
type CreemWebhookEvent struct {
	Id        string `json:"id"`
	EventType string `json:"eventType"`
	CreatedAt int64  `json:"created_at"`
	Object    struct {
		Id        string `json:"id"`
		Object    string `json:"object"`
		RequestId string `json:"request_id"`
		Order     struct {
			Object      string `json:"object"`
			Id          string `json:"id"`
			Customer    string `json:"customer"`
			Product     string `json:"product"`
			Amount      int64  `json:"amount"`
			Currency    string `json:"currency"`
			SubTotal    int64  `json:"sub_total"`
			TaxAmount   int64  `json:"tax_amount"`
			AmountDue   int64  `json:"amount_due"`
			AmountPaid  int64  `json:"amount_paid"`
			Status      string `json:"status"`
			Type        string `json:"type"`
			Transaction string `json:"transaction"`
			CreatedAt   string `json:"created_at"`
			UpdatedAt   string `json:"updated_at"`
			Mode        string `json:"mode"`
		} `json:"order"`
		Product struct {
			Id                string  `json:"id"`
			Object            string  `json:"object"`
			Name              string  `json:"name"`
			Description       string  `json:"description"`
			Price             int64   `json:"price"`
			Currency          string  `json:"currency"`
			BillingType       string  `json:"billing_type"`
			BillingPeriod     string  `json:"billing_period"`
			Status            string  `json:"status"`
			TaxMode           string  `json:"tax_mode"`
			TaxCategory       string  `json:"tax_category"`
			DefaultSuccessUrl *string `json:"default_success_url"`
			CreatedAt         string  `json:"created_at"`
			UpdatedAt         string  `json:"updated_at"`
			Mode              string  `json:"mode"`
		} `json:"product"`
		Units       int   `json:"units"`
		CustomPrice int64 `json:"custom_price"`
		Customer    struct {
			Id        string `json:"id"`
			Object    string `json:"object"`
			Email     string `json:"email"`
			Name      string `json:"name"`
			Country   string `json:"country"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
			Mode      string `json:"mode"`
		} `json:"customer"`
		Status   string            `json:"status"`
		Metadata map[string]string `json:"metadata"`
		Mode     string            `json:"mode"`
	} `json:"object"`
}

func CreemWebhook(c *gin.Context) {
	if !isCreemWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// 读取body内容用于打印，同时保留原始数据供后续使用
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, creemWebhookBodyLimit)
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem webhook 读取请求体失败 path=%q client_ip=%s error=%q", c.Request.URL.Path, c.ClientIP(), err.Error()))
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// 获取签名头
	signature := c.GetHeader(CreemSignatureHeader)
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem webhook 收到请求 path=%q client_ip=%s body_size=%d", c.Request.URL.Path, c.ClientIP(), len(bodyBytes)))
	if signature == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem webhook 缺少签名 path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// 验证签名
	if !verifyCreemSignature(string(bodyBytes), signature, setting.CreemWebhookSecret) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem webhook 验签失败 path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem webhook 验签成功 path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))

	// 重新设置body供后续的ShouldBindJSON使用
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// 解析新格式的webhook数据
	var webhookEvent CreemWebhookEvent
	if err := c.ShouldBindJSON(&webhookEvent); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem webhook 解析失败 path=%q client_ip=%s error=%q", c.Request.URL.Path, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem webhook 解析成功 event_type=%s event_id=%s request_id=%s order_id=%s order_status=%s", webhookEvent.EventType, webhookEvent.Id, webhookEvent.Object.RequestId, webhookEvent.Object.Order.Id, webhookEvent.Object.Order.Status))

	// 根据事件类型处理不同的webhook
	switch webhookEvent.EventType {
	case "checkout.completed":
		handleCheckoutCompleted(c, &webhookEvent)
	default:
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem webhook 忽略事件 event_type=%s event_id=%s", webhookEvent.EventType, webhookEvent.Id))
		c.Status(http.StatusOK)
	}
}

func validateCreemPaymentFacts(gatewayTradeNo string, paymentProductId string, paymentCurrency string, paymentMode string, money float64, event *CreemWebhookEvent) error {
	if event == nil || event.Object.Status != "completed" || event.Object.Order.Status != "paid" || event.Object.Order.Type != "onetime" ||
		(event.Object.Units != 0 && event.Object.Units != 1) {
		return fmt.Errorf("Creem checkout state mismatch")
	}
	if event.Object.Id == "" || event.Object.Order.Id == "" || event.Object.Order.Product == "" || event.Object.Product.Id == "" ||
		event.Object.Order.Product != event.Object.Product.Id {
		return fmt.Errorf("Creem product mismatch")
	}
	if !strings.EqualFold(event.Object.Order.Currency, event.Object.Product.Currency) {
		return fmt.Errorf("Creem currency mismatch")
	}

	legacyOrder := gatewayTradeNo == "" && paymentProductId == "" && paymentCurrency == "" && paymentMode == ""
	if legacyOrder {
		if event.Object.Mode != creemExpectedMode() || event.Object.Order.Mode != creemExpectedMode() {
			return fmt.Errorf("Creem mode mismatch")
		}
		minorUnit := stripeCurrencyMinorUnit(stripe.Currency(strings.ToLower(event.Object.Order.Currency)))
		if minorUnit <= 0 {
			return fmt.Errorf("Creem currency invalid")
		}
		expectedAmount := decimal.NewFromFloat(money).Mul(decimal.NewFromInt(minorUnit)).Round(0).IntPart()
		if expectedAmount <= 0 || event.Object.Order.Amount != expectedAmount || event.Object.Product.Price != expectedAmount {
			return fmt.Errorf("Creem product amount mismatch")
		}
		return nil
	}
	if gatewayTradeNo == "" || paymentProductId == "" || paymentCurrency == "" || paymentMode == "" {
		return fmt.Errorf("Creem local payment facts incomplete")
	}
	if event.Object.Id != gatewayTradeNo {
		return fmt.Errorf("Creem checkout mismatch")
	}
	if event.Object.Product.Id != paymentProductId || !strings.EqualFold(event.Object.Order.Currency, paymentCurrency) {
		return fmt.Errorf("Creem stored payment facts mismatch")
	}
	if event.Object.Mode != paymentMode || event.Object.Order.Mode != paymentMode ||
		(event.Object.Product.Mode != "" && event.Object.Product.Mode != paymentMode) {
		return fmt.Errorf("Creem mode mismatch")
	}
	minorUnit := stripeCurrencyMinorUnit(stripe.Currency(strings.ToLower(paymentCurrency)))
	if minorUnit <= 0 {
		return fmt.Errorf("Creem currency invalid")
	}
	expectedAmount := decimal.NewFromFloat(money).Mul(decimal.NewFromInt(minorUnit)).Round(0).IntPart()
	if expectedAmount <= 0 || (event.Object.CustomPrice != 0 && event.Object.CustomPrice != expectedAmount) || event.Object.Order.Amount != expectedAmount {
		return fmt.Errorf("Creem product amount mismatch")
	}
	return nil
}

// 处理支付完成事件
func handleCheckoutCompleted(c *gin.Context, event *CreemWebhookEvent) {
	referenceId := strings.TrimSpace(event.Object.RequestId)
	if referenceId == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem webhook 缺少 request_id event_id=%s order_id=%s", event.Id, event.Object.Order.Id))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if event.Object.Order.Type != "onetime" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 拒绝非一次性订单 trade_no=%s order_id=%s order_type=%s", referenceId, event.Object.Order.Id, event.Object.Order.Type))
		c.Status(http.StatusOK)
		return
	}
	if event.Object.Status != "completed" || event.Object.Order.Status != "paid" {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem Checkout 尚未完成支付 trade_no=%s order_id=%s checkout_status=%s order_status=%s", referenceId, event.Object.Order.Id, event.Object.Status, event.Object.Order.Status))
		c.Status(http.StatusOK)
		return
	}

	LockOrder(referenceId)
	defer UnlockOrder(referenceId)
	providerPayload := common.GetJsonString(map[string]interface{}{
		"event_id":        event.Id,
		"checkout_id":     event.Object.Id,
		"checkout_status": event.Object.Status,
		"order_id":        event.Object.Order.Id,
		"order_status":    event.Object.Order.Status,
		"product_id":      event.Object.Product.Id,
		"amount":          event.Object.Order.Amount,
		"currency":        strings.ToUpper(event.Object.Order.Currency),
		"mode":            event.Object.Order.Mode,
	})
	order, err := model.FindSubscriptionOrderByTradeNo(referenceId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 查询订阅订单失败 trade_no=%s order_id=%s error=%q", referenceId, event.Object.Order.Id, err.Error()))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if order != nil {
		if order.PaymentProvider != model.PaymentProviderCreem {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 订阅订单支付网关不匹配 trade_no=%s order_id=%s", referenceId, event.Object.Order.Id))
			c.Status(http.StatusOK)
			return
		}
		if err := validateCreemPaymentFacts(order.GatewayTradeNo, order.PaymentProductId, order.PaymentCurrency, order.PaymentMode, order.Money, event); err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 订阅支付事实不匹配 trade_no=%s order_id=%s error=%q", referenceId, event.Object.Order.Id, err.Error()))
			c.Status(http.StatusOK)
			return
		}
		if err := model.CompleteSubscriptionOrderWithPaymentDetails(referenceId, providerPayload, model.PaymentProviderCreem, "", event.Object.Id, strings.ToUpper(event.Object.Order.Currency)); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 订阅订单处理失败 trade_no=%s order_id=%s error=%q", referenceId, event.Object.Order.Id, err.Error()))
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 订阅订单处理成功 trade_no=%s order_id=%s", referenceId, event.Object.Order.Id))
		c.Status(http.StatusOK)
		return
	}

	topUp, err := model.FindTopUpByTradeNo(referenceId)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 查询充值订单失败 trade_no=%s order_id=%s error=%q", referenceId, event.Object.Order.Id, err.Error()))
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 充值订单不存在 trade_no=%s order_id=%s", referenceId, event.Object.Order.Id))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if topUp.PaymentProvider != model.PaymentProviderCreem {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 充值订单支付网关不匹配 trade_no=%s order_id=%s", referenceId, event.Object.Order.Id))
		c.Status(http.StatusOK)
		return
	}
	if err := validateCreemPaymentFacts(topUp.GatewayTradeNo, topUp.PaymentProductId, topUp.PaymentCurrency, topUp.PaymentMode, topUp.Money, event); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 充值支付事实不匹配 trade_no=%s order_id=%s error=%q", referenceId, event.Object.Order.Id, err.Error()))
		c.Status(http.StatusOK)
		return
	}

	alreadySettled := topUp.Status == common.TopUpStatusSuccess
	if err := model.RechargeCreemWithPaymentDetails(referenceId, event.Object.Customer.Email, event.Object.Customer.Name, event.Object.Id, strings.ToUpper(event.Object.Order.Currency), c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 充值处理失败 trade_no=%s order_id=%s client_ip=%s error=%q", referenceId, event.Object.Order.Id, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if !alreadySettled {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 充值成功 trade_no=%s order_id=%s quota=%d money=%.2f client_ip=%s", referenceId, event.Object.Order.Id, topUp.Amount, topUp.Money, c.ClientIP()))
	}
	c.Status(http.StatusOK)
}

type CreemCheckoutRequest struct {
	ProductId   string `json:"product_id"`
	RequestId   string `json:"request_id"`
	Units       int    `json:"units"`
	CustomPrice int64  `json:"custom_price"`
	Customer    struct {
		Email string `json:"email"`
	} `json:"customer"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type CreemCheckoutProductReference string

func (reference *CreemCheckoutProductReference) UnmarshalJSON(data []byte) error {
	var productId string
	if err := common.Unmarshal(data, &productId); err == nil {
		*reference = CreemCheckoutProductReference(productId)
		return nil
	}

	var product struct {
		Id string `json:"id"`
	}
	if err := common.Unmarshal(data, &product); err != nil {
		return err
	}
	*reference = CreemCheckoutProductReference(product.Id)
	return nil
}

type CreemCheckoutResponse struct {
	CheckoutUrl string                        `json:"checkout_url"`
	Id          string                        `json:"id"`
	Mode        string                        `json:"mode"`
	Status      string                        `json:"status"`
	Product     CreemCheckoutProductReference `json:"product"`
	ProductId   string                        `json:"product_id"`
	RequestId   string                        `json:"request_id"`
	Units       int                           `json:"units"`
	CustomPrice int64                         `json:"custom_price"`
	Order       struct {
		Id       string `json:"id"`
		Mode     string `json:"mode"`
		Product  string `json:"product"`
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
		Type     string `json:"type"`
	} `json:"order"`
}

type CreemRemoteProduct struct {
	Id          string `json:"id"`
	Mode        string `json:"mode"`
	Price       int64  `json:"price"`
	Currency    string `json:"currency"`
	BillingType string `json:"billing_type"`
	Status      string `json:"status"`
}

func creemAPIBaseURL() string {
	if setting.CreemTestMode {
		return "https://test-api.creem.io/v1"
	}
	return "https://api.creem.io/v1"
}

func creemMoneyMinorUnits(money float64, currency string) (int64, error) {
	minorUnit := stripeCurrencyMinorUnit(stripe.Currency(strings.ToLower(strings.TrimSpace(currency))))
	if minorUnit <= 0 {
		return 0, fmt.Errorf("Creem currency invalid")
	}
	amount := decimal.NewFromFloat(money).Mul(decimal.NewFromInt(minorUnit)).Round(0).IntPart()
	if amount < 100 || amount > 99999999 {
		return 0, fmt.Errorf("Creem amount out of range")
	}
	return amount, nil
}

func fetchCreemProduct(ctx context.Context, productId string) (*CreemRemoteProduct, error) {
	apiUrl := creemAPIBaseURL() + "/products?product_id=" + url.QueryEscape(productId)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("创建Creem产品请求失败: %v", err)
	}
	req.Header.Set("x-api-key", setting.CreemApiKey)

	resp, err := creemHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("查询Creem产品失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("Creem产品API http status %d", resp.StatusCode)
	}

	var product CreemRemoteProduct
	if err := common.DecodeJson(io.LimitReader(resp.Body, 1<<20), &product); err != nil {
		return nil, fmt.Errorf("解析Creem产品失败: %v", err)
	}
	return &product, nil
}

func genCreemLink(ctx context.Context, referenceId string, product *CreemProduct, email string) (*CreemCheckoutResponse, error) {
	if setting.CreemApiKey == "" {
		return nil, fmt.Errorf("未配置Creem API密钥")
	}
	if product == nil || product.ProductId == "" || referenceId == "" {
		return nil, fmt.Errorf("Creem checkout facts incomplete")
	}
	customPrice, err := creemMoneyMinorUnits(product.Price, product.Currency)
	if err != nil {
		return nil, err
	}
	remoteProduct, err := fetchCreemProduct(ctx, product.ProductId)
	if err != nil {
		return nil, err
	}
	if remoteProduct.Id != product.ProductId || remoteProduct.Mode != creemExpectedMode() ||
		!strings.EqualFold(remoteProduct.Currency, product.Currency) || remoteProduct.BillingType != "onetime" ||
		remoteProduct.Status != "active" {
		return nil, fmt.Errorf("Creem product facts mismatch")
	}

	// 根据测试模式选择 API 端点
	apiUrl := creemAPIBaseURL() + "/checkouts"
	if setting.CreemTestMode {
		logger.LogInfo(ctx, fmt.Sprintf("Creem 使用测试环境 api_url=%s", apiUrl))
	}

	// 构建请求数据，确保包含用户邮箱
	requestData := CreemCheckoutRequest{
		ProductId:   product.ProductId,
		RequestId:   referenceId, // 这个作为订单ID传递给Creem
		Units:       1,
		CustomPrice: customPrice,
		Customer: struct {
			Email string `json:"email"`
		}{
			Email: email, // 用户邮箱会在支付页面预填充
		},
	}

	// 序列化请求数据
	jsonData, err := common.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("序列化请求数据失败: %v", err)
	}

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", setting.CreemApiKey)

	logger.LogInfo(ctx, fmt.Sprintf("Creem 支付请求已发送 api_url=%s product_id=%s trade_no=%s", apiUrl, product.ProductId, referenceId))

	// 发送请求
	resp, err := creemHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	logger.LogInfo(ctx, fmt.Sprintf("Creem API 响应已收到 trade_no=%s product_id=%s status_code=%d", referenceId, product.ProductId, resp.StatusCode))

	// 检查响应状态
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("Creem API http status %d ", resp.StatusCode)
	}
	// 解析响应
	var checkoutResp CreemCheckoutResponse
	err = common.Unmarshal(body, &checkoutResp)
	if err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	checkoutUrl, err := url.Parse(checkoutResp.CheckoutUrl)
	if err != nil || checkoutUrl.Scheme != "https" || checkoutUrl.Host == "" {
		return nil, fmt.Errorf("Creem API resp no checkout url ")
	}
	responseProductId := string(checkoutResp.Product)
	if responseProductId == "" {
		responseProductId = checkoutResp.ProductId
	}
	if checkoutResp.Id == "" || (checkoutResp.Mode != "" && checkoutResp.Mode != creemExpectedMode()) ||
		responseProductId != product.ProductId || checkoutResp.RequestId != referenceId ||
		checkoutResp.Units != 1 || checkoutResp.CustomPrice != customPrice {
		return nil, fmt.Errorf("Creem checkout facts mismatch")
	}
	if checkoutResp.ProductId != "" && checkoutResp.ProductId != product.ProductId {
		return nil, fmt.Errorf("Creem checkout product mismatch")
	}
	if checkoutResp.Order.Id != "" && (checkoutResp.Order.Product != product.ProductId ||
		checkoutResp.Order.Amount != customPrice || !strings.EqualFold(checkoutResp.Order.Currency, product.Currency) ||
		(checkoutResp.Order.Mode != "" && checkoutResp.Order.Mode != creemExpectedMode()) ||
		(checkoutResp.Order.Type != "" && checkoutResp.Order.Type != "onetime")) {
		return nil, fmt.Errorf("Creem checkout order mismatch")
	}

	logger.LogInfo(ctx, fmt.Sprintf("Creem 支付链接创建成功 trade_no=%s response_id=%s", referenceId, checkoutResp.Id))
	return &checkoutResp, nil
}
