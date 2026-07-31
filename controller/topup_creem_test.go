package controller

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCreemRejectsRecurringOrderBeforeSubscriptionCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/creem/webhook", nil)
	event := &CreemWebhookEvent{}
	event.Object.RequestId = "subscription-order"
	event.Object.Order.Id = "creem-recurring-order"
	event.Object.Order.Status = "paid"
	event.Object.Order.Type = "recurring"

	handleCheckoutCompleted(context, event)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestCreemTestModeStillRequiresWebhookSecret(t *testing.T) {
	originalTestMode := setting.CreemTestMode
	t.Cleanup(func() { setting.CreemTestMode = originalTestMode })
	setting.CreemTestMode = true
	assert.False(t, verifyCreemSignature(`{"id":"event"}`, "signature", ""))
}

func TestValidateCreemPaymentFactsUsesOriginalProductAmount(t *testing.T) {
	event := &CreemWebhookEvent{}
	event.Object.Id = "ch_123"
	event.Object.RequestId = "MO1TCR00000000000000000000000000"
	event.Object.Status = "completed"
	event.Object.Mode = "test"
	event.Object.Units = 1
	event.Object.CustomPrice = 1000
	event.Object.Order.Id = "ord_123"
	event.Object.Order.Product = "prod_123"
	event.Object.Order.Amount = 1000
	event.Object.Order.SubTotal = 1000
	event.Object.Order.TaxAmount = 230
	event.Object.Order.AmountPaid = 1230
	event.Object.Order.Currency = "USD"
	event.Object.Order.Status = "paid"
	event.Object.Order.Type = "onetime"
	event.Object.Order.Mode = "test"
	event.Object.Product.Id = "prod_123"
	event.Object.Product.Price = 1000
	event.Object.Product.Currency = "USD"
	event.Object.Product.Mode = "test"

	require.NoError(t, validateCreemPaymentFacts("ch_123", "prod_123", "USD", "test", 10, event))

	tests := []struct {
		name   string
		mutate func(*CreemWebhookEvent)
	}{
		{name: "product", mutate: func(event *CreemWebhookEvent) { event.Object.Order.Product = "prod_other" }},
		{name: "checkout", mutate: func(event *CreemWebhookEvent) { event.Object.Id = "ch_other" }},
		{name: "currency", mutate: func(event *CreemWebhookEvent) { event.Object.Order.Currency = "EUR" }},
		{name: "original amount", mutate: func(event *CreemWebhookEvent) { event.Object.Order.Amount = 999 }},
		{name: "custom price", mutate: func(event *CreemWebhookEvent) { event.Object.CustomPrice = 999 }},
		{name: "mode", mutate: func(event *CreemWebhookEvent) { event.Object.Order.Mode = "prod" }},
		{name: "units", mutate: func(event *CreemWebhookEvent) { event.Object.Units = 2 }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			copyEvent := *event
			testCase.mutate(&copyEvent)
			require.Error(t, validateCreemPaymentFacts("ch_123", "prod_123", "USD", "test", 10, &copyEvent))
		})
	}

	event.Object.Product.Price = 999
	require.NoError(t, validateCreemPaymentFacts("ch_123", "prod_123", "USD", "test", 10, event))
}

func TestValidateCreemPaymentFactsBindsLegacyOrderAmount(t *testing.T) {
	event := &CreemWebhookEvent{}
	event.Object.Id = "ch_legacy"
	event.Object.Status = "completed"
	event.Object.Mode = creemExpectedMode()
	event.Object.Units = 1
	event.Object.Order.Id = "ord_legacy"
	event.Object.Order.Product = "prod_legacy"
	event.Object.Order.Amount = 1000
	event.Object.Order.Currency = "USD"
	event.Object.Order.Status = "paid"
	event.Object.Order.Type = "onetime"
	event.Object.Order.Mode = creemExpectedMode()
	event.Object.Product.Id = "prod_legacy"
	event.Object.Product.Price = 1000
	event.Object.Product.Currency = "USD"

	require.NoError(t, validateCreemPaymentFacts("", "", "", "", 10, event))

	event.Object.Order.Amount = 100
	event.Object.Product.Price = 100
	require.ErrorContains(t, validateCreemPaymentFacts("", "", "", "", 10, event), "amount")
}

func TestValidateCreemPaymentFactsAcceptsOfficialWebhookShape(t *testing.T) {
	event := &CreemWebhookEvent{}
	event.Object.Id = "ch_official"
	event.Object.Status = "completed"
	event.Object.Mode = "test"
	event.Object.Order.Id = "ord_official"
	event.Object.Order.Product = "prod_official"
	event.Object.Order.Amount = 1000
	event.Object.Order.Currency = "USD"
	event.Object.Order.Status = "paid"
	event.Object.Order.Type = "onetime"
	event.Object.Order.Mode = "test"
	event.Object.Product.Id = "prod_official"
	event.Object.Product.Price = 725
	event.Object.Product.Currency = "USD"
	event.Object.Product.Mode = "test"
	// Creem's official checkout.completed fixture omits units and custom_price.
	require.Zero(t, event.Object.Units)
	require.Zero(t, event.Object.CustomPrice)

	require.NoError(t, validateCreemPaymentFacts("ch_official", "prod_official", "USD", "test", 10, event))
}

func TestHandleCreemCheckoutRetriesDatabaseLookupFailures(t *testing.T) {
	tests := []struct {
		name       string
		modelFails func(any) bool
	}{
		{name: "subscription lookup", modelFails: func(value any) bool { _, ok := value.(*model.SubscriptionOrder); return ok }},
		{name: "topup lookup", modelFails: func(value any) bool { _, ok := value.(*model.TopUp); return ok }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			db := setupTopUpWebhookSettlementTest(t)
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("creem_lookup_failure", func(tx *gorm.DB) {
				if testCase.modelFails(tx.Statement.Model) {
					tx.AddError(errors.New("database unavailable"))
				}
			}))

			event := &CreemWebhookEvent{}
			event.Object.RequestId = "MO1TCR00000000000000000000000000"
			event.Object.Id = "ch_123"
			event.Object.Status = "completed"
			event.Object.Order.Id = "ord_123"
			event.Object.Order.Status = "paid"
			event.Object.Order.Type = "onetime"

			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/creem/webhook", nil)
			handleCheckoutCompleted(context, event)

			assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		})
	}
}

func TestCreemWebhookRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalSecret := setting.CreemWebhookSecret
	t.Cleanup(func() { setting.CreemWebhookSecret = originalSecret })
	setting.CreemWebhookSecret = "webhook-secret"

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/creem/webhook", strings.NewReader(strings.Repeat("x", creemWebhookBodyLimit+1)))
	CreemWebhook(context)

	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}

type creemRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper creemRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func setupCreemCheckoutTest(t *testing.T) {
	t.Helper()
	originalApiKey := setting.CreemApiKey
	originalTestMode := setting.CreemTestMode
	originalHTTPClient := creemHTTPClient
	setting.CreemApiKey = "creem_test_key"
	setting.CreemTestMode = true
	t.Cleanup(func() {
		setting.CreemApiKey = originalApiKey
		setting.CreemTestMode = originalTestMode
		creemHTTPClient = originalHTTPClient
	})
}

func creemHTTPResponse(t *testing.T, value any) *http.Response {
	t.Helper()
	payload, err := common.Marshal(value)
	require.NoError(t, err)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(payload))),
	}
}

func TestGenCreemLinkPinsConfiguredAmount(t *testing.T) {
	setupCreemCheckoutTest(t)
	requests := 0
	creemHTTPClient = &http.Client{Timeout: time.Second, Transport: creemRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests++
		assert.Equal(t, "creem_test_key", request.Header.Get("x-api-key"))
		switch request.Method + " " + request.URL.Path {
		case http.MethodGet + " /v1/products":
			assert.Equal(t, "prod_123", request.URL.Query().Get("product_id"))
			return creemHTTPResponse(t, CreemRemoteProduct{
				Id: "prod_123", Mode: "test", Price: 725, Currency: "USD", BillingType: "onetime", Status: "active",
			}), nil
		case http.MethodPost + " /v1/checkouts":
			var checkoutRequest CreemCheckoutRequest
			require.NoError(t, common.DecodeJson(request.Body, &checkoutRequest))
			assert.Equal(t, "prod_123", checkoutRequest.ProductId)
			assert.Equal(t, "MO1SCR00000000000000000000000000", checkoutRequest.RequestId)
			assert.Equal(t, 1, checkoutRequest.Units)
			assert.Equal(t, int64(1000), checkoutRequest.CustomPrice)
			assert.Empty(t, checkoutRequest.Metadata)
			return creemHTTPResponse(t, map[string]any{
				"id": "checkout_123", "mode": "test", "status": "pending",
				"product": map[string]string{"id": "prod_123"}, "request_id": checkoutRequest.RequestId,
				"units": 1, "custom_price": 1000, "checkout_url": "https://checkout.creem.io/test",
			}), nil
		default:
			require.Failf(t, "unexpected Creem request", "%s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	checkout, err := genCreemLink(t.Context(), "MO1SCR00000000000000000000000000", &CreemProduct{
		ProductId: "prod_123", Name: "Local subscription", Price: 10, Currency: "USD",
	}, "buyer@example.com")
	require.NoError(t, err)
	assert.Equal(t, "checkout_123", checkout.Id)
	assert.Equal(t, 2, requests)
}

func TestGenCreemLinkRejectsRemoteProductDrift(t *testing.T) {
	setupCreemCheckoutTest(t)
	tests := []struct {
		name   string
		mutate func(*CreemRemoteProduct)
	}{
		{name: "product", mutate: func(product *CreemRemoteProduct) { product.Id = "prod_other" }},
		{name: "mode", mutate: func(product *CreemRemoteProduct) { product.Mode = "prod" }},
		{name: "currency", mutate: func(product *CreemRemoteProduct) { product.Currency = "EUR" }},
		{name: "billing type", mutate: func(product *CreemRemoteProduct) { product.BillingType = "recurring" }},
		{name: "status", mutate: func(product *CreemRemoteProduct) { product.Status = "archived" }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			remoteProduct := CreemRemoteProduct{
				Id: "prod_123", Mode: "test", Price: 1000, Currency: "USD", BillingType: "onetime", Status: "active",
			}
			testCase.mutate(&remoteProduct)
			requests := 0
			creemHTTPClient = &http.Client{Timeout: time.Second, Transport: creemRoundTripper(func(request *http.Request) (*http.Response, error) {
				requests++
				assert.Equal(t, http.MethodGet, request.Method)
				return creemHTTPResponse(t, remoteProduct), nil
			})}

			_, err := genCreemLink(t.Context(), "MO1TCR00000000000000000000000000", &CreemProduct{
				ProductId: "prod_123", Name: "Local topup", Price: 10, Currency: "USD", Quota: 100,
			}, "buyer@example.com")
			require.ErrorContains(t, err, "product facts mismatch")
			assert.Equal(t, 1, requests)
		})
	}
}

func TestGenCreemLinkRejectsCheckoutBindingMismatch(t *testing.T) {
	setupCreemCheckoutTest(t)
	creemHTTPClient = &http.Client{Timeout: time.Second, Transport: creemRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodGet {
			return creemHTTPResponse(t, CreemRemoteProduct{
				Id: "prod_123", Mode: "test", Price: 1000, Currency: "USD", BillingType: "onetime", Status: "active",
			}), nil
		}
		return creemHTTPResponse(t, map[string]any{
			"id": "checkout_123", "mode": "test", "product": "prod_123", "request_id": "another-order",
			"units": 1, "custom_price": 1000, "checkout_url": "https://checkout.creem.io/test",
		}), nil
	})}

	_, err := genCreemLink(t.Context(), "MO1TCR00000000000000000000000000", &CreemProduct{
		ProductId: "prod_123", Name: "Local topup", Price: 10, Currency: "USD", Quota: 100,
	}, "buyer@example.com")
	require.ErrorContains(t, err, "checkout facts mismatch")
}
