package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	pluginIdentifier   = "cpa-timeout-guard"
	defaultTimeoutText = "30s"
	settingsStatePath  = "plugins/cpa-timeout-guard.json"
)

var currentConfig atomic.Value
var baseConfig atomic.Value
var hasSettingsOverride atomic.Bool
var configMu sync.Mutex

type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}

type pluginConfig struct {
	Enabled           bool   `yaml:"enabled" json:"enabled"`
	FirstTokenTimeout string `yaml:"first_token_timeout" json:"first_token_timeout"`
}

func defaultPluginConfig() pluginConfig {
	return pluginConfig{FirstTokenTimeout: defaultTimeoutText}
}

func configure(raw []byte) error {
	var lifecycle lifecycleRequest
	if len(raw) > 0 {
		if errDecode := json.Unmarshal(raw, &lifecycle); errDecode != nil {
			return fmt.Errorf("decode lifecycle request: %w", errDecode)
		}
	}
	cfg := defaultPluginConfig()
	if len(lifecycle.ConfigYAML) > 0 {
		if errDecode := yaml.Unmarshal(lifecycle.ConfigYAML, &cfg); errDecode != nil {
			return fmt.Errorf("decode plugin config: %w", errDecode)
		}
	}
	cfg.FirstTokenTimeout = strings.TrimSpace(cfg.FirstTokenTimeout)
	if cfg.FirstTokenTimeout == "" {
		cfg.FirstTokenTimeout = defaultTimeoutText
	}
	if _, errParse := time.ParseDuration(cfg.FirstTokenTimeout); errParse != nil {
		return fmt.Errorf("invalid first_token_timeout %q: %w", cfg.FirstTokenTimeout, errParse)
	}
	configMu.Lock()
	defer configMu.Unlock()
	baseConfig.Store(cfg)
	active := cfg
	if saved, ok := readSettingsOverride(); ok {
		active = saved
		hasSettingsOverride.Store(true)
	} else {
		hasSettingsOverride.Store(false)
	}
	currentConfig.Store(active)
	return nil
}

func loadedConfig() pluginConfig {
	if raw := currentConfig.Load(); raw != nil {
		if cfg, ok := raw.(pluginConfig); ok {
			return cfg
		}
	}
	return defaultPluginConfig()
}

func (cfg pluginConfig) timeout() time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(cfg.FirstTokenTimeout))
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

func saveSettingsOverride(cfg pluginConfig) error {
	cfg.FirstTokenTimeout = strings.TrimSpace(cfg.FirstTokenTimeout)
	if cfg.FirstTokenTimeout == "" {
		return fmt.Errorf("first_token_timeout is required")
	}
	if _, errParse := time.ParseDuration(cfg.FirstTokenTimeout); errParse != nil {
		return fmt.Errorf("invalid first_token_timeout %q: %w", cfg.FirstTokenTimeout, errParse)
	}
	encoded, errMarshal := json.Marshal(cfg)
	if errMarshal != nil {
		return fmt.Errorf("encode settings: %w", errMarshal)
	}
	configMu.Lock()
	defer configMu.Unlock()
	if errMkdir := os.MkdirAll(filepath.Dir(settingsStatePath), 0o700); errMkdir != nil {
		return fmt.Errorf("create plugin settings directory: %w", errMkdir)
	}
	tmpPath := settingsStatePath + ".tmp"
	if errWrite := os.WriteFile(tmpPath, encoded, 0o600); errWrite != nil {
		return fmt.Errorf("write plugin settings: %w", errWrite)
	}
	if errChmod := os.Chmod(tmpPath, 0o600); errChmod != nil {
		return fmt.Errorf("secure plugin settings: %w", errChmod)
	}
	if errRename := os.Rename(tmpPath, settingsStatePath); errRename != nil {
		_ = os.Remove(settingsStatePath)
		if errRetry := os.Rename(tmpPath, settingsStatePath); errRetry != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("replace plugin settings: %w", errRetry)
		}
	}
	currentConfig.Store(cfg)
	hasSettingsOverride.Store(true)
	return nil
}

func resetSettingsOverride() error {
	configMu.Lock()
	defer configMu.Unlock()
	if errRemove := os.Remove(settingsStatePath); errRemove != nil && !os.IsNotExist(errRemove) {
		return fmt.Errorf("remove plugin settings override: %w", errRemove)
	}
	if base := baseConfig.Load(); base != nil {
		currentConfig.Store(base.(pluginConfig))
	}
	hasSettingsOverride.Store(false)
	return nil
}

func readSettingsOverride() (pluginConfig, bool) {
	raw, errRead := os.ReadFile(settingsStatePath)
	if errRead != nil {
		return pluginConfig{}, false
	}
	var cfg pluginConfig
	if errDecode := json.Unmarshal(raw, &cfg); errDecode != nil {
		return pluginConfig{}, false
	}
	cfg.FirstTokenTimeout = strings.TrimSpace(cfg.FirstTokenTimeout)
	if cfg.FirstTokenTimeout == "" {
		return pluginConfig{}, false
	}
	if _, errParse := time.ParseDuration(cfg.FirstTokenTimeout); errParse != nil {
		return pluginConfig{}, false
	}
	return cfg, true
}

func settingsSource() string {
	if hasSettingsOverride.Load() {
		return "plugin_state"
	}
	return "config_yaml"
}

func pluginRegistration() registration {
	formats := supportedFormats()
	return registration{
		SchemaVersion: schemaVersion,
		Metadata: pluginMetadata{
			Name:             "CPA Timeout Guard",
			Version:          pluginVersion,
			Author:           "KonstantinosZornes",
			GitHubRepository: "https://github.com/KonstantinosZornes/cpa-timeout-guard",
			ConfigFields: []pluginConfigField{
				{Name: "enabled", Type: "boolean", Default: false, Description: "Enable the first-token deadline for streaming HTTP requests."},
				{Name: "first_token_timeout", Type: "string", Default: defaultTimeoutText, Description: "Maximum elapsed time from delegated model execution start to the first meaningful token."},
			},
		},
		Capabilities: registrationCapability{
			ModelRouter:           true,
			Executor:              true,
			ManagementAPI:         true,
			ExecutorModelScope:    "static",
			ExecutorInputFormats:  formats,
			ExecutorOutputFormats: formats,
		},
	}
}

func supportedFormats() []string {
	return []string{
		"openai",
		"openai-response",
		"claude",
		"gemini",
		"codex",
		"antigravity",
		"interactions",
	}
}
