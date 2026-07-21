package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const (
	lanTuAPIBase           = "https://api.ltzf.cn"
	lanTuNativePath        = "/api/wxpay/native"
	lanTuJumpH5Path        = "/api/wxpay/jump_h5"
	lanTuQueryPath         = "/api/wxpay/get_pay_order"
	lanTuQueryInterval     = 5 * time.Second
	lanTuResponseBodyLimit = 64 << 10
)

var (
	lanTuHTTPClient = &http.Client{Timeout: 10 * time.Second}
	// ponytail: process-local per-order gates avoid cross-user interference;
	// move them to Redis if multiple nodes actively perform LanTu recovery queries.
	lanTuQueryMu   sync.Mutex
	lanTuLastQuery = map[string]time.Time{}
)

type lanTuConfig struct {
	mchID     string
	secretKey string
}

type lanTuPayRequest struct {
	Amount int64  `json:"amount"`
	Client string `json:"client"`
}

type lanTuAPIResponse struct {
	Code      int             `json:"code"`
	Data      json.RawMessage `json:"data"`
	Message   string          `json:"msg"`
	RequestID string          `json:"request_id"`
}

type lanTuNativeData struct {
	CodeURL   string `json:"code_url"`
	QRCodeURL string `json:"QRcode_url"`
}

type lanTuQueryResponse struct {
	Code      int            `json:"code"`
	Data      lanTuQueryData `json:"data"`
	Message   string         `json:"msg"`
	RequestID string         `json:"request_id"`
}

type lanTuQueryData struct {
	MchID      string `json:"mch_id"`
	OutTradeNo string `json:"out_trade_no"`
	PayNo      string `json:"pay_no"`
	TotalFee   string `json:"total_fee"`
	PayStatus  int    `json:"pay_status"`
}

func getLanTuConfig() *lanTuConfig {
	mchID := strings.TrimSpace(setting.LantuMchID)
	secretKey := strings.TrimSpace(setting.LantuSecretKey)
	if mchID == "" || secretKey == "" {
		return nil
	}
	return &lanTuConfig{mchID: mchID, secretKey: secretKey}
}

func normalizeLanTuClient(userAgent string, requested string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "native":
		return "native", nil
	case "h5":
		return "h5", nil
	case "":
		mobileAgent := strings.ToLower(userAgent)
		if strings.Contains(mobileAgent, "mobile") || strings.Contains(mobileAgent, "android") ||
			strings.Contains(mobileAgent, "iphone") || strings.Contains(mobileAgent, "ipad") ||
			strings.Contains(mobileAgent, "micromessenger") {
			return "h5", nil
		}
		return "native", nil
	default:
		return "", errors.New("unsupported client")
	}
}

func RequestLanTuPay(c *gin.Context) {
	if !isLanTuTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "蓝兔支付未启用"})
		return
	}
	config := getLanTuConfig()
	if config == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "蓝兔支付未启用"})
		return
	}

	var req lanTuPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount < getLanTuMinTopUp() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	amount, err := normalizeTopUpAmount(req.Amount)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	client, err := normalizeLanTuClient(c.Request.UserAgent(), req.Client)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	userID := c.GetInt("id")
	group := c.GetString("group")
	if group == "" {
		group, err = model.GetUserGroup(userID, true)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
			return
		}
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 || math.IsNaN(payMoney) || math.IsInf(payMoney, 0) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额无效"})
		return
	}
	payMoneyText := decimal.NewFromFloat(payMoney).StringFixed(2)
	payMoney, _ = decimal.RequireFromString(payMoneyText).Float64()

	tradeNo, err := model.NewTopUpTradeNo(model.PaymentProviderLanTu, userID)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("蓝兔支付生成订单号失败 user_id=%d error=%q", userID, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	topUp := &model.TopUp{
		UserId:          userID,
		Amount:          amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMode:     client,
		PaymentCurrency: "CNY",
		PaymentMethod:   model.PaymentMethodLanTu,
		PaymentProvider: model.PaymentProviderLanTu,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("蓝兔支付创建本地订单失败 user_id=%d trade_no=%s error=%q", userID, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	callbackURL := common.BuildURL(service.GetCallbackAddress(), "/api/user/lantu/notify")
	returnURL := paymentReturnPath("/wallet")
	signedParams := map[string]string{
		"mch_id":       config.mchID,
		"out_trade_no": tradeNo,
		"total_fee":    payMoneyText,
		"body":         "Wallet top-up",
		"timestamp":    strconv.FormatInt(time.Now().Unix(), 10),
		"notify_url":   callbackURL,
	}
	form := url.Values{}
	for key, value := range signedParams {
		form.Set(key, value)
	}
	form.Set("sign", common.GenerateLanTuSignature(signedParams, config.secretKey))
	form.Set("time_expire", "5m")
	path := lanTuNativePath
	if client == "h5" {
		path = lanTuJumpH5Path
		form.Set("return_url", returnURL)
		form.Set("quit_url", returnURL)
	}

	payLink, payLinkKind, requestID, err := createLanTuOrder(c.Request.Context(), path, form)
	if err != nil {
		if updateErr := model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderLanTu, common.TopUpStatusFailed); updateErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("蓝兔支付标记订单失败状态失败 trade_no=%s error=%q", tradeNo, updateErr.Error()))
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("蓝兔支付上游下单失败 trade_no=%s request_id=%s error=%q", tradeNo, requestID, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link":      payLink,
			"pay_link_kind": payLinkKind,
			"pay_money":     payMoneyText,
			"trade_no":      tradeNo,
			"client":        client,
		},
	})
}

func getLanTuMinTopUp() int64 {
	return getConfiguredMinTopUp(setting.LantuMinTopUp)
}

func createLanTuOrder(ctx context.Context, path string, form url.Values) (string, string, string, error) {
	var response lanTuAPIResponse
	if err := lanTuPostForm(ctx, path, form, &response); err != nil {
		return "", "", "", err
	}
	if response.Code != 0 {
		return "", "", response.RequestID, errors.New("upstream rejected order")
	}

	if path == lanTuJumpH5Path {
		var payLink string
		if err := common.Unmarshal(response.Data, &payLink); err != nil || !validLanTuPayLink(payLink, false) {
			return "", "", response.RequestID, errors.New("invalid upstream payment link")
		}
		return payLink, "url", response.RequestID, nil
	}

	var data lanTuNativeData
	if err := common.Unmarshal(response.Data, &data); err != nil {
		return "", "", response.RequestID, errors.New("invalid upstream payment response")
	}
	if validLanTuPayLink(data.QRCodeURL, false) {
		return data.QRCodeURL, "qr_image", response.RequestID, nil
	}
	if validLanTuPayLink(data.CodeURL, true) {
		return data.CodeURL, "qr_text", response.RequestID, nil
	}
	return "", "", response.RequestID, errors.New("invalid upstream payment link")
}

func validLanTuPayLink(raw string, allowWeChat bool) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if allowWeChat && parsed.Scheme == "weixin" {
		return true
	}
	return parsed.Scheme == "https" && parsed.Host != ""
}

func lanTuPostForm(ctx context.Context, path string, form url.Values, result any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, lanTuAPIBase+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := lanTuHTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("upstream http status %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, lanTuResponseBodyLimit+1))
	if err != nil {
		return err
	}
	if len(body) > lanTuResponseBodyLimit {
		return errors.New("upstream response too large")
	}
	if err := common.Unmarshal(body, result); err != nil {
		return errors.New("invalid upstream response")
	}
	return nil
}

func LanTuPayNotify(c *gin.Context) {
	config := getLanTuConfig()
	if config == nil {
		c.String(http.StatusOK, "FAIL")
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		c.String(http.StatusOK, "FAIL")
		return
	}

	form := c.Request.PostForm
	outTradeNo := strings.TrimSpace(form.Get("out_trade_no"))
	signedParams := map[string]string{
		"code":         strings.TrimSpace(form.Get("code")),
		"timestamp":    strings.TrimSpace(form.Get("timestamp")),
		"mch_id":       strings.TrimSpace(form.Get("mch_id")),
		"order_no":     strings.TrimSpace(form.Get("order_no")),
		"out_trade_no": outTradeNo,
		"pay_no":       strings.TrimSpace(form.Get("pay_no")),
		"total_fee":    strings.TrimSpace(form.Get("total_fee")),
	}
	for _, value := range signedParams {
		if value == "" {
			c.String(http.StatusOK, "FAIL")
			return
		}
	}
	if signedParams["code"] != "0" || signedParams["mch_id"] != config.mchID ||
		!common.VerifyLanTuSignature(signedParams, form.Get("sign"), config.secretKey) {
		c.String(http.StatusOK, "FAIL")
		return
	}
	if payChannel := strings.TrimSpace(form.Get("pay_channel")); payChannel != "" && payChannel != "wxpay" {
		c.String(http.StatusOK, "FAIL")
		return
	}

	LockOrder(outTradeNo)
	defer UnlockOrder(outTradeNo)
	topUp := model.GetTopUpByTradeNo(outTradeNo)
	if !validLanTuTopUp(topUp) || !lanTuMoneyMatches(topUp.Money, signedParams["total_fee"]) {
		c.String(http.StatusOK, "FAIL")
		return
	}
	if topUp.Status != common.TopUpStatusPending {
		forgetLanTuQuery(outTradeNo)
		c.String(http.StatusOK, "SUCCESS")
		return
	}
	if !reserveLanTuQuery(outTradeNo, time.Now()) {
		c.String(http.StatusOK, "FAIL")
		return
	}

	order, err := queryLanTuOrder(c.Request.Context(), config, topUp)
	if err != nil || order.PayStatus != 1 {
		reason := "payment not confirmed"
		if err != nil {
			reason = err.Error()
		}
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("蓝兔支付回调确认失败 trade_no=%s error=%q", outTradeNo, reason))
		c.String(http.StatusOK, "FAIL")
		return
	}
	gatewayTradeNo := order.PayNo
	if gatewayTradeNo == "" {
		gatewayTradeNo = signedParams["pay_no"]
	}
	if err := model.RechargeLanTuWithPaymentDetails(outTradeNo, gatewayTradeNo, "CNY", c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("蓝兔支付结算失败 trade_no=%s error=%q", outTradeNo, err.Error()))
		c.String(http.StatusOK, "FAIL")
		return
	}
	forgetLanTuQuery(outTradeNo)
	c.String(http.StatusOK, "SUCCESS")
}

func GetLanTuOrderStatus(c *gin.Context) {
	tradeNo := strings.TrimSpace(c.Query("trade_no"))
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if tradeNo == "" || !validLanTuTopUp(topUp) ||
		(topUp.UserId != c.GetInt("id") && c.GetInt("role") < common.RoleAdminUser) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "订单不存在"})
		return
	}

	if topUp.Status == common.TopUpStatusPending && reserveLanTuQuery(tradeNo, time.Now()) {
		LockOrder(tradeNo)
		topUp = model.GetTopUpByTradeNo(tradeNo)
		config := getLanTuConfig()
		if config != nil && topUp != nil && topUp.Status == common.TopUpStatusPending {
			order, err := queryLanTuOrder(c.Request.Context(), config, topUp)
			if err == nil && order.PayStatus == 1 {
				if settleErr := model.RechargeLanTuWithPaymentDetails(tradeNo, order.PayNo, "CNY", c.ClientIP()); settleErr != nil {
					logger.LogError(c.Request.Context(), fmt.Sprintf("蓝兔支付查询恢复结算失败 trade_no=%s error=%q", tradeNo, settleErr.Error()))
				}
			}
		}
		UnlockOrder(tradeNo)
		topUp = model.GetTopUpByTradeNo(tradeNo)
	}
	if topUp != nil && topUp.Status != common.TopUpStatusPending {
		forgetLanTuQuery(tradeNo)
	}

	status := common.TopUpStatusPending
	if topUp != nil {
		status = topUp.Status
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    gin.H{"trade_no": tradeNo, "status": status},
	})
}

func validLanTuTopUp(topUp *model.TopUp) bool {
	return topUp != nil && (topUp.PaymentProvider == model.PaymentProviderLanTu ||
		(topUp.PaymentProvider == "" && topUp.PaymentMethod == model.PaymentMethodLanTu))
}

func lanTuMoneyMatches(expected float64, actual string) bool {
	expectedAmount, expectedErr := decimal.NewFromString(strconv.FormatFloat(expected, 'f', 2, 64))
	actualAmount, actualErr := decimal.NewFromString(actual)
	return expectedErr == nil && actualErr == nil && expectedAmount.Equal(actualAmount)
}

func queryLanTuOrder(ctx context.Context, config *lanTuConfig, topUp *model.TopUp) (*lanTuQueryData, error) {
	signedParams := map[string]string{
		"mch_id":       config.mchID,
		"out_trade_no": topUp.TradeNo,
		"timestamp":    strconv.FormatInt(time.Now().Unix(), 10),
	}
	form := url.Values{}
	for key, value := range signedParams {
		form.Set(key, value)
	}
	form.Set("sign", common.GenerateLanTuSignature(signedParams, config.secretKey))

	var response lanTuQueryResponse
	if err := lanTuPostForm(ctx, lanTuQueryPath, form, &response); err != nil {
		return nil, err
	}
	if response.Code != 0 || response.Data.MchID != config.mchID ||
		response.Data.OutTradeNo != topUp.TradeNo || !lanTuMoneyMatches(topUp.Money, response.Data.TotalFee) ||
		(response.Data.PayStatus != 0 && response.Data.PayStatus != 1) {
		return nil, errors.New("upstream order verification failed")
	}
	return &response.Data, nil
}

func reserveLanTuQuery(tradeNo string, now time.Time) bool {
	lanTuQueryMu.Lock()
	defer lanTuQueryMu.Unlock()
	for order, lastQuery := range lanTuLastQuery {
		if !now.Before(lastQuery.Add(lanTuQueryInterval)) {
			delete(lanTuLastQuery, order)
		}
	}
	if _, reserved := lanTuLastQuery[tradeNo]; reserved {
		return false
	}
	lanTuLastQuery[tradeNo] = now
	return true
}

func forgetLanTuQuery(tradeNo string) {
	lanTuQueryMu.Lock()
	delete(lanTuLastQuery, tradeNo)
	lanTuQueryMu.Unlock()
}
