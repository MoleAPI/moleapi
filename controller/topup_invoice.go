package controller

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type topUpInvoiceView struct {
	SystemName      string
	InvoiceNo       string
	Status          string
	UserID          int
	CustomerName    string
	CustomerEmail   string
	TradeNo         string
	GatewayTradeNo  string
	PaymentMethod   string
	PaymentProvider string
	Amount          string
	Money           string
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
    body {
      margin: 0;
      background: #f5f7fa;
      color: #1f2933;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      line-height: 1.5;
    }
    .invoice {
      max-width: 840px;
      margin: 32px auto;
      padding: 40px;
      background: #fff;
      border: 1px solid #e5e8ef;
      border-radius: 8px;
      box-shadow: 0 10px 30px rgba(15, 23, 42, 0.08);
    }
    .actions {
      display: flex;
      justify-content: flex-end;
      margin-bottom: 24px;
    }
    button {
      height: 34px;
      padding: 0 14px;
      border: 1px solid #d0d5dd;
      border-radius: 6px;
      background: #fff;
      color: #344054;
      cursor: pointer;
      font-size: 14px;
    }
    button:hover { background: #f8fafc; }
    header {
      display: flex;
      justify-content: space-between;
      gap: 24px;
      padding-bottom: 28px;
      border-bottom: 1px solid #e5e8ef;
    }
    .brand {
      margin: 0 0 8px;
      color: #667085;
      font-size: 14px;
      font-weight: 600;
      text-transform: uppercase;
    }
    h1 {
      margin: 0;
      color: #101828;
      font-size: 30px;
      font-weight: 700;
      letter-spacing: 0;
    }
    .status {
      align-self: flex-start;
      padding: 6px 10px;
      border-radius: 999px;
      background: #ecfdf3;
      color: #067647;
      font-size: 13px;
      font-weight: 600;
      white-space: nowrap;
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 20px 32px;
      margin: 28px 0;
    }
    .field-label {
      margin-bottom: 4px;
      color: #667085;
      font-size: 12px;
      font-weight: 600;
      text-transform: uppercase;
    }
    .field-value {
      color: #101828;
      font-size: 15px;
      overflow-wrap: anywhere;
    }
    table {
      width: 100%;
      margin-top: 16px;
      border-collapse: collapse;
    }
    th, td {
      padding: 14px 12px;
      border-bottom: 1px solid #e5e8ef;
      text-align: left;
      vertical-align: top;
    }
    th {
      background: #f8fafc;
      color: #475467;
      font-size: 12px;
      font-weight: 700;
      text-transform: uppercase;
    }
    td { font-size: 15px; }
    .amount {
      color: #101828;
      font-size: 20px;
      font-weight: 700;
      text-align: right;
    }
    .right { text-align: right; }
    footer {
      margin-top: 28px;
      color: #667085;
      font-size: 12px;
    }
    @media (max-width: 640px) {
      .invoice {
        margin: 0;
        padding: 24px;
        border: 0;
        border-radius: 0;
        box-shadow: none;
      }
      header, .grid {
        grid-template-columns: 1fr;
        display: grid;
      }
      .right, .amount { text-align: left; }
    }
    @media print {
      body { background: #fff; }
      .invoice {
        max-width: none;
        margin: 0;
        padding: 0;
        border: 0;
        box-shadow: none;
      }
      .actions { display: none; }
      @page { margin: 18mm; }
    }
  </style>
</head>
<body>
  <main class="invoice">
    <div class="actions">
      <button type="button" onclick="window.print()">Print / Save PDF</button>
    </div>
    <header>
      <div>
        <p class="brand">{{.SystemName}}</p>
        <h1>Top-up Invoice</h1>
      </div>
      <div class="status">{{.Status}}</div>
    </header>

    <section class="grid" aria-label="Invoice summary">
      <div>
        <div class="field-label">Invoice No.</div>
        <div class="field-value">{{.InvoiceNo}}</div>
      </div>
      <div>
        <div class="field-label">Issued At</div>
        <div class="field-value">{{.IssuedAt}}</div>
      </div>
      <div>
        <div class="field-label">Customer</div>
        <div class="field-value">{{.CustomerName}}</div>
      </div>
      <div>
        <div class="field-label">User ID</div>
        <div class="field-value">{{.UserID}}</div>
      </div>
      <div>
        <div class="field-label">Email</div>
        <div class="field-value">{{.CustomerEmail}}</div>
      </div>
      <div>
        <div class="field-label">Created At</div>
        <div class="field-value">{{.CreatedAt}}</div>
      </div>
    </section>

    <table aria-label="Top-up details">
      <thead>
        <tr>
          <th>Order</th>
          <th>Payment</th>
          <th class="right">Amount</th>
        </tr>
      </thead>
      <tbody>
        <tr>
          <td>
            <div class="field-label">Order No.</div>
            <div class="field-value">{{.TradeNo}}</div>
            <div class="field-label" style="margin-top: 12px;">Gateway Order No.</div>
            <div class="field-value">{{.GatewayTradeNo}}</div>
          </td>
          <td>
            <div class="field-label">Provider</div>
            <div class="field-value">{{.PaymentProvider}}</div>
            <div class="field-label" style="margin-top: 12px;">Method</div>
            <div class="field-value">{{.PaymentMethod}}</div>
          </td>
          <td class="right">
            <div class="field-label">Top-up Amount</div>
            <div class="field-value">{{.Amount}}</div>
            <div class="field-label" style="margin-top: 12px;">Paid Amount</div>
            <div class="amount">{{.Money}}</div>
          </td>
        </tr>
      </tbody>
    </table>

    <section class="grid" aria-label="Completion details">
      <div>
        <div class="field-label">Completed At</div>
        <div class="field-value">{{.CompletedAt}}</div>
      </div>
      <div>
        <div class="field-label">Payment Status</div>
        <div class="field-value">{{.Status}}</div>
      </div>
    </section>

    <footer>
      Generated from the completed top-up record stored by {{.SystemName}}.
    </footer>
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
	if topUp == nil {
		common.ApiErrorMsg(c, "充值订单不存在")
		return
	}

	userId := c.GetInt("id")
	role := c.GetInt("role")
	if role < common.RoleAdminUser && topUp.UserId != userId {
		common.ApiErrorMsg(c, "无权查看该订单")
		return
	}

	if topUp.Status != common.TopUpStatusSuccess {
		common.ApiErrorMsg(c, "仅成功订单支持查看 Invoice")
		return
	}

	var user *model.User
	if loadedUser, userErr := model.GetUserById(topUp.UserId, false); userErr == nil {
		user = loadedUser
	}

	htmlBytes, err := renderTopUpInvoice(topUp, user)
	if err != nil {
		common.ApiErrorMsg(c, "生成 Invoice 失败")
		return
	}

	disposition := "inline"
	if c.Query("download") == "1" {
		disposition = "attachment"
	}

	filename := fmt.Sprintf("invoice-%s.html", sanitizeTopUpInvoiceFilename(topUp.TradeNo))
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
	c.Data(http.StatusOK, "text/html; charset=utf-8", htmlBytes)
}

func renderTopUpInvoice(topUp *model.TopUp, user *model.User) ([]byte, error) {
	if topUp == nil {
		return nil, fmt.Errorf("topup is nil")
	}

	topUp.FillAmountDisplay()
	view := topUpInvoiceView{
		SystemName:      common.SystemName,
		InvoiceNo:       fmt.Sprintf("INV-%d", topUp.Id),
		Status:          formatTopUpInvoiceStatus(topUp.Status),
		UserID:          topUp.UserId,
		CustomerName:    formatTopUpInvoiceCustomer(user, topUp.UserId),
		CustomerEmail:   formatTopUpInvoiceEmail(user),
		TradeNo:         valueOrDash(topUp.TradeNo),
		GatewayTradeNo:  valueOrDash(topUp.GatewayTradeNo),
		PaymentMethod:   formatTopUpInvoicePaymentLabel(topUp.PaymentMethod),
		PaymentProvider: formatTopUpInvoicePaymentLabel(topUp.PaymentProvider),
		Amount:          formatTopUpInvoiceAmount(topUp),
		Money:           formatTopUpInvoiceMoney(topUp.Money),
		CreatedAt:       formatTopUpInvoiceTime(topUp.CreateTime),
		CompletedAt:     formatTopUpInvoiceTime(topUp.CompleteTime),
		IssuedAt:        time.Now().Format("2006-01-02 15:04:05 MST"),
	}

	var buf bytes.Buffer
	if err := topUpInvoiceTemplate.Execute(&buf, view); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatTopUpInvoiceCustomer(user *model.User, userId int) string {
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
	return fmt.Sprintf("User #%d", userId)
}

func formatTopUpInvoiceEmail(user *model.User) string {
	if user == nil || strings.TrimSpace(user.Email) == "" {
		return "-"
	}
	return strings.TrimSpace(user.Email)
}

func formatTopUpInvoiceAmount(topUp *model.TopUp) string {
	if topUp == nil {
		return "-"
	}
	if topUp.Amount == 0 && strings.HasPrefix(strings.ToLower(topUp.TradeNo), "sub") {
		return "Subscription plan"
	}
	if strings.TrimSpace(topUp.AmountDisplay) != "" {
		return topUp.AmountDisplay
	}
	return strconv.FormatInt(topUp.Amount, 10)
}

func formatTopUpInvoiceMoney(money float64) string {
	return fmt.Sprintf("%.2f", money)
}

func formatTopUpInvoiceTime(timestamp int64) string {
	if timestamp <= 0 {
		return "-"
	}
	return time.Unix(timestamp, 0).Format("2006-01-02 15:04:05 MST")
}

func formatTopUpInvoiceStatus(status string) string {
	switch status {
	case common.TopUpStatusSuccess:
		return "Paid"
	case common.TopUpStatusPending:
		return "Pending"
	case common.TopUpStatusFailed:
		return "Failed"
	case common.TopUpStatusExpired:
		return "Expired"
	default:
		return valueOrDash(status)
	}
}

func formatTopUpInvoicePaymentLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "-"
	case "stripe":
		return "Stripe"
	case "creem":
		return "Creem"
	case "waffo":
		return "Waffo"
	case "waffo_pancake":
		return "Waffo Pancake"
	case "lantu":
		return "LanTu Pay"
	case "epay":
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
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-', r == '_', r == '.':
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
