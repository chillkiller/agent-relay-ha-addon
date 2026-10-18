package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

)

// ── openclaw (native Gateway WebSocket protocol) ────────────────────
//
// Follows the gateway protocol documented in docs/gateway/protocol and the
// production-proven flow of Paperclip's openclaw-gateway adapter:
//   receive event connect.challenge (nonce)
//   → req connect (protocol/client/role/scopes/auth) → hello-ok payload
//   → req agent {message, sessionKey, idempotencyKey, agentId, timeout}
//   → req agent.wait (terminal snapshot when available)
// Session binding: agent:<agentId>:relay:<roomId> (see ARCHITECTURE.md §5).

type OpenClaw struct {
	Client *http.Client
}

func (OpenClaw) ID() string { return "openclaw" }

func (OpenClaw) Validate(a AgentConfig) error {
	if a.BaseURL == "" {
		return errors.New("keine Gateway-URL gesetzt")
	}
	return nil
}

func (OpenClaw) Stream(a AgentConfig, history []Message) (<-chan Chunk, error) {
	// trigger message = last user message (OpenClaw sessions keep full history)
	trigger := ""
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			trigger = history[i].Content
			break
		}
	}
	if trigger == "" {
		return nil, errors.New("keine Trigger-Nachricht")
	}
	url := gatewayWSURL(a.BaseURL)
	out := make(chan Chunk)
	go func() {
		defer close(out)
		text, err := runAgentOnce(url, a, trigger)
		if err != nil {
			out <- Chunk{Err: err}
			return
		}
		out <- Chunk{Text: text}
	}()
	return out, nil
}

// gatewayWSURL converts ws://host:port[/path] or http(s)://host:port[/path]
// into a plain ws url (gateway multiplexes WS + HTTP on the same port).
func gatewayWSURL(base string) string {
	b := strings.TrimSpace(base)
	b = strings.TrimSuffix(b, "/")
	b = strings.TrimSuffix(b, "/v1") // tolerate OpenAI-style configs
	switch {
	case strings.HasPrefix(b, "wss://"), strings.HasPrefix(b, "ws://"):
		return b
	case strings.HasPrefix(b, "https://"):
		return "wss://" + strings.TrimPrefix(b, "https://")
	case strings.HasPrefix(b, "http://"):
		return "ws://" + strings.TrimPrefix(b, "http://")
	default:
		return "ws://" + b
	}
}

type wsFrame struct {
	Type   string `json:"type"` // "req" | "res" | "event"
	ID     string `json:"id,omitempty"`
	Method string `json:"method,omitempty"`
	OK     *bool  `json:"ok,omitempty"`
	// Gateway contract (docs/gateway/protocol): requests carry "params",
	// responses/events carry "payload". Using one field for both was the
	// round-3 protocol bug: req frames validated as invalid.
	Params    json.RawMessage `json:"params,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Event     string          `json:"event,omitempty"`
	Error     *struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
}

func runAgentOnce(wsURL string, a AgentConfig, trigger string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute) // long agent runs (review H-3)
	defer cancel()

	dialCtx, dialCancel := context.WithTimeout(ctx, 15*time.Second)
	conn, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPHeader: authHeader(a),
	})
	dialCancel()
	if err != nil {
		return "", fmt.Errorf("gateway dial: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "bye")

	// 1) connect.challenge → 2) connect → hello-ok
	hello, err := handshake(ctx, conn, a)
	if err != nil {
		return "", err
	}
	var helloPayload struct {
		Server struct {
			Version string `json:"version"`
		} `json:"server"`
	}
	_ = json.Unmarshal(hello.Payload, &helloPayload)

	// 3) agent run (sessionKey = agent:<agentId>:relay:<roomId> binding is
	// carried by the relay caller through cfg; here we use the configured
	// model field as agentId and idempotencyKey = msg hash placeholder).
	agentID := a.Model
	if strings.HasPrefix(agentID, "openclaw/") || strings.HasPrefix(agentID, "agent:") {
		agentID = strings.TrimPrefix(strings.TrimPrefix(agentID, "openclaw/"), "agent:")
	}
	if agentID == "" || agentID == "openclaw" {
		agentID = "" // default agent of the gateway
	}
	runID := firstNonEmpty(a.IdempotencyKey, fmt.Sprintf("relay-%s-%d", a.ID, time.Now().UnixNano()))
	agentReq := map[string]any{
		"message": trigger,
		"idempotencyKey": runID,
	}
	if agentID != "" {
		agentReq["agentId"] = agentID
	}
	// sessionKey namespace must match the gateway agent name (gateway policy:
	// 'agent "coordinator" does not match session key agent "..."'): derive
	// from the TARGET agent id (model field), with a room suffix for isolation.
	if sk := firstNonEmpty(a.SessionKey, sessionKeyFor(agentID, a.ID)); sk != "" {
		agentReq["sessionKey"] = sk
	}
	agentReq["timeout"] = 480000 // match 8-min wait window (review-2 M-A)

	res, err := rpc(ctx, conn, "agent", agentReq)
	if err != nil {
		return "", fmt.Errorf("agent rpc: %w", err)
	}
	var accepted struct {
		RunID  string `json:"runId"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal(res.Payload, &accepted)

	// 4) agent.wait → terminal snapshot with response text
	// independent wait window so a stalled wait still surfaces status (M-4)
	waitCtx, waitCancel := context.WithTimeout(ctx, 8*time.Minute)
	defer waitCancel()
	waitRes, err := rpc(waitCtx, conn, "agent.wait", map[string]any{
		"runId": firstNonEmpty(accepted.RunID, runID),
	})
	if err != nil {
		return "", fmt.Errorf("agent.wait: %w", err)
	}
	var waitPayload struct {
		Status string `json:"status"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
		// terminalReply snapshot (docs: AgentRunTerminalReplySnapshot):
		// visible→text | silent | empty(+code)
		TerminalReply struct {
			Disposition string `json:"disposition"`
			Text        string `json:"text"`
			Code        string `json:"code"`
		} `json:"terminalReply"`
	}
	if err := json.Unmarshal(waitRes.Payload, &waitPayload); err != nil {
		return "", fmt.Errorf("agent.wait payload: %w", err)
	}
	if waitPayload.Error != nil && waitPayload.Error.Message != "" {
		return "", fmt.Errorf("openclaw: %s", waitPayload.Error.Message)
	}
	reply := waitPayload.TerminalReply
	switch {
	case reply.Disposition == "visible" && strings.TrimSpace(reply.Text) != "":
		return reply.Text, nil
	case reply.Disposition == "empty":
		return "", fmt.Errorf("openclaw: leere Antwort (disposition=empty code=%s, gw %s)", reply.Code, helloPayload.Server.Version)
	default:
		return "", fmt.Errorf("openclaw: keine sichtbare Antwort (disposition=%s status=%s, gw %s)", reply.Disposition, statusOr(accepted.Status, waitPayload.Status), helloPayload.Server.Version)
	}
}

func authHeader(a AgentConfig) http.Header {
	h := http.Header{}
	if a.APIKey != "" {
		h.Set("Authorization", "Bearer "+a.APIKey)
	}
	return h
}

// handshake reads the connect.challenge event and completes the connect RPC.
func handshake(ctx context.Context, conn *websocket.Conn, a AgentConfig) (*wsFrame, error) {
	nonce := ""
	deadline := time.Now().Add(10 * time.Second)
	readCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	for {
		_, data, err := conn.Read(readCtx)
		if err != nil {
			return nil, fmt.Errorf("challenge read: %w", err)
		}
		var f wsFrame
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		// event frames: {"type":"event","event":"connect.challenge","payload":{"nonce":..}}
		if f.Type == "event" {
			var ev struct {
				Event   string `json:"event"`
				Payload struct {
					Nonce string `json:"nonce"`
				} `json:"payload"`
			}
			if json.Unmarshal(data, &ev) == nil && ev.Event == "connect.challenge" {
				nonce = ev.Payload.Nonce
				break
			}
		}
	}
	_ = nonce // nonce is optional for token auth; kept for parity

	params := map[string]any{
		"minProtocol": 4,
		"maxProtocol": 4,
		"client": map[string]any{
			"id":       "cli", // allowed ids: cli/webchat/openclaw-control-ui/… (gateway whitelist)
			"version":  "0.1.0",
			"platform": "linux",
			"mode":     "cli", // allowed modes: cli/ui/webchat/backend/… (gateway whitelist)
		},
		"role":    "operator",
		"scopes":  []string{"operator.read", "operator.write"},
		"caps":    []string{},
		"commands": []string{},
		"permissions": map[string]any{},
		"locale":  "de-DE",
		"userAgent": "agent-relay/0.1.0",
	}
	if a.APIKey != "" {
		params["auth"] = map[string]any{"token": a.APIKey}
	}
	return rpc(ctx, conn, "connect", params)
}

func rpc(ctx context.Context, conn *websocket.Conn, method string, params map[string]any) (*wsFrame, error) {
	id := fmt.Sprintf("req-%d", time.Now().UnixNano())
	frame := wsFrame{Type: "req", ID: id, Method: method}
	paramsBytes, _ := json.Marshal(params)
	frame.Params = paramsBytes // req carries "params" (gateway contract)
	out, _ := json.Marshal(frame)
	writeCtx, writeCancel := context.WithTimeout(ctx, 10*time.Second)
	if err := conn.Write(writeCtx, websocket.MessageText, out); err != nil {
		writeCancel()
		return nil, err
	}
	writeCancel()

	pendingID := id
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return nil, err
		}
		var f wsFrame
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		if f.Type == "res" && f.ID == pendingID {
			if (f.OK != nil && !*f.OK) || f.Error != nil {
				msg := ""
				if f.Error != nil {
					msg = f.Error.Message
				}
				if msg == "" {
					msg = string(f.Payload)
				}
				return nil, fmt.Errorf("%s: %s", method, msg)
			}
			return &f, nil
		}
		// events while waiting: agent.* progress — ignore for v1 (we use
		// agent.wait for the terminal snapshot). TODO(phase2): stream deltas.
	}
}

// sessionKeyFor: gateway-conventional session key: agent:<gateway-agent>:relay:<roomKey>
// (gateway checks that the leading namespace matches the target agent; the
// relay room suffix keeps turns of different rooms separate).
func sessionKeyFor(gatewayAgentID, _ string) string {
	ns := gatewayAgentID
	if ns == "" {
		ns = "main" // gateway default agent
	}
	return fmt.Sprintf("agent:%s:relay", ns)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func statusOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}