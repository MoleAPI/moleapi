package controller

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type SubscriptionCreemPayRequest struct {
	PlanId int `json:"plan_id"`
}

func SubscriptionRequestCreemPay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionCreemPayRequest

	// Keep body for debugging consistency (like RequestCreemPay)
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Creem failed to read subscription payment request error=%q", err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "read query error"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		paymentError(c, common.NewMessage("Invalid parameters"))
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	if !plan.Enabled {
		common.ApiErrorT(c, "Subscription plan is not enabled")
		return
	}
	if plan.CreemProductId == "" {
		common.ApiErrorT(c, "CreemProductId is not configured for this plan")
		return
	}
	if plan.PriceAmount <= 0 || math.IsNaN(plan.PriceAmount) || math.IsInf(plan.PriceAmount, 0) || strings.TrimSpace(plan.Currency) == "" {
		common.ApiErrorT(c, "Invalid plan payment information")
		return
	}
	if setting.CreemWebhookSecret == "" && !setting.CreemTestMode {
		common.ApiErrorT(c, "Creem webhook is not configured")
		return
	}

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	if user == nil {
		common.ApiErrorT(c, "User does not exist")
		return
	}

	if plan.MaxPurchasePerUser > 0 {
		count, err := model.CountUserSubscriptionsByPlan(userId, plan.Id)
		if err != nil {
			common.ApiErrorI18n(c, i18n.MsgDatabaseError)
			return
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			common.ApiErrorT(c, "Purchase limit for this plan has been reached")
			return
		}
	}

	referenceId, err := model.NewSubscriptionTradeNo(model.PaymentProviderCreem, userId)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Creem failed to create subscription order number user_id=%d plan_id=%d error=%q", userId, plan.Id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	// create pending order first
	order := &model.SubscriptionOrder{
		UserId:           userId,
		PlanId:           plan.Id,
		Money:            plan.PriceAmount,
		TradeNo:          referenceId,
		PaymentProductId: plan.CreemProductId,
		PaymentMode:      creemExpectedMode(),
		PaymentCurrency:  strings.ToUpper(strings.TrimSpace(plan.Currency)),
		PaymentMethod:    model.PaymentMethodCreem,
		PaymentProvider:  model.PaymentProviderCreem,
		CreateTime:       time.Now().Unix(),
		Status:           common.TopUpStatusPending,
	}
	if err := order.SetPlanSnapshot(plan); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	if err := order.Insert(); err != nil {
		paymentError(c, common.NewMessage("Failed to create order"))
		return
	}

	// Reuse Creem checkout generator by building a lightweight product reference.
	product := &CreemProduct{
		ProductId: plan.CreemProductId,
		Name:      plan.Title,
		Price:     plan.PriceAmount,
		Currency:  strings.ToUpper(strings.TrimSpace(plan.Currency)),
		Quota:     0,
	}

	checkout, err := genCreemLink(c.Request.Context(), referenceId, product, user.Email)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Creem failed to create subscription payment link trade_no=%s product_id=%s error=%q", referenceId, product.ProductId, err.Error()))
		_ = model.UpdatePendingSubscriptionOrderStatus(referenceId, model.PaymentProviderCreem, common.TopUpStatusFailed)
		paymentError(c, common.NewMessage("Failed to start payment"))
		return
	}
	if err := model.BindPendingSubscriptionOrderPaymentFacts(referenceId, model.PaymentProviderCreem, checkout.Id, order.PaymentCurrency, order.PaymentProductId, order.PaymentMode, order.Money); err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Creem failed to save subscription checkout details trade_no=%s checkout_id=%s error=%q", referenceId, checkout.Id, err.Error()))
		_ = model.UpdatePendingSubscriptionOrderStatus(referenceId, model.PaymentProviderCreem, common.TopUpStatusFailed)
		paymentError(c, common.NewMessage("Failed to start payment"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"checkout_url": checkout.CheckoutUrl,
			"order_id":     referenceId,
		},
	})
}
