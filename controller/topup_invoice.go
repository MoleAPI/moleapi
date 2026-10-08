package controller

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/go-opentype/fonts/notosanssc"
	"github.com/go-pdf/fpdf"
)

var TopUpInvoiceLogo []byte

type topUpInvoiceDetails struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Company    string `json:"company"`
	TaxID      string `json:"tax_id"`
	Address    string `json:"address"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

type topUpInvoiceView struct {
	LogoDataURI     template.URL        `json:"-"`
	SystemName      string              `json:"system_name"`
	InvoiceNo       string              `json:"invoice_no"`
	InvoiceDetails  topUpInvoiceDetails `json:"invoice_details"`
	CustomerName    string              `json:"customer_name"`
	CustomerEmail   string              `json:"customer_email"`
	CustomerCompany string              `json:"customer_company"`
	CustomerTaxID   string              `json:"customer_tax_id"`
	CustomerAddress string              `json:"customer_address"`
	TradeNo         string              `json:"trade_no"`
	GatewayTradeNo  string              `json:"gateway_trade_no"`
	PaymentMethod   string              `json:"payment_method"`
	PaymentProvider string              `json:"payment_provider"`
	TopUpAmount     string              `json:"top_up_amount"`
	CreditedQuota   string              `json:"credited_quota"`
	PaidAmount      string              `json:"paid_amount"`
	CreatedAt       string              `json:"created_at"`
	CompletedAt     string              `json:"completed_at"`
	IssuedAt        string              `json:"issued_at"`
	CanEdit         bool                `json:"can_edit"`
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
    main { max-width: 860px; margin: 32px auto; padding: 40px; background: #fff; border: 1px solid #e5e8ef; border-radius: 16px; }
    header { display: flex; justify-content: space-between; gap: 24px; padding-bottom: 24px; border-bottom: 1px solid #e5e8ef; }
    .brand-lockup { display: flex; align-items: center; gap: 14px; }
    .brand-icon { width: 52px; height: 52px; border-radius: 14px; object-fit: cover; }
    h1 { margin: 0; color: #101828; font-size: 30px; }
    .brand, .label, footer { color: #667085; font-size: 12px; }
    .brand, .label { font-weight: 600; text-transform: uppercase; }
    .label { margin-bottom: 4px; }
    .actions { display: flex; justify-content: flex-end; gap: 8px; margin-bottom: 24px; }
    button { min-height: 36px; padding: 0 14px; border: 1px solid #d0d5dd; border-radius: 8px; background: #fff; color: #344054; cursor: pointer; font-size: 14px; }
    button:hover { background: #f8fafc; }
    button.primary { border-color: #101828; background: #101828; color: #fff; }
    .status { align-self: flex-start; padding: 6px 10px; border-radius: 999px; background: #ecfdf3; color: #067647; font-size: 13px; font-weight: 600; }
    .grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px 32px; margin: 28px 0; }
    .value { color: #101828; font-size: 15px; overflow-wrap: anywhere; }
    .value + .label { margin-top: 12px; }
    .wide { grid-column: 1 / -1; }
    table { width: 100%; border-collapse: collapse; }
    th, td { padding: 14px 12px; border-bottom: 1px solid #e5e8ef; text-align: left; vertical-align: top; }
    th { background: #f8fafc; color: #475467; font-size: 12px; text-transform: uppercase; }
    .paid { color: #101828; font-size: 20px; font-weight: 700; }
    footer { margin-top: 28px; }
    dialog { width: min(560px, calc(100vw - 32px)); padding: 0; border: 0; border-radius: 14px; box-shadow: 0 24px 64px rgba(16, 24, 40, .22); }
    dialog::backdrop { background: rgba(16, 24, 40, .5); }
    form { padding: 24px; }
    form h2 { margin: 0 0 20px; font-size: 20px; }
    .form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
    label { display: grid; gap: 6px; color: #344054; font-size: 13px; font-weight: 600; }
    label.full { grid-column: 1 / -1; }
    input { width: 100%; height: 40px; padding: 0 11px; border: 1px solid #d0d5dd; border-radius: 8px; color: #101828; font: inherit; }
    input:focus { border-color: #667085; outline: 2px solid #e4e7ec; }
    .form-error { margin-top: 14px; color: #b42318; font-size: 13px; }
    .form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
    @media (max-width: 640px) { main { margin: 0; padding: 24px; border: 0; } header, .grid { grid-template-columns: 1fr; display: grid; } }
    @media (max-width: 640px) { .form-grid { grid-template-columns: 1fr; } label.full { grid-column: auto; } }
    @media print { * { print-color-adjust: exact; -webkit-print-color-adjust: exact; } body { background: #fff; } main { max-width: none; margin: 0; padding: 0; border: 0; } .actions, dialog { display: none; } @page { margin: 18mm; } }
  </style>
</head>
<body>
  <main>
    <div class="actions">
      {{if .CanEdit}}<button type="button" id="edit-details">Edit information</button>{{end}}
      <button type="button" class="primary" id="download-pdf">Download PDF</button>
    </div>
    <header>
      <div class="brand-lockup">
        {{if .LogoDataURI}}<img class="brand-icon" src="{{.LogoDataURI}}" alt="{{.SystemName}}">{{end}}
        <div>
          <div class="brand">{{.SystemName}}</div>
          <h1>Invoice</h1>
        </div>
      </div>
      <div class="status">Paid</div>
    </header>

    <section class="grid" aria-label="Invoice summary">
      <div><div class="label">Invoice No.</div><div class="value">{{.InvoiceNo}}</div></div>
      <div><div class="label">Issued At</div><div class="value">{{.IssuedAt}}</div></div>
      <div><div class="label">Customer</div><div class="value" id="customer-name">{{.CustomerName}}</div></div>
      <div><div class="label">Email</div><div class="value" id="customer-email">{{.CustomerEmail}}</div></div>
      <div><div class="label">Company</div><div class="value" id="customer-company">{{.CustomerCompany}}</div></div>
      <div><div class="label">Tax / VAT ID</div><div class="value" id="customer-tax-id">{{.CustomerTaxID}}</div></div>
      <div class="wide"><div class="label">Billing address</div><div class="value" id="customer-address">{{.CustomerAddress}}</div></div>
      <div><div class="label">Created At</div><div class="value">{{.CreatedAt}}</div></div>
      <div><div class="label">Completed At</div><div class="value">{{.CompletedAt}}</div></div>
    </section>

    <table aria-label="Top-up details">
      <thead><tr><th>Item</th><th>Payment</th><th>Amount</th></tr></thead>
      <tbody><tr>
        <td>
          <div class="label">Description</div><div class="value">{{.SystemName}} Credits</div>
          <div class="label">Order No.</div><div class="value">{{.TradeNo}}</div>
          <div class="label">Gateway Order No.</div><div class="value">{{.GatewayTradeNo}}</div>
        </td>
        <td>
          <div class="label">Provider</div><div class="value">{{.PaymentProvider}}</div>
          <div class="label">Method</div><div class="value">{{.PaymentMethod}}</div>
        </td>
        <td>
          <div class="label">Top-up Amount</div><div class="value">{{.TopUpAmount}}</div>
          <div class="label">Credited Quota</div><div class="value">{{.CreditedQuota}}</div>
          <div class="label">Paid Amount</div><div class="paid">{{.PaidAmount}}</div>
        </td>
      </tr></tbody>
    </table>

    <footer>This invoice was generated from the completed top-up record stored by {{.SystemName}}.</footer>
  </main>

  <dialog id="details-dialog" aria-labelledby="details-title">
    <form id="details-form">
      <h2 id="details-title">Edit invoice information</h2>
      <div class="form-grid">
        <label>Name<input name="name" maxlength="120" value="{{.InvoiceDetails.Name}}"></label>
        <label>Email<input name="email" type="email" maxlength="254" value="{{.InvoiceDetails.Email}}"></label>
        <label>Company name<input name="company" maxlength="160" value="{{.InvoiceDetails.Company}}"></label>
        <label>Tax / VAT ID<input name="tax_id" maxlength="80" value="{{.InvoiceDetails.TaxID}}"></label>
        <label class="full">Street address<input name="address" maxlength="240" value="{{.InvoiceDetails.Address}}"></label>
        <label>City<input name="city" maxlength="100" value="{{.InvoiceDetails.City}}"></label>
        <label>State / Province<input name="state" maxlength="100" value="{{.InvoiceDetails.State}}"></label>
        <label>Postal code<input name="postal_code" maxlength="32" value="{{.InvoiceDetails.PostalCode}}"></label>
        <label>Country / Region<input name="country" maxlength="100" value="{{.InvoiceDetails.Country}}"></label>
      </div>
      <div class="form-error" id="form-error" role="alert" hidden></div>
      <div class="form-actions">
        <button type="button" id="cancel-details">Cancel</button>
        <button type="submit" class="primary" id="save-details">Save</button>
      </div>
    </form>
  </dialog>

  <script>
    const dialog = document.getElementById('details-dialog')
    const form = document.getElementById('details-form')
    const error = document.getElementById('form-error')
    const save = document.getElementById('save-details')
    const field = (name) => form.elements.namedItem(name)
    const text = (id, value) => { document.getElementById(id).textContent = value || '-' }
    const address = () => [field('address').value, field('city').value, field('state').value, field('postal_code').value, field('country').value].map((value) => value.trim()).filter(Boolean).join(', ')

    document.getElementById('edit-details')?.addEventListener('click', () => dialog.showModal())
    document.getElementById('cancel-details').addEventListener('click', () => dialog.close())
    document.getElementById('download-pdf').addEventListener('click', () => { window.location.href = window.location.pathname + '?download=1' })
    form.addEventListener('submit', async (event) => {
      event.preventDefault()
      error.hidden = true
      save.disabled = true
      const details = {
        name: field('name').value,
        email: field('email').value,
        company: field('company').value,
        tax_id: field('tax_id').value,
        address: field('address').value,
        city: field('city').value,
        state: field('state').value,
        postal_code: field('postal_code').value,
        country: field('country').value
      }
      try {
        const response = await fetch(window.location.pathname, {
          method: 'PUT',
          credentials: 'same-origin',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(details)
        })
        const payload = await response.json()
        if (!response.ok || !payload.success) throw new Error(payload.message || 'Unable to save invoice information.')
        text('customer-name', details.name.trim())
        text('customer-email', details.email.trim())
        text('customer-company', details.company.trim())
        text('customer-tax-id', details.tax_id.trim())
        text('customer-address', address())
        dialog.close()
      } catch (cause) {
        error.textContent = cause instanceof Error ? cause.message : 'Unable to save invoice information.'
        error.hidden = false
      } finally {
        save.disabled = false
      }
    })
  </script>
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
	if topUp.PaymentProvider == model.PaymentProviderWaffoPancake || topUp.PaymentMethod == model.PaymentProviderWaffoPancake {
		common.ApiErrorMsg(c, "Waffo Pancake 订单仅支持官方发票")
		return
	}

	user, _ := model.GetUserById(topUp.UserId, false)
	view := newTopUpInvoiceView(topUp, user)
	view.CanEdit = !isAdminView
	if c.Query("format") == "json" {
		if isAdminView {
			recordManageAuditFor(c, topUp.UserId, "topup.invoice_view", map[string]interface{}{
				"topup_id": topUp.Id,
				"trade_no": topUp.TradeNo,
			})
		}
		c.Header("Cache-Control", "private, no-store")
		common.ApiSuccess(c, view)
		return
	}
	isDownload := c.Query("download") == "1"
	filename := fmt.Sprintf("invoice-%s.html", sanitizeTopUpInvoiceFilename(topUp.TradeNo))
	contentType := "text/html; charset=utf-8"
	var invoiceBytes []byte
	if isDownload {
		invoiceBytes, err = renderTopUpInvoicePDF(topUp, user)
		contentType = "application/pdf"
		filename = fmt.Sprintf("invoice-%s.pdf", sanitizeTopUpInvoiceFilename(topUp.TradeNo))
	} else {
		invoiceBytes, err = renderTopUpInvoiceView(view)
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
	c.Header("Content-Security-Policy", "sandbox allow-scripts allow-modals allow-same-origin allow-forms allow-downloads")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, contentType, invoiceBytes)
}

func UpdateTopUpInvoice(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "Invalid order ID")
		return
	}

	topUp := model.GetTopUpById(id)
	if topUp == nil || topUp.UserId != c.GetInt("id") {
		common.ApiErrorMsg(c, "Top-up order not found")
		return
	}
	if topUp.Status != common.TopUpStatusSuccess {
		common.ApiErrorMsg(c, "Only completed orders can update invoice information")
		return
	}
	if topUp.PaymentProvider == model.PaymentProviderWaffoPancake || topUp.PaymentMethod == model.PaymentProviderWaffoPancake {
		common.ApiErrorMsg(c, "Waffo Pancake orders use the official provider invoice")
		return
	}

	var details topUpInvoiceDetails
	if err := common.DecodeJson(c.Request.Body, &details); err != nil {
		common.ApiErrorMsg(c, "Invalid invoice information")
		return
	}
	if err := normalizeTopUpInvoiceDetails(&details); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	encoded, err := common.Marshal(details)
	if err != nil {
		common.ApiErrorMsg(c, "Unable to save invoice information")
		return
	}
	if err := topUp.UpdateInvoiceDetails(string(encoded)); err != nil {
		common.ApiErrorMsg(c, "Unable to save invoice information")
		return
	}
	common.ApiSuccess(c, nil)
}

func normalizeTopUpInvoiceDetails(details *topUpInvoiceDetails) error {
	fields := []struct {
		label string
		value *string
		limit int
	}{
		{"Name", &details.Name, 120},
		{"Email", &details.Email, 254},
		{"Company name", &details.Company, 160},
		{"Tax / VAT ID", &details.TaxID, 80},
		{"Street address", &details.Address, 240},
		{"City", &details.City, 100},
		{"State / Province", &details.State, 100},
		{"Postal code", &details.PostalCode, 32},
		{"Country / Region", &details.Country, 100},
	}
	for _, field := range fields {
		*field.value = strings.TrimSpace(*field.value)
		if utf8.RuneCountInString(*field.value) > field.limit {
			return fmt.Errorf("%s is too long", field.label)
		}
		if strings.IndexFunc(*field.value, unicode.IsControl) >= 0 {
			return fmt.Errorf("%s contains unsupported characters", field.label)
		}
	}
	if details.Email != "" {
		address, err := mail.ParseAddress(details.Email)
		if err != nil || address.Name != "" || address.Address != details.Email {
			return fmt.Errorf("Email is invalid")
		}
	}
	if utf8.RuneCountInString(formatTopUpInvoiceAddress(*details)) > 300 {
		return fmt.Errorf("Billing address is too long")
	}
	return nil
}

func renderTopUpInvoice(topUp *model.TopUp, user *model.User) ([]byte, error) {
	if topUp == nil {
		return nil, fmt.Errorf("topup is nil")
	}
	return renderTopUpInvoiceView(newTopUpInvoiceView(topUp, user))
}

func renderTopUpInvoiceView(view topUpInvoiceView) ([]byte, error) {
	var buf bytes.Buffer
	if err := topUpInvoiceTemplate.Execute(&buf, view); err != nil {
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
	// ponytail: one embedded face covers every stored value; add another face only if dynamic text needs distinct styling.
	pdf.AddUTF8FontFromBytes("NotoSansSC", "", notosanssc.TTF)
	pdf.SetTitle(view.SystemName+" Invoice "+view.InvoiceNo, true)
	pdf.SetAuthor(view.SystemName, true)
	pdf.SetMargins(18, 18, 18)
	pdf.SetAutoPageBreak(true, 18)
	pdf.AddPage()
	pdf.SetDrawColor(229, 232, 239)
	pdf.SetLineWidth(0.2)

	if len(TopUpInvoiceLogo) > 0 {
		options := fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}
		pdf.RegisterImageOptionsReader("invoice-logo", options, bytes.NewReader(TopUpInvoiceLogo))
		pdf.ClipRoundedRect(18, 18, 18, 18, 4, false)
		pdf.ImageOptions("invoice-logo", 18, 18, 18, 18, false, options, 0, "")
		pdf.ClipEnd()
	}
	pdf.SetXY(42, 18)
	pdf.SetTextColor(102, 112, 133)
	pdf.SetFont("NotoSansSC", "", 9)
	pdf.CellFormat(0, 6, pdfSafeText(view.SystemName), "", 1, "L", false, 0, "")
	pdf.SetX(42)
	pdf.SetTextColor(16, 24, 40)
	pdf.SetFont("Helvetica", "B", 24)
	pdf.CellFormat(0, 12, "Invoice", "", 0, "L", false, 0, "")
	pdf.SetXY(170, 22)
	pdf.SetFillColor(236, 253, 243)
	pdf.SetTextColor(6, 118, 71)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(22, 8, "Paid", "", 1, "C", true, 0, "")
	pdf.Line(18, 42, 192, 42)

	rows := [][2][2]string{
		{{"Invoice No.", view.InvoiceNo}, {"Issued At", view.IssuedAt}},
		{{"Customer", view.CustomerName}, {"Email", view.CustomerEmail}},
		{{"Company", view.CustomerCompany}, {"Tax / VAT ID", view.CustomerTaxID}},
	}
	summaryY := 52.0
	colW := 78.0
	for _, row := range rows {
		leftHeight := drawTopUpInvoicePDFField(pdf, 18, summaryY, colW, row[0][0], row[0][1])
		rightHeight := drawTopUpInvoicePDFField(pdf, 114, summaryY, colW, row[1][0], row[1][1])
		summaryY += math.Max(17, math.Max(leftHeight, rightHeight)+3)
	}

	addressHeight := drawTopUpInvoicePDFField(pdf, 18, summaryY, 174, "Billing Address", view.CustomerAddress)
	summaryY += math.Max(17, addressHeight+3)
	leftHeight := drawTopUpInvoicePDFField(pdf, 18, summaryY, colW, "Created At", view.CreatedAt)
	rightHeight := drawTopUpInvoicePDFField(pdf, 114, summaryY, colW, "Completed At", view.CompletedAt)
	summaryY += math.Max(17, math.Max(leftHeight, rightHeight)+3)

	tableY := summaryY + 10
	widths := []float64{62, 54, 58}
	headers := []string{"Order", "Payment", "Amount"}
	pdf.SetXY(18, tableY)
	pdf.SetFillColor(248, 250, 252)
	pdf.SetTextColor(71, 84, 103)
	pdf.SetFont("Helvetica", "B", 9)
	for i, header := range headers {
		pdf.CellFormat(widths[i], 8, header, "B", 0, "L", true, 0, "")
	}

	rowY := tableY + 10
	drawTopUpInvoicePDFStack(pdf, 18, rowY, widths[0]-4, [][2]string{
		{"Description", view.SystemName + " Credits"},
		{"Order No.", view.TradeNo},
		{"Gateway Order No.", view.GatewayTradeNo},
	})
	drawTopUpInvoicePDFStack(pdf, 80, rowY, widths[1]-4, [][2]string{
		{"Provider", view.PaymentProvider},
		{"Method", view.PaymentMethod},
	})
	drawTopUpInvoicePDFStack(pdf, 134, rowY, widths[2]-4, [][2]string{
		{"Top-up Amount", view.TopUpAmount},
		{"Credited Quota", view.CreditedQuota},
		{"Paid Amount", view.PaidAmount},
	})
	pdf.Line(18, rowY+45, 192, rowY+45)

	pdf.SetXY(18, rowY+54)
	pdf.SetTextColor(102, 112, 133)
	pdf.SetFont("NotoSansSC", "", 9)
	pdf.MultiCell(0, 5, pdfSafeText("This invoice was generated from the completed top-up record stored by "+view.SystemName+"."), "", "L", false)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func drawTopUpInvoicePDFField(pdf *fpdf.Fpdf, x, y, w float64, label string, value string) float64 {
	pdf.SetXY(x, y)
	pdf.SetTextColor(102, 112, 133)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.CellFormat(w, 4, pdfSafeText(label), "", 1, "L", false, 0, "")
	pdf.SetXY(x, y+5)
	pdf.SetTextColor(16, 24, 40)
	pdf.SetFont("NotoSansSC", "", 10)
	value = pdfSafeText(value)
	lineCount := len(pdf.SplitText(value, w))
	if lineCount == 0 {
		lineCount = 1
	}
	pdf.MultiCell(w, 5, value, "", "L", false)
	return 5 + float64(lineCount)*5
}

func drawTopUpInvoicePDFStack(pdf *fpdf.Fpdf, x, y, w float64, fields [][2]string) {
	for _, field := range fields {
		drawTopUpInvoicePDFField(pdf, x, y, w, field[0], field[1])
		y += 13
	}
}

func newTopUpInvoiceView(topUp *model.TopUp, user *model.User) topUpInvoiceView {
	details := loadTopUpInvoiceDetails(topUp, user)
	return topUpInvoiceView{
		LogoDataURI:     topUpInvoiceLogoDataURI(),
		SystemName:      common.SystemName,
		InvoiceNo:       fmt.Sprintf("INV-%d", topUp.Id),
		InvoiceDetails:  details,
		CustomerName:    valueOrDash(details.Name),
		CustomerEmail:   valueOrDash(details.Email),
		CustomerCompany: valueOrDash(details.Company),
		CustomerTaxID:   valueOrDash(details.TaxID),
		CustomerAddress: formatTopUpInvoiceAddress(details),
		TradeNo:         valueOrDash(topUp.TradeNo),
		GatewayTradeNo:  valueOrDash(topUp.GatewayTradeNo),
		PaymentMethod:   formatTopUpInvoicePaymentLabel(topUp.PaymentMethod),
		PaymentProvider: formatTopUpInvoicePaymentLabel(topUp.PaymentProvider),
		TopUpAmount:     strconv.FormatInt(topUp.Amount, 10),
		CreditedQuota:   formatTopUpInvoiceCredit(topUp),
		PaidAmount:      formatTopUpInvoiceMoney(topUp),
		CreatedAt:       formatTopUpInvoiceTime(topUp.CreateTime),
		CompletedAt:     formatTopUpInvoiceTime(topUp.CompleteTime),
		IssuedAt:        time.Now().Format("2006-01-02 15:04:05 MST"),
		CanEdit:         true,
	}
}

func loadTopUpInvoiceDetails(topUp *model.TopUp, user *model.User) topUpInvoiceDetails {
	if strings.TrimSpace(topUp.InvoiceDetails) != "" {
		var details topUpInvoiceDetails
		if common.UnmarshalJsonStr(topUp.InvoiceDetails, &details) == nil {
			return details
		}
	}
	return topUpInvoiceDetails{
		Name:  formatTopUpInvoiceCustomer(user, topUp.UserId),
		Email: formatTopUpInvoiceEmail(user),
	}
}

func formatTopUpInvoiceAddress(details topUpInvoiceDetails) string {
	parts := make([]string, 0, 5)
	for _, part := range []string{details.Address, details.City, details.State, details.PostalCode, details.Country} {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	return valueOrDash(strings.Join(parts, ", "))
}

func topUpInvoiceLogoDataURI() template.URL {
	if len(TopUpInvoiceLogo) == 0 {
		return ""
	}
	return template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(TopUpInvoiceLogo))
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
		case r < 32 || r == 127:
			continue
		default:
			builder.WriteRune(r)
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
		return ""
	}
	return strings.TrimSpace(user.Email)
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
