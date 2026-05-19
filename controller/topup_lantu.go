package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const (
	lanTuDefaultApiBase = "https://api.ltzf.cn"
	lanTuNativePath     = "/api/wxpay/native"
	lanTuJumpH5Path     = "/api/wxpay/jump_h5"
	lanTuGetOrderPath   = "/api/wxpay/get_pay_order"
	lanTuFailCode       = 1 // 查询订单失败状态码（legacy 约定）
	lanTuUnpaidCode     = 0 // 未支付状态码（legacy 约定）
	lanTuOrderTTL       = 300
	lanTuDefaultBody    = "充值"
	lanTuPaymentMethod  = "lantu"
)

type LanTuConfig struct {
	MchId     string
	SecretKey string
	ApiBase   string
}

func GetLanTuPayConfig() *LanTuConfig {
	if common.LantuMchId == "" || common.LantuSecretKey == "" {
		return nil
	}
	return &LanTuConfig{
		MchId:     common.LantuMchId,
		SecretKey: common.LantuSecretKey,
		// LanTu gateway base is fixed by the upstream platform.
		// Do not let admins paste their own callback domain here by accident.
		ApiBase: lanTuDefaultApiBase,
	}
}

type LanTuPayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
	// client: "h5" (mobile) or "native" (desktop qr). If empty, server will auto-detect by User-Agent.
	Client string `json:"client"`
}

func isMobileUserAgent(ua string) bool {
	ua = strings.ToLower(ua)
	// Keep this conservative: if unsure, treat as desktop (native qrcode) to avoid jump_h5 desktop error page.
	if strings.Contains(ua, "mobile") {
		return true
	}
	if strings.Contains(ua, "android") || strings.Contains(ua, "iphone") || strings.Contains(ua, "ipod") {
		return true
	}
	if strings.Contains(ua, "ipad") {
		return true
	}
	if strings.Contains(ua, "windows phone") {
		return true
	}
	// WeChat in-app browser should use H5 jump mode.
	if strings.Contains(ua, "micromessenger") {
		return true
	}
	return false
}

func normalizeLanTuClient(c *gin.Context, v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "h5":
		return "h5"
	case "native":
		return "native"
	default:
		if c != nil && c.Request != nil && isMobileUserAgent(c.Request.UserAgent()) {
			return "h5"
		}
		return "native"
	}
}

// RequestLanTuPay creates a LanTu WeChat payment:
// - desktop: native qrcode (scan)
// - mobile: H5 jump
func RequestLanTuPay(c *gin.Context) {
	var req LanTuPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}

	cfg := GetLanTuPayConfig()
	if cfg == nil {
		c.JSON(200, gin.H{"message": "error", "data": "当前管理员未配置蓝兔支付信息"})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	// trade no aligns with Epay formatting, so logs/search feel consistent.
	tradeNo, err := model.GenerateUniqueLanTuTradeNo(id)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	// notify url should be reachable by LanTu platform
	callBackAddress := service.GetCallbackAddress()
	notifyUrl := common.BuildURL(callBackAddress, "/api/user/lantu/notify")

	referer := c.GetHeader("Referer")
	if referer == "" {
		referer = system_setting.ServerAddress
	}

	body := fmt.Sprintf("%s%s %s", common.SystemName, lanTuDefaultBody, strconv.FormatFloat(float64(req.Amount), 'f', 0, 64))

	// Sign only required fields (legacy behavior).
	paramsToSign := map[string]string{
		"mch_id":       cfg.MchId,
		"out_trade_no": tradeNo,
		"total_fee":    strconv.FormatFloat(payMoney, 'f', 2, 64),
		"body":         body,
		"timestamp":    strconv.FormatInt(time.Now().Unix(), 10),
		"notify_url":   notifyUrl,
	}
	sign := common.GenerateSignature(paramsToSign, cfg.SecretKey)

	params := map[string]string{
		"mch_id":       paramsToSign["mch_id"],
		"out_trade_no": paramsToSign["out_trade_no"],
		"total_fee":    paramsToSign["total_fee"],
		"body":         paramsToSign["body"],
		"timestamp":    paramsToSign["timestamp"],
		"notify_url":   paramsToSign["notify_url"],
		"sign":         sign,
	}

	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(int64(amount))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   lanTuPaymentMethod,
		PaymentProvider: model.PaymentProviderLanTu,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	client := normalizeLanTuClient(c, req.Client)
	endpointPath := lanTuNativePath
	if client == "h5" {
		// H5 supports return/quit urls (optional by doc).
		params["quit_url"] = referer
		params["return_url"] = referer
		endpointPath = lanTuJumpH5Path
	}

	payLink, payLinkKind, reqID, err := lanTuDoPay(c, params, common.BuildURL(cfg.ApiBase, endpointPath))
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link":      payLink,
			"pay_link_kind": payLinkKind, // "qr_image" | "qr_text" | "url"
			// Helpful for QR modal verification; aligns with upstream total_fee (2 decimals).
			"pay_money": paramsToSign["total_fee"],
			"trade_no":  tradeNo,
			// Helps debugging upstream issues (safe to ignore on frontend).
			"request_id": reqID,
			// "native" -> QR code URL; "h5" -> jump URL.
			"client": client,
		},
	})
}

func LanTuPayNotify(c *gin.Context) {
	// Accept GET for simple health check
	outTradeNo := c.PostForm("out_trade_no")
	mchId := c.PostForm("mch_id")
	if outTradeNo == "" && mchId == "" && c.Request.Method == "GET" {
		c.String(http.StatusOK, "LANTU_CALLBACK_OK")
		return
	}

	if outTradeNo == "" {
		_ = c.Request.ParseForm()
		outTradeNo = c.PostForm("out_trade_no")
		mchId = c.PostForm("mch_id")
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("LanTu webhook 收到请求 path=%q client_ip=%s out_trade_no=%s mch_id=%s", c.Request.RequestURI, c.ClientIP(), outTradeNo, mchId))

	cfg := GetLanTuPayConfig()
	if cfg == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("LanTu webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.String(http.StatusOK, "FAIL")
		return
	}
	// Some callbacks may omit mch_id; fall back to configured value.
	if mchId == "" {
		mchId = cfg.MchId
	}
	if outTradeNo == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("LanTu webhook 缺少订单号 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.String(http.StatusOK, "FAIL")
		return
	}

	// Prevent double-credit on repeated callbacks.
	LockOrder(outTradeNo)
	defer UnlockOrder(outTradeNo)

	topUp := model.GetTopUpByTradeNo(outTradeNo)
	if topUp == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("LanTu webhook 未找到本地订单 trade_no=%s client_ip=%s", outTradeNo, c.ClientIP()))
		c.String(http.StatusOK, "FAIL")
		return
	}
	if topUp.PaymentProvider != model.PaymentProviderLanTu {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("LanTu webhook 订单支付网关不匹配 trade_no=%s payment_provider=%s client_ip=%s", outTradeNo, topUp.PaymentProvider, c.ClientIP()))
		c.String(http.StatusOK, "SUCCESS")
		return
	}

	// Already processed.
	if topUp.Status != common.TopUpStatusPending {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("LanTu webhook 订单已处理，忽略重复回调 trade_no=%s status=%s client_ip=%s", outTradeNo, topUp.Status, c.ClientIP()))
		c.String(http.StatusOK, "SUCCESS")
		return
	}

	// Expire old orders.
	if time.Now().Unix()-topUp.CreateTime > lanTuOrderTTL {
		topUp.Status = common.TopUpStatusExpired
		_ = topUp.Update()
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("LanTu webhook 订单已过期 trade_no=%s client_ip=%s", outTradeNo, c.ClientIP()))
		c.String(http.StatusOK, "SUCCESS")
		return
	}

	paid, gatewayTradeNo, err := lanTuCheckPaid(cfg, mchId, outTradeNo)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("LanTu webhook 查询上游订单失败 trade_no=%s mch_id=%s client_ip=%s error=%q", outTradeNo, mchId, c.ClientIP(), err.Error()))
		c.String(http.StatusOK, "FAIL")
		return
	}
	if !paid {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("LanTu webhook 上游订单未支付 trade_no=%s mch_id=%s client_ip=%s", outTradeNo, mchId, c.ClientIP()))
		c.String(http.StatusOK, "FAIL")
		return
	}

	if gatewayTradeNo != "" {
		topUp.GatewayTradeNo = gatewayTradeNo
	}
	topUp.Status = common.TopUpStatusSuccess
	topUp.CompleteTime = common.GetTimestamp()
	if err := topUp.Update(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("LanTu webhook 更新订单状态失败 trade_no=%s client_ip=%s error=%q", outTradeNo, c.ClientIP(), err.Error()))
		c.String(http.StatusOK, "FAIL")
		return
	}

	dAmount := decimal.NewFromInt(int64(topUp.Amount))
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	bonusRate := operation_setting.GetTopupBonusRate(topUp.Amount)
	dBonusMultiplier := decimal.NewFromFloat(1.0 + bonusRate)
	quotaToAdd := int(dAmount.Mul(dQuotaPerUnit).Mul(dBonusMultiplier).IntPart())
	if err := model.IncreaseUserQuota(topUp.UserId, quotaToAdd, true); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("LanTu webhook 更新用户额度失败 trade_no=%s user_id=%d client_ip=%s error=%q", outTradeNo, topUp.UserId, c.ClientIP(), err.Error()))
		c.String(http.StatusOK, "FAIL")
		return
	}
	inviterId, inviterRewardQuota, inviterRewardGranted, rewardErr := model.RewardInviterOnFirstTopup(topUp.UserId)
	if rewardErr != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("LanTu webhook 发放邀请首充奖励失败 trade_no=%s user_id=%d client_ip=%s error=%q", outTradeNo, topUp.UserId, c.ClientIP(), rewardErr.Error()))
	} else if inviterRewardGranted {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("LanTu webhook 发放邀请首充奖励成功 trade_no=%s inviter_id=%d reward=%d client_ip=%s", outTradeNo, inviterId, inviterRewardQuota, c.ClientIP()))
	}

	model.RecordTopupLog(topUp.UserId, fmt.Sprintf("使用蓝兔支付充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaToAdd), topUp.Money), c.ClientIP(), topUp.PaymentMethod, model.PaymentProviderLanTu)
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("LanTu webhook 充值成功 trade_no=%s gateway_trade_no=%s user_id=%d quota=%d money=%.2f client_ip=%s", outTradeNo, gatewayTradeNo, topUp.UserId, quotaToAdd, topUp.Money, c.ClientIP()))
	c.String(http.StatusOK, "SUCCESS")
}

// GetLanTuOrderStatus returns the local order status for frontend polling.
// It does not call LanTu upstream; LanTuPayNotify is responsible for verification/crediting.
func GetLanTuOrderStatus(c *gin.Context) {
	tradeNo := strings.TrimSpace(c.Query("trade_no"))
	if tradeNo == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "trade_no is required"})
		return
	}

	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "order not found"})
		return
	}
	// Ensure users can only query their own orders.
	if uid := c.GetInt("id"); uid > 0 && topUp.UserId != uid {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "forbidden"})
		return
	}

	// Expire old orders.
	if topUp.Status == common.TopUpStatusPending && time.Now().Unix()-topUp.CreateTime > lanTuOrderTTL {
		topUp.Status = common.TopUpStatusExpired
		_ = topUp.Update()
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"status":  topUp.Status,
	})
}

func lanTuDoPay(ctx *gin.Context, params map[string]string, endpoint string) (payLink string, payLinkKind string, requestID string, err error) {
	formData := make([]string, 0, len(params))
	for k, v := range params {
		formData = append(formData, k+"="+url.QueryEscape(v))
	}
	payload := strings.NewReader(strings.Join(formData, "&"))

	req, err := http.NewRequest("POST", endpoint, payload)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("LanTu pay build request failed: endpoint=%s err=%v", endpoint, err))
		return "", "", "", errors.New("调起支付失败")
	}
	req.Header.Add("content-type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", "", errors.New("调起支付失败")
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	var result map[string]interface{}
	if err := common.Unmarshal(body, &result); err != nil {
		logger.LogError(ctx, fmt.Sprintf("LanTu pay response json parse failed: endpoint=%s status=%s body=%q err=%v", endpoint, res.Status, string(body), err))
		return "", "", "", errors.New("支付响应解析失败")
	}

	code := strings.TrimSpace(fmt.Sprintf("%v", result["code"]))
	msg := strings.TrimSpace(fmt.Sprintf("%v", result["msg"]))
	if msg == "" || msg == "<nil>" {
		// Non-standard gateways may use message/error instead of msg.
		if v, ok := result["message"]; ok && v != nil {
			msg = strings.TrimSpace(fmt.Sprintf("%v", v))
		} else if v, ok := result["error"]; ok && v != nil {
			msg = strings.TrimSpace(fmt.Sprintf("%v", v))
		}
	}
	reqID := strings.TrimSpace(fmt.Sprintf("%v", result["request_id"]))
	if reqID == "<nil>" {
		reqID = ""
	}

	// Spec: code 0 success, 1 failure. If code is missing/unrecognized, treat as failure (and log raw body).
	if code != "0" {
		if msg == "" || msg == "<nil>" {
			msg = "创建支付失败"
		}
		if code == "" || code == "<nil>" {
			logger.LogError(ctx, fmt.Sprintf("LanTu pay response missing/unrecognized code: endpoint=%s status=%s body=%q", endpoint, res.Status, string(body)))
		}
		if reqID != "" {
			msg = fmt.Sprintf("%s (request_id=%s)", msg, reqID)
		}
		return "", "", reqID, errors.New(msg)
	}

	dataVal, ok := result["data"]
	if !ok || dataVal == nil {
		logger.LogError(ctx, fmt.Sprintf("LanTu pay response missing data: endpoint=%s status=%s body=%q", endpoint, res.Status, string(body)))
		if reqID != "" {
			return "", "", reqID, errors.New("支付响应缺少 data (request_id=" + reqID + ")")
		}
		return "", "", reqID, errors.New("支付响应缺少 data")
	}

	var link string
	var linkKey string
	switch v := dataVal.(type) {
	case string:
		link = strings.TrimSpace(v)
		linkKey = "data"
	case map[string]interface{}:
		// Some deployments return object payloads (e.g. order_url / QRcode_url); prefer a URL-like field.
		candidates := []string{"QRcode_url", "qrcode_url", "order_url", "pay_url", "url", "h5_url", "jump_url", "code_url"}
		for _, k := range candidates {
			if vv, ok := v[k]; ok && vv != nil {
				s := strings.TrimSpace(fmt.Sprintf("%v", vv))
				if s != "" && s != "<nil>" {
					link = s
					linkKey = k
					break
				}
			}
		}
	default:
		link = strings.TrimSpace(fmt.Sprintf("%v", dataVal))
		linkKey = "data"
	}
	if link == "" {
		logger.LogError(ctx, fmt.Sprintf("LanTu pay response empty data: endpoint=%s status=%s body=%q", endpoint, res.Status, string(body)))
		if reqID != "" {
			return "", "", reqID, errors.New("支付链接为空 (request_id=" + reqID + ")")
		}
		return "", "", reqID, errors.New("支付链接为空")
	}

	kind := "url"
	switch linkKey {
	case "QRcode_url", "qrcode_url":
		kind = "qr_image"
	case "code_url":
		kind = "qr_text"
	default:
		if strings.HasPrefix(strings.ToLower(link), "weixin://") {
			kind = "qr_text"
		}
	}

	return link, kind, reqID, nil
}

func lanTuCheckPaid(cfg *LanTuConfig, mchId, outTradeNo string) (bool, string, error) {
	params := map[string]string{
		"mch_id":       mchId,
		"out_trade_no": outTradeNo,
		"timestamp":    strconv.FormatInt(time.Now().Unix(), 10),
	}
	params["sign"] = common.GenerateSignature(params, cfg.SecretKey)

	formData := fmt.Sprintf("mch_id=%s&out_trade_no=%s&timestamp=%s&sign=%s",
		url.QueryEscape(params["mch_id"]),
		url.QueryEscape(params["out_trade_no"]),
		url.QueryEscape(params["timestamp"]),
		url.QueryEscape(params["sign"]),
	)
	payload := strings.NewReader(formData)

	endpointUrl := common.BuildURL(cfg.ApiBase, lanTuGetOrderPath)
	req, err := http.NewRequest("POST", endpointUrl, payload)
	if err != nil {
		return false, "", err
	}
	req.Header.Add("content-type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, "", err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	var result map[string]interface{}
	if err := common.Unmarshal(body, &result); err != nil {
		return false, "", err
	}

	if result["code"] == nil {
		return false, "", errors.New("订单查询返回数据异常")
	}
	code := strings.TrimSpace(fmt.Sprintf("%v", result["code"]))
	if code == "1" {
		msg := strings.TrimSpace(fmt.Sprintf("%v", result["msg"]))
		if msg == "" || msg == "<nil>" {
			msg = "订单查询失败"
		}
		return false, "", errors.New(msg)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok || data == nil {
		return false, "", errors.New("订单查询响应格式错误")
	}

	payStatusFloat, ok := data["pay_status"].(float64)
	if !ok {
		return false, "", errors.New("支付状态字段格式错误")
	}
	payStatus := int(payStatusFloat)
	return payStatus != lanTuUnpaidCode, getFirstNonEmptyString(data, "trade_no", "transaction_id", "pay_order_id", "order_id"), nil
}

func getFirstNonEmptyString(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok && value != nil {
			str := strings.TrimSpace(fmt.Sprintf("%v", value))
			if str != "" && str != "<nil>" {
				return str
			}
		}
	}
	return ""
}
