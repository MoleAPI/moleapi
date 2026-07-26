package setting

var (
	NowPaymentsEnabled   bool
	NowPaymentsApiKey    string
	NowPaymentsIPNSecret string
	NowPaymentsSandbox   bool
	NowPaymentsCurrency  string  = "USD"
	NowPaymentsUnitPrice float64 = 1.0
	NowPaymentsMinTopUp  int     = 1
)
