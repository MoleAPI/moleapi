package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// normalizeModelReasoning runs after parameter overrides. Only reasoning fields
// are decoded and replaced; messages, signatures, schemas and large numbers
// elsewhere in the request retain their original bytes.
func normalizeModelReasoning(body map[string]common.RawMessage, info *RelayInfo) error {
	model := info.UpstreamModelName
	if value := body["model"]; len(value) > 0 {
		if err := common.Unmarshal(value, &model); err != nil {
			return err
		}
	}
	if model == "" {
		model = info.OriginModelName
	}
	controls := make(map[string]common.RawMessage)
	for _, key := range []string{"reasoning_effort", "reasoning", "thinking", "enable_thinking", "output_config", "max_tokens", "generationConfig", "generation_config"} {
		if value, exists := body[key]; exists {
			controls[key] = value
		}
	}
	encoded, err := common.Marshal(controls)
	if err != nil {
		return err
	}
	patch := make(map[string]any)
	format := info.GetFinalRequestRelayFormat()
	switch format {
	case types.RelayFormatGemini:
		var req dto.GeminiChatRequest
		if err := common.Unmarshal(encoded, &req); err != nil {
			return err
		}
		if err := relayconvert.ApplyGeminiThinkingConfigChecked(&req, info); err != nil {
			return err
		}
		var generation map[string]common.RawMessage
		generationJSON := body["generationConfig"]
		if value := body["generation_config"]; len(value) > 0 {
			generationJSON = value
		}
		if len(generationJSON) > 0 {
			if err := common.Unmarshal(generationJSON, &generation); err != nil {
				return err
			}
		}
		if generation == nil {
			generation = make(map[string]common.RawMessage)
		}
		thinkingJSON := generation["thinkingConfig"]
		if value := generation["thinking_config"]; len(value) > 0 {
			thinkingJSON = value
		}
		delete(generation, "thinking_config")
		if req.GenerationConfig.ThinkingConfig == nil {
			delete(generation, "thinkingConfig")
		} else {
			var thinking map[string]common.RawMessage
			if len(thinkingJSON) > 0 {
				if err := common.Unmarshal(thinkingJSON, &thinking); err != nil {
					return err
				}
			}
			if thinking == nil {
				thinking = make(map[string]common.RawMessage)
			}
			for _, key := range []string{"thinkingLevel", "thinking_level", "thinkingBudget", "thinking_budget", "includeThoughts", "include_thoughts"} {
				delete(thinking, key)
			}
			config := req.GenerationConfig.ThinkingConfig
			if config.ThinkingLevel != "" {
				thinking["thinkingLevel"], err = common.Marshal(config.ThinkingLevel)
				if err != nil {
					return err
				}
			}
			if config.ThinkingBudget != nil {
				thinking["thinkingBudget"], err = common.Marshal(config.ThinkingBudget)
				if err != nil {
					return err
				}
			}
			if config.IncludeThoughts != nil {
				thinking["includeThoughts"], err = common.Marshal(config.IncludeThoughts)
				if err != nil {
					return err
				}
			}
			value, err := common.Marshal(thinking)
			if err != nil {
				return err
			}
			generation["thinkingConfig"] = value
		}
		patch["generationConfig"] = generation
		patch["generation_config"] = nil
	case types.RelayFormatClaude:
		var req dto.ClaudeRequest
		if err := common.Unmarshal(encoded, &req); err != nil {
			return err
		}
		capabilityModel := model
		if i := strings.Index(capabilityModel, "claude-"); i >= 0 {
			capabilityModel = capabilityModel[i:]
		}
		req.Model = capabilityModel
		if reasoning.IsKnownClaudeModel(capabilityModel) {
			if err := relayconvert.ApplyClaudeThinkingModel(&req, info); err != nil {
				return err
			}
		} else {
			intent, err := reasoning.FromClaude(&req)
			if err != nil {
				return err
			}
			rendered := reasoning.RenderOpenAI(model, intent)
			if !rendered.Known || !intent.HasStrength() {
				return nil
			}
			var output map[string]common.RawMessage
			if len(req.OutputConfig) > 0 {
				if err := common.Unmarshal(req.OutputConfig, &output); err != nil {
					return err
				}
			}
			if output == nil {
				output = make(map[string]common.RawMessage)
			}
			delete(output, "effort")
			if rendered.Effort != "" && rendered.Effort != reasoning.EffortNone {
				output["effort"], err = common.Marshal(rendered.Effort)
				if err != nil {
					return err
				}
			}
			req.OutputConfig = nil
			if len(output) > 0 {
				req.OutputConfig, err = common.Marshal(output)
				if err != nil {
					return err
				}
			}
			req.Thinking = nil
			if rendered.ThinkingType != "" {
				req.Thinking = &dto.Thinking{Type: rendered.ThinkingType}
			}
			info.SetReasoningEffort(string(rendered.Effort))
		}
		patch["thinking"], patch["output_config"] = nil, nil
		if req.Thinking != nil {
			patch["thinking"] = req.Thinking
		}
		if len(req.OutputConfig) > 0 {
			patch["output_config"] = req.OutputConfig
		}
	case types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses:
		var chat dto.GeneralOpenAIRequest
		if err := common.Unmarshal(encoded, &chat); err != nil {
			return err
		}
		chat.Model = model
		intent, err := reasoning.FromOpenAIChat(&chat)
		if err != nil {
			return err
		}
		rendered := reasoning.RenderOpenAI(model, intent)
		if !rendered.Known {
			return nil
		}
		var nested map[string]common.RawMessage
		if len(chat.Reasoning) > 0 && string(chat.Reasoning) != "null" {
			if err := common.Unmarshal(chat.Reasoning, &nested); err != nil {
				return err
			}
		}
		if nested == nil {
			nested = make(map[string]common.RawMessage)
		}
		for _, key := range []string{"effort", "enabled", "max_tokens", "exclude"} {
			delete(nested, key)
		}
		patch["reasoning_effort"], patch["reasoning"], patch["thinking"], patch["enable_thinking"] = nil, nil, nil, nil
		if format == types.RelayFormatOpenAIResponses {
			if rendered.Effort != "" {
				nested["effort"], err = common.Marshal(rendered.Effort)
				if err != nil {
					return err
				}
			}
			if value := nested["summary"]; len(value) > 0 {
				var summary string
				if err := common.Unmarshal(value, &summary); err != nil {
					return err
				}
				switch strings.ToLower(strings.TrimSpace(summary)) {
				case "", "none", "off", "disabled":
					delete(nested, "summary")
				case "concise", "detailed", "auto":
					nested["summary"], err = common.Marshal(strings.ToLower(strings.TrimSpace(summary)))
				default:
					nested["summary"] = common.RawMessage(`"auto"`)
				}
				if err != nil {
					return err
				}
			}
		} else {
			if rendered.Effort != "" && rendered.ThinkingType != "disabled" {
				patch["reasoning_effort"] = rendered.Effort
			}
			if rendered.ThinkingType != "" {
				var thinking dto.Thinking
				if len(chat.THINKING) > 0 && string(chat.THINKING) != "null" {
					if err := common.Unmarshal(chat.THINKING, &thinking); err != nil {
						return err
					}
				}
				thinking.Type, thinking.BudgetTokens, thinking.Display = rendered.ThinkingType, nil, ""
				patch["thinking"] = &thinking
			}
		}
		if len(nested) > 0 {
			patch["reasoning"] = nested
		}
		if intent.HasStrength() {
			effective := rendered.Effort
			if effective == "" && rendered.ThinkingType == "disabled" {
				effective = reasoning.EffortNone
			}
			info.SetReasoningEffort(string(effective))
			if effective != reasoning.EffectiveEffort(intent) {
				info.RecordConversionDiagnostics(context.Background(), []types.ConversionDiagnostic{{
					Code: "reasoning_effort_adjusted", Path: "reasoning", Severity: types.ConversionDiagnosticWarning,
					Message: fmt.Sprintf("model %q uses reasoning effort %q and thinking switch %q for the requested controls", model, effective, rendered.ThinkingType),
				}})
			}
		}
	default:
		return nil
	}
	for key, value := range patch {
		if value == nil {
			delete(body, key)
			continue
		}
		encoded, err := common.Marshal(value)
		if err != nil {
			return err
		}
		body[key] = encoded
	}
	return nil
}
