# CPA Timeout Guard

[![Release](https://img.shields.io/github/v/release/KonstantinosZornes/cpa-timeout-guard?display_name=tag)](https://github.com/KonstantinosZornes/cpa-timeout-guard/releases)
[![License](https://img.shields.io/github/license/KonstantinosZornes/cpa-timeout-guard)](LICENSE)

CPA Timeout Guard is a native plugin for [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI). It provides configurable request deadlines. The first supported deadline limits time to the first meaningful token for streaming HTTP model requests.

The plugin starts its timer as delegated model execution begins, before CPA selects a credential and opens the upstream stream. The timer stops when the output contains the first protocol-aware token event. Later generation is not time-limited.

## Current deadline: first token

`first_token_timeout` applies to streaming HTTP requests in OpenAI Chat Completions, OpenAI Responses/Codex, Claude, Gemini, Antigravity, and Interactions formats.

- Setup events, role announcements, response metadata, and SSE heartbeats do not satisfy the deadline.
- Text, reasoning, refusal, tool-argument, and supported multimodal output deltas satisfy it.
- The plugin forwards stream bytes unchanged, including metadata that arrives before the first token.
- It closes the delegated model stream when the deadline expires.
- Non-streaming requests and WebSocket upgrades stay on CPA's normal path.

If the deadline expires before response bytes are committed, the caller receives HTTP 504. If metadata or keep-alives have already started the stream, CPA reports an in-stream error.

## Compatibility

The plugin delegates model execution through CPA's `host.model.execute_stream` callback, preserving built-in credential selection and protocol translation. CPA must include callback-scope cleanup for an in-flight host model stream so timeout cancellation also works while the upstream request is waiting for response headers. The required host change is in `internal/pluginhost/host_model_stream_callbacks.go` and `internal/pluginhost/model_stream_bridge.go` in the CPA source tree.

Plugin executor routing is unavailable when CLIProxyAPIHome mode is enabled. The plugin does not wrap WebSocket upgrades.

## Build from source

The plugin is a standard Go module. Build from `plugin/` with Go 1.26.10+ and a C compiler:

```sh
cd plugin
go mod download
CGO_ENABLED=1 go build -buildmode=c-shared -trimpath \
  -ldflags "-s -w" \
  -o ../dist/cpa-timeout-guard.so .
```

Use `.dylib` on macOS and `.dll` on Windows. Build on the target operating system and architecture, or download a release asset. The management page source is `plugin/management-ui/dashboard.html`; Go embeds it directly into the native library.

## Configure

Add the plugin library to CLIProxyAPI's `config.yaml`:

```yaml
plugins:
  enabled: true
  path:
    - /absolute/path/to/cpa-timeout-guard.so
  configs:
    cpa-timeout-guard:
      enabled: true
      priority: 100
      first_token_timeout: 30s
```

`first_token_timeout` accepts Go duration strings such as `2s`, `1500ms`, or `1m`. It defaults to `30s`. A value of `0` or a negative duration disables the guard.

### Management dashboard

The plugin registers a **CPA Timeout Guard** dashboard in CPA's plugin menu. From the dashboard you can enable or disable the guard, adjust the deadline, and restore the values from `config.yaml`.

Dashboard changes are stored in `plugins/cpa-timeout-guard.json` with owner-only file permissions. They override the plugin's YAML values until you restore the YAML settings. The embedded page receives the active management session from CPAMC through a same-origin message, including when “remember password” is off. It also supports CPA's remembered `cli-proxy-auth` storage format. The plugin never persists the management credential.


## Install from the plugin store

Add this registry source in CLIProxyAPI's plugin store settings, refresh the catalog, then install **CPA Timeout Guard** (`cpa-timeout-guard`):

```text
https://raw.githubusercontent.com/KonstantinosZornes/cpa-timeout-guard/main/registry.json
```

## GitHub releases

Publish a GitHub Release with a semantic version tag such as `v0.1.0`. GitHub Actions builds Linux amd64/arm64, macOS amd64/arm64, and Windows amd64 libraries. It attaches Plugin Store-compatible ZIP archives and a `checksums.txt` file to the release. Linux builds target GLIBC 2.17.

The same workflow can be rerun from **Actions → Release CPA Timeout Guard plugin → Run workflow** by entering an existing release tag.
