package relay

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var legacyCodexToolModel = regexp.MustCompile(`^gpt-4o(-mini)?(-[0-9]{4}-[0-9]{2}-[0-9]{2})?$`)

func omitIncompatibleCodexAutomationTool(httpRequest *http.Request, request *dto.OpenAIResponsesRequest) {
	if httpRequest == nil || len(request.Tools) == 0 || !legacyCodexToolModel.MatchString(request.Model) {
		return
	}
	if !strings.EqualFold(httpRequest.Header.Get("Originator"), "Codex Desktop") &&
		!strings.HasPrefix(strings.ToLower(httpRequest.UserAgent()), "codex desktop/") {
		return
	}
	switch strings.TrimSpace(string(request.ToolChoice)) {
	case "", "null", `"auto"`, `"none"`:
	default:
		return
	}
	tools := gjson.ParseBytes(request.Tools)
	if !tools.IsArray() {
		return
	}
	// ponytail: omit only the invalid optional tool. Flattening its alternatives
	// would relax the argument rules for creating or deleting scheduled tasks.
	kept := make([]string, 0)
	changed := false
	for _, tool := range tools.Array() {
		if incompatibleCodexAutomationTool(tool, false) {
			changed = true
			continue
		}
		if tool.Get("type").String() == "namespace" && tool.Get("name").String() == "codex_app" {
			members := tool.Get("tools")
			if members.IsArray() {
				memberList := members.Array()
				keptMembers := make([]string, 0, len(memberList))
				for _, member := range memberList {
					if !incompatibleCodexAutomationTool(member, true) {
						keptMembers = append(keptMembers, member.Raw)
					}
				}
				if len(keptMembers) != len(memberList) {
					changed = true
					if len(keptMembers) == 0 {
						continue
					}
					if updated, err := sjson.SetRaw(tool.Raw, "tools", "["+strings.Join(keptMembers, ",")+"]"); err == nil {
						tool.Raw = updated
					}
				}
			}
		}
		kept = append(kept, tool.Raw)
	}
	if changed {
		request.Tools = []byte("[" + strings.Join(kept, ",") + "]")
	}
}

func incompatibleCodexAutomationTool(tool gjson.Result, codexNamespace bool) bool {
	if tool.Get("type").String() != "function" {
		return false
	}
	switch tool.Get("name").String() {
	case "automation_update":
		if !codexNamespace && !strings.HasPrefix(tool.Get("description").String(), "Create, update, view, or delete recurring automations in the Codex app.") {
			return false
		}
	case "mcp__codex_app__automation_update", "codex_app__automation_update", "codex_app::automation_update":
	default:
		return false
	}
	params := tool.Get("parameters")
	if params.Type == gjson.Null {
		return false
	}
	if !params.IsObject() || params.Get("type").String() != "object" {
		return true
	}
	for _, key := range []string{"oneOf", "anyOf", "allOf", "enum", "const", "not"} {
		if params.Get(key).Exists() {
			return true
		}
	}
	return false
}
