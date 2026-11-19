package common

import (
	"fmt"
	"io"
	"regexp"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

var samplingModelPattern = regexp.MustCompile(`(?i)(?:^|[/.:])(?:claude(?:[-.:/]|$)|gpt-([0-9]+)(?:[-.@:/]|$))`)

// PrepareModelRequestBody applies model controls after conversion and overrides,
// before HTTP transport or provider signing. Explicit passthrough keeps native
// reasoning fields; the existing mandatory sampling policy still applies.
// The caller retains ownership of body and must also close the returned closer.
func PrepareModelRequestBody(body io.Reader, info *RelayInfo) (io.Reader, io.Closer, error) {
	closer := io.NopCloser(body)
	if info == nil || info.ChannelMeta == nil {
		return body, closer, nil
	}
	normalizeReasoning := false
	if !model_setting.GetGlobalSettings().PassThroughRequestEnabled && !info.ChannelSetting.PassThroughBodyEnabled && info.ChannelType != constant.ChannelTypeOpenRouter {
		switch info.GetFinalRequestRelayFormat() {
		case types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, types.RelayFormatClaude, types.RelayFormatGemini:
			model := info.UpstreamModelName
			if model == "" {
				model = info.OriginModelName
			}
			normalizeReasoning = reasoning.RenderOpenAI(model, reasoning.Intent{}).Known
		}
	}
	dropSampling, dropStop := false, false
	for _, model := range []string{info.UpstreamModelName, info.OriginModelName} {
		match := samplingModelPattern.FindStringSubmatch(model)
		if match == nil {
			continue
		}
		if match[1] == "" {
			dropSampling = true
			continue
		}
		major, err := strconv.Atoi(match[1])
		if err == nil && major >= 5 {
			dropSampling, dropStop = true, true
		}
	}
	if !dropSampling && !normalizeReasoning {
		return body, closer, nil
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, nil, err
	}
	var request map[string]common.RawMessage
	if err := common.Unmarshal(data, &request); err != nil {
		return nil, nil, fmt.Errorf("filter model sampling parameters: %w", err)
	}
	if request == nil {
		return nil, nil, fmt.Errorf("filter model sampling parameters: expected a JSON object")
	}
	if normalizeReasoning {
		if err := normalizeModelReasoning(request, info); err != nil {
			return nil, nil, reasoning.AsClientError(err)
		}
	}
	// ponytail: one mandatory policy for these model families, without per-channel switches.
	if dropSampling {
		for _, key := range []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "seed", "logit_bias", "logprobs", "top_logprobs"} {
			delete(request, key)
		}
	}
	if dropStop {
		delete(request, "stop")
	}
	data, err = common.Marshal(request)
	if err != nil {
		return nil, nil, err
	}
	return NewOutboundJSONBody(data)
}

// NewOutboundJSONBody wraps the already-marshaled upstream request body into a
// BodyStorage. When disk cache is enabled and the payload exceeds the configured
// threshold, the data is written to a temp file and the original []byte can be
// GC'd, significantly reducing the heap residency while waiting for the
// upstream provider to respond (the dominant cost for large base64 payloads).
//
// In memory mode the underlying memoryStorage reuses the same backing array,
// so this is equivalent to bytes.NewReader(data) in terms of memory usage.
//
// The caller MUST invoke closer.Close() once the upstream call has finished
// (typically via defer) to release the disk file / memory accounting.
//
// The returned body exposes its size and replay capability without exposing
// io.Closer. Request construction uses that metadata to populate ContentLength
// and GetBody, while the caller retains ownership of the underlying storage
// through the separately returned closer.
func NewOutboundJSONBody(data []byte) (body common.ReplayableBody, closer io.Closer, err error) {
	storage, err := common.CreateBodyStorage(data)
	if err != nil {
		return nil, nil, err
	}
	return common.NewReplayableBodyReader(storage), storage, nil
}
