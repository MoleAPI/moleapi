package reasoning

import "strings"

// OpenAIRender describes reasoning controls on an OpenAI-compatible wire.
// An empty Effort removes an unsupported ladder; ThinkingType carries switches
// for hybrid models. Known=false leaves an unfamiliar model's native fields alone.
type OpenAIRender struct {
	Effort       Effort
	ThinkingType string
	Known        bool
}

// NeedsChatThinking identifies APIs whose thinking control is a Chat-only
// switch, so Responses clients use the existing Chat conversion bridge.
func NeedsChatThinking(model string) bool {
	rendered := RenderOpenAI(model, Intent{Mode: ModeEnabled, Effort: EffortMedium})
	return rendered.Known && rendered.Effort == "" && rendered.ThinkingType != ""
}

func RenderOpenAI(model string, intent Intent) OpenAIRender {
	model = strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(model, "/"); i >= 0 {
		model = model[i+1:]
	}
	if i := strings.Index(model, "claude-"); i >= 0 {
		model = model[i:]
	}
	effort := EffectiveEffort(intent)
	rendered := OpenAIRender{Effort: effort, Known: true}
	switch {
	case strings.HasPrefix(model, "gemini-"):
		capabilities := geminiCapabilitiesFor(model)
		if capabilities.kind == geminiThinkingUnknown {
			rendered.Known = false
			return rendered
		}
		if capabilities.kind == geminiThinkingNotConfigurable {
			rendered.Effort = ""
			return rendered
		}
		if effort == "" {
			return rendered
		}
		if capabilities.kind == geminiThinkingLevel {
			if effort == EffortNone {
				effort = EffortMinimal
			}
			level, _ := geminiLevelForEffort(model, effort)
			rendered.Effort = Effort(level)
		} else {
			switch effort {
			case EffortNone:
				if !capabilities.supportsDisable {
					rendered.Effort = EffortLow
				}
			case EffortMinimal:
				rendered.Effort = EffortLow
			case EffortMax, EffortXHigh:
				rendered.Effort = EffortHigh
			}
		}
	case strings.HasPrefix(model, "claude-"):
		if effort != "" && effort != EffortNone {
			rendered.Effort = normalizeClaudeEffort(effort, claudeCapabilitiesFor(model))
		}
	case strings.HasPrefix(model, "gpt-6"):
		if effort == EffortMinimal || effort == EffortNone && !strings.HasPrefix(model, "gpt-6-sol") && !strings.HasPrefix(model, "gpt-6-luna") && !strings.HasPrefix(model, "gpt-6.1-terra") {
			rendered.Effort = EffortLow
		}
	case strings.HasPrefix(model, "gpt-5"):
		switch {
		case strings.Contains(model, "chat"), strings.HasPrefix(model, "gpt-5-search"):
			rendered.Effort = ""
		case strings.HasPrefix(model, "gpt-5-pro"):
			if effort != "" {
				rendered.Effort = EffortHigh
			}
		case strings.HasPrefix(model, "gpt-5.2"), strings.HasPrefix(model, "gpt-5.3"), strings.HasPrefix(model, "gpt-5.4"), strings.HasPrefix(model, "gpt-5.5"), strings.HasPrefix(model, "gpt-5.6"), strings.Contains(model, "codex-max"):
			if effort == EffortMax {
				rendered.Effort = EffortXHigh
			}
			if effort == EffortMinimal {
				rendered.Effort = EffortLow
			}
			if strings.Contains(model, "-pro") && (effort == EffortNone || effort == EffortMinimal || effort == EffortLow) {
				rendered.Effort = EffortMedium
			} else if strings.Contains(model, "codex") && effort == EffortNone {
				rendered.Effort = EffortLow
			}
		default:
			if effort == EffortMax || effort == EffortXHigh {
				rendered.Effort = EffortHigh
			}
			if strings.HasPrefix(model, "gpt-5.1") || strings.Contains(model, "codex") {
				if effort == EffortMinimal {
					rendered.Effort = EffortLow
				}
				if strings.Contains(model, "codex") && effort == EffortNone {
					rendered.Effort = EffortLow
				}
			} else if effort == EffortNone {
				rendered.Effort = EffortMinimal
			}
		}
	case strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"), strings.HasPrefix(model, "o4"):
		if strings.HasPrefix(model, "o1-mini") || strings.HasPrefix(model, "o1-preview") {
			rendered.Effort = ""
		} else if effort == EffortNone || effort == EffortMinimal {
			rendered.Effort = EffortLow
		} else if effort == EffortXHigh || effort == EffortMax {
			rendered.Effort = EffortHigh
		}
	case strings.HasPrefix(model, "gpt-"):
		rendered.Effort = ""
	case strings.HasPrefix(model, "deepseek-v4"), strings.HasPrefix(model, "deepseek-flash"), strings.HasPrefix(model, "glm-5.2"), strings.HasPrefix(model, "glm-5.3"), strings.HasPrefix(model, "kimi-k3"):
		if intent.HasStrength() {
			rendered.ThinkingType = "enabled"
			if effort == EffortNone {
				if strings.HasPrefix(model, "glm-5.3") || strings.HasPrefix(model, "kimi-k3") {
					rendered.Effort = EffortLow
				} else {
					rendered.ThinkingType = "disabled"
				}
			} else if effort == EffortMinimal {
				rendered.Effort = EffortLow
			} else if effort == EffortMedium || effort == EffortXHigh {
				// DeepSeek explicitly maps xhigh to high, reserving max for max/ultra.
				rendered.Effort = EffortHigh
			}
			// K3 always reasons and rejects the K2.x thinking switch.
			if strings.HasPrefix(model, "kimi-k3") {
				rendered.ThinkingType = ""
			}
		}
	case strings.HasPrefix(model, "glm-4.5"), strings.HasPrefix(model, "glm-4.6"), strings.HasPrefix(model, "glm-4.7"), strings.HasPrefix(model, "glm-5"), strings.HasPrefix(model, "deepseek-v3"), model == "deepseek-chat", strings.HasPrefix(model, "kimi-k2.5"), strings.HasPrefix(model, "kimi-k2.6"):
		rendered.Effort = ""
		if intent.HasStrength() {
			rendered.ThinkingType = "enabled"
			if effort == EffortNone {
				rendered.ThinkingType = "disabled"
			}
		}
	case strings.HasPrefix(model, "kimi-"), strings.HasPrefix(model, "moonshot-"), strings.HasPrefix(model, "glm-"), strings.HasPrefix(model, "deepseek-"):
		// ponytail: fixed-thinking/legacy models use their native default instead
		// of inventing a ladder. Extend the explicit families above for new APIs.
		rendered.Effort = ""
	default:
		rendered.Known = false
	}
	return rendered
}
