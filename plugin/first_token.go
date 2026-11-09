package main

import (
	"bytes"
	"encoding/json"
	"strings"
)

const maxSSEEventBytes = 1 << 20

type tokenDetector struct {
	format string
	buffer []byte
}

func newTokenDetector(format string) *tokenDetector {
	return &tokenDetector{format: strings.ToLower(strings.TrimSpace(format))}
}

func (d *tokenDetector) observe(chunk []byte) bool {
	if d == nil || len(chunk) == 0 {
		return false
	}
	d.buffer = append(d.buffer, chunk...)
	for {
		start, width := findEventBoundary(d.buffer)
		if start < 0 {
			if len(d.buffer) > maxSSEEventBytes {
				d.buffer = nil
			}
			return false
		}
		event := bytes.Clone(d.buffer[:start])
		d.buffer = d.buffer[start+width:]
		if meaningfulTokenEvent(d.format, event) {
			return true
		}
	}
}

func findEventBoundary(payload []byte) (int, int) {
	lf := bytes.Index(payload, []byte("\n\n"))
	crlf := bytes.Index(payload, []byte("\r\n\r\n"))
	if lf < 0 && crlf < 0 {
		return -1, 0
	}
	if crlf >= 0 && (lf < 0 || crlf < lf) {
		return crlf, 4
	}
	return lf, 2
}

func meaningfulTokenEvent(format string, event []byte) bool {
	data := sseData(event)
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return false
	}
	var root map[string]any
	if errUnmarshal := json.Unmarshal(data, &root); errUnmarshal != nil || root == nil {
		return false
	}
	switch format {
	case "openai":
		return openAITokenEvent(root)
	case "openai-response", "codex":
		return responsesTokenEvent(root)
	case "claude":
		return claudeTokenEvent(root)
	case "gemini", "antigravity":
		return geminiTokenEvent(root)
	case "interactions":
		return interactionsTokenEvent(root)
	default:
		return false
	}
}

func sseData(event []byte) []byte {
	var lines [][]byte
	for _, line := range bytes.Split(event, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		value := bytes.TrimPrefix(line, []byte("data:"))
		value = bytes.TrimPrefix(value, []byte(" "))
		lines = append(lines, value)
	}
	if len(lines) == 0 {
		trimmed := bytes.TrimSpace(event)
		if json.Valid(trimmed) {
			return bytes.Clone(trimmed)
		}
		return nil
	}
	return bytes.Join(lines, []byte("\n"))
}

func openAITokenEvent(root map[string]any) bool {
	choices, _ := root["choices"].([]any)
	for _, rawChoice := range choices {
		choice, _ := rawChoice.(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		for _, key := range []string{"content", "reasoning_content", "refusal", "function_call"} {
			if hasMeaningfulValue(delta[key]) {
				return true
			}
		}
		if hasMeaningfulValue(delta["tool_calls"]) {
			return true
		}
	}
	return false
}

func responsesTokenEvent(root map[string]any) bool {
	typeName, _ := root["type"].(string)
	if !strings.Contains(typeName, ".delta") {
		return false
	}
	return hasMeaningfulValue(root["delta"]) || hasMeaningfulValue(root["text"])
}

func claudeTokenEvent(root map[string]any) bool {
	typeName, _ := root["type"].(string)
	switch typeName {
	case "content_block_delta":
		delta, _ := root["delta"].(map[string]any)
		return hasMeaningfulValue(delta["text"]) || hasMeaningfulValue(delta["thinking"]) || hasMeaningfulValue(delta["partial_json"])
	case "content_block_start":
		block, _ := root["content_block"].(map[string]any)
		return hasMeaningfulValue(block["text"]) || hasMeaningfulValue(block["name"])
	default:
		return false
	}
}

func geminiTokenEvent(root map[string]any) bool {
	candidates, _ := root["candidates"].([]any)
	for _, rawCandidate := range candidates {
		candidate, _ := rawCandidate.(map[string]any)
		content, _ := candidate["content"].(map[string]any)
		parts, _ := content["parts"].([]any)
		for _, rawPart := range parts {
			part, _ := rawPart.(map[string]any)
			if hasMeaningfulValue(part["text"]) || hasMeaningfulValue(part["functionCall"]) || hasMeaningfulValue(part["function_call"]) {
				return true
			}
		}
	}
	return false
}

func interactionsTokenEvent(root map[string]any) bool {
	eventType, _ := root["event_type"].(string)
	if eventType == "step.delta" {
		delta, _ := root["delta"].(map[string]any)
		return hasMeaningfulValue(delta["text"]) || hasMeaningfulValue(delta["arguments"]) || hasMeaningfulValue(delta["content"])
	}
	return false
}

func hasMeaningfulValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		for _, item := range typed {
			if hasMeaningfulValue(item) {
				return true
			}
		}
	case map[string]any:
		for key, item := range typed {
			if key != "index" && key != "id" && key != "type" && hasMeaningfulValue(item) {
				return true
			}
		}
	case float64, bool:
		return true
	}
	return false
}
