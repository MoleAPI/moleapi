package controller

import (
	"bytes"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
)

type topUpInvoiceView struct {
	SystemName      string
	InvoiceNo       string
	CustomerName    string
	CustomerEmail   string
	TradeNo         string
	GatewayTradeNo  string
	PaymentMethod   string
	PaymentProvider string
	CreditedQuota   string
	PaidAmount      string
	CreatedAt       string
	CompletedAt     string
	IssuedAt        string
}

var topUpInvoiceTemplate = template.Must(template.New("topup-invoice").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.SystemName}} Invoice {{.InvoiceNo}}</title>
  <style>
    * { box-sizing: border-box; }
    body { margin: 0; background: #f5f7fa; color: #1f2933; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; line-height: 1.5; }
    main { max-width: 800px; margin: 32px auto; padding: 40px; background: #fff; border: 1px solid #e5e8ef; border-radius: 8px; }
    header { display: flex; justify-content: space-between; gap: 24px; padding-bottom: 24px; border-bottom: 1px solid #e5e8ef; }
    h1 { margin: 0; color: #101828; font-size: 30px; }
    .brand, .label, footer { color: #667085; font-size: 12px; }
    .brand, .label { font-weight: 600; text-transform: uppercase; }
    .label { margin-bottom: 4px; }
    .actions { display: flex; justify-content: flex-end; margin-bottom: 24px; }
    button { height: 34px; padding: 0 14px; border: 1px solid #d0d5dd; border-radius: 6px; background: #fff; color: #344054; cursor: pointer; font-size: 14px; }
    button:hover { background: #f8fafc; }
    .status { align-self: flex-start; padding: 6px 10px; border-radius: 999px; background: #ecfdf3; color: #067647; font-size: 13px; font-weight: 600; }
    .grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px 32px; margin: 28px 0; }
    .value { color: #101828; font-size: 15px; overflow-wrap: anywhere; }
    .value + .label { margin-top: 12px; }
    table { width: 100%; border-collapse: collapse; }
    th, td { padding: 14px 12px; border-bottom: 1px solid #e5e8ef; text-align: left; vertical-align: top; }
    th { background: #f8fafc; color: #475467; font-size: 12px; text-transform: uppercase; }
    .paid { color: #101828; font-size: 20px; font-weight: 700; }
    footer { margin-top: 28px; }
    @media (max-width: 640px) { main { margin: 0; padding: 24px; border: 0; } header, .grid { grid-template-columns: 1fr; display: grid; } }
    @media print { body { background: #fff; } main { max-width: none; margin: 0; padding: 0; border: 0; } .actions { display: none; } @page { margin: 18mm; } }
  </style>
</head>
<body>
  <main>
    <div class="actions">
      <button type="button" onclick="window.print()">Print / Save PDF</button>
    </div>
    <header>
      <div>
        <div class="brand">{{.SystemName}}</div>
        <h1>Top-up Invoice</h1>
      </div>
      <div class="status">Paid</div>
    </header>

    <section class="grid" aria-label="Invoice summary">
      <div><div class="label">Invoice No.</div><div class="value">{{.InvoiceNo}}</div></div>
      <div><div class="label">Issued At</div><div class="value">{{.IssuedAt}}</div></div>
      <div><div class="label">Customer</div><div class="value">{{.CustomerName}}</div></div>
      <div><div class="label">Email</div><div class="value">{{.CustomerEmail}}</div></div>
      <div><div class="label">Created At</div><div class="value">{{.CreatedAt}}</div></div>
      <div><div class="label">Completed At</div><div class="value">{{.CompletedAt}}</div></div>
    </section>

    <table aria-label="Top-up details">
      <thead><tr><th>Order</th><th>Payment</th><th>Amount</th></tr></thead>
      <tbody><tr>
        <td>
          <div class="label">Order No.</div><div class="value">{{.TradeNo}}</div>
          <div class="label">Gateway Order No.</div><div class="value">{{.GatewayTradeNo}}</div>
        </td>
        <td>
          <div class="label">Provider</div><div class="value">{{.PaymentProvider}}</div>
          <div class="label">Method</div><div class="value">{{.PaymentMethod}}</div>
        </td>
        <td>
          <div class="label">Credited Quota</div><div class="value">{{.CreditedQuota}}</div>
          <div class="label">Paid Amount</div><div class="paid">{{.PaidAmount}}</div>
        </td>
      </tr></tbody>
    </table>

    <footer>This invoice was generated from the completed top-up record stored by {{.SystemName}}.</footer>
  </main>
</body>
</html>`))

func GetTopUpInvoice(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的订单ID")
		return
	}

	topUp := model.GetTopUpById(id)
	requesterID := c.GetInt("id")
	isAdminView := topUp != nil && topUp.UserId != requesterID && c.GetInt("role") >= common.RoleAdminUser
	if topUp == nil || (topUp.UserId != requesterID && !isAdminView) {
		common.ApiErrorMsg(c, "充值订单不存在")
		return
	}
	if topUp.Status != common.TopUpStatusSuccess {
		common.ApiErrorMsg(c, "仅成功订单支持下载凭证")
		return
	}

	user, _ := model.GetUserById(topUp.UserId, false)
	isDownload := c.Query("download") == "1"
	var invoiceBytes []byte
	contentType := "text/html; charset=utf-8"
	filename := fmt.Sprintf("invoice-%s.html", sanitizeTopUpInvoiceFilename(topUp.TradeNo))
	if isDownload {
		invoiceBytes, err = renderTopUpInvoicePDF(topUp, user)
		contentType = "application/pdf"
		filename = fmt.Sprintf("invoice-%s.pdf", sanitizeTopUpInvoiceFilename(topUp.TradeNo))
	} else {
		invoiceBytes, err = renderTopUpInvoice(topUp, user)
	}
	if err != nil {
		common.ApiErrorMsg(c, "生成充值凭证失败")
		return
	}
	if isAdminView {
		recordManageAuditFor(c, topUp.UserId, "topup.invoice_view", map[string]interface{}{
			"topup_id": topUp.Id,
			"trade_no": topUp.TradeNo,
		})
	}

	c.Header("Cache-Control", "private, no-store")
	disposition := "inline"
	if isDownload {
		disposition = "attachment"
	}
	c.Header("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
	c.Header("Content-Security-Policy", "sandbox allow-scripts allow-modals")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, contentType, invoiceBytes)
}

func renderTopUpInvoice(topUp *model.TopUp, user *model.User) ([]byte, error) {
	if topUp == nil {
		return nil, fmt.Errorf("topup is nil")
	}

	var buf bytes.Buffer
	if err := topUpInvoiceTemplate.Execute(&buf, newTopUpInvoiceView(topUp, user)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderTopUpInvoicePDF(topUp *model.TopUp, user *model.User) ([]byte, error) {
	if topUp == nil {
		return nil, fmt.Errorf("topup is nil")
	}

	view := newTopUpInvoiceView(topUp, user)
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(view.SystemName+" Invoice "+view.InvoiceNo, true)
	pdf.SetAuthor(view.SystemName, true)
	pdf.SetMargins(18, 18, 18)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 20)
	pdf.CellFormat(0, 10, pdfSafeText("Top-up Invoice"), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(102, 112, 133)
	pdf.CellFormat(0, 7, pdfSafeText(view.SystemName), "", 1, "L", false, 0, "")
	pdf.Ln(4)

	pdf.SetTextColor(16, 24, 40)
	rows := [][2]string{
		{"Invoice No.", view.InvoiceNo},
		{"Issued At", view.IssuedAt},
		{"Customer", view.CustomerName},
		{"Email", view.CustomerEmail},
		{"Created At", view.CreatedAt},
		{"Completed At", view.CompletedAt},
		{"Order No.", view.TradeNo},
		{"Gateway Order No.", view.GatewayTradeNo},
		{"Provider", view.PaymentProvider},
		{"Method", view.PaymentMethod},
		{"Credited Quota", view.CreditedQuota},
		{"Paid Amount", view.PaidAmount},
	}

	for _, row := range rows {
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(42, 7, pdfSafeText(row[0]), "B", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		pdf.MultiCell(0, 7, pdfSafeText(row[1]), "B", "L", false)
	}

	pdf.Ln(8)
	pdf.SetTextColor(102, 112, 133)
	pdf.SetFont("Helvetica", "", 9)
	pdf.MultiCell(0, 5, pdfSafeText("This invoice was generated from the completed top-up record stored by "+view.SystemName+"."), "", "L", false)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func newTopUpInvoiceView(topUp *model.TopUp, user *model.User) topUpInvoiceView {
	return topUpInvoiceView{
		SystemName:      common.SystemName,
		InvoiceNo:       fmt.Sprintf("INV-%d", topUp.Id),
		CustomerName:    formatTopUpInvoiceCustomer(user, topUp.UserId),
		CustomerEmail:   formatTopUpInvoiceEmail(user),
		TradeNo:         valueOrDash(topUp.TradeNo),
		GatewayTradeNo:  valueOrDash(topUp.GatewayTradeNo),
		PaymentMethod:   formatTopUpInvoicePaymentLabel(topUp.PaymentMethod),
		PaymentProvider: formatTopUpInvoicePaymentLabel(topUp.PaymentProvider),
		CreditedQuota:   formatTopUpInvoiceCredit(topUp),
		PaidAmount:      formatTopUpInvoiceMoney(topUp),
		CreatedAt:       formatTopUpInvoiceTime(topUp.CreateTime),
		CompletedAt:     formatTopUpInvoiceTime(topUp.CompleteTime),
		IssuedAt:        time.Now().Format("2006-01-02 15:04:05 MST"),
	}
}

func pdfSafeText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}

	var builder strings.Builder
	for _, r := range value {
		switch {
		case r == '\n', r == '\r', r == '\t':
			builder.WriteByte(' ')
		case r >= 32 && r <= 126:
			builder.WriteRune(r)
		default:
			builder.WriteByte('?')
		}
	}
	if builder.Len() == 0 {
		return "-"
	}
	return builder.String()
}

func formatTopUpInvoiceCustomer(user *model.User, userID int) string {
	if user != nil {
		if name := strings.TrimSpace(user.DisplayName); name != "" {
			return name
		}
		if name := strings.TrimSpace(user.Username); name != "" {
			return name
		}
		if email := strings.TrimSpace(user.Email); email != "" {
			return email
		}
	}
	return fmt.Sprintf("User #%d", userID)
}

func formatTopUpInvoiceEmail(user *model.User) string {
	if user == nil {
		return "-"
	}
	return valueOrDash(user.Email)
}

func formatTopUpInvoiceCredit(topUp *model.TopUp) string {
	if topUp.CreditedQuota > 0 {
		return strconv.Itoa(topUp.CreditedQuota)
	}
	if topUp.Amount > 0 && (topUp.PaymentProvider == model.PaymentProviderCreem || topUp.PaymentMethod == model.PaymentMethodCreem) {
		return strconv.FormatInt(topUp.Amount, 10)
	}
	return "-"
}

func formatTopUpInvoiceMoney(topUp *model.TopUp) string {
	if math.IsNaN(topUp.Money) || math.IsInf(topUp.Money, 0) {
		return "-"
	}
	amount := fmt.Sprintf("%.2f", topUp.Money)
	if currency := strings.ToUpper(strings.TrimSpace(topUp.PaymentCurrency)); currency != "" {
		return currency + " " + amount
	}
	return amount
}

func formatTopUpInvoiceTime(timestamp int64) string {
	if timestamp <= 0 {
		return "-"
	}
	return time.Unix(timestamp, 0).Format("2006-01-02 15:04:05 MST")
}

func formatTopUpInvoicePaymentLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "-"
	case model.PaymentProviderStripe:
		return "Stripe"
	case model.PaymentProviderCreem:
		return "Creem"
	case model.PaymentProviderWaffo:
		return "Waffo"
	case model.PaymentProviderWaffoPancake:
		return "Waffo Pancake"
	case model.PaymentProviderLanTu:
		return "LanTu Pay"
	case model.PaymentProviderEpay:
		return "Epay"
	case "alipay":
		return "Alipay"
	case "wxpay":
		return "WeChat Pay"
	default:
		return value
	}
}

func sanitizeTopUpInvoiceFilename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "topup"
	}

	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}

	filename := strings.Trim(builder.String(), "._-")
	if filename == "" {
		return "topup"
	}
	return filename
}

func valueOrDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}
