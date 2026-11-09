package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func routeModel(raw []byte) ([]byte, error) {
	var req rpcModelRouteRequest
	if errUnmarshal := json.Unmarshal(raw, &req); errUnmarshal != nil {
		return nil, errUnmarshal
	}
	cfg := loadedConfig()
	if !cfg.Enabled || cfg.timeout() <= 0 || !req.Stream || isWebSocketRequest(req.Headers) || !supportsFormat(req.SourceFormat) {
		return okEnvelope(modelRouteResponse{Handled: false})
	}
	return okEnvelope(modelRouteResponse{
		Handled:    true,
		TargetKind: "self",
		Reason:     "first_token_timeout_guard",
	})
}

func supportsFormat(raw string) bool {
	format := strings.ToLower(strings.TrimSpace(raw))
	for _, supported := range supportedFormats() {
		if format == supported {
			return true
		}
	}
	return false
}

func isWebSocketRequest(headers http.Header) bool {
	return strings.EqualFold(strings.TrimSpace(headers.Get("Upgrade")), "websocket")
}
