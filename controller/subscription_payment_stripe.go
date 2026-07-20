package controller

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
)

type SubscriptionStripePayRequest struct {
	PlanId int `json:"plan_id"`
}

func SubscriptionRequestStripePay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionStripePayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	if plan.StripePriceId == "" {
		common.ApiErrorMsg(c, "该套餐未配置 StripePriceId")
		return
	}
	if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
		common.ApiErrorMsg(c, "Stripe 未配置或密钥无效")
		return
	}
	if setting.StripeWebhookSecret == "" {
		common.ApiErrorMsg(c, "Stripe Webhook 未配置")
		return
	}

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}

	if plan.MaxPurchasePerUser > 0 {
		count, err := model.CountUserSubscriptionsByPlan(userId, plan.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			common.ApiErrorMsg(c, "已达到该套餐购买上限")
			return
		}
	}

	referenceId, err := model.NewSubscriptionTradeNo(model.PaymentProviderStripe)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 生成订阅订单号失败 user_id=%d plan_id=%d error=%q", userId, plan.Id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	order := &model.SubscriptionOrder{
		UserId:           userId,
		PlanId:           plan.Id,
		Money:            plan.PriceAmount,
		TradeNo:          referenceId,
		PaymentProductId: plan.StripePriceId,
		PaymentMode:      stripePaymentMode,
		PaymentMethod:    model.PaymentMethodStripe,
		PaymentProvider:  model.PaymentProviderStripe,
		CreateTime:       time.Now().Unix(),
		Status:           common.TopUpStatusPending,
	}
	if err := order.SetPlanSnapshot(plan); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	if err := order.Insert(); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	checkoutSession, err := genStripeSubscriptionLink(referenceId, user.StripeCustomer, user.Email, plan.StripePriceId)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 订阅支付链接创建失败 trade_no=%s plan_id=%d error=%q", referenceId, plan.Id, err.Error()))
		_ = model.UpdatePendingSubscriptionOrderStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusFailed)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	minorUnit := stripeCurrencyMinorUnit(checkoutSession.Currency)
	if checkoutSession.ID == "" || checkoutSession.ClientReferenceID != referenceId || minorUnit <= 0 || checkoutSession.AmountSubtotal <= 0 {
		_ = model.UpdatePendingSubscriptionOrderStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 订阅 Checkout Session 事实不完整 trade_no=%s plan_id=%d", referenceId, plan.Id))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	paymentCurrency := strings.ToUpper(string(checkoutSession.Currency))
	paymentMoney := decimal.NewFromInt(checkoutSession.AmountSubtotal).Div(decimal.NewFromInt(minorUnit)).InexactFloat64()
	if err := model.BindPendingSubscriptionOrderPaymentFacts(referenceId, model.PaymentProviderStripe, checkoutSession.ID, paymentCurrency, plan.StripePriceId, stripePaymentMode, paymentMoney); err != nil {
		_ = model.UpdatePendingSubscriptionOrderStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 保存订阅 Checkout Session 失败 trade_no=%s plan_id=%d error=%q", referenceId, plan.Id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": checkoutSession.URL,
		},
	})
}

func genStripeSubscriptionLink(referenceId string, customerId string, email string, priceId string) (*stripe.CheckoutSession, error) {
	stripe.Key = setting.StripeApiSecret

	params := newStripeSubscriptionCheckoutParams(referenceId, customerId, email, priceId)
	result, err := session.New(params)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func newStripeSubscriptionCheckoutParams(referenceId string, customerId string, email string, priceId string) *stripe.CheckoutSessionParams {
	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(paymentReturnPath("/wallet")),
		CancelURL:         stripe.String(paymentReturnPath("/wallet")),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceId),
				Quantity: stripe.Int64(1),
			},
		},
		Mode: stripe.String(string(stripe.CheckoutSessionModePayment)),
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
