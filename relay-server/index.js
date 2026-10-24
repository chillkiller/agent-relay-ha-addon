#!/usr/bin/env node
/**
 * relay-server — eigenständiger Frontend-Server (HA-Addon-Kern)
 *
 * Rolle: serviert die geklonte OpenClaw Control-UI (relay-ui/, 58 MB static,
 * md5-identisch zu dist/control-ui) wie das Gateway sie ausliefert:
 *   - index-Stamping (base-path-Attribut, terminal-Flag, ?v=buildId-Cache-Busting,
 *     Route-Preload-Template-Stripping) — mit Gateway-Auslieferung diff-geprüft
 *   - /ws Pass-through (HTTP-Upgrade) → ws://127.0.0.1:18789/ws
 *   - Slim-Zusatz-APIs /api/health|rooms|tasks|registry (JSON-Store)
 *
 * Beweisgrad-Konvention: JEDE Property oben beweist den Codepfad der Herkunft.
 */
"use strict";

const http = require("node:http");
const net = require("node:net");
const fs = require("node:fs");
const path = require("node:path");
const { createRosterSync } = require("./gateway-roster.js");

const PORT = Number(process.env.PORT || 8787);
const HOST = process.env.HOST || "0.0.0.0";
const GW_HOST = process.env.GW_HOST || "127.0.0.1";
const GW_PORT = Number(process.env.GW_PORT || 18789);
const UI_ROOT = process.env.UI_ROOT || path.join(__dirname, "..", "relay-ui");
const DATA_DIR = process.env.RELAY_DATA_DIR || path.join(__dirname, "data");
const STATE_FILE = path.join(DATA_DIR, "state.json");

fs.mkdirSync(DATA_DIR, { recursive: true });

/* ---------------- Store (Slim) ---------------- */
const DEFAULT_STATE = { rooms: [], tasks: [], registry: [], onboarding: {} };
function loadState() {
  try { return { ...DEFAULT_STATE, ...JSON.parse(fs.readFileSync(STATE_FILE, "utf8")) }; }
  catch { return { ...DEFAULT_STATE }; }
}
function saveState(s) {
  const tmp = STATE_FILE + ".tmp";
  fs.writeFileSync(tmp, JSON.stringify(s, null, 2));
  fs.renameSync(tmp, STATE_FILE);
}
let state = loadState();

/* ---------------- Static + Stamp ---------------- */
const MIME = {
  ".js": "text/javascript", ".mjs": "text/javascript", ".css": "text/css",
  ".html": "text/html", ".json": "application/json", ".svg": "image/svg+xml",
  ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif",
  ".webp": "image/webp", ".avif": "image/avif", ".ico": "image/x-icon",
  ".webmanifest": "application/manifest+json", ".woff": "font/woff", ".woff2": "font/woff2",
  ".ttf": "font/ttf", ".otf": "font/otf", ".map": "application/json",
  ".txt": "text/plain", ".xml": "application/xml", ".wasm": "application/wasm",
  ".br": "application/octet-stream", ".gz": "application/gzip", ".md": "text/markdown",
};

const BUILD_ID_RE = /data-openclaw-control-ui-build-id="([^"]+)"/;

/* Overlay-Injektion: relay-dna.js + boot-tag ins <head>-Ende (vor </head>) */
function injectOverlay(html) {
  if (!process.env.RELAY_OVERLAY || process.env.RELAY_OVERLAY === "0") return html;
  if (html.includes("/relay-dna.js")) return html; // idempotent
  const bootTag = process.env.RELAY_LAUNCH_TOKEN
    ? "<script>window.__RELAY_LAUNCH__=" + JSON.stringify({ gatewayUrl: (process.env.RELAY_LAUNCH_GW_URL || ((process.env.GW_HOST || "127.0.0.1") + ":" + (process.env.GW_PORT || ""))), token: process.env.RELAY_LAUNCH_TOKEN, mode: process.env.RELAY_LAUNCH_MODE || "" }) + "</script>"
    : "";
  return html.replace("</head>", '<script src="/relay-dna.js" data-relay-overlay="1"></script>' + bootTag + "</head>");
}

function stampIndex(raw) {
  let html = raw;
  // (1) html-Attribute: base-path leer + terminal-Flag (Gateway-Stamp, diff-bewiesen);
  // Guard auf den ECHTEN html-Tag (m-Flag: Doctype steht davor; Script-Strings sind unberührt)
  const htmlTag = html.match(/^<html(?:\s[^>]*)?>/m);
  if (htmlTag && !(htmlTag[0] || "").includes("data-openclaw-terminal-enabled")) {
    html = html.replace(htmlTag[0], '<html data-openclaw-control-ui-base-path="" data-openclaw-terminal-enabled="true"' + htmlTag[0].slice(5));
  }
  // (2) Route-Preload-Templates strippt das Gateway (diff: leere Zeilen bleiben)
  html = html.replace(/<template data-openclaw-route-preloads="[^"]*">[\s\S]*?<\/template>/g, "");
  // (3) relative Asset-Pfade absolut setzen ("./" → "/", wie Gateway-Auslieferung)
  html = html.replace(/(href|src)="\.\//g, '$1="/');
  // (4) ?v=buildId auf die vier Head-Links (diff: nur diese haben ?v)
  const bid = (html.match(BUILD_ID_RE) || [])[1] || "";
  if (bid) {
    html = html.replace(
      /(href)="\/(favicon\.svg|favicon-32\.png|apple-touch-icon\.png|manifest\.webmanifest)"/g,
      (m, attr, file) => `${attr}="/${file}?v=${encodeURIComponent(bid)}"`
    );
  }
  return html;
}

let indexCache = { mtime: 0, stamped: null, raw: null };
function readIndex() {
  const file = path.join(UI_ROOT, "index.html");
  const st = fs.statSync(file);
  if (st.mtimeMs !== indexCache.mtime) {
    const raw = fs.readFileSync(file, "utf8");
    indexCache = { mtime: st.mtimeMs, raw, stamped: injectOverlay(stampIndex(raw)) };
  }
  return indexCache;
}

function sendError(res, code, msg) {
  res.writeHead(code, { "Content-Type": "text/plain; charset=utf-8", "X-Content-Type-Options": "nosniff" });
  res.end(msg);
}

function serveStatic(req, res, pathnameIn) {
  let pathname = pathnameIn;
  if (pathname === "/") pathname = "/index.html"; // Root-Stempel-Zweig muss greifen
  // SPA-Fallback: unbekannte Pfade ohne Extension → index (History-Routing)
  let rel = decodeURIComponent(pathname);
  if (rel === "/") rel = "/index.html";
  const file = path.join(UI_ROOT, rel);
  if (!file.startsWith(path.resolve(UI_ROOT) + path.sep) && file !== path.join(UI_ROOT, "index.html")) {
    return sendError(res, 403, "forbidden");
  }
  fs.stat(file, (err, st) => {
    if (err || !st.isFile()) {
      if (!path.extname(pathname)) {
        // SPA-Fallback
        const idx = readIndex();
        res.writeHead(200, { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-cache", "X-Content-Type-Options": "nosniff" });
        return res.end(idx.stamped);
      }
      return sendError(res, 404, "not found");
    }
    // index.html → stamped
    if (pathname === "/index.html") {
      const idx = readIndex();
      res.writeHead(200, { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-cache", "X-Content-Type-Options": "nosniff" });
      return res.end(idx.stamped);
    }
    // Kompressions-Varianten (.br/.gz) wenn der Client sie annimmt — Content-Length
    // MUSS die Größe des SERVIERTEN (komprimierten) Streams sein (Gateway beweist 45
    // für 45-Byte-entry-br; Rohgröße trunciert im Browser → Module stumm, bewiesen)
    const ae = String(req.headers["accept-encoding"] || "");
    const ext = path.extname(file);
    const mimeType = MIME[ext] || "application/octet-stream";
    const headers = {
      "Content-Type": mimeType,
      "X-Content-Type-Options": "nosniff",
      "Cache-Control": "/assets/|/fonts/".includes("/" + rel.split("/")[1] + "/") ? "public, max-age=31536000, immutable" : "no-cache",
    };
    if (ae.includes("br") && fs.existsSync(file + ".br")) {
      headers["Content-Encoding"] = "br";
      return streamFile(file + ".br", res, headers);
    }
    if (ae.includes("gzip") && fs.existsSync(file + ".gz")) {
      headers["Content-Encoding"] = "gzip";
      return streamFile(file + ".gz", res, headers);
    }
    streamFile(file, res, headers);
  });
}

function streamFile(file, res, headers) {
  fs.stat(file, (err, st2) => {
    if (err) return sendError(res, 500, "stream stat failed");
    headers["Content-Length"] = st2.size;
    res.writeHead(200, headers);
    if (req_ === "HEAD") return res.end();
    fs.createReadStream(file).pipe(res);
  });
}

/* Roster-Sync (gateway-roster.js) — Server-seitige agents.list-Loop */
let roster = null;
let req_ = "GET"; // roher GET/POST-Track für streamFile (HEAD-Sonderfall)

/* ---------------- Zusatz-API ---------------- */
function api(res, method, pathname, body) {
  const json = (code, obj) => {
    const data = JSON.stringify(obj);
    res.writeHead(code, { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store" });
    res.end(data);
  };
  try {
    if (pathname === "/api/health") {
      return json(200, { ok: true, name: "relay-frontend", pid: process.pid, gateway: { host: GW_HOST, port: GW_PORT } });
    }
    if (pathname === "/api/rooms") {
      if (method === "GET") return json(200, { rooms: state.rooms });
      if (method === "POST") {
        const room = { id: "rm_" + Date.now().toString(36), kind: String(body.kind || "chat"), name: String(body.name || "Neuer Raum"), topic: String(body.topic || ""), createdAt: Date.now() };
        state.rooms.push(room); saveState(state);
        return json(201, room);
      }
    }
    if (pathname === "/api/tasks") {
      if (method === "GET") return json(200, { tasks: state.tasks });
      if (method === "POST") {
        const task = { id: "tk_" + Date.now().toString(36), projectId: String(body.projectId || ""), title: String(body.title || "Aufgabe"), status: String(body.status || "todo"), createdAt: Date.now() };
        state.tasks.push(task); saveState(state);
        return json(201, task);
      }
    }
    if (pathname === "/api/registry" && method === "GET") return json(200, { agents: (roster && roster.state.agents.length ? roster.state.agents : state.registry), gateway: roster ? { connected: roster.state.connected, serverVersion: roster.state.serverVersion, connId: roster.state.connId, methods: roster.state.methods, lastSyncAt: roster.state.lastSyncAt, error: roster.state.error } : null });
    if (pathname.startsWith("/api/rooms/") && method === "GET") {
      const id = pathname.split("/")[3];
      const room = state.rooms.find((r) => r.id === id);
      return room ? json(200, room) : json(404, { error: "room not found" });
    }
    if (pathname.startsWith("/api/tasks/") && method === "PATCH") {
      const id = pathname.split("/")[3];
      const task = state.tasks.find((t) => t.id === id);
      if (!task) return json(404, { error: "task not found" });
      Object.assign(task, body && typeof body === "object" ? body : {});
      saveState(state);
      return json(200, task);
    }
    return sendError(res, 404, "api not found");
  } catch (e) {
    return sendError(res, 500, "api error: " + e.message);
  }
}

/* ---------------- Server ---------------- */
const server = http.createServer((req, res) => {
  req_ = req.method === "HEAD" ? "HEAD" : req.method;
  const u = new URL(req.url, "http://localhost");
  const pathname = u.pathname;
  const chunks = [];
  req.on("data", (c) => { if (chunks.length < 8e6) chunks.push(c); });
  req.on("end", () => {
    let body = {};
    if (chunks.length) { try { body = JSON.parse(Buffer.concat(chunks).toString("utf8") || "{}"); } catch { body = {}; } }
    if (pathname.startsWith("/api/")) return api(res, req.method, pathname, body);
    if (pathname === "/launch" && req_ !== "HEAD") {
      // UI-Launch-Lane (Onboarding): gatewayUrl+token als HASH-FRAGMENT (bundle-Beweis:
      // SPA akzeptiert #token=; Query warnt+cleanup). Token verlässt den Prozess nur in
      // dieses eine HTML-Gerüst an den anfragenden Browser (LAN-Lane, GaRoN-Instanz).
      const tok = process.env.RELAY_LAUNCH_TOKEN || "";
      const gurl = process.env.RELAY_LAUNCH_GW_URL || (GW_HOST + ":" + GW_PORT);
      const frag = tok ? ("#gatewayUrl=" + encodeURIComponent(gurl) + "&token=" + encodeURIComponent(tok)) : ("#gatewayUrl=" + encodeURIComponent(gurl));
      res.writeHead(200, { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store", "Referrer-Policy": "no-referrer", "X-Content-Type-Options": "nosniff" });
      return res.end('<!doctype html><meta http-equiv="refresh" content="0;url=/' + frag.replace(/"/g, "&quot;") + '"><body>UI-Launch…</body>');
    }
    if (pathname === "/relay-dna.js") {
      const f = path.join(__dirname, "relay-dna.js");
      if (fs.existsSync(f)) {
        res.writeHead(200, { "Content-Type": "text/javascript; charset=utf-8", "Cache-Control": "no-cache" });
        return fs.createReadStream(f).pipe(res);
      }
      return sendError(res, 404, "overlay missing");
    }
    return serveStatic(req_ === "HEAD" ? req : req, res, pathname);
  });
  req.on("error", () => {});
});

/* /ws Pass-through: HTTP-Upgrade → raw TCP zum Gateway (Frames unberührt).
   JEDER WS-Upgrade wird durchgeleitet: die SPA defaultet auf `location.host`+"/ws"
   (Konsole bewies ws://127.0.0.1:8787/-Aufrufe) — nur Nicht-/ws-Pfade antworten 404. */
server.on("upgrade", (req, socket, head) => {
  const pathname = new URL(req.url, "http://localhost").pathname;
  // JEDER Upgrade wird zum Gateway durchgeleitet (SPA ruft '/' und '/ws' per WS):
  if (!(pathname === "/ws" || pathname === "/")) {
    socket.write("HTTP/1.1 404 Not Found\r\nConnection: close\r\n\r\n");
    return socket.destroy();
  }
  const upstream = net.connect(GW_PORT, GW_HOST, () => {
    // URL der Anfrage IMMER auf Gateway-/ws mapen (SPA ruft auch "/" per WS auf):
    let reqLines = req.method + " /ws HTTP/1.1\r\n";
    for (let i = 0; i < req.rawHeaders.length; i += 2) {
      const k = req.rawHeaders[i], v = req.rawHeaders[i + 1];
      if (/^host$/i.test(k)) { reqLines += "Host: " + GW_HOST + ":" + GW_PORT + "\r\n"; continue; }
      if (/^origin$/i.test(k) && process.env.RELAY_WS_REWRITE_ORIGIN !== "0") {
        // Origin auf das Gateway-Ziel umschreiben (Lokal-Lane; HA-Ingress-Ursprünne später via Config)
        reqLines += "Origin: http://" + GW_HOST + ":" + GW_PORT + "\r\n";
        continue;
      }
      reqLines += k + ": " + v + "\r\n";
    }
    upstream.write(reqLines + "\r\n");
    if (head && head.length) upstream.write(head);
  });
  upstream.on("data", (d) => { if (!socket.destroyed && socket.writable) socket.write(d); });
  socket.on("data", (d) => { if (!upstream.destroyed && upstream.writable) upstream.write(d); });
  const die = () => { if (!upstream.destroyed) upstream.destroy(); if (!socket.destroyed) socket.destroy(); };
  upstream.on("error", die); socket.on("error", die);
  upstream.on("close", die); socket.on("close", () => { if (!upstream.destroyed) upstream.destroy(); });
});

/* HEAD korrekt behandeln (ohne streamFile-Zweig zu stören) */
server.on("request", () => {});

server.listen(PORT, HOST, () => {
  roster = createRosterSync({ gwHost: GW_HOST, gwPort: GW_PORT, refreshMs: 20000 });
  console.log(`[relay-server] pid=${process.pid} listening http://${HOST}:${PORT} → ui=${UI_ROOT} | /ws → ${GW_HOST}:${GW_PORT} | overlay=${process.env.RELAY_OVERLAY ? "on" : "off"} | launch=${process.env.RELAY_LAUNCH_TOKEN ? "on" : "off"}`);
});