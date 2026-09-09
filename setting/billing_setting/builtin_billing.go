package billing_setting

// Built-in token prices use actual USD per million tokens. Keep new model
// defaults here instead of splitting them across the legacy ratio tables.
var builtinBillingExpr = map[string]string{
	// https://developers.openai.com/api/docs/models/gpt-6-astra
	// Standard pricing; the long-context rates apply to the whole request.
	// Do not infer service-tier discounts from incoming request parameters:
	// channels filter service_tier by default, so it may not reach the upstream.
	"gpt-6-astra": `len <= 272000 ? tier("standard", p * 10 + c * 50 + cr * 1 + cc * 12.5) : tier("long_context", p * 20 + c * 75 + cr * 2 + cc * 25)`,
	// https://developers.openai.com/api/docs/models/gpt-image-2.5-flare
	// https://developers.openai.com/api/docs/models/gpt-image-2.5-sunburst
	// Token rates match GPT Image 2: text input $5, cached input $1.25,
	// image input $8, and image output $30 per million tokens.
	"gpt-image-2.5-flare":               `tier("base", p * 5 + cr * 1.25 + img * 8 + img_o * 30)`,
	"gpt-image-2.5-flare-2026-09-08":    `tier("base", p * 5 + cr * 1.25 + img * 8 + img_o * 30)`,
	"gpt-image-2.5-sunburst":            `tier("base", p * 5 + cr * 1.25 + img * 8 + img_o * 30)`,
	"gpt-image-2.5-sunburst-2026-09-08": `tier("base", p * 5 + cr * 1.25 + img * 8 + img_o * 30)`,
}
