# Changelog

## 0.1.1

- Align dashboard credential lookup with CPAStickyProxy's remembered `cli-proxy-auth` storage handling.
- Add a management page title bar and guidance when no remembered management key is available.
- Translate the management dashboard into Simplified Chinese, Traditional Chinese, English, and Russian.
- Detect raw JSON chunks as well as SSE events when identifying the first token, and stop the timer immediately after that token.

## Unreleased

- Add a configurable first meaningful token deadline for streaming HTTP model requests.
- Add an authenticated settings API and embedded management dashboard.
- Add cross-platform GitHub release packaging with SHA-256 checksums.
