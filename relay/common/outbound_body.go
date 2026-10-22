package common

import (
	"fmt"
	"io"
	"regexp"
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

var samplingModelPattern = regexp.MustCompile(`(?i)(?:^|[/.:])(?:claude(?:[-.:/]|$)|gpt-([0-9]+)(?:[-.@:/]|$))`)

// FilterModelSamplingBody applies the gateway policy after conversion and overrides,
// including passthrough bodies, before HTTP transport or provider signing.
// The caller retains ownership of body and must also close the returned closer.
func FilterModelSamplingBody(body io.Reader, info *RelayInfo) (io.Reader, io.Closer, error) {
	closer := io.NopCloser(body)
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
	if !dropSampling {
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
	// ponytail: one mandatory policy for these model families, without per-channel switches.
	for _, key := range []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "seed", "logit_bias", "logprobs", "top_logprobs"} {
		delete(request, key)
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
