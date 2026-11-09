package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

//go:embed management-ui/dashboard.html
var managementFiles embed.FS

const (
	managementSettingsPath  = "/v0/management/cpa-timeout-guard/settings"
	managementDashboardPath = "/v0/resource/plugins/cpa-timeout-guard/dashboard"
)

type settingsUpdate struct {
	Enabled           *bool  `json:"enabled"`
	FirstTokenTimeout string `json:"first_token_timeout"`
}

func managementRegistrationResponse() managementRegistration {
	return managementRegistration{
		Routes: []managementRoute{
			{Method: http.MethodGet, Path: "/cpa-timeout-guard/settings"},
			{Method: http.MethodPut, Path: "/cpa-timeout-guard/settings"},
			{Method: http.MethodDelete, Path: "/cpa-timeout-guard/settings"},
		},
		Resources: []resourceRoute{{
			Path:        "/dashboard",
			Menu:        "CPA Timeout Guard",
			Description: "Configure request deadlines for CPA model streams.",
		}},
	}
}

func handleManagement(raw []byte) ([]byte, error) {
	var req managementRequest
	if errDecode := json.Unmarshal(raw, &req); errDecode != nil {
		return managementJSON(http.StatusBadRequest, map[string]any{"ok": false, "code": "invalid_request"})
	}
	if req.Method == http.MethodGet && req.Path == managementDashboardPath {
		page, errRead := managementFiles.ReadFile("management-ui/dashboard.html")
		if errRead != nil {
			return managementJSON(http.StatusInternalServerError, map[string]any{"ok": false, "code": "dashboard_unavailable"})
		}
		return marshalManagement(managementResponse{
			StatusCode: http.StatusOK,
			Headers: map[string][]string{
				"Content-Type":            {"text/html; charset=utf-8"},
				"Cache-Control":           {"no-store"},
				"X-Content-Type-Options":  {"nosniff"},
				"Referrer-Policy":         {"no-referrer"},
				"Content-Security-Policy": {"default-src 'self'; connect-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'; form-action 'self'; frame-ancestors 'self'"},
			},
			Body: page,
		})
	}
	if req.Path != managementSettingsPath {
		return managementJSON(http.StatusNotFound, map[string]any{"ok": false, "code": "route_not_found"})
	}
	switch strings.ToUpper(strings.TrimSpace(req.Method)) {
	case http.MethodGet:
		cfg := loadedConfig()
		return managementJSON(http.StatusOK, map[string]any{
			"ok":                  true,
			"enabled":             cfg.Enabled,
			"first_token_timeout": cfg.FirstTokenTimeout,
			"source":              settingsSource(),
		})
	case http.MethodPut:
		var update settingsUpdate
		if errDecode := json.Unmarshal(req.Body, &update); errDecode != nil || update.Enabled == nil {
			return managementJSON(http.StatusBadRequest, map[string]any{"ok": false, "code": "invalid_settings"})
		}
		update.FirstTokenTimeout = strings.TrimSpace(update.FirstTokenTimeout)
		if update.FirstTokenTimeout == "" {
			return managementJSON(http.StatusBadRequest, map[string]any{"ok": false, "code": "timeout_required"})
		}
		if _, errParse := time.ParseDuration(update.FirstTokenTimeout); errParse != nil {
			return managementJSON(http.StatusBadRequest, map[string]any{"ok": false, "code": "invalid_timeout"})
		}
		cfg := pluginConfig{Enabled: *update.Enabled, FirstTokenTimeout: update.FirstTokenTimeout}
		if errSave := saveSettingsOverride(cfg); errSave != nil {
			return managementJSON(http.StatusInternalServerError, map[string]any{"ok": false, "code": "settings_save_failed"})
		}
		return managementJSON(http.StatusOK, map[string]any{
			"ok":                  true,
			"enabled":             cfg.Enabled,
			"first_token_timeout": cfg.FirstTokenTimeout,
			"source":              settingsSource(),
		})
	case http.MethodDelete:
		if errReset := resetSettingsOverride(); errReset != nil {
			return managementJSON(http.StatusInternalServerError, map[string]any{"ok": false, "code": "settings_reset_failed"})
		}
		cfg := loadedConfig()
		return managementJSON(http.StatusOK, map[string]any{
			"ok":                  true,
			"enabled":             cfg.Enabled,
			"first_token_timeout": cfg.FirstTokenTimeout,
			"source":              settingsSource(),
		})
	default:
		return managementJSON(http.StatusMethodNotAllowed, map[string]any{"ok": false, "code": "method_not_allowed"})
	}
}

func managementJSON(status int, payload any) ([]byte, error) {
	body, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return nil, fmt.Errorf("encode management response: %w", errMarshal)
	}
	return marshalManagement(managementResponse{
		StatusCode: status,
		Headers: map[string][]string{
			"Content-Type":  {"application/json; charset=utf-8"},
			"Cache-Control": {"no-store"},
		},
		Body: body,
	})
}

func marshalManagement(response managementResponse) ([]byte, error) {
	raw, errMarshal := json.Marshal(response)
	if errMarshal != nil {
		return nil, fmt.Errorf("encode management response envelope: %w", errMarshal)
	}
	return okEnvelopeRaw(raw)
}

func okEnvelopeRaw(result []byte) ([]byte, error) {
	return json.Marshal(rpcEnvelope{OK: true, Result: result})
}
