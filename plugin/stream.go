package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type hostModelStreamReadResult struct {
	chunk hostModelStreamReadResponse
	err   error
}

type hostModelStreamOpenResult struct {
	response hostModelStreamResponse
	err      error
}

func executeStream(raw []byte) ([]byte, error) {
	var req rpcExecutorRequest
	if errUnmarshal := json.Unmarshal(raw, &req); errUnmarshal != nil {
		return nil, errUnmarshal
	}
	if strings.TrimSpace(req.StreamID) == "" {
		return errorEnvelope("executor_error", "stream_id is required", http.StatusBadRequest), nil
	}
	if !req.Stream {
		return errorEnvelope("executor_error", "stream=true is required", http.StatusBadRequest), nil
	}
	timeout := loadedConfig().timeout()
	if timeout <= 0 {
		return errorEnvelope("executor_error", "first-token timeout is disabled", http.StatusBadRequest), nil
	}
	go runGuardedStream(req, timeout)
	return okEnvelope(map[string]any{
		"headers": http.Header{"Content-Type": []string{"text/event-stream"}},
	})
}

func runGuardedStream(req rpcExecutorRequest, timeout time.Duration) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	timedOut := &atomic.Bool{}
	openC := make(chan hostModelStreamOpenResult, 1)
	go func() {
		opened := openHostModelStream(req)
		openC <- opened
		if timedOut.Load() && strings.TrimSpace(opened.response.StreamID) != "" {
			_ = closeHostModelStream(opened.response.StreamID)
		}
	}()

	var opened hostModelStreamOpenResult
	select {
	case opened = <-openC:
		if errOpen := handleHostModelOpenError(req.StreamID, opened); errOpen != nil {
			return
		}
	case <-deadline.C:
		timedOut.Store(true)
		select {
		case opened = <-openC:
			if strings.TrimSpace(opened.response.StreamID) != "" {
				_ = closeHostModelStream(opened.response.StreamID)
			}
		default:
		}
		closePluginStream(req.StreamID, timeoutError(timeout))
		return
	}

	streamID := strings.TrimSpace(opened.response.StreamID)
	if streamID == "" {
		closePluginStream(req.StreamID, newPluginError("upstream_stream_unavailable", "host model execution returned no stream id", http.StatusBadGateway))
		return
	}
	defer func() { _ = closeHostModelStream(streamID) }()
	if opened.response.StatusCode >= http.StatusBadRequest {
		closePluginStream(req.StreamID, newPluginError("upstream_error", fmt.Sprintf("host model returned status %d", opened.response.StatusCode), opened.response.StatusCode))
		return
	}

	detector := newTokenDetector(req.SourceFormat)
	tokenSeen := false
	deadlineC := deadline.C
	for {
		readC := make(chan hostModelStreamReadResult, 1)
		go func() {
			chunk, errRead := readHostModelStream(streamID)
			readC <- hostModelStreamReadResult{chunk: chunk, err: errRead}
		}()

		var read hostModelStreamReadResult
		if tokenSeen {
			read = <-readC
		} else {
			select {
			case read = <-readC:
			case <-deadlineC:
				_ = closeHostModelStream(streamID)
				closePluginStream(req.StreamID, timeoutError(timeout))
				return
			}
		}
		if read.err != nil {
			closePluginStream(req.StreamID, newPluginError("upstream_stream_error", read.err.Error(), hostHTTPStatus(read.err, http.StatusBadGateway)))
			return
		}
		if read.chunk.Error != "" {
			closePluginStream(req.StreamID, newPluginError("upstream_stream_error", read.chunk.Error, http.StatusBadGateway))
			return
		}
		if len(read.chunk.Payload) > 0 {
			if !tokenSeen && detector.observe(read.chunk.Payload) {
				tokenSeen = true
				stopTimer(deadline)
				deadlineC = nil
			}
			if errEmit := emitPluginStreamChunk(req.StreamID, read.chunk.Payload); errEmit != nil {
				_ = closeHostModelStream(streamID)
				return
			}
		}
		if read.chunk.Done {
			closePluginStream(req.StreamID, nil)
			return
		}
	}
}

func openHostModelStream(req rpcExecutorRequest) hostModelStreamOpenResult {
	body := req.Payload
	if len(body) == 0 {
		body = req.OriginalRequest
	}
	path := ""
	if value, ok := req.Metadata[requestPathMetadataKey].(string); ok {
		path = strings.TrimSpace(value)
	}
	callerScope, _ := req.Metadata[callerScopeMetadataKey].(string)
	request := rpcHostModelExecutionRequest{
		hostModelExecutionRequest: hostModelExecutionRequest{
			EntryProtocol: req.SourceFormat,
			ExitProtocol:  req.SourceFormat,
			Model:         req.Model,
			Stream:        true,
			Body:          body,
			Headers:       req.Headers.Clone(),
			Query:         cloneValues(req.Query),
			Alt:           req.Alt,
			Path:          path,
			CallerScope:   strings.TrimSpace(callerScope),
		},
		HostCallbackID: req.HostCallbackID,
	}
	raw, errCall := callHost(methodHostModelExecuteStream, request)
	if errCall != nil {
		return hostModelStreamOpenResult{err: errCall}
	}
	var response hostModelStreamResponse
	if errDecode := json.Unmarshal(raw, &response); errDecode != nil {
		return hostModelStreamOpenResult{err: fmt.Errorf("decode host model stream response: %w", errDecode)}
	}
	return hostModelStreamOpenResult{response: response}
}

func handleHostModelOpenError(pluginStreamID string, opened hostModelStreamOpenResult) error {
	if opened.err == nil {
		return nil
	}
	closePluginStream(pluginStreamID, newPluginError("upstream_open_error", opened.err.Error(), hostHTTPStatus(opened.err, http.StatusBadGateway)))
	return opened.err
}

func readHostModelStream(streamID string) (hostModelStreamReadResponse, error) {
	raw, errCall := callHost(methodHostModelStreamRead, hostModelStreamReadRequest{StreamID: streamID})
	if errCall != nil {
		return hostModelStreamReadResponse{}, errCall
	}
	var chunk hostModelStreamReadResponse
	if errDecode := json.Unmarshal(raw, &chunk); errDecode != nil {
		return hostModelStreamReadResponse{}, fmt.Errorf("decode host model stream chunk: %w", errDecode)
	}
	return chunk, nil
}

func closeHostModelStream(streamID string) error {
	_, errCall := callHost(methodHostModelStreamClose, hostModelStreamCloseRequest{StreamID: streamID})
	return errCall
}

func emitPluginStreamChunk(streamID string, payload []byte) error {
	_, errCall := callHost(methodHostStreamEmit, map[string]any{
		"stream_id": streamID,
		"payload":   payload,
	})
	return errCall
}

func closePluginStream(streamID string, detail *pluginError) {
	if strings.TrimSpace(streamID) == "" {
		return
	}
	_, _ = callHost(methodHostStreamClose, rpcStreamCloseRequest{
		StreamID:    streamID,
		ErrorDetail: detail,
	})
}

func timeoutError(timeout time.Duration) *pluginError {
	return newPluginError("first_token_timeout", fmt.Sprintf("no meaningful output token received within %s", timeout), http.StatusGatewayTimeout)
}

func stopTimer(timer *time.Timer) {
	if timer == nil || timer.Stop() {
		return
	}
	select {
	case <-timer.C:
	default:
	}
}

func cloneValues(src map[string][]string) map[string][]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string][]string, len(src))
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	return dst
}
