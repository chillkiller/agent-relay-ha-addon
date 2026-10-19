# Code Review: agent-desktop-ha-app — Agent Relay (development, HEAD c6bf4e6)

**domain:** agent-relay | **runId:** review-2026-10-18-1 | **label:** code-review
**date:** 2026-10-18 | **reviewer:** agent:reviewer | **build:** OK (`go build ./...`), `go vet` clean

---

## Summary

| Severity | Count |
|----------|-------|
| CRITICAL | 3 |
| HIGH     | 4 |
| MEDIUM   | 5 |
| LOW      | 4 |
| NIT      | 4 |
| **Total**| 20   |

One **guaranteed panic** (map concurrent read/write on `Server.subs`), one **unbounded goroutine leak** in turn scheduling, **plain-text token persistence** in the DB, and a **mismatch between configured env vars and code** that silently drops the HA options. Five issues require action before production; the rest are hardening opportunities.

---

## CRITICAL

### C-1 · `Server.subs` map — guaranteed Go 1.22 panic

**File:** `internal/api/handlers.go`, lines ~53–67

**Evidence:**
```go
// broadcast is called by hubAdapter.Publish (runtime side), holding only
// s.mu.RLock (read lock):
func (s *Server) broadcast(data map[string]any) {
    b, _ := json.Marshal(data)
    s.mu.RLock()
    defer s.mu.RUnlock()
    for c := range s.subs {          // ← concurrent map read (RLock OK for readers)
        if err := c.Write(...); err != nil {
            go func(c *websocket.Conn) {
                _ = c.Close(...)
                s.mu.Lock()           // ← A
                delete(s.subs, conn)  // ← B (concurrent with other readers!)
                s.mu.Unlock()
            }(c)
        }
    }
}
```
```go
// In ws():
s.mu.Lock()
s.subs[conn] = true    // ← concurrent map write (Lock held, OK)
s.mu.Unlock()
```
```go
// In ws() read loop exit:
s.mu.Lock()
delete(s.subs, conn)   // ← concurrent with broadcast's for-range
s.mu.Unlock()
```

`go version go1.22` makes map read/write during iteration **panic with "fatal error: concurrent map read and map write"**. The `delete` in the goroutine spawned by broadcast runs concurrently with any in-flight `for c := range s.subs` iteration.

**Fix:** Collect connections to close in a local slice under the RLock, then close and delete them outside the read lock:

```go
func (s *Server) broadcast(data map[string]any) {
    b, _ := json.Marshal(data)
    s.mu.RLock()
    var toClose []*websocket.Conn
    for c := range s.subs {
        if err := c.Write(context.Background(), websocket.MessageText, b); err != nil {
            toClose = append(toClose, c)
        }
    }
    s.mu.RUnlock()
    for _, c := range toClose {
        go func(c *websocket.Conn) {
            _ = c.Close(websocket.StatusInternalError, "write failed")
            s.mu.Lock()
            delete(s.subs, c)
            s.mu.Unlock()
        }(c)
    }
}
```

---

### C-2 · `openclawToken` stored and returned in plain text

**File:** `internal/store/repo.go`, `GetSettings()` — returns raw token
**File:** `internal/api/handlers.go`, `getSettings` — serialises it to JSON
**File:** `web/public/app.js`, `renderSettings()` — renders into a password input

**Evidence:**
```go
// repo.go
func (d *DB) GetSettings() Settings {
    out := map[string]any{"openclawToken": nil, ...}
    ...
    out[k] = raw  // ← plain text from SQLite
    return Settings(out)
}
```

```js
// app.js — reads the plain-text token back
f.openclawToken.value = state.settings.openclawToken ?? "";  // ← revealed
```

An attacker with read access to `/data/relay.db` (HA addon volume) or an HA user with API access can read the full Bearer token.

**Fix:** Never store the raw token. Store a `openclawTokenHash` (SHA-256) in SQLite and only return the token when updating (write-only pattern). Alternatively use HA's secrets mechanism. The GET response should return a masked value (`"***"`).

---

### C-3 · REST API has no authentication gate — relies solely on HA Ingress

**File:** `internal/api/handlers.go` + `internal/api/handlers2.go`

All `mux.HandleFunc` routes (`/api/agents`, `/api/rooms`, `/api/tasks`, `/api/settings`, `/ws`) are unauthenticated. `Server.New` does not inject any auth middleware. The `Server.ws` handler does not check `r.Header.Get("Origin")`.

In the HA add-on context, ingress should enforce auth. However:
- `host_network: false` in `config.yaml` reduces but does not eliminate exposure of port 8099.
- If the add-on is installed on a Home Assistant instance with a compromised account, the relay API is fully accessible without additional credentials.
- LAN access (port 8099 on the host) bypasses ingress entirely.

**Fix:** Add an explicit HA Ingress token check as defence-in-depth. At minimum, validate the `X-HA-Ingress` header or check the request's source IP against HA's expected proxy range. Document the LAN access security model clearly.

---

## HIGH

### H-1 · Goroutine leak: `OnUserMessage` turn goroutines never terminate

**File:** `internal/runtime/runtime.go`, `OnUserMessage()` lines ~63–77

**Evidence:**
```go
func (r *Runtime) OnUserMessage(m *store.Message) {
    ...
    for i, a := range targets {
        agent := a
        delay := time.Duration(i) * 900 * time.Millisecond
        go func() {
            select {
            case <-time.After(delay):   // ← timers run for the lifetime of the process
            case <-r.stop:              // ← r.stop is NEVER closed; goroutine is orphaned
                return
            }
            if err := r.turn(agent, m); err != nil { ... }
        }()
    }
}
```

`r.stop` is created in `New()` but never closed. All turn goroutines accumulate for every message. Under sustained use (many rooms, many messages), this is an unbounded memory and CPU leak.

**Fix:** Close `r.stop` when the server shuts down (e.g. via a `Shutdown()` method on Runtime or a closer passed to `New`). Alternatively, use `context.WithCancel` per message and attach it to the runtime context.

---

### H-2 · `RELAY_OPENAI_BASE_URL` and `RELAY_OPENAI_API_KEY` env vars in run.sh are never read

**File:** `agent-relay/run.sh` lines 13–14
**File:** `cmd/agentrelay/main.go`

**Evidence:**
```bash
# run.sh
export RELAY_OPENAI_BASE_URL="$(opt openclaw_base_url)"   # set
export RELAY_OPENAI_API_KEY="$(opt openclaw_token)"       # set
```
```go
// main.go — reads only these:
baseURL, _ := settings["openclawBaseUrl"].(string)  // from DB settings
apiKey, _ := settings["openclawToken"].(string)     // from DB settings
```

The env vars are set but never read. All agent configuration must be entered via the web UI settings page. The env vars are dead code and misleading for operators.

**Fix:** Either remove the env var exports from `run.sh`, or wire them as fallback in `main.go`:
```go
cfg.BaseURL = first non-empty of { a.BaseURL, os.Getenv("RELAY_OPENAI_BASE_URL"), settings["openclawBaseUrl"] }
cfg.APIKey  = first non-empty of { a.APIKey,  os.Getenv("RELAY_OPENAI_API_KEY"),  settings["openclawToken"]  }
```

---

### H-3 · OpenClaw adapter: context timeout too short for realistic agent tasks

**File:** `internal/adapter/openclaw.go`, `runAgentOnce()` line ~92

**Evidence:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 190*time.Second)
// agent.wait inherits ctx — no independent timeout
waitRes, err := rpc(ctx, conn, "agent.wait", map[string]any{ ... })
```

Paperclip-style agent runs for complex tasks routinely exceed 3 minutes. The 190 s limit includes: gateway processing time + network round-trips + agent execution. For tasks that take 3–10 minutes (coding, research, multi-step reasoning), the context cancels before `agent.wait` returns, even if the gateway computed the result correctly.

**Fix:** Increase to at least 600 s (10 min). Add an independent timeout on the `agent.wait` RPC call itself:
```go
waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Minute)
defer waitCancel()
waitRes, err := rpc(waitCtx, conn, "agent.wait", ...)
```

---

### H-4 · `UpdateRoom` does not update `memberAgentIDs` — room members silently dropped on edit

**File:** `internal/store/repo.go`, `UpdateRoom()`

**Evidence:**
```go
func (d *DB) UpdateRoom(r *Room) error {
    _, err := d.db.Exec(`UPDATE rooms SET name=?, project_id=?, updated_at=? WHERE id=?`,
        r.Name, pid, now(), r.ID)  // ← memberAgentIDs not updated
    return err
}
```

`InsertRoom` correctly inserts `room_members` rows, but `UpdateRoom` only writes `name`, `project_id`, `updated_at`. Editing a room via `PATCH /api/rooms/{id}` silently drops all member associations.

**Fix:** Add member sync to `UpdateRoom`:
```go
func (d *DB) UpdateRoom(r *Room) error {
    tx, err := d.db.Begin()
    if err != nil { return err }
    _, err = tx.Exec(`UPDATE rooms SET name=?, project_id=?, updated_at=? WHERE id=?`,
        r.Name, pid, now(), r.ID)
    if err != nil { tx.Rollback(); return err }
    _, err = tx.Exec(`DELETE FROM room_members WHERE room_id=?`, r.ID)
    if err != nil { tx.Rollback(); return err }
    for _, aid := range r.MemberAgentIDs {
        _, err = tx.Exec(`INSERT OR IGNORE INTO room_members VALUES (?,?)`, r.ID, aid)
        if err != nil { tx.Rollback(); return err }
    }
    return tx.Commit()
}
```

---

## MEDIUM

### M-1 · HTTP client timeout (200 s) shorter than context deadline (180 s) — inverted relationship

**File:** `internal/adapter/runners.go`, `streamOpenAI()` lines ~64–67

**Evidence:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
defer cancel()
...
client := &http.Client{Timeout: 200 * time.Second}  // ← longer than ctx
```

When the context fires at 180 s, the HTTP request is cancelled. This is the inverse of the intended relationship. Additionally, the 200 s client timeout is itself longer than the 180 s context.

**Fix:**
```go
client := &http.Client{Timeout: 170 * time.Second}  // ≤ 180 s context
```

---

### M-2 · OpenAI adapter: `skip-own-message` logic is inverted for non-assistant-role messages

**File:** `internal/adapter/sse.go`, `buildRequestBody()` lines ~29–36

**Evidence:**
```go
for _, m := range history {
    content := m.Content
    if m.Role == "assistant" && a.Name != "" && m.Name == a.Name {
        continue // skip the agent's own previous messages when running
    }
    msgs = append(msgs, message{Role: m.Role, Content: content})
}
```

The `history` built in `runtime.go` sets `Role: "assistant"` for agent messages, but `m.Name` is the `SenderName` from the DB (display name, e.g. `"Bob"`). The agent config's `Name` is also `"Bob"`. This should match — but if the agent's messages come through the echo adapter or another adapter that doesn't set `SenderName` to `a.Name`, the name won't match and nothing is skipped.

More critically: the condition `m.Role == "assistant"` means the check only applies to `assistant` role entries. The history function in `runtime.go` always writes `Role: "assistant"` for agent messages, so this is technically correct for the current code path. However, the check `m.Name == a.Name` compares display names which may not be stable identifiers.

**Fix:** Use a stable agent ID instead of name comparison:
```go
if m.Role == "assistant" && m.AgentID == a.ID {  // requires extending Message struct
    continue
}
```

---

### M-3 · `streamBuf` not cleared on room switch — stale tokens applied to wrong room

**File:** `web/public/app.js`, `loadMessages()` vs `handleEvent()`

**Evidence:**
```js
async function loadMessages() {
    state.messages = {};           // ← cleared
    // streamBuf is NOT cleared here
    ...
}
```

If a streaming message from Room A arrives via WS while the user has switched to Room B, `agent.token` events will update `streamBuf[m.id]` for the old room. When the user switches back, `loadMessages()` resets `state.messages` but `streamBuf` retains orphaned entries. The orphaned `streamBuf` entry could cause a brief incorrect display if the message ID collides (nanosecond IDs make this unlikely in practice, but the logic is incorrect).

**Fix:**
```js
async function loadMessages() {
    state.messages = {};
    streamBuf = {};  // ← reset on room switch
    ...
}
```

---

### M-4 · `agent.wait` RPC inherits the same parent context with no independent timeout

**File:** `internal/adapter/openclaw.go`, `runAgentOnce()`

Even when H-3 (increase timeout to 600 s) is fixed, the `agent.wait` call shares the 600 s context with the entire `runAgentOnce` function. If the `agent` RPC takes 590 s and `agent.wait` takes 10 s, the combined operation works. But if the context fires while `agent.wait` is blocking (network hiccup), the whole run is lost.

**Fix:** After increasing the parent context timeout (H-3), give `agent.wait` its own independent timeout:
```go
waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Minute)
defer waitCancel()
waitRes, err := rpc(waitCtx, conn, "agent.wait", ...)
```

---

### M-5 · Broadcast write error handler spawns unlimited goroutines under sustained write failures

**File:** `internal/api/handlers.go`, `broadcast()`

**Evidence:**
```go
for c := range s.subs {
    if err := c.Write(...); err != nil {
        go func(c *websocket.Conn) {  // ← one goroutine per failing connection
            _ = c.Close(...)
            s.mu.Lock()
            delete(s.subs, conn)
            s.mu.Unlock()
        }(c)
    }
}
```

Under a DoS or a slow peer that causes repeated write failures, this spawns unbounded goroutines. Each call to `c.Write` that fails spawns a new goroutine to close the connection. A malicious or flooding client could cause goroutine accumulation before the close completes.

**Fix:** Use a bounded channel or sync pool for close operations:
```go
type closeReq struct {
    conn   *websocket.Conn
    closed chan struct{}
}
closeChan := make(chan closeReq, len(s.subs))
for c := range s.subs {
    if err := c.Write(...); err != nil {
        req := closeReq{conn: c, closed: make(chan struct{})}
        closeChan <- req
        go func(req closeReq) {
            _ = req.conn.Close(websocket.StatusInternalError, "write failed")
            s.mu.Lock()
            delete(s.subs, req.conn)
            s.mu.Unlock()
            close(req.closed)
        }(req)
    }
}
```

---

## LOW

### L-1 · `hasJSON` reads the full request body into memory on every PATCH handler call

**File:** `internal/api/handlers.go`, `hasJSON()`

**Evidence:**
```go
func hasJSON(r *http.Request, key string) bool {
    body, err := io.ReadAll(r.Body)  // ← full body buffered for every field check
    ...
    r.Body = io.NopCloser(bytes.NewReader(body))
    var m map[string]json.RawMessage
    ...
}
```

`hasJSON` is called up to 4 times per `patchAgent` request (model, baseUrl, apiKey, enabled). Each call re-reads and re-buffers the full body. For large payloads or frequent updates, this is wasteful.

**Fix:** Read the body once in a wrapper, or use `json.Decoder` with token streaming. Alternatively, accept that the body is buffered once per handler call (current `decode` already reads it once); `hasJSON` could instead rely on the already-decoded struct field presence:

```go
// Replace hasJSON(r, "model") checks with:
if b.APIKey != "" || b.Model != "" { ... }
```

---

### L-2 · No WS origin check in `Server.ws`

**File:** `internal/api/handlers2.go`, `ws()`

**Evidence:**
```go
conn, err := websocket.Accept(w, r, nil)  // ← no Origin check
```

An origin check would defend against cross-site WebSocket hijacking if HA's ingress is misconfigured or bypassed.

**Fix:**
```go
if origin := r.Header.Get("Origin"); origin != "" && origin != "https://"+r.Host {
    http.Error(w, "forbidden", 403)
    return
}
```

---

### L-3 · `UpdateSetting` silently ignores errors

**File:** `internal/store/repo.go`, `UpdateSetting()`

```go
func (d *DB) UpdateSetting(key string, value string) {
    _, _ = d.db.Exec(...)  // ← errors silently swallowed
}
```

**Fix:** At minimum log the error. For a settings update (user-initiated), returning an error to the caller is appropriate.

---

### L-4 · `InsertMessage` and `UpdateMessage` run without explicit transactions

**File:** `internal/store/repo.go`

If the process crashes between `InsertMessage` returning and the subsequent WS broadcast, the DB record exists but clients never see the message. For a crash-resilient system, these should be wrapped in transactions.

**Fix:** Use `db.Begin()` / `tx.Commit()` for message insert + status update sequences. The current WAL mode provides some durability guarantees, but explicit transactions give stronger ordering.

---

## NIT

### N-1 · `Runtime.lastUser` is populated but never read

**File:** `internal/runtime/runtime.go`, `Runtime.lastUser` field + assignment

```go
type Runtime struct {
    ...
    lastUser  map[string]*store.Message // roomID -> last user message snapshot
}
func (r *Runtime) OnUserMessage(m *store.Message) {
    ...
    r.lastUser[m.RoomID] = m  // ← written, never read
}
```

Dead field. Remove it or wire it to a future "reply context" feature.

---

### N-2 · `var _ = context.Background` is a no-op at package level

**File:** `internal/runtime/runtime.go`, last line

```go
var _ = context.Background // reserve for future ctx plumbing
```

Unused identifier. Either remove or replace with a comment.

---

### N-3 · `runtime.history()`: no-op assignment

**File:** `internal/runtime/runtime.go`, `history()` lines ~170–172

```go
_ = agentID  // ← no-op; agentID is function parameter
```

Remove the assignment.

---

### N-4 · `roomChanged` takes a `*Server` argument it doesn't use

**File:** `internal/api/handlers.go`

```go
func roomChanged(s *Server) {  // ← s unused
    s.broadcast(...)
}
```

Should be `func roomChanged()` (no receiver needed; uses the package-level `broadcast` closure).

---

## Additional Observations (Informational)

### env var handling in run.sh vs main.go

`run.sh` sets `RELAY_OPENAI_BASE_URL` and `RELAY_OPENAI_API_KEY`, but `main.go` reads from DB settings only. See **H-2**.

### echo adapter streaming speed

The echo adapter delivers 12 runes per 25 ms → ~480 runes/s. A 1000-token response takes ~25 s. This is a demo adapter; production adapters should stream at the speed of the upstream API.

### Idempotency key is time-based, not message-based

`runID := fmt.Sprintf("relay-%x-%d", a.ID, time.Now().UnixNano())` in `openclaw.go` uses a nanosecond timestamp. For retries to be idempotent, the key should be derived from the message content hash (`userMsg.ID` is available in `turn()`).

### `webui` in config.yaml points to port 8099 directly

The `webui` URL `http://[HOST]:[PORT:8099]/` opens the relay on the host's port directly, bypassing HA Ingress authentication. This is documented as intentional ("LAN access"), but the config comment could be clearer that this is unauthenticated.

### SQLite on `/data` in HA Add-on — WAL appropriate

`config.yaml` sets `RELAY_DATA_DIR=/data` (via `run.sh`), which is the HA addon persistent volume. This is a local filesystem (not NFS/SMB), so SQLite WAL mode is appropriate. The `tryWAL` fallback to DELETE journal is correct defensive coding.

### `ARCHITECTURE.md` §5: sessionKey documentation vs implementation

The doc says `sessionKey = agent:<agentId>:relay:<roomId>` but `openclaw.go` uses `sessionKey = fmt.Sprintf("relay:%s", a.ID)` (no agent ID component, no room ID component). This means all sessions for the same agent share the same session key regardless of room. Either the doc is ahead of implementation, or the implementation is simplified and the doc needs correction.

---

## Severity Counts

| Tag      | Count |
|----------|-------|
| CRITICAL | 3     |
| HIGH     | 4     |
| MEDIUM   | 5     |
| LOW      | 4     |
| NIT      | 4     |
| **Total**| 20    |

## Blocking Items Before Merge

1. **C-1** — `Server.subs` map race → guaranteed Go 1.22 panic. Fix with the toClose slice pattern.
2. **C-2** — Plain-text token in DB + API response → credential exposure. Fix with write-only token pattern.
3. **H-1** — Goroutine leak in turn scheduling → unbounded memory growth. Close `r.stop` on shutdown.
4. **H-2** — Dead env vars in `run.sh` → operator confusion. Remove or wire as fallback.
5. **H-4** — `UpdateRoom` drops room members → data loss on room edit. Add member sync to `UpdateRoom`.
