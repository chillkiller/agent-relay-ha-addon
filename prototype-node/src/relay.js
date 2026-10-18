// Relay: REST API + WebSocket live events + static SPA + agent runtime wiring.

const path = require("node:path");
const fs = require("node:fs");
const express = require("express");
const { WebSocketServer } = require("ws");

const store = require("./store");
const agents = require("./agents");

const HUMAN_NAME = process.env.RELAY_USER_NAME || "gaRON";

function uid(prefix) {
  return agents.uid(prefix);
}

function createRelay({ dataDir, port, host }) {
  const dbPath = path.join(dataDir, "relay.db");
  store.open(dbPath);
  store.failStaleStreaming();

  const app = express();
  app.use(express.json({ limit: "4mb" }));

  // ── WebSocket hub ──────────────────────────────────────────────────
  const wss = new WebSocketServer({ noServer: true });
  function emitEvent(event) {
    const payload = JSON.stringify({ ts: Date.now(), ...event });
    for (const client of wss.clients) {
      if (client.readyState === 1) client.send(payload);
    }
  }
  agents.bindEmitter(emitEvent);

  function ensureRoomMessage(roomId, message) {
    if (message.roomId !== roomId) return null;
    return message;
  }

  // ── health ─────────────────────────────────────────────────────────
  app.get("/api/health", (_req, res) => {
    res.json({
      ok: true,
      name: store.getSettings().relayName,
      adapters: agents.listAdapters(),
      agents: store.listAgents().length,
      rooms: store.listRooms().length,
    });
  });

  // ── settings ───────────────────────────────────────────────────────
  app.get("/api/settings", (_req, res) => res.json(store.getSettings()));
  app.patch("/api/settings", (req, res) => {
    res.json(store.updateSettings(req.body || {}));
  });

  // ── agents ─────────────────────────────────────────────────────────
  app.get("/api/agents", (_req, res) => res.json(store.listAgents()));
  app.post("/api/agents", (req, res) => {
    const { name } = req.body || {};
    if (!name) return res.status(400).json({ error: "name erforderlich" });
    const agent = store.insertAgent({ id: uid("agent"), ...req.body });
    res.status(201).json(agent);
  });
  app.patch("/api/agents/:id", (req, res) => {
    const agent = store.updateAgent(req.params.id, req.body || {});
    if (!agent) return res.status(404).json({ error: "Agent nicht gefunden" });
    res.json(agent);
  });
  app.delete("/api/agents/:id", (req, res) => {
    store.deleteAgent(req.params.id);
    res.status(204).end();
  });

  // ── projects ───────────────────────────────────────────────────────
  app.get("/api/projects", (_req, res) => res.json(store.listProjects()));
  app.post("/api/projects", (req, res) => {
    const { name } = req.body || {};
    if (!name) return res.status(400).json({ error: "name erforderlich" });
    const project = store.insertProject({ id: uid("proj"), ...req.body });
    res.status(201).json(project);
  });
  app.patch("/api/projects/:id", (req, res) => {
    const project = store.updateProject(req.params.id, req.body || {});
    if (!project) return res.status(404).json({ error: "Projekt nicht gefunden" });
    res.json(project);
  });
  app.delete("/api/projects/:id", (req, res) => {
    store.deleteProject(req.params.id);
    res.status(204).end();
  });

  // ── rooms ──────────────────────────────────────────────────────────
  app.get("/api/rooms", (_req, res) => res.json(store.listRooms()));
  app.post("/api/rooms", (req, res) => {
    const { name, agentIds, projectId } = req.body || {};
    if (!name) return res.status(400).json({ error: "name erforderlich" });
    const room = store.insertRoom({ id: uid("room"), name, agentIds, projectId });
    emitEvent({ type: "rooms.changed" });
    res.status(201).json(room);
  });
  app.patch("/api/rooms/:id", (req, res) => {
    const room = store.updateRoom(req.params.id, req.body || {});
    if (!room) return res.status(404).json({ error: "Raum nicht gefunden" });
    emitEvent({ type: "rooms.changed" });
    res.json(room);
  });
  app.delete("/api/rooms/:id", (req, res) => {
    store.deleteRoom(req.params.id);
    emitEvent({ type: "rooms.changed" });
    res.status(204).end();
  });
  app.post("/api/rooms/:id/members", (req, res) => {
    const { agentId } = req.body || {};
    if (!agentId) return res.status(400).json({ error: "agentId erforderlich" });
    store.addRoomMember(req.params.id, agentId);
    emitEvent({ type: "rooms.changed" });
    res.status(201).json({ ok: true });
  });
  app.delete("/api/rooms/:id/members/:agentId", (req, res) => {
    store.removeRoomMember(req.params.id, req.params.agentId);
    emitEvent({ type: "rooms.changed" });
    res.status(204).end();
  });

  // ── messages ───────────────────────────────────────────────────────
  app.get("/api/rooms/:id/messages", (req, res) => {
    res.json(store.listMessages(req.params.id, Number(req.query.limit) || 200));
  });

  app.post("/api/rooms/:id/messages", (req, res) => {
    const room = store.getRoom(req.params.id);
    if (!room) return res.status(404).json({ error: "Raum nicht gefunden" });
    const content = String(req.body?.content ?? "").trim();
    if (!content) return res.status(400).json({ error: "content erforderlich" });

    const message = store.insertMessage({
      id: uid("msg"),
      roomId: room.id,
      senderName: HUMAN_NAME,
      senderKind: "human",
      content,
      status: "complete",
    });
    if (ensureRoomMessage(room.id, message)) {
      emitEvent({ type: "message.created", message });
    }
    // multi-agent turn: respond sequentially to this one message
    agents
      .runAgentsForMessage(message)
      .catch((err) => console.error("[relay] agent run failed:", err.message));
    res.status(201).json(message);
  });

  // ── tasks ──────────────────────────────────────────────────────────
  app.get("/api/tasks", (req, res) => {
    res.json(store.listTasks(req.query.projectId));
  });
  app.post("/api/tasks", (req, res) => {
    const { title, projectId } = req.body || {};
    if (!title) return res.status(400).json({ error: "title erforderlich" });
    const task = store.insertTask({ id: uid("task"), projectId, ...req.body });
    res.status(201).json(task);
  });
  app.patch("/api/tasks/:id", (req, res) => {
    const task = store.updateTask(req.params.id, req.body || {});
    if (!task) return res.status(404).json({ error: "Task nicht gefunden" });
    res.json(task);
  });
  app.delete("/api/tasks/:id", (req, res) => {
    store.deleteTask(req.params.id);
    res.status(204).end();
  });

  // ── static SPA ─────────────────────────────────────────────────────
  const publicDir = path.join(__dirname, "..", "public");
  app.use(express.static(publicDir));
  app.get("*", (_req, res) => {
    const index = path.join(publicDir, "index.html");
    fs.readFile(index, (err, data) => {
      if (err) return res.status(500).send("public/index.html fehlt");
      res.type("html").send(data);
    });
  });

  // ── server + ws ────────────────────────────────────────────────────
  const server = app.listen(port, host, () => {
    console.log(`[relay] listening on http://${host}:${port}`);
  });
  server.on("upgrade", (req, socket, head) => {
    const { pathname } = new URL(req.url, "http://localhost");
    if (pathname !== "/ws") return socket.destroy();
    wss.handleUpgrade(req, socket, head, (ws) => {
      wss.emit("connection", ws, req);
      ws.send(JSON.stringify({ ts: Date.now(), type: "hello" }));
    });
  });

  return { app, server, wss, emitEvent };
}

module.exports = { createRelay };