/**
 * gateway-roster — Server-seitiger Gateway-Roster-Sync (0 Runtime-Dependencies).
 *
 * Spricht das bewiesene Gateway-WS-Protokoll (connect.challenge → connect → hello-ok
 * → agents.list) über Node ≥22 natives WebSocket. Token NIEMALS als Literal: nur aus
 * Env (OPENCLAW_GATEWAY_TOKEN) oder der lokalen openclaw.json (loadToken-Gesetz).
 */
"use strict";

const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const HOME_OPENCLAW = path.join(os.homedir(), ".openclaw", "openclaw.json");

function loadToken() {
  if (process.env.OPENCLAW_GATEWAY_TOKEN) return process.env.OPENCLAW_GATEWAY_TOKEN;
  try {
    const cfg = JSON.parse(fs.readFileSync(HOME_OPENCLAW, "utf8"));
    const tok = cfg.gateway && cfg.gateway.auth && cfg.gateway.auth.token;
    if (typeof tok === "string" && tok) return tok;
  } catch { /* token via env */ }
  return null;
}

let uid = 0;
const uid2 = () => "rs_" + (++uid);

function createRosterSync({ gwHost, gwPort, refreshMs }) {
  const state = {
    connected: false, serverVersion: null, connId: null, methods: 0,
    agents: [], lastSyncAt: null, error: null,
  };
  let timer = null;
  let ws = null;
  let stopped = false;

  function send(obj) { if (ws && ws.readyState === 1) ws.send(JSON.stringify(obj)); }

  function request(method, params, timeoutMs = 12000) {
    const id = uid2();
    return new Promise((resolve, reject) => {
      const t = setTimeout(() => { subs.delete(id); reject(new Error("roster timeout " + method)); }, timeoutMs);
      subs.set(id, { resolve, timer: t });
      send({ type: "req", id, method, params: params || {} });
    });
  }
  const subs = new Map();

  function handleConnectRes(frame) {
    if (!frame.ok) { state.error = "connect rejected: " + JSON.stringify(frame.error || "").slice(0, 200); return schedule(); }
    const p = frame.payload || {};
    state.connected = true;
    state.serverVersion = p.server && p.server.version;
    state.connId = p.server && p.server.connId;
    state.methods = (p.features && p.features.methods ? p.features.methods : []).length;
    syncRoster();
  }

  async function syncRoster() {
    try {
      const ag = await request("agents.list", {});
      const arr = Array.isArray(ag) ? ag : (ag.agents || ag.items || []);
      state.agents = arr.map((a) => ({
        id: a.id || a.agentId || a.key || "",
        name: a.name || a.label || a.id || "",
        model: a.model || (a.runtime && a.runtime.model) || "",
        identity: (a.identity && (a.identity.emoji || a.identity.theme)) || "",
      }));
      state.lastSyncAt = new Date().toISOString();
      state.error = null;
    } catch (e) {
      state.error = "sync: " + e.message;
    }
  }

  function connect() {
    if (stopped) return;
    const tok = loadToken();
    if (!tok) { state.error = "kein Token (env OPENCLAW_GATEWAY_TOKEN oder ~/.openclaw/openclaw.json)"; return schedule(); }
    try { ws = new WebSocket(`ws://${gwHost}:${gwPort}/ws`); }
    catch (e) { state.error = "ws setup: " + e.message; return schedule(); }

    ws.addEventListener("open", () => {
      send({
        type: "req", id: "connect_1", method: "connect",
        params: {
          minProtocol: 4, maxProtocol: 4,
          client: { id: "gateway-client", version: "0.3.0", platform: "linux", mode: "backend", displayName: "Agent Relay UI" },
          role: "operator",
          scopes: ["operator.read"],
          caps: [], commands: [], permissions: {},
          auth: { token: tok },
          locale: "de-DE", userAgent: "relay-server-roster/0.3.0",
        },
      });
    });
    ws.addEventListener("message", (ev) => {
      let frame; try { frame = JSON.parse(String(ev.data)); } catch { return; }
      if (frame.type === "event" && frame.event === "connect.challenge") return;
      if (frame.type === "res" && frame.id === "connect_1") return handleConnectRes(frame);
      if (frame.type === "res" && subs.has(frame.id)) {
        const s = subs.get(frame.id); subs.delete(frame.id); clearTimeout(s.timer);
        if (frame.ok) s.resolve(frame.payload);
        else s.resolve(null);
      }
    });
    ws.addEventListener("close", () => { state.connected = false; schedule(); });
    ws.addEventListener("error", () => { state.connected = false; schedule(); });
  }

  function schedule() {
    if (stopped || timer) return;
    timer = setTimeout(() => { timer = null; connect(); }, refreshMs || 15000);
  }

  connect();

  return {
    state,
    stop() { stopped = true; if (timer) clearTimeout(timer); try { ws && ws.close(); } catch {} },
  };
}

module.exports = { createRosterSync, loadToken };