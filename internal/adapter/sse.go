package adapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// buildRequestBody creates the OpenAI chat completions request JSON body.
func buildRequestBody(a AgentConfig, model string, history []Message) io.Reader {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	msgs := []message{}
	if a.SystemPrompt != "" {
		msgs = append(msgs, message{Role: "system", Content: a.SystemPrompt})
	}
	for _, m := range history {
		content := m.Content
		if m.Role == "assistant" && a.ID != "" && m.AgentID == a.ID {
			continue // stable id match (review-2 M-2), never display names
		}
		msgs = append(msgs, message{Role: m.Role, Content: content})
	}
	req := map[string]any{
		"model":    model,
		"messages": msgs,
		"stream":   true,
	}
	// agent identity for session binding on gateways that support it
	body, _ := json.Marshal(req)
	return strings.NewReader(string(body))
}

// parseSSE parses an OpenAI SSE stream and calls back for each delta.
func parseSSE(ctx context.Context, body io.Reader, onDelta func(string)) error {
	r := bufio.NewReaderSize(body, 4096)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimRight(line, "\n\r")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			return nil
		}
		var frame struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			continue // skip malformed keepalives etc.
		}
		if frame.Error != nil && frame.Error.Message != "" {
			return fmt.Errorf("llm: %s", frame.Error.Message)
		}
		if len(frame.Choices) > 0 && frame.Choices[0].Delta.Content != "" {
			onDelta(frame.Choices[0].Delta.Content)
		}
	}
}