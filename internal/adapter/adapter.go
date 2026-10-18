// Package adapter: pluggable agent runners.
package adapter

import "strings"

// Message is one turn in the room history fed to a runner.
type Message struct {
	Role    string // "user" | "assistant" | "system"
	Name    string // sender display name (best-effort support)
	Content string
}

// Runner yields response tokens for a room turn.
type Runner func(yield func(string) error) error // not used; kept for reference

// Streamer produces response chunks for the given history.
type Streamer interface {
	// Stream sends chunks on the channel; error terminates with failure.
	Stream(history []Message) (<-chan Chunk, error)
}

// Chunk is a delta or an error signal mid-stream.
type Chunk struct {
	Text string
	Err  error
}

// Adapter turns an agent config into a Streamer.
type Adapter interface {
	ID() string
	// Validate gets the agent config; returns error for missing fields.
	Validate(a AgentConfig) error
	// Stream runs a turn with agent config + room history.
	Stream(a AgentConfig, history []Message) (<-chan Chunk, error)
}

// AgentConfig is the agent-facing configuration.
type AgentConfig struct {
	ID           string
	Name         string
	SystemPrompt string
	Model        string
	BaseURL      string
	APIKey       string
	// IdempotencyKey (optional) derives retry-safe run ids (openclaw adapter).
	IdempotencyKey string
}

// Settings are resolved global settings snapshot.
type Settings struct {
	RelayName       string
	OpenclawBaseURL string
	OpenclawToken   string
}

func normalizeBase(b string) string {
	b = strings.TrimRight(b, "/")
	if b == "" {
		return ""
	}
	return b
}

func CompletionsURL(base string) string {
	b := normalizeBase(base)
	switch {
	case b == "":
		return ""
	case strings.HasSuffix(b, "/chat/completions"):
		return b
	case strings.HasSuffix(b, "/v1"): // common base forms
		return b + "/chat/completions"
	default:
		return b + "/v1/chat/completions"
	}
}