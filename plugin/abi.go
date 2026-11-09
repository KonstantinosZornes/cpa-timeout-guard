package main

import (
	"encoding/json"
	"net/http"
	"net/url"
)

const (
	nativeABIVersion uint32 = 1
	schemaVersion    uint32 = 10

	methodPluginRegister     = "plugin.register"
	methodPluginReconfigure  = "plugin.reconfigure"
	methodManagementRegister = "management.register"
	methodManagementHandle   = "management.handle"
	methodModelRoute         = "model.route"
	methodExecutorIdentifier = "executor.identifier"
	methodExecutorExecute    = "executor.execute"
	methodExecutorStream     = "executor.execute_stream"
	methodExecutorCount      = "executor.count_tokens"

	methodHostModelExecuteStream = "host.model.execute_stream"
	methodHostModelStreamRead    = "host.model.stream_read"
	methodHostModelStreamClose   = "host.model.stream_close"
	methodHostStreamEmit         = "host.stream.emit"
	methodHostStreamClose        = "host.stream.close"

	callerScopeMetadataKey = "caller_scope"
	requestPathMetadataKey = "request_path"
)

type rpcEnvelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *pluginError    `json:"error,omitempty"`
}

type pluginError struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	Retryable         bool   `json:"retryable,omitempty"`
	HTTPStatus        int    `json:"http_status,omitempty"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
}

func newPluginError(code, message string, status int) *pluginError {
	return &pluginError{Code: code, Message: message, HTTPStatus: status}
}

func (e *pluginError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *pluginError) StatusCode() int {
	if e == nil {
		return 0
	}
	return e.HTTPStatus
}

type registration struct {
	SchemaVersion uint32                 `json:"schema_version"`
	Metadata      pluginMetadata         `json:"metadata"`
	Capabilities  registrationCapability `json:"capabilities"`
}

type pluginMetadata struct {
	Name             string              `json:"Name"`
	Version          string              `json:"Version"`
	Author           string              `json:"Author"`
	GitHubRepository string              `json:"GitHubRepository,omitempty"`
	ConfigFields     []pluginConfigField `json:"ConfigFields,omitempty"`
}

type pluginConfigField struct {
	Name        string `json:"Name"`
	Type        string `json:"Type"`
	Default     any    `json:"Default,omitempty"`
	Description string `json:"Description"`
}

type registrationCapability struct {
	ModelRouter           bool     `json:"model_router"`
	Executor              bool     `json:"executor"`
	ManagementAPI         bool     `json:"management_api"`
	ExecutorModelScope    string   `json:"executor_model_scope"`
	ExecutorInputFormats  []string `json:"executor_input_formats"`
	ExecutorOutputFormats []string `json:"executor_output_formats"`
}

type managementRegistration struct {
	Routes    []managementRoute `json:"routes,omitempty"`
	Resources []resourceRoute   `json:"resources,omitempty"`
}

type managementRoute struct {
	Method string `json:"Method"`
	Path   string `json:"Path"`
}

type resourceRoute struct {
	Path        string `json:"Path"`
	Menu        string `json:"Menu"`
	Description string `json:"Description"`
}

type managementRequest struct {
	Method  string              `json:"Method"`
	Path    string              `json:"Path"`
	Headers map[string][]string `json:"Headers"`
	Query   map[string][]string `json:"Query"`
	Body    []byte              `json:"Body"`
}

type managementResponse struct {
	StatusCode int                 `json:"StatusCode"`
	Headers    map[string][]string `json:"Headers"`
	Body       []byte              `json:"Body"`
}

type modelRouteRequest struct {
	Plugin             pluginMetadata `json:"Plugin"`
	PluginID           string         `json:"PluginID"`
	SourceFormat       string         `json:"SourceFormat"`
	RequestedModel     string         `json:"RequestedModel"`
	Stream             bool           `json:"Stream"`
	Headers            http.Header    `json:"Headers"`
	Query              url.Values     `json:"Query"`
	Body               []byte         `json:"Body"`
	Metadata           map[string]any `json:"Metadata"`
	AvailableProviders []string       `json:"AvailableProviders"`
}

type rpcModelRouteRequest struct {
	modelRouteRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type modelRouteResponse struct {
	Handled    bool   `json:"Handled"`
	TargetKind string `json:"TargetKind"`
	Target     string `json:"Target,omitempty"`
	Reason     string `json:"Reason,omitempty"`
}

type executorRequest struct {
	AuthID          string            `json:"AuthID"`
	AuthProvider    string            `json:"AuthProvider"`
	Model           string            `json:"Model"`
	Format          string            `json:"Format"`
	Stream          bool              `json:"Stream"`
	Alt             string            `json:"Alt"`
	Headers         http.Header       `json:"Headers"`
	Query           url.Values        `json:"Query"`
	OriginalRequest []byte            `json:"OriginalRequest"`
	SourceFormat    string            `json:"SourceFormat"`
	Payload         []byte            `json:"Payload"`
	Metadata        map[string]any    `json:"Metadata"`
	StorageJSON     []byte            `json:"StorageJSON"`
	AuthMetadata    map[string]any    `json:"AuthMetadata"`
	AuthAttributes  map[string]string `json:"AuthAttributes"`
}

type rpcExecutorRequest struct {
	executorRequest
	StreamID       string `json:"stream_id,omitempty"`
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type executorResponse struct {
	Payload  []byte         `json:"Payload"`
	Headers  http.Header    `json:"Headers"`
	Metadata map[string]any `json:"Metadata,omitempty"`
}

type hostModelExecutionRequest struct {
	EntryProtocol  string      `json:"entry_protocol"`
	ExitProtocol   string      `json:"exit_protocol"`
	Model          string      `json:"model"`
	Stream         bool        `json:"stream"`
	Body           []byte      `json:"body"`
	Headers        http.Header `json:"headers"`
	Query          url.Values  `json:"query"`
	Alt            string      `json:"alt,omitempty"`
	ForcedProvider string      `json:"forced_provider,omitempty"`
	AuthID         string      `json:"auth_id,omitempty"`
	Path           string      `json:"path,omitempty"`
	CallerScope    string      `json:"caller_scope,omitempty"`
}

type rpcHostModelExecutionRequest struct {
	hostModelExecutionRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type hostModelStreamResponse struct {
	StatusCode int         `json:"status_code"`
	Headers    http.Header `json:"headers"`
	StreamID   string      `json:"stream_id"`
}

type hostModelStreamReadRequest struct {
	StreamID string `json:"stream_id"`
}

type hostModelStreamReadResponse struct {
	Payload []byte `json:"payload"`
	Error   string `json:"error"`
	Done    bool   `json:"done"`
}

type hostModelStreamCloseRequest struct {
	StreamID string `json:"stream_id"`
}

type rpcStreamEmitRequest struct {
	StreamID    string       `json:"stream_id"`
	Payload     []byte       `json:"payload,omitempty"`
	ErrorDetail *pluginError `json:"error_detail,omitempty"`
}

type rpcStreamCloseRequest struct {
	StreamID    string       `json:"stream_id"`
	ErrorDetail *pluginError `json:"error_detail,omitempty"`
}
