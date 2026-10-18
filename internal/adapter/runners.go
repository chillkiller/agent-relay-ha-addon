package adapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ── echo ────────────────────────────────────────────────────────────

type Echo struct{}

func (Echo) ID() string { return "echo" }

func (Echo) Validate(AgentConfig) error { return nil }

func (Echo) Stream(a AgentConfig, history []Message) (<-chan Chunk, error) {
	last := ""
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			last = history[i].Content
			break
		}
	}
	out := make(chan Chunk)
	go func() {
		defer close(out)
		text := fmt.Sprintf("✅ Echo-Agent „%s“ hat die Nachricht im Raum gesehen.\n\n> %s", a.Name, last)
		runes := []rune(text)
		for i := 0; i < len(runes); i += 12 {
			end := i + 12
			if end > len(runes) {
				end = len(runes)
			}
			out <- Chunk{Text: string(runes[i:end])}
			time.Sleep(25 * time.Millisecond)
		}
	}()
	return out, nil
}

// ── openai ──────────────────────────────────────────────────────────

type OpenAI struct {
	Client *http.Client
}

func (OpenAI) ID() string { return "openai" }

func (OpenAI) Validate(a AgentConfig) error {
	if a.BaseURL == "" {
		return errors.New("keine Basis-URL gesetzt (Agent oder Einstellungen)")
	}
	return nil
}

var hdrAuthorization = "Authorization"

func (o OpenAI) Stream(a AgentConfig, history []Message) (<-chan Chunk, error) {
	base := a.BaseURL
	if base == "" {
		return nil, errors.New("keine Basis-URL")
	}
	url := CompletionsURL(base)
	model := a.Model
	if model == "" {
		model = "openclaw"
	}
	out := make(chan Chunk)
	go func() {
		defer close(out)
		err := streamOpenAI(url, a, model, history, out)
		if err != nil {
			out <- Chunk{Err: err}
		}
	}()
	return out, nil
}

func streamOpenAI(url string, a AgentConfig, model string, history []Message, out chan<- Chunk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	body := buildRequestBody(a, model, history)
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if a.APIKey != "" {
		req.Header.Set(hdrAuthorization, "Bearer "+a.APIKey)
	}

	client := &http.Client{Timeout: 170 * time.Second} // < 180s ctx (review M-1)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("verbindung: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf := make([]byte, 400)
		n, _ := resp.Body.Read(buf)
		return fmt.Errorf("endpoint %s: %s", resp.Status, string(buf[:n]))
	}

	// parse SSE stream
	return parseSSE(ctx, resp.Body, func(delta string) {
		out <- Chunk{Text: delta}
	})
}

