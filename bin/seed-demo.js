// seed-demo.js — Agent Relay Demoseed: buzz/paperclip-Stil-Realität zum Testen.
// Startet gegen http://127.0.0.1:8099 (Relay).
// Idempotent: checkt, ob seed-Marker existiert.
const B = "http://127.0.0.1:8099";

async function api(method, path, body) {
  const res = await fetch(`${B}${path}`, {
    method,
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return null;
  return res.json();
}

async function main() {
  // Marker: Relay-Name prüfen — geseedet?
  const h = await api("GET", "/api/health");
  const roomsNow = await (await fetch(B + "/api/rooms")).json();
  const chatExists = roomsNow.some((r) => (r.kind || "chat") === "chat" && r.name === "General");
  if (chatExists && h.agents >= 6) {
    console.log("seed komplett vorhanden (chat-rooms + agents) — überspringe");
    return;
  }
  console.log("seeding…", roomsNow.length, "rooms,", h.agents, "agents, chat-rooms fehlen:", !chatExists);

  // ── Agenten (buzz/paperclip-Team) ──
  const agentSpecs = [
    { name: "Marvin", emoji: "🧭", color: "#5e6ad2", adapter: "echo",
      systemPrompt: "Coordinator & Chief of Staff — plant, koordiniert, accountable. Kalmer, präziser Ton." },
    { name: "Codex", emoji: "🧑‍💻", color: "#ff5c5c", adapter: "echo",
      systemPrompt: "Code-Tiefe: baut, reviewed, prüft Protokolle. Kompakt, exakt." },
    { name: "Copilot", emoji: "🐦", color: "#14b8a6", adapter: "echo",
      systemPrompt: "Security & Verifikation — prüft Kontraste, WCAG, API-Verhalten. Direkt." },
    { name: "Hermes", emoji: "⚡", color: "#d29922", adapter: "echo",
      systemPrompt: "Schnelle Recherche + Zusammenfassung. TL;DR zuerst." },
    { name: "Muse", emoji: "🎨", color: "#8a7ff0", adapter: "echo",
      systemPrompt: "Design-Blick: Form, Farbe, Dichte. Präzise Befunde mit Pixel-Werten." },
    { name: "Guide", emoji: "📕", color: "#22c55e", adapter: "echo",
      systemPrompt: "Prompt-Philosophie: funktionale Instruktionen statt vager Adjektive." },
  ];
  const agents = [];
  for (const spec of agentSpecs) {
    const a = await api("POST", "/api/agents", spec);
    agents.push(a);
  }

  // ── Projekte (paperclip-Stil: Control-Plane mit Boards) ──
  const projSpecs = [
    { name: "Agent Relay v1", description: "Relay-App für die HA-Sidebar — OpenClaw-DNA × buzz-Funktionen", color: "#5e6ad2" },
    { name: "HA-Integration", description: "Add-on-Paket, Ingress, Phase-2: Streaming", color: "#14b8a6" },
    { name: "Hausautomatisierung", description: "Leuchtmittel, Heizung, Szenen — Buzz-artige Räume fürs Zuhause", color: "#d29922" },
    { name: "Research: Agent-Protokolle", description: "MCP vs ACP vs nativ — Lücken-Analyse", color: "#8a7ff0" },
  ];
  const projects = [];
  for (const spec of projSpecs) {
    const p = await api("POST", "/api/projects", spec);
    projects.push(p);
  }

  // ── Räume: Chat-Räume + Projekt-Räume (mit Tasks) ──
  const roomSpecs = [
    // buzz-Feel: Kanal-Gruppen mit Themen-Namen
    { name: "General", kind: "chat", agents: agents.map(a => a.id) },
    { name: "Design-Lab", kind: "chat", agents: [agents[0].id, agents[4].id] }, // Marvin + Muse
    { name: "Reviews", kind: "chat", agents: [agents[0].id, agents[1].id, agents[2].id] }, // Marvin + Codex + Copilot
    { name: "Noise-Floor", kind: "chat", agents: [agents[3].id] }, // Hermes allein
  ];
  // Projekt-Räume: projekte mit tasks
  const projRoomMap = [];
  for (const p of projects) {
    const room = await api("POST", "/api/rooms", {
      name: p.name, kind: "project",
      projectId: p.id,
      agentIds: agents.map(a => a.id).slice(0, 3),
    });
    projRoomMap.push({ project: p, room });
  }

  // ── Tasks in den Projekt-Räumen ──
  const taskSets = [
    [ // Agent Relay v1
      { title: "Quick-Wins: Reaktionen, Suche, Presence", status: "doing", assignee: "Codex" },
      { title: "Sidebar-v2 Dropdown-Verankerung", status: "done", assignee: "Copilot" },
      { title: "Attachments-Upload", status: "todo", assignee: "Hermes" },
      { title: "Notifications-System", status: "todo", assignee: "Marvin" },
      { title: "HA-Boot-Realtest auf ha68", status: "todo", assignee: "Copilot" },
    ],
    [ // HA-Integration
      { title: "Ingress-Panel-Titel", status: "done", assignee: "Copilot" },
      { title: "Watchdog-URL-Template", status: "doing", assignee: "Codex" },
      { title: "Backup-Exclude-Liste", status: "todo", assignee: "Guide" },
    ],
    [ // Hausautomatisierung
      { title: "Szenen-Räume im Relay", status: "todo", assignee: "Marvin" },
      { title: "Presence-Dots live an HA-States koppeln", status: "todo", assignee: "Hermes" },
    ],
    [ // Research
      { title: "MCP-vs-ACP-Matrix", status: "done", assignee: "Codex" },
      { title: "Hermes-Adapter-Skizze", status: "todo", assignee: "Guide" },
    ],
  ];
  for (let i = 0; i < projRoomMap.length; i++) {
    for (const t of taskSets[i]) {
      await api("POST", "/api/tasks", { ...t, projectId: projRoomMap[i].project.id });
    }
  }

  // ── Chat-Räume ANLEGEN (der fehlende Block: roomSpecs wurde definiert, nie gebaut) ──
  const chatRooms = [];
  for (const spec of roomSpecs) {
    const room = await api("POST", "/api/rooms", { name: spec.name, kind: spec.kind, agentIds: spec.agents });
    chatRooms.push(room);
  }
  console.log("chat-rooms angelegt:", chatRooms.map((r) => r.name).join(", "));

  // ── Chat-Verlauf seeden (sinnvolle Conversation-Fragmente) ──
  const chatScript = [
    [0, "human", "Team, der Relay-Stand sieht gut aus — was fehlt eurer Meinung nach?"],
    [0, "agent:2", "Funktionen: Attachments + Notifications. Struktur: Threads für Tiefe."],
    [0, "human", "@Muse wie siehst du die Sidebar-Dichte?"],
    [0, "agent:4", "Runde-10-Stand: eng, aber der Composer braucht mehr Luft. gap 8px statt 2px bei Autorenwechsel."],
    [1, "human", "@Codex der Composer sitzt nicht mittig — fixe die margin."],
    [1, "agent:1", "R11-Fix gebaut: margin auto statt float-rechts. Checkliste abgehakt."],
    [2, "human", "Review-Status: 31 Feature-Lücken, 3 Quick-Wins live."],
    [3, "human", "@Hermes kurzer Status-Report für GaRoN?"],
    [3, "agent:3", "TL;DR: Quick-Wins live, Design-Zyklen geschlossen, Funktions-Audit liegt bereit."],
  ];
  for (const [roomIdx, kind, content] of chatScript) {
    const room = chatRooms[roomIdx];
    if (kind === "human") {
      await api("POST", `/api/rooms/${room.id}/messages`, { content });
      await new Promise(r => setTimeout(r, 350));
    } else {
      // agent reply via mention
      const agentId = agents[kind.split(":")[1]].id;
      const content2 = content.replace(/@(\w+)/, (m, n) => `@${n}`);
      await api("POST", `/api/rooms/${room.id}/messages`, { content: content2 });
      await new Promise(r => setTimeout(r, 1200)); // agent turn läuft
    }
    await new Promise(r => setTimeout(r, 300));
  }

  // ── Reaktionen seeden (auf die letzten 2 Messages je Raum) ──
  const rooms = await api("GET", "/api/rooms");
  for (const room of rooms) {
    const msgs = await api("GET", `/api/rooms/${room.id}/messages`);
    for (const m of msgs.slice(-2)) {
      for (const [actor, emoji] of [["GaRoN", "👍"], ["Marvin", "🔥"]]) {
        await api("POST", `/api/messages/${m.id}/reactions`, { actor, emoji });
      }
    }
  }

  console.log("seed fertig:", rooms.length, "rooms,", agents.length, "agents,", projects.length, "projects,");
}

main().catch((e) => { console.error("seed failed:", e.message); process.exit(1); });