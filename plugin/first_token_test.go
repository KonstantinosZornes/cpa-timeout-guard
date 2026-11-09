package main

import "testing"

func TestTokenDetectorOpenAIJSONChunks(t *testing.T) {
	detector := newTokenDetector("openai")
	role := []byte(`{"choices":[{"delta":{"role":"assistant"},"finish_reason":null}]}`)
	if detector.observe(role) {
		t.Fatal("role announcement must not satisfy first-token deadline")
	}
	content := []byte(`{"choices":[{"delta":{"content":"hello"},"finish_reason":null}]}`)
	if !detector.observe(content) {
		t.Fatal("text delta must satisfy first-token deadline")
	}
}

func TestTokenDetectorOpenAIJSONSplitAcrossChunks(t *testing.T) {
	detector := newTokenDetector("openai")
	if detector.observe([]byte(`{"choices":[{"delta":{"content":"hel`)) {
		t.Fatal("partial JSON must not be treated as a token event")
	}
	if !detector.observe([]byte(`lo"},"finish_reason":null}]}`)) {
		t.Fatal("completed JSON text delta must satisfy first-token deadline")
	}
}

func TestTokenDetectorSSEMetadataAndToken(t *testing.T) {
	detector := newTokenDetector("openai-response")
	metadata := []byte("data: {\"type\":\"response.created\"}\n\n")
	if detector.observe(metadata) {
		t.Fatal("response metadata must not satisfy first-token deadline")
	}
	token := []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
	if !detector.observe(token) {
		t.Fatal("Responses text delta must satisfy first-token deadline")
	}
}

func TestTokenDetectorIgnoresHeartbeat(t *testing.T) {
	detector := newTokenDetector("claude")
	if detector.observe([]byte(": keep-alive\n\n")) {
		t.Fatal("SSE heartbeat must not satisfy first-token deadline")
	}
}
