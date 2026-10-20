// Agent Relay SPA — plain JS, kein Build-Schritt.
// Design: Control-UI-DNA mit Linear-Tokens (docs/DESIGN.md).
(() => {
  "use strict";

  const HUMAN = "human"; // Pseudo-ID für den eigenen Chip im Roster

  const state = {
    view: "chat",
    rooms: [],
    agents: [],
    projects: [],
    currentRoomId: null,
    currentInlinePid: null,
    activeTab: "chat",
    messages: {},
    lastMetaKey: "", // msg-grouping: key des letzten gerenderten Kopfes
    adapters: [],
    settings: {},
    reactions: {},
    ws: null,
    // Sprint-1 (docs/RE/2026-10-20-transfer-matrix-frontend.md: Zeilen 3/11/22):
    following: true,  // Follow-Scroll-Vertrag: nur near-bottom auto-follow
    audit: [],        // P3#4 Audit-Register (neueste zuerst)
    newBelow: 0,      // Pill-Zähler "N neu ↓"
    lastRead: {},     // room.<roomId>.lastRead (buzz-Pattern, localStorage-backed)
    threadMode: null, // P1#4: { rootId } während eine Thread-Antwort komponiert wird
    chatTaskId: null, // P2#1: offenes Chatter-Panel (taskId)
    skills: [], // P2#3
    routines: [], // P2#4: recurring Agent-Trigger
    boardFilter: "all", // P2#6: Swimlane-Filter
    roomRoles: {}, // P2#8: map[agentId]→role je aktueller Raum: Skill-Store (paperclip company-skills)
  };

  const $ = (sel) => document.querySelector(sel);
  const esc = (s) =>
    String(s ?? "").replace(/[&<>"']/g, (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

  // ── API (Bearer aus sessionStorage, 401 → einmalig abfragen) ────────
  function bearer() { return sessionStorage.getItem("relayBearer") || ""; }

  // ── P2#2: Approvals (paperclip approvals.ts, Matrix-16-Card-Muster) ──
  // Banner über dem Composer: je pending Approval eine Karte (Agent-Name,
  // Reason, Grant/Deny-Buttons); state im WS-Event-Feed (kein Poll).
  let pendingApprovals = [];
  async function renderApprovalBanner() {
    const wrap = $("#approvalBanner");
    if (!wrap) return;
    // Agent-Namen auflösen (cards brauchen Namen/Kontext)
    const agents = state.agents;
    const cards = pendingApprovals.map((ap) => {
      const ag = agents.find((x) => x.id === ap.agentId);
      const who = ag ? `${ag.emoji} ${esc(ag.name)}` : esc(ap.agentId);
      return `
        <div class="approval-card" data-apid="${esc(ap.id)}">
          <div class="ap-row">
            <span class="badge-pill ap-pill">● Freigabe</span>
            <strong>${who}</strong>
            <span class="ap-reason">${esc(ap.reason || "")}</span>
          </div>
          <div class="ap-actions">
            <button class="btn primary" data-apact="granted" data-apid="${esc(ap.id)}">Genehmigen</button>
            <button class="btn danger" data-apact="denied" data-apid="${esc(ap.id)}">Ablehnen</button>
          </div>
        </div>`;
    }).join("");
    wrap.innerHTML = cards;
    wrap.hidden = pendingApprovals.length === 0;
    wrap.querySelectorAll("[data-apact]").forEach((b) =>
      b.addEventListener("click", async () => {
        const id = b.dataset.apid, decision = b.dataset.apact;
        try {
          await api("POST", `/api/approvals/${id}/${decision === "granted" ? "grant" : "deny"}`);
          // Banner-Update kommt via approval.decided-WS — kein lokales popping
        } catch (err) { toast(err.message); }
      }));
  }
  function renderAgentsBadge() {
    // Agent-Karten: ⛔-Badge wenn approvalGate (übersichtlich, kein extra Feld)
    document.querySelectorAll("#agentCards .card").forEach((card) => {
      const id = card.querySelector("[data-edit]")?.dataset.edit;
      const ag = state.agents.find((x) => x.id === id);
      if (!ag) return;
      let badge = card.querySelector(".gate-badge");
      if (ag.approvalGate) {
        if (!badge) {
          badge = document.createElement("span");
          badge.className = "gate-badge";
          badge.title = "Antworten brauchen Freigabe (Approval-Gate)";
          const sub = card.querySelector(".sub");
          if (sub) sub.after(badge);
        }
        badge.textContent = " ⛔ Gate";
      } else if (badge) badge.remove();
    });
  }
  // ── P1#3: Browser-Notifications (Codex-Matrix 1.3) ──
  // Pref in localStorage[relay.notify]="on"|"off"; permission nur on-demand
  // beim Einschalten; Notification nur für fremd-Raum-Nachrichten UND
  // (tab hidden ODER Raum nicht aktiv) — nie Doppel-Ping im offenen Raum.
  function notifyPref() { try { return localStorage.getItem("relay.notify") === "on"; } catch { return false; } }
  function setNotifyPref(on) {
    try {
      if (on) localStorage.setItem("relay.notify", "on");
      else localStorage.setItem("relay.notify", "off");
    } catch {}
    const btn = $("#notifyToggle");
    if (btn) btn.textContent = `🔔 Benachrichtigungen: ${on ? "an" : "aus"}`;
  }
  function updateNotifyLabel() {
    const btn = $("#notifyToggle");
    if (btn) btn.textContent = `🔔 Benachrichtigungen: ${notifyPref() ? "an" : "aus"}`;
  }
  async function toggleNotify() {
    if (notifyPref()) { setNotifyPref(false); return; }
    if (!("Notification" in window)) { toast("Kein Notification-Support im Browser"); return; }
    let perm = Notification.permission;
    if (perm === "denied") { toast("Benachrichtigungen im Browser blockiert"); return; }
    if (perm === "default") perm = await Notification.requestPermission();
    if (perm === "granted") {
      setNotifyPref(true);
      toast("🔔 Benachrichtigungen aktiv");
    } else toast("Berechtigung verweigert");
  }
  function sendNotify(roomId, m) {
    if (!notifyPref() || !("Notification" in window) || Notification.permission !== "granted") return;
    if (document.visibilityState === "visible" && roomId === state.currentRoomId) return;
    const room = state.rooms.find((r) => r.id === roomId);
    const who = m.senderKind === "human" ? (m.senderName || "du") : (m.senderName || "Agent");
    const body = (m.content || "").replace(/\s+/g, " ").slice(0, 120);
    const n = new Notification(`${room?.emoji || "#"} ${room?.name || "Raum"} — ${who}`, {
      body,
      tag: `relay-${roomId}`, // koalesce je Raum (OpenClaw toast-cooldown-Muster)
      silent: false,
    });
    n.addEventListener("click", () => {
      try { window.focus(); selectRoom(roomId); n.close(); } catch {}
    });
  }

  // Row-Subtitle (Matrix-22, buzz-Pattern): letzter Message-Text als Digest
  // unter dem Raumnamen — "what is happening" im Sidebar-Scan. Live aus dem
  // Message-Puffer (aktueller Raum) oder rooms[].lastMessage (Backend-Subselect).
  function roomSubtitle(roomId) {
    if (state.currentRoomId === roomId) {
      const list = Object.values(state.messages)
        .filter((m) => m.roomId === roomId && m.senderKind !== "system")
        .sort((a, b) => String(a.createdAt).localeCompare(String(b.createdAt)));
      const last = list[list.length - 1];
      return last ? String(last.content || "") : "";
    }
    const room = state.rooms.find((r) => r.id === roomId);
    return room?.lastMessage || "";
  }
  // Read-Marker-Restore beim Boot (localStorage-Schicht, buzz-Pattern)
  function restoreLastRead() {
    try {
      for (let i = 0; i < localStorage.length; i++) {
        const k = localStorage.key(i);
        if (k && k.startsWith("relay.lastRead.")) {
          state.lastRead[k.slice("relay.lastRead.".length)] = Number(localStorage.getItem(k)) || 0;
        }
      }
    } catch {}
  }

  async function api(method, path, body) {
    const headers = body ? { "Content-Type": "application/json" } : {};
    if (bearer()) headers["Authorization"] = `Bearer ${bearer()}`;
    const res = await fetch(url(path), {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });
    if (res.status === 401) {
      const tok = prompt("API-Token erforderlich (Add-on-Option api_bearer_token):");
      if (tok) {
        sessionStorage.setItem("relayBearer", tok);
        return api(method, path, body);
      }
      throw new Error("nicht autorisiert");
    }
    if (res.status === 204) return null;
    const json = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(json.error || res.statusText);
    return json;
  }

  // Basis-Prefix für HA-Ingress: UI läuft unter /api/has…/<token>/ — relative
  // Prefix-Mathe; alles unter dem aktuellen Pfad ohne Dateinamen.
  const BASE = location.pathname.endsWith("/")
    ? location.pathname.slice(0, -1)
    : location.pathname.replace(/\/[^/]*$/, "");
  const url = (p) => `${BASE}${p}`;

  // ═════════════════════════════════════════════════════════════════
  // WebSocket mit exponentiellem Backoff
  // ═════════════════════════════════════════════════════════════════
  let wsAttempts = 0;
  function connectWS() {
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const tokQ = bearer() ? `?token=${encodeURIComponent(bearer())}` : "";
    const ws = new WebSocket(`${proto}://${location.host}${url("/ws")}${tokQ}`);
    state.ws = ws;
    ws.onopen = () => { wsAttempts = 0; setConn(true); };
    ws.onclose = () => {
      setConn(false);
      const delay = Math.min(15000, 500 * Math.pow(2, wsAttempts++));
      setTimeout(connectWS, delay);
    };
    ws.onmessage = (ev) => {
      let msg; try { msg = JSON.parse(ev.data); } catch { return; }
      handleEvent(msg);
    };
  }
  function setConn(on) {
    $("#connDot").classList.toggle("on", on);
    $("#connText").textContent = on ? "verbunden" : "offline — neuer Versuch…";
  }

  const typingTimers = {};
  function handleEvent(e) {
    switch (e.type) {
      case "message.created": {
        if (e.message.roomId !== state.currentRoomId) {
          // Read-Marker (Matrix-3, buzz useLiveChannelUpdates.ts:34-40):
          // anderer Raum → Unread-Zähler + Dot, ohne Öffnen
          bumpUnread(e.message.roomId);
          // P1#3: Browser-Notification für fremde Raum-Nachrichten
          if (e.message.senderKind !== "human") sendNotify(e.message.roomId, e.message);
          break;
        }
        // P1#3: tab verdeckt + Agent-Antwort im offenen Raum → ebenfalls notify
        if (e.message.senderKind !== "human" && document.visibilityState !== "visible") sendNotify(e.message.roomId, e.message);
        state.messages[e.message.id] = e.message;
        // Pill-Zählung hier (nicht in renderMessages — kein Doppelzähler):
        // nur wenn non-follow UND fremde Nachricht (Eigen-Send resynced immer)
        if (!state.following && !nearBottom() && e.message.senderKind !== "human") {
          state.newBelow++;
        }
        renderMessages();
        break;
      }
      case "agent.started": {
        if (!e.message || e.message.roomId !== state.currentRoomId) break;
        state.messages[e.message.id] = e.message;
        streamBuf[e.message.id] = "";
        markAgentActive(e.message.agentId);
        renderRoomList();
        // Sprint-3b: Turn-Laufzeit-Chronometer (Worked-for-Head, Matrix-12)
        turnTimers[e.message.id] = { t0: Date.now(), tokens: 0 };
        renderMessages();
        break;
      }
      case "typing": {
        markAgentActive(e.agentId); // Sprint-4: Typing = Activity-Glimmen
        if (e.roomId !== state.currentRoomId) break;
        renderTyping(e.agentName);
        clearTimeout(typingTimers[e.agentId]);
        typingTimers[e.agentId] = setTimeout(() => { delete typingTimers[e.agentId]; renderTyping(); }, 12000);
        break;
      }
      case "agent.token": {
        streamBuf[e.messageId] = (streamBuf[e.messageId] ?? "") + e.delta;
        if (turnTimers[e.messageId]) turnTimers[e.messageId].tokens++;
        // Sprint-3b: Worked-for-Head live aktualisieren (Sekunden-Zähler)
        const headEl = document.querySelector(`[data-mid="${e.messageId}"] .turn-head-label`);
        if (headEl && turnTimers[e.messageId]) {
          headEl.textContent = `schreibt seit ${Math.round((Date.now() - turnTimers[e.messageId].t0) / 1000)} s`;
        }
        const el = document.querySelector(`[data-mid="${e.messageId}"] .content`);
        if (el) el.textContent = streamBuf[e.messageId];
        // Follow-Scroll-Vertrag: Token- Frames folgen nur near-bottom,
        // sonst Pill-Zähler (kein Scroll-Kidnapping mehr, Matrix-11)
        if (state.following && nearBottom()) scrollBottom(false);
        else if (!state.following) { state.newBelow++; updateNewPill(); }
        break;
      }
      case "agent.finished": {
        delete streamBuf[e.message.id ?? ""];
        // Sprint-3b: Turn-Zusammenfassung für den Fold-Head (Matrix-12)
        if (e.message?.id && turnTimers[e.message.id]) {
          const tt = turnTimers[e.message.id];
          const ms = Date.now() - tt.t0;
          turnStats[e.message.id] = { secs: Math.max(1, Math.round(ms / 1000)), tokens: tt.tokens };
          delete turnTimers[e.message.id];
        }
        // P1#3: Notify-Zweig MUSS vor der Membership-Prüfung stehen —
        // fremd-Raum-Nachrichten sind nie in state.messages (nur aktiver
        // Raum wird geladen); der alte in-block Hook feuerte deshalb nie.
        if (e.message?.senderKind === "agent" && e.message.roomId &&
            (e.message.roomId !== state.currentRoomId || document.visibilityState !== "visible")) {
          sendNotify(e.message.roomId, e.message);
        }
        if (e.message && state.messages[e.message.id]) {
          state.messages[e.message.id] = e.message;
          renderMessages();
        }
        if (e.message?.agentId) {
          markAgentActive(e.message.agentId);
          renderRoomList();
          clearTimeout(typingTimers[e.message.agentId]);
          delete typingTimers[e.message.agentId];
          renderTyping();
        }
        break;
      }
      case "agents.changed": {
        // Sprint-4 (Matrix-4): Enable-Toggle live in Roster+Karten+Room-Dots
        // spiegeln, ohne Poll — loadAgents rendert Roster/Karten/Chips,
        // renderRoomList die Raum-Dots aus der neuen Presence-Ableitung
        loadAgents();
        renderRoomList();
        break;
      }
      case "approval.required": {
        // P2#2: Turn hält an — Banner zeigt Entscheidungskarten (Matrix-16:
        // AuditCard = Kind-Badge + Reason + Buttons nur bei pending)
        pendingApprovals.push(e.approval);
        renderApprovalBanner();
        break;
      }
      case "task.comment.created": {
        // P2#1: offenes Panel live füttern (anderes Fenster/Agent)
        const panel = document.querySelector(`.task-chat[data-for-task="${e.taskId}"]`);
        if (panel && e.comment) {
          panel.querySelector(".task-chat-list").insertAdjacentHTML("beforeend", `
            <div class="tc-item ${e.comment.authorKind}"><span class="tc-who">${e.comment.authorKind === "agent" ? "🤖" : "🧑"} ${esc(e.comment.authorName)}</span><span class="tc-text">${esc(e.comment.content)}</span><span class="tc-time">${esc(fmtTime(e.comment.createdAt))}</span></div>`);
          panel.querySelector(".task-chat-list").scrollTop = 9999;
        }
        break;
      }
      case "routines.changed": {
        // P2#4: Roster der Routines live (Ticker feuert next_run neu)
        loadRoutines();
        break;
      }
      case "approval.decided": {
        // Banner-Karte entfernen (decision kommt mit approval-Objekt)
        pendingApprovals = pendingApprovals.filter((x) => x.id !== e.approval?.id);
        renderApprovalBanner();
        updateDashBadge(); // P2#7-Badge live
        loadAudit(); // P3#4: Approval-Entscheidung = Register-Fall
        break;
      }
      case "message.updated": {
        // P1#1: Edit-Acceptance — frische Version laden (authoritative DB).
        // handleEvent ist sync — then/catch statt await.
        if (e.messageId) {
          api("GET", `/api/messages/${e.messageId}`).then((m) => {
            if (m && m.roomId === state.currentRoomId) {
              state.messages[m.id] = m; renderMessages();
              loadRooms(); // Room-Subtitle live mitführen (Matrix-22)
            }
          }).catch(() => {});
        }
        break;
      }
      case "rooms.changed": loadRooms(); break;
      case "system": {
        if (e.roomId === state.currentRoomId) pushLocalSystem(e.text);
        break;
      }
    }
  }

  // Read-Marker (buzz-Pattern, Matrix-3): fremd-Raum-Nachricht → Zähler +
  // Dot im Raum-Button; DOM-Update statt Full-Rerender (kein Flackern)
  function bumpUnread(roomId) {
    if (state.lastRead[roomId] === undefined) {
      try { state.lastRead[roomId] = Number(localStorage.getItem(`relay.lastRead.${roomId}`)) || 0; } catch { state.lastRead[roomId] = 0; }
    }
    const r = state.rooms.find((x) => x.id === roomId);
    if (!r) return;
    r.unread = (r.unread || 0) + 1;
    const btn = document.querySelector(`.room-item[data-room="${roomId}"]`);
    if (!btn) return;
    const dot = btn.querySelector(".room-presence");
    if (dot) dot.classList.add("unread");
    let badge = btn.querySelector(".badge");
    if (!badge) {
      btn.insertAdjacentHTML("beforeend", `<span class="badge unread-badge"></span>`);
      badge = btn.querySelector(".badge");
    }
    badge.textContent = String(r.unread);
    badge.classList.add("unread-badge");
  }

  function pushLocalSystem(text) {
    const wrap = document.createElement("div");
    wrap.className = "msg system";
    wrap.innerHTML = `<div class="content">${esc(text)}</div>`;
    const list = $("#messages");
    list.appendChild(wrap);
    list.scrollTop = list.scrollHeight;
  }
  function renderTyping(name) {
    $("#typingBar").textContent = name ? `${name} schreibt…` : "";
  }

  // ═════════════════════════════════════════════════════════════════
  // Views
  // ═════════════════════════════════════════════════════════════════
  function switchView(view) {
    state.view = view;
    for (const v of ["chat", "agents", "projects", "settings", "dashboard", "audit"]) {
      $(`#view-${v}`).hidden = v !== view;
    }
    document.querySelectorAll(".nav-btn").forEach((b) =>
      b.classList.toggle("active", b.dataset.view === view));
    if (view === "agents") renderAgents();   /* R11-fix: Cards fehlen, wenn Load beim Inaktivsein lief */
    if (view === "dashboard") renderDashboard();
    if (view === "projects") loadProjectsView();
    if (view === "settings") renderSettingsForm();
    if (view === "audit") loadAudit();
    document.body.classList.remove("sidebar-open");
  }

   // Sidebar v2: einklappbare Gruppen oben + User-Dropdown unten
  function toggleSidebarGroup(gid) {
    const g = document.querySelector(`[data-group="${gid}"]`);
    if (!g) return;
    const collapsed = g.getAttribute("aria-expanded") === "false";
    const nowCollapsed = !collapsed;
    g.setAttribute("aria-expanded", nowCollapsed ? "false" : "true");
    const head = g.querySelector("[data-group-toggle]");
    if (head) head.querySelector(".chev").textContent = nowCollapsed ? "▸" : "▾";
    localStorage.setItem(`sidebar:collapsed:${gid}`, nowCollapsed ? "1" : "0");
  }
  function bindSidebarGroups(root) {
    root.querySelectorAll("[data-group-toggle]").forEach((btn) => {
      btn.addEventListener("click", () => toggleSidebarGroup(btn.dataset.groupToggle));
    });
  }

  // ═════════════════════════════════════════════════════════════════
  // Räume (gruppiert: RÄUME / PROJEKTE) + Drag&Drop-Ziele
  // ═════════════════════════════════════════════════════════════════
  async function loadRooms() {
    const prev = Object.fromEntries(state.rooms.map((r) => [r.id, r]));
    state.rooms = await api("GET", "/api/rooms");
    // clientseitige Zustände über Reloads konservieren (unread-Zähler,
    // lastRead-Marker — Backend kennt die Read-Marker nicht)
    for (const r of state.rooms) {
      const p = prev[r.id];
      if (p) {
        if (p.unread) r.unread = p.unread;
      } else if (state.lastRead[r.id] === undefined) {
        try { state.lastRead[r.id] = Number(localStorage.getItem(`relay.lastRead.${r.id}`)) || 0; } catch {}
      }
    }
    renderRoomList();
  }

  function renderRoomList() {
    const mount = $("#navGroups");
    const el = mount || $("#roomList");
    if (!el) return;
    // Model v2: RÄUME = chat + dm gemischt (dm mit 🔒-marker, projekt-bezug
    // als kleine 🗂-Badge am room-row); PROJEKTE ist die eigene Gruppe unten
    const rooms_ = state.rooms.filter((r) => r.kind !== "project" && r.humanMember !== false);
    const ghost = state.rooms.filter((r) => r.humanMember === false);
    const item = (r) => {
      const mIds = r.memberAgentIds || r.agentIds || [];
      // Sprint-4 (Matrix-4): Room-Dot leitet aus Member-Presence ab —
      // "on" wenn ein Member online, "idle" wenn nur idle-Member
      const presences = mIds.map((id) => state.agents.find((ag) => ag.id === id && ag.enabled))
        .filter(Boolean).map(agentPresence);
      const dotState = presences.includes("online") ? "on" : (presences.includes("idle") ? "idle" : "");
      const dot = `<span class="room-presence ${dotState}${r.unread ? " unread" : ""}" aria-hidden="true"></span>`;
      // Model v2: dm = privé-marker; projekt-bezug = 🗂-mini-badge
      const ico = r.kind === "dm" ? "🔒" : (r.projectId ? "🗂" : "#");
      // Row-Subtitle (Matrix-22): Digest- Zeile, einzeilig ellipsiert
      const sub = esc(roomSubtitle(r.id).replace(/\s+/g, " ").slice(0, 80));
      // Unread-Dot (Matrix-3): buzz-Pattern — Zähler-Badge statt messageCount
      const marker = r.unread ? `<span class="badge unread-badge" title="${r.unread} ungelesen">${r.unread}</span>`
        : (r.messageCount ? `<span class="badge">${r.messageCount}</span>` : "");
      return `
      <button class="room-item ${r.id === state.currentRoomId ? "active" : ""}"
              data-room="${r.id}" data-kind="${esc(r.kind || "chat")}"
              ${r.projectId ? 'data-proj="1"' : ""} draggable="false">
        ${r.kind === "dm" ? '<span class="room-ico dm-ico">🔒</span>' : `<span class="room-ico">${r.projectId ? "🗂" : "#"}</span>`}${dot}
        <span class="room-text" style="flex:1;min-width:0;text-align:left">
          <span class="marquee marquee__text" style="display:block">${esc(r.name)}</span>
          <span class="room-sub" style="display:block">${sub}</span>
        </span>
        <span class="room-actions">
          <span class="room-action" data-act="rename" data-room="${r.id}" title="Umbenennen">✎</span>
          <span class="room-action" data-act="del" data-room="${r.id}" title="Löschen">🗑</span>
        </span>
        ${marker}
      </button>`;
    };
    const groupId = (label) => label.toLowerCase().replace(/[^a-z]/g, "");
    const group = (label, rooms) => {
      if (!rooms.length) return "";
      const gid = groupId(label);
      const collapsed = localStorage.getItem(`sidebar:collapsed:${gid}`) === "1";
      return `
        <button class="room-group" data-group-toggle="${gid}" aria-expanded="${collapsed ? "false" : "true"}">
          <span class="chev">▸</span>${label}<span class="badge">${rooms.length}</span>
        </button>
        <div class="room-group-body" data-group-body="${gid}" data-collapsed="${collapsed ? "1" : "0"}">
          ${rooms.map(item).join("")}
        </div>`;
    };
    // Sidebar v2: navGroups als einklappbare Menüs oben
    if (mount) {
      const isCollapsed = (gid) => localStorage.getItem(`sidebar:collapsed:${gid}`) === "1";
      const teamHtml = (() => {
        const gid = "team"; const collapsed = isCollapsed(gid);
        const chip = (a, p) => `<span class="roster-chip${p === "off" ? " off" : p === "idle" ? " idle" : ""}" draggable="true" data-agent="${a.id}" title="${esc(a.name)}: ${p === "online" ? "online" : p === "idle" ? "idle" : "aus"} — in einen Raum ziehen">
          <span class="avatar p-${p}" style="background:${esc(a.color)}">${esc(a.emoji)}</span>
          <span class="chip-name">${esc(a.name)}</span>
          <span class="presence ${p === "off" ? "" : p}" title="${p === "online" ? "online" : p === "idle" ? "idle" : "aus"}"></span>
        </span>`;
        const chips = [
          `<span class="roster-chip" draggable="true" data-agent="${HUMAN}" title="In einen Raum ziehen (du)"><span class="avatar" style="background:var(--ok)">G</span><span class="chip-name">du</span><span class="presence on"></span></span>`,
          ...state.agents.map((a) => { const p = agentPresence(a); return chip(a, p); }),
        ].join("");
        return `<div class="nav-group" data-group="${gid}" aria-expanded="${collapsed ? "false" : "true"}">
          <button class="nav-group-head" data-group-toggle="${gid}"><span class="chev">${collapsed ? "▸" : "▾"}</span>Team <span class="badge" style="margin-left:auto;font-size:11px;color:var(--ink-4)">${state.agents.length + 1}</span></button>
          <div class="nav-group-body"><div class="roster-list" id="rosterList">${chips}</div><button id="newAgentQuick" class="roster-add" style="margin-top:8px">＋ Agent</button></div>
        </div>`;
      })();
      const navGroup = (label, gid, inner) => {
        if (!inner) return "";
        const collapsed = isCollapsed(gid);
        return `<div class="nav-group" data-group="${gid}" aria-expanded="${collapsed ? "false" : "true"}">
          <button class="nav-group-head" data-group-toggle="${gid}"><span class="chev">${collapsed ? "▸" : "▾"}</span>${label}</button>
          <div class="nav-group-body">${inner}</div>
        </div>`;
      };
      const projLinked = state.rooms.filter((r) => r.projectId && r.humanMember !== false);
      // CHATS (GaRoN 20:53): 1. Ebene = Mitglieder (Agents mit DMs + du),
      // 2. Ebene = die DM-Sessions mit dem Mitglied (ausgeklappt). dm-Räume
      // sind e2e-privat (🔒, genau 1 Agent) — gruppiert nach Gegenüber.
      const dmRooms = state.rooms.filter((r) => r.kind === "dm");
      const sessionItem = (dm) => item(dm);
      const chatsHtml = (() => {
        const byTarget = new Map();
        for (const dm of dmRooms) {
          const aid = (dm.memberAgentIds || [])[0] || HUMAN;
          if (!byTarget.has(aid)) byTarget.set(aid, []);
          byTarget.get(aid).push(dm);
        }
        const memberBlock = (aid, sessions) => {
          const mgid = `chats:${aid}`;
          const collapsed = isCollapsed(mgid);
          const isHuman = aid === HUMAN;
          const ag = state.agents.find((x) => x.id === aid);
          const avatar = isHuman
            ? `<span class="avatar" style="background:var(--ok)">G</span>`
            : `<span class="avatar" style="background:${esc(ag?.color || "#6ea8fe")}">${esc(ag?.emoji || "🤖")}</span>`;
          const name = isHuman ? "du" : esc(ag?.name || "Agent");
          const unread = sessions.reduce((s, x) => s + (x.unread || 0), 0);
          const rows = sessions.map(sessionItem).join("") || `<div class="chat-session-empty">— keine Sessions —</div>`;
          return `<div class="nav-group chat-member" data-group="${mgid}" aria-expanded="${collapsed ? "false" : "true"}">
            <button class="nav-group-head chat-member-head" data-group-toggle="${mgid}">
              ${avatar}<span class="chip-name">${name}</span>
              ${unread ? `<span class="badge chat-unread" style="margin-left:6px">${unread}</span>` : ""}
              <span class="chev" style="margin-left:auto">${collapsed ? "▸" : "▾"}</span>
            </button>
            <div class="nav-group-body chat-sessions">${rows}</div>
          </div>`;
        };
        if (!dmRooms.length) {
          return navGroup("CHATS", "chats", `<div class="chat-session-empty">🔒 Noch keine DM-Chats — Team-Chip ins Chat-Fenster ziehen.</div>`);
        }
        return `<div class="nav-group" data-group="chats" aria-expanded="${isCollapsed("chats") ? "false" : "true"}">
          <button class="nav-group-head" data-group-toggle="chats"><span class="chev">${isCollapsed("chats") ? "▸" : "▾"}</span>CHATS <span class="badge" style="margin-left:auto;font-size:11px;color:var(--ink-4)">${dmRooms.length}</span></button>
          <div class="nav-group-body">${[...byTarget.entries()].map(([aid, ss]) => memberBlock(aid, ss)).join("")}</div>
        </div>`;
      })();
      const roomGroupsHtml = group("RÄUME", rooms_.filter((r) => r.kind === "chat")) + group("PROJEKTE", projLinked) + group("AUSGELADEN", ghost);
      // group() liefert bereits room-group markup — direkt einhängen + Team anhängen
      mount.innerHTML = roomGroupsHtml + chatsHtml + teamHtml;
      bindSidebarGroups(mount);
      const qn = document.getElementById("quickNavGroups");
      if (qn) bindSidebarGroups(qn);
      // Team-Drag binden
      bindRosterDrag(mount);
      const qa = mount.querySelector("#newAgentQuick"); if (qa) qa.addEventListener("click", () => agentModal());
    } else {
      const projLinked = state.rooms.filter((r) => r.projectId && r.humanMember !== false);
    el.innerHTML = group("STREAMS", rooms_.filter((r) => r.kind !== "dm")) + group("PROJEKTE", projLinked) + group("AUSGELADEN", ghost);
    }

    // B1: toggle persistence (Control-UI-Muster: aria-expanded + chevron rotation)
    // guard: nav-group-Toggles (bindSidebarGroups) haben kein data-group-body —
    // ohne Guard wirft dieser zweite Handler je Klick TypeError (GaRoN 20:24-Zug).
    el.querySelectorAll("[data-group-toggle]").forEach((t) => {
      if (t.closest(".nav-group")) return; // nav-groups sind oben gebunden — sonst Doppel-Toggle
      t.addEventListener("click", () => {
        const gid = t.dataset.groupToggle;
        const body = el.querySelector(`[data-group-body="${gid}"]`);
        const nowCollapsed = body.dataset.collapsed !== "1";
        body.dataset.collapsed = nowCollapsed ? "1" : "0";
        t.setAttribute("aria-expanded", nowCollapsed ? "false" : "true");
        localStorage.setItem(`sidebar:collapsed:${gid}`, nowCollapsed ? "1" : "0");
      });
    });

    el.querySelectorAll("[data-act]").forEach((a) =>
      a.addEventListener("click", async (ev) => {
        ev.stopPropagation();
        const rid = a.dataset.room;
        const room = state.rooms.find((r) => r.id === rid);
        if (!room) return;
        if (a.dataset.act === "rename") renameRoomModal(room);
        if (a.dataset.act === "del") {
          if (!confirm(`Raum „${room.name}" inklusive Verlauf löschen?`)) return;
          await api("DELETE", `/api/rooms/${room.id}`);
          if (state.currentRoomId === rid) { state.currentRoomId = null; }
          await loadRooms();
          if (!state.currentRoomId && state.rooms.length) selectRoom(state.rooms[0].id);
        }
      }));
    el.querySelectorAll("[data-room]").forEach((b) => {
      b.dataset.draggable = "true";
      b.setAttribute("draggable", "true");
      b.addEventListener("dragstart", (ev) => {
        ev.dataTransfer.setData("text/room-id", b.dataset.room);
        ev.dataTransfer.effectAllowed = "move";
      });
      b.addEventListener("dragover", (ev2) => {
        if (!ev2.dataTransfer.types.includes("text/room-id")) return;
        ev2.preventDefault();
        const r = b.getBoundingClientRect();
        const before = (ev2.clientY - r.top) < r.height / 2;
        b.classList.toggle("drop-before", before);
        b.classList.toggle("drop-after", !before);
      });
      b.addEventListener("drop", (ev2) => { handleRoomReorderDrop(ev2, b); });
      b.addEventListener("dragover", (ev) => { ev.preventDefault(); b.classList.add("dragover"); });
      b.addEventListener("dragleave", () => b.classList.remove("dragover"));
      b.addEventListener("drop", async (ev) => {
        ev.preventDefault(); b.classList.remove("dragover");
        const agentId = ev.dataTransfer.getData("text/agent-id");
        if (!agentId) return;
        if (agentId === HUMAN) {
          try {
            await api("PATCH", `/api/rooms/${b.dataset.room}`, { humanMember: true });
            await loadRooms();
            selectRoom(b.dataset.room);
          } catch (err) { toast(err.message); }
          return;
        }
        try {
          if (b.dataset.kind === "project") {
            // in Projekt-Raum: einladen = Roster-Erweiterung (PATCH agentIds)
            const room = state.rooms.find((r) => r.id === b.dataset.room);
            const ids = [...(room?.memberAgentIds || []), agentId];
            await api("PATCH", `/api/rooms/${b.dataset.room}`, { agentIds: ids });
          } else {
            await api("POST", `/api/rooms/${b.dataset.room}/members`, { agentId });
          }
          await loadRooms();
          if (b.dataset.room === state.currentRoomId) await selectRoom(state.currentRoomId);
        } catch (err) { toast(err.message); }
      });
    });
  }

  function handleRoomReorderDrop(ev, targetRow) {
    const dragged = ev.dataTransfer.getData("text/room-id");
    if (!dragged || dragged === targetRow.dataset.room) { targetRow.classList.remove("drop-before", "drop-after"); return; }
    ev.preventDefault(); ev.stopPropagation();
    targetRow.classList.remove("drop-before", "drop-after");
    const ids = state.rooms.map((r) => r.id);
    ids.splice(ids.indexOf(dragged), 1);
    let ti = ids.indexOf(targetRow.dataset.room);
    if (targetRow.classList.contains("drop-after")) ti += 1;
    ids.splice(ti, 0, dragged);
    api("POST", "/api/rooms/reorder", { order: ids })
      .then(() => loadRooms())
      .catch((err) => toast(err.message));
  }

  async function selectRoom(id) {
    document.body.classList.remove("sidebar-open");
    state.currentRoomId = id;
    state.activeTab = "chat";
    state.lastMetaKey = "";
    // Read-Marker (Matrix-3): Öffnen markiert gelesen — jetzt AUTHORITATIV
    // (server room_reads; localStorage bleibt Fallback-Schicht): Reload-
    // überlebende Zähler (Befund P1#10 — client-only Zähler starben).
    state.lastRead[id] = Date.now();
    try { localStorage.setItem(`relay.lastRead.${id}`, String(state.lastRead[id])); } catch {}
    delete state.rooms.find((r) => r.id === id)?.unread;
    api("POST", `/api/rooms/${id}/read`).catch(() => {});
    state.newBelow = 0; updateNewPill();
    // Sprint-3a: Draft des Raums wiederherstellen (Matrix-9)
    const draft = restoreDraft(id);
    input.value = draft;
    sendBtn.disabled = !draft.trim();
    autoGrow();
    // Sprint-5b: document.title als Breadcrumb (Matrix-21, TDZ-safe:
    // room wird erst unten deklariert — Lookup direkt hier)
    const roomEarly = state.rooms.find((r) => r.id === id);
    document.title = (roomEarly?.name ? roomEarly.name + " · " : "") + "Agent Relay";
    renderRoomList();
    switchView("chat");
    const room = state.rooms.find((r) => r.id === id);
    $("#activeRoomIco").textContent = room?.kind === "project" ? "🗂" : "#";
    $("#activeRoomName").textContent = room?.name ?? "";
    updateBoardBadge(room);
    renderMemberChips(room);
    // P2#8 + Befund 3 (Codex-Audit): Rollen aus dem REST-Vertrag laden
    api("GET", `/api/rooms/${id}/roles`).then((roles) => {
      if (roles && state.currentRoomId === id) {
        state.roomRoles = roles || {};
        renderMemberChips(state.rooms.find((r) => r.id === state.currentRoomId));
      }
    }).catch(() => {});
    renderPinnedBanner(id);
    const isProject = room?.kind === "project";
    $("#roomTabs").style.display = isProject ? "inline-flex" : "none";
    document.querySelectorAll(".room-tab").forEach((t) =>
      t.classList.toggle("active", t.dataset.tab === "chat"));
    showTab("chat");
    await loadMessages();
    // B1 Muse: kein Autofokus beim Raumwechsel — der Composer-Rahmen bleibt
    // im Ruhe-Zustand (Hairline); Fokus gehört nur zur echten Nutzer-Absicht.
  }

  async function updateBoardBadge(room) {
    const badge = $("#boardBadge");
    if (!badge) return;
    if (!room || room.kind !== "project" || !room.projectId) { badge.hidden = true; return; }
    try {
      const tasks = await api("GET", `/api/tasks?projectId=${room.projectId}`);
      const open = tasks.filter((t) => t.status !== "done").length;
      badge.textContent = open;
      badge.hidden = open === 0;
    } catch { badge.hidden = true; }
  }

  function renderMemberChips(room) {
    const el = $("#memberChips");
    const members = (room?.memberAgentIds || [])
      .map((aid) => state.agents.find((a) => a.id === aid))
      .filter(Boolean);
    if (!members.length) {
      el.innerHTML = `<span class="hint">Noch keine Agents im Raum — unten im Team ziehen oder ＋ </span>`;
      return;
    }
    el.innerHTML = members.map((m) => `
      <span class="chip ${memberRole(m.id) === "admin" ? "role-admin" : ""}" data-aid="${m.id}"
            title="${memberRole(m.id) ? "Rolle: " + esc(memberRole(m.id)) + " · shift-Klick zurücksetzen" : "Klick: entfernen · shift-Klick: Rolle"}">
        <span class="swatch" style="background:${esc(m.color)}"></span>
        ${esc(m.emoji)} ${esc(m.name)}${memberRole(m.id) ? ' <span class="role-tag">★</span>' : ""}
      </span>`).join("");
    el.querySelectorAll("[data-aid]").forEach((c) =>
      c.addEventListener("click", async (ev) => {
        // P2#8: Shift-Klick = Rollen-Cycle ('' → admin → ''); sonst entfernen
        if (ev.shiftKey) {
          const next = memberRole(c.dataset.aid) === "admin" ? "" : "admin";
          try {
            await api("PATCH", `/api/rooms/${state.currentRoomId}/members/${c.dataset.aid}/role`, { role: next });
            state.roomRoles[c.dataset.aid] = next;
            renderMemberChips(state.rooms.find((r) => r.id === state.currentRoomId));
          } catch (err) { toast(err.message); }
          return;
        }
        if (!confirm(`${c.textContent.replace("★", "").trim()} aus dem Raum entfernen?`)) return;
        await api("DELETE", `/api/rooms/${state.currentRoomId}/members/${c.dataset.aid}`);
        await loadRooms(); await selectRoom(state.currentRoomId);
      }));
  }

  // ═════════════════════════════════════════════════════════════════
  // Nachrichten (Slack-Dichte: Meta nur bei Gruppenstart)
  // ═════════════════════════════════════════════════════════════════
  const streamBuf = {};
  // Sprint-3b: Turn-Chronometrie (Matrix-12, "Worked for…"-Fold)
  const turnTimers = {};
  const turnStats = {};
  // Sprint-4 (Matrix-4): Agent-Aktivität je Raum-Fenster für Presence-
  // Ableitung — idle = enabled UND >=90 s ohne Turn; online sonst; off
  // (disabled) überschreibt beides. WS-geboren (agents.changed/typing),
  // kein Poll (buzz presence/hooks.ts-Muster).
  const agentActivity = {};
  function markAgentActive(id) { if (id) agentActivity[id] = Date.now(); }
  function agentPresence(a) {
    if (!a.enabled) return "off";
    const act = agentActivity[a.id];
    if (!act) return "idle";
    return Date.now() - act < 90000 ? "online" : "idle";
  }

  async function loadMessages() {
    if (!state.currentRoomId) return;
    state.messages = {};
    Object.keys(streamBuf).forEach((k) => delete streamBuf[k]);
    const list = await api("GET", `/api/rooms/${state.currentRoomId}/messages`);
    for (const m of list) state.messages[m.id] = m;
    await loadReactions();
    renderMessages(true);
  }

  function renderMessages(force = false) {
    const wrap = $("#messages");
    const msgs = Object.values(state.messages)
      .filter((m) => m.roomId === state.currentRoomId)
      .sort((a, b) => String(a.createdAt).localeCompare(String(b.createdAt)));
    // Follow-Scroll-Vertrag (Matrix-11, OpenClaw chat.md:723-732):
    // "at or near the end" (48 px-Toleranz) — nur dann auto-follow.
    // Pill zählt hier NICHT: neue Messages zählt der message.created-Handler
    // (sonst doppelter Zähler bei jedem Re-Render).
    const near = force || nearBottom();
    if (!msgs.length) {
      wrap.innerHTML = emptyStateHtml();
      return;
    }
    wrap.innerHTML = msgs.map((m, i) => msgHtml(m, msgs[i - 1])).join("");
    wireReactionHandlers();
    if (near) { scrollBottom(true); state.following = true; }
    else { state.following = false; updateNewPill(); }
  }

  const QUICK_REACTIONS = ["👍", "❤️", "😄", "🤔", "🔥"];
  async function loadReactions() {
    const ids = Object.keys(state.messages);
    if (!ids.length) { state.reactions = {}; return; }
    try {
      state.reactions = await api("POST", "/api/reactions/batch", { messageIds: ids }) || {};
    } catch { state.reactions = {}; }
    for (const [mid, list] of Object.entries(state.reactions)) {
      for (const r of list) {
        if (r.actors.includes("gaRON") || r.actors.includes("Du")) r.actors = ["Du", ...r.actors.filter((x) => x !== "Du")];
      }
    }
  }

  function wireReactionHandlers() {
    const wrap = $("#messages");
    // P1#4: thread-count → zur ersten Antwort scrollen + flash-highlight
    wrap.querySelectorAll("[data-topen]").forEach((b) =>
      b.addEventListener("click", () => {
        const rootId = b.dataset.topen;
        const first = Object.values(state.messages)
          .filter((m) => m.parentId === rootId)
          .sort((x, y) => String(x.createdAt).localeCompare(String(y.createdAt)))[0];
        if (!first) return;
        const el = document.querySelector(`[data-mid="${first.id}"]`);
        if (el) { el.scrollIntoView({ behavior: "smooth", block: "center" }); el.classList.add("flash"); setTimeout(() => el.classList.remove("flash"), 1600); }
      }));
    // Sprint-5c: Quick-Reaction-Buttons verdrahten (Matrix-34)
    wrap.querySelectorAll("[data-qr]").forEach((b) =>
      b.addEventListener("click", async () => {
        const [mid, emoji] = b.dataset.qr.split(":");
        try {
          await api("POST", `/api/messages/${mid}/reactions`, { actor: "Du", emoji });
          await loadReactions(); renderMessages();
        } catch (err) { toast(err.message); }
      }));
    // Sprint-3c: Hover-Aktionen verdrahten (Matrix-33)
    wrap.querySelectorAll("[data-mact]").forEach((b) =>
      b.addEventListener("click", (ev) => {
        ev.stopPropagation();
        const id = b.dataset.x;
        const m = state.messages[id];
        if (!m) return;
        if (b.dataset.mact === "copy") {
          (navigator.clipboard?.writeText(m.content) || Promise.reject())
            .then(() => toast("Kopiert")).catch(() => toast("Kopieren fehlgeschlagen"));
        }
        if (b.dataset.mact === "reply") {
          const who = m.senderKind === "human" ? "du" : m.senderName;
          const quote = m.content.split("\n").slice(0, 3).map((l) => `> ${l}`).join("\n");
          // P1#4: Thread-Antwort — parent_id = Wurzel (bei Reply auf eine
          // Antwort deren Wurzel), Composer-Chip zeigt den Anker
          const rootId = m.parentId || m.id;
          state.threadMode = { rootId };
          const chip = $("#threadChip");
          if (chip) {
            chip.hidden = false;
            chip.querySelector(".tc-label").textContent = `↩ Antwort auf ${who}`;
            chip.querySelector(".tc-x").onclick = () => clearThreadMode();
          }
          input.value = `@${m.senderKind === "human" ? "" : who} ${quote}\n\n`;
          input.dispatchEvent(new Event("input"));
          input.focus();
        }
        if (b.dataset.mact === "edit") editMessageModal(m);
      }));
    wrap.querySelectorAll("[data-rxadd]").forEach((b) =>
      b.addEventListener("click", (ev) =>
        openReactionPicker(ev.currentTarget, b.dataset.rxadd)));
    wrap.querySelectorAll("[data-rx]").forEach((b) =>
      b.addEventListener("click", async () => {
        const [mid, emoji] = b.dataset.rx.split(":");
        const mine = b.classList.contains("mine");
        try {
          await api(mine ? "DELETE" : "POST",
                    mine ? `/api/messages/${mid}/reactions/${encodeURIComponent(emoji)}?actor=Du`
                         : `/api/messages/${mid}/reactions`,
                    mine ? null : { actor: "Du", emoji });
        } catch (err) { toast(err.message); }
        await loadReactions(); renderMessages();
      }));
  }

  // Sprint-3c: Eigene Nachricht bearbeiten (Modal; PATCH-Endpoint folgt P1#1 —
  // solange fehlt serverseitiges PATCH speichert der Submit lokal nicht;
  // Guard unten fängt den Fehler und rekonstruiert den Text)
  function editMessageModal(m) {
    openModal("Nachricht bearbeiten", [
      { label: "Text", name: "content", type: "textarea", value: m.content, rows: 4 },
    ], async (form) => {
      // openModal-Konvention: onSubmit bekommt das form-Element (agentModal-
      // Muster data=form) — form.content.value, nicht data.content.
      const c = (form.content?.value || "").trim();
      if (!c) return;
      try {
        await api("PATCH", `/api/messages/${m.id}`, { content: c });
      } catch (err) { toast(err.message); }
    });
  }

  function openReactionPicker(anchor, messageId) {
    openPopover(anchor, "Reaktion", QUICK_REACTIONS.map((e) => ({
      label: `${e} ${e}`,
      run: async () => {
        await api("POST", `/api/messages/${messageId}/reactions`, { actor: "Du", emoji: e });
        await loadReactions(); renderMessages();
      },
    })));
  }

  // Follow-Scroll-Vertrag (Matrix-11): "at or near the end" = 48 px wie in
  // der OpenClaw-Chat-UI (docs/gateway/protocol … chat.md:723-732)
  function nearBottom() {
    const wrap = $("#messages");
    return wrap.scrollHeight - wrap.scrollTop - wrap.clientHeight <= 48;
  }
  // "N neu ↓"-Pill (Matrix-11): über dem Composer, Klick = scroll to bottom,
  // Zähler zählt neue Frames während non-follow
  function updateNewPill() {
    const pill = $("#newBelowPill");
    if (!pill) return;
    const n = state.newBelow || 0;
    pill.hidden = n === 0;
    if (n) pill.textContent = `${n} neu · ↓`;
  }
  function scrollBottom(force) {
    const wrap = $("#messages");
    wrap.scrollTop = force ? wrap.scrollHeight : wrap.scrollHeight;
    state.following = true;
    state.newBelow = 0;
    updateNewPill();
  }

  function emptyStateHtml() {
    const room = state.rooms.find((r) => r.id === state.currentRoomId);
    if (!room) return `<div class="empty-state"><div class="big">⌁</div>Erstelle einen Raum — <kbd>＋ Raum</kbd> links unten.</div>`;
    if (room.kind === "project") {
      return `<div class="empty-state"><div class="big">🗂</div>Projekt-Raum bereit.<div class="hint">Chat hier abhalten oder zum <strong>Board</strong>-Tab wechseln. Agents per Drag & Drop ins Roster.</div></div>`;
    }
    return `<div class="empty-state"><div class="big">#</div>Noch ruhig hier.<div class="hint">Schreib die erste Nachricht — <kbd>@Name</kbd> ruft gezielt einen Agent auf.</div></div>`;
  }

  function msgHtml(m, prev) {
    if (m.senderKind === "system") {
      return `<div class="msg system" data-mid="${m.id}"><div class="content">${esc(m.content)}</div></div>`;
    }
    const agent = m.agentId ? state.agents.find((a) => a.id === m.agentId) : null;
    const rx = (state.reactions[m.id] || [])
      .map((r) => `<button class="rx-chip ${r.actors.includes("Du") ? "mine" : ""}" data-rx="${m.id}:${esc(r.emoji)}" title="${esc(r.actors.join(", "))}">${esc(r.emoji)} ${r.count}</button>`).join("");
    // Sprint-3c (Matrix-33): Hover-Aktionen (Kopieren/Antworten/Bearbeiten)
    // neben dem Reaktions-＋ — opacity-0→1 per .msg:hover (CSS), Touch immer
    const actions = m.senderKind === "system" ? "" :
      `<span class="msg-act">
        <button class="m-action" data-mact="copy" data-x="${m.id}" title="Kopieren">⧉</button>
        <button class="m-action" data-mact="reply" data-x="${m.id}" title="Antworten (zitieren)">↩</button>
        ${m.senderKind === "human" ? `<button class="m-action" data-mact="edit" data-x="${m.id}" title="Bearbeiten">✎</button>` : ""}
      </span>`;
    // P1#4: Thread-Marker — Antworten zeigen "↩", Wurzeln zeigen ReplyCount
    const threadMark = m.parentId ? `<div class="thread-ref">↩ Antwort</div>` : "";
    const siblings = m.parentId ? 0 : Object.values(state.messages)
      .filter((x) => x.roomId === m.roomId && x.parentId === m.id).length;
    const replyBadge = siblings > 0
      ? `<button class="thread-count" data-topen="${m.id}" title="Antworten anzeigen">${siblings} ↩</button>` : "";
    // P1#2: Anhänge unter der Message rendern (Vorschau: Bilder inline,
    // Dateien als Chip mit Download-Link)
    const atts = (m.attachments || []);
    const attHtml = atts.length ? `<div class="att-list">` + atts.map((at) => {
      const url = `api/attachments/${at.id}`;
      if ((at.mime || "").startsWith("image/")) {
        return `<a class="att att-img" href="${url}" target="_blank" rel="noopener"><img src="${url}" alt="${esc(at.filename)}" loading="lazy" /><span class="att-name">${esc(at.filename)}</span></a>`;
      }
      return `<a class="att att-file" href="${url}"><span class="att-ico">📄</span><span class="att-name">${esc(at.filename)}</span><span class="att-size">${fmtSize(at.size)}</span></a>`;
    }).join("") + `</div>` : "";
    // Sprint-5c (Matrix-34): Quick-Reaction-Hoverbar — 4 Emojis als
    // hover-sichtbare Buttons (opacity-0→1); ein Klick = POST+Rerender.
    // Voll-Picker (＋) bleibt daneben.
    const quickBar = m.senderKind === "system" ? "" :
      `<span class="quick-rx">` + ["👍", "❤️", "🎉", "👀"].map((e) =>
        `<button class="qr" data-qr="${m.id}:${e}" title="Reagiere ${e}">${e}</button>`).join("") + `</span>`;
    const rxBar = m.senderKind === "system" ? "" :
      `<div class="rx-bar">${rx}<button class="rx-add" data-rxadd="${m.id}" title="Reaktion">＋</button>${quickBar}${actions}</div>`;
    const who = m.senderKind === "human" ? "du" : m.senderName;
    const groupFirst = !prev || prev.senderKind !== m.senderKind || prev.senderName !== m.senderName;
    const time = fmtTime(m.createdAt);
    const streaming = m.status === "streaming" && streamBuf[m.id] !== undefined;
    const cls = ["msg", m.senderKind, m.status === "error" ? "error" : "", streaming ? "streaming" : "", groupFirst ? "group-first" : ""].filter(Boolean).join(" ");
    const swatch = agent ? `<span class="swatch" style="background:${esc(agent.color)}"></span>` : "";
    // Sprint-3b (Matrix-12): Agent-Turns als einklappbare Gruppe
    // (details/summary — plain-JS-Form des OpenClaw-Disclosures).
    // Head: "schreibt seit X s" live / nach Fertigstellung "X s · Y Frames".
    if (agent && (streaming || m.status === "complete")) {
      const st = turnStats[m.id];
      const headLabel = streaming
        ? `<span class="turn-head-label">schreibt…</span>`
        : (st ? `<span class="turn-head-label">${st.secs} s · ${st.tokens} Frames</span>` : `<span class="turn-head-label">fertig</span>`);
      const body = streaming ? (streamBuf[m.id] ?? "") : m.content;
      return `
      <details class="${cls} turn" data-mid="${m.id}" ${streaming ? "open" : ""}>
        <summary class="meta">
          ${swatch}<span class="who">${esc(who)}</span>
          ${headLabel}
          ${replyBadge}
          <span class="time">${esc(time)}</span>
        </summary>
        ${threadMark}
        <div class="content">${renderContent(body)}</div>
        ${attHtml}
        ${streaming ? "" : rxBar}
      </details>`;
    }
    return `
      <div class="${cls}" data-mid="${m.id}">
        <div class="meta">${swatch}<span class="who">${esc(who)}</span><span class="time">${esc(time)}</span></div>
        ${threadMark}
        <div class="content">${renderContent(streaming ? streamBuf[m.id] : m.content)}</div>
        ${attHtml}
        ${replyBadge}
        ${rxBar}
      </div>`;
  }
  function humanName() {
    // Backend-Fallback-Identität (RELAY_USER_NAME, default gaRON): der Browser
    // kennt die env nicht — sessionStorage-Bridge (Relay-Name aus settings)
    const n = state.settings?.relayUser ?? "";
    return String(n || "gaRON");
  }
  // ── P2#12: Slash-Command-Handling (Paperclip-Vertrag) ──
  function handleSlashCommand(raw) {
    const [cmd, ...rest] = raw.trim().split(/\s+/);
    const arg = rest.join(" ").trim();
    const room = state.rooms.find((r) => r.id === state.currentRoomId);
    const cmds = {
      "/help": () => {
        openModal("Slash-Commands", [], () => {});
        const dlg = $("#modal");
        const list = document.createElement("div");
        list.className = "shortcut-list";
        list.innerHTML = [
          ["/task Titel", "Neue Aufgabe im Projekt-Raum"],
          ["/topic Thema", "Raum-Thema setzen (P1#8)"],
          ["/pin", "Letzte Nachricht anpinnen"],
          ["/unpin", "Lospinen"],
          ["/help", "Diese Übersicht"],
        ].map(([k, d]) => `<div class="shortcut-row"><span>${esc(d)}</span><kbd>${esc(k)}</kbd></div>`).join("");
        dlg.querySelector("form").before(list);
      },
      "/task": async () => {
        if (!room) return;
        if (room.kind === "project" || !room.projectId) { toast("Slash /task braucht Projekt-Raum"); return; }
        if (!arg) { toast("Usage: /task Titel"); return; }
        await api("POST", "/api/tasks", { projectId: room.projectId, title: arg, status: "todo" });
        renderInlineBoard();
        toast("Aufgabe angelegt");
      },
      "/topic": async () => {
        if (!room || !arg) { toast("Usage: /topic Thema"); return; }
        await api("PATCH", `/api/rooms/${room.id}`, { topic: arg });
        await loadRooms();
        toast("Thema gesetzt");
      },
      "/pin": async () => { await pinLastMessage(room); },
      "/unpin": async () => { await pinLastMessage(room, true); },
    };
    const fn = cmds[cmd.toLowerCase()];
    if (fn) fn();
    else { toast(`Unbekannt: ${cmd} — /help`); input.value = ""; sendBtn.disabled = true; }
  }
  async function pinLastMessage(room, unpin) {
    if (!room) return;
    const lastId = Object.values(state.messages)
      .filter((m) => m.roomId === room.id && m.senderKind !== "system")
      .sort((x, y) => String(y.createdAt).localeCompare(String(x.createdAt)))[0]?.id || "";
    const id = unpin ? "" : lastId;
    if (!unpin && !lastId) { toast("Keine Nachricht zum Anpinnen"); return; }
    try {
      await api("PATCH", `/api/rooms/${room.id}`, { pinnedMessageId: id });
      await loadRooms();
      renderPinnedBanner(room.id);
    } catch (err) { toast(err.message); }
  }
  function fmtSize(n) {
    if (n == null) return "";
    if (n > 1048576) return (n / 1048576).toFixed(1) + " MB";
    if (n > 1024) return Math.round(n / 1024) + " KB";
    return n + " B";
  }


  function renderContent(raw) {
    // escape then turn @Name tokens into mention chips
    let s = esc(raw);
    // match @ followed by word chars/hyphens, render as chip
    s = s.replace(/@([\w-]+)/g, '<span class="mention-chip">@$1</span>');
    return s;
  }

  function fmtTime(iso) {
    try {
      return new Date(iso).toLocaleTimeString("de-DE", { hour: "2-digit", minute: "2-digit" });
    } catch { return ""; }
  }

  // ═════════════════════════════════════════════════════════════════
  // Composer + Mention-Popover
  // ═════════════════════════════════════════════════════════════════
  const input = $("#composerInput");
  const sendBtn = $("#composerSend");

  // ── Sprint-3a: Drafts pro Raum (Matrix-9, buzz draftSubmitKey.ts) ──
  // write-on-input (debounced 300 ms) in localStorage[relay.draft.<roomId>],
  // Restore beim Raumwechsel, Clear beim Senden.
  let draftTimer = null;
  function saveDraft() {
    const rid = state.currentRoomId;
    if (!rid) return;
    clearTimeout(draftTimer);
    draftTimer = setTimeout(() => {
      try {
        if (input.value) localStorage.setItem(`relay.draft.${rid}`, input.value);
        else localStorage.removeItem(`relay.draft.${rid}`);
      } catch {}
    }, 300);
  }
  function restoreDraft(rid) {
    try { return localStorage.getItem(`relay.draft.${rid}`) || ""; } catch { return ""; }
  }

  // "N neu ↓"-Pill (Matrix-11): Klick = scroll to bottom + Resync
  $("#newBelowPill")?.addEventListener("click", () => {
    scrollBottom(true); renderMessages(true);
  });
  // Scroll-Listener: manuelles Scrollen setzt following korrekt (OpenClaw-
  // auto-follow-controller-Muster): nur Near-Bottom = following
  $("#messages")?.addEventListener("scroll", () => {
    const near = nearBottom();
    state.following = near;
    if (near && state.newBelow) { state.newBelow = 0; updateNewPill(); }
  }, { passive: true });

  input.addEventListener("input", () => {
    sendBtn.disabled = !input.value.trim();
    autoGrow();
    mentionCheck();
    saveDraft();
  });
  input.addEventListener("keydown", (ev) => {
    if (mentionKey(ev)) return;
    if (ev.key === "Enter" && !ev.shiftKey) {
      ev.preventDefault();
      $("#composer").requestSubmit();
    }
  });
  function autoGrow() {
    input.style.height = "auto";
    input.style.height = Math.min(input.scrollHeight, 160) + "px";
  }

  $("#composer").addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const content = input.value.trim();
    if (!content || !state.currentRoomId) return;
    input.value = ""; autoGrow(); sendBtn.disabled = true; hideMention();
    // P2#12: Slash-Commands (Paperclip /-Vertrag): /task, /topic, /pin,
    // /help — der Rest bleibt normales Senden; Commands konsumieren den Text.
    if (content.startsWith("/")) {
      handleSlashCommand(content);
      return;
    }
    // Sprint-3a: gesendeter Draft wird gelöscht (buzz-Pattern)
    try { localStorage.removeItem(`relay.draft.${state.currentRoomId}`); } catch {}
    // P1#4: Thread-Anker der Antwort
    const parentId = state.threadMode?.rootId || "";
    let posted;
    try {
      posted = await api("POST", `/api/rooms/${state.currentRoomId}/messages`, parentId ? { content, parentId } : { content });
    } catch (err) {
      pushLocalSystem(err.message);
      input.value = content; autoGrow(); sendBtn.disabled = false;
      return;
    }
    // P1#2: angehängte Dateien an die neue Message hängen (multipart je
    // Datei — POST /api/messages/{id}/attachments) und Auswahlliste leeren
    const files = composerFiles?.files;
    if (posted?.id && files && files.length) {
      for (const f of files) {
        const fd = new FormData();
        fd.append("file", f);
        try {
          await fetch(url(`/api/messages/${posted.id}/attachments`), {
            method: "POST",
            headers: bearer() ? { "Authorization": `Bearer ${bearer()}` } : {},
            body: fd,
          });
        } catch (err) { toast(err.message); }
      }
      composerFiles.value = "";
      updateAttachLabel();
    }
    if (parentId) clearThreadMode();
  });
  function renderPinnedBanner(roomId) {
    const wrap = $("#pinnedBanner");
    if (!wrap) return;
    const room = state.rooms.find((r) => r.id === roomId);
    const pid = room?.pinnedMessageId;
    const m = pid ? state.messages[pid] : null;
    if (!pid || !m) { wrap.hidden = true; wrap.innerHTML = ""; return; }
    wrap.hidden = false;
    wrap.innerHTML = '<span class="pb-ico">📌</span><span class="pb-text">'
      + esc((m.content || "").replace(/\s+/g, " ").slice(0, 140))
      + '</span><button type="button" class="pb-x" title="Lospinen">✕</button>';
    wrap.querySelector(".pb-x").addEventListener("click", async () => {
      try {
        await api("PATCH", `/api/rooms/${roomId}`, { pinnedMessageId: "" });
        await loadRooms();
        renderPinnedBanner(roomId);
      } catch (err) { toast(err.message); }
    });
  }
  function clearThreadMode() {
    state.threadMode = null;
    const chip = $("#threadChip");
    if (chip) chip.hidden = true;
  }

  // P1#2: Attach-Button + Auswahl-Anzeige (📎 N)
  const composerFiles = $("#composerFiles");
  const attachBtn = $("#composerAttach");
  function updateAttachLabel() {
    const n = composerFiles?.files?.length || 0;
    attachBtn.textContent = n ? `📎 ${n}` : "📎";
    if (n) input.placeholder = `Nachricht + ${n} ${n === 1 ? "Anhang" : "Anhänge"}…`;
    else input.placeholder = "Nachricht in den Raum…  (@Agent ruft gezielt auf)";
  }
  attachBtn?.addEventListener("click", () => composerFiles?.click());
  composerFiles?.addEventListener("change", updateAttachLabel);

  // Mention: "@frag" im Text → Vorschlagsliste
  let mentionSel = 0, mentionChoices = [];
  function mentionCheck() {
    const upTo = input.value.slice(0, input.selectionStart ?? 0);
    const m = upTo.match(/@([\w-]*)$/);
    if (!m) return hideMention();
    const q = m[1].toLowerCase();
    const activeInRoom = (state.rooms.find((r) => r.id === state.currentRoomId)?.memberAgentIds) || [];
    mentionChoices = state.agents.filter((a) => a.enabled)
      .filter((a) => a.name.toLowerCase().includes(q) || a.id.toLowerCase().includes(q))
      .slice(0, 6);
    // P1#4-Lane 2 (Matrix-7): nicht im Raum = invite-Zeile statt Sackgasse
    const candidates = state.agents.filter((a) => a.enabled && !activeInRoom.includes(a.id))
      .filter((a) => a.name.toLowerCase().includes(q) || a.id.toLowerCase().includes(q));
    if (!mentionChoices.length && !candidates.length) return hideMention();
    mentionSel = 0;
    const pop = $("#mentionPop");
    const rowsH = mentionChoices.map((a, i) => `
      <button type="button" class="${i === mentionSel ? "sel" : ""}" data-mid="${a.id}">
        <span class="swatch" style="background:${esc(a.color)}"></span>${esc(a.emoji)} <strong>${esc(a.name)}</strong>
      </button>`).join("")
      // invite-inline: "@x einladen?" je Nicht-Member-Kandidat (Matrix-7: inline statt Dialog)
      + candidates.slice(0, 3).map((a) => `
      <button type="button" class="invite-row" data-invid="${a.id}" title="Zum Raum hinzuziehen + erwähnen">
        <span class="inv-ico">＋</span> ${esc(a.emoji)} <strong>${esc(a.name)}</strong> einladen?
      </button>`).join("");
    pop.innerHTML = rowsH;
    pop.hidden = false;
    pop.querySelectorAll("[data-mid]").forEach((b) =>
      b.addEventListener("click", () => applyMention(b.dataset.mid)));
    pop.querySelectorAll("[data-invid]").forEach((b) =>
      b.addEventListener("click", async () => {
        const aid = b.dataset.invid;
        const rid = state.currentRoomId;
        if (!rid) return;
        try {
          const room = state.rooms.find((r) => r.id === rid);
          const kind = room?.kind;
          if (kind === "project") {
            const ids = [...(room?.memberAgentIds || []), aid];
            await api("PATCH", `/api/rooms/${rid}`, { agentIds: ids });
          } else {
            await api("POST", `/api/rooms/${rid}/members`, { agentId: aid });
          }
          await loadRooms();
          await selectRoom(rid);
          applyMention(aid);
        } catch (err) { toast(err.message); }
      }));
  }
  function mentionKey(ev) {
    if ($("#mentionPop").hidden) return false;
    const pop = $("#mentionPop");
    if (ev.key === "ArrowDown" || ev.key === "ArrowUp") {
      ev.preventDefault();
      mentionSel = (mentionSel + (ev.key === "ArrowDown" ? 1 : mentionChoices.length - 1)) % mentionChoices.length;
      pop.querySelectorAll("button").forEach((b, i) => b.classList.toggle("sel", i === mentionSel));
      return true;
    }
    if (ev.key === "Enter" || ev.key === "Tab") {
      ev.preventDefault(); applyMention(mentionChoices[mentionSel].id); return true;
    }
    if (ev.key === "Escape") { hideMention(); return true; }
    return false;
  }
  function applyMention(agentId) {
    const a = state.agents.find((x) => x.id === agentId);
    if (!a) return;
    const start = input.selectionStart ?? input.value.length;
    const before = input.value.slice(0, start).replace(/@[\w-]*$/, `@${a.name} `);
    input.value = before + input.value.slice(start);
    hideMention(); autoGrow(); input.focus();
  }
  function hideMention() { $("#mentionPop").hidden = true; }

  function bindRosterDrag(root) {
    (root || document).querySelectorAll('[draggable="true"][data-agent]').forEach((chip) => {
      chip.addEventListener("dragstart", (ev) => {
        ev.dataTransfer.setData("text/agent-id", chip.dataset.agent);
        ev.dataTransfer.effectAllowed = "copyMove";
      });
    });
    bindRoomDropTargets((root || document));
  }

  // Drop-Ziele im Chat (GaRoN 20:49: „Wo soll man die Mitglieder rein ziehen?
  // In den Chat geht bisher nicht“): member-row, chat-head + #messages
  // akzeptieren text/agent-id → Member des aktuellen Raums (HUMAN = humanMember).
  function bindRoomDropTargets(rootEl) {
    const targets = (rootEl.nodeType === 9 ? [rootEl.querySelector(".member-row"), rootEl.querySelector(".chat-head"), rootEl.querySelector("#messages")] : [rootEl.closest(".member-row") || rootEl]).filter(Boolean);
    targets.forEach((t) => {
      if (!t || t.__rosterDrop) return;
      t.__rosterDrop = true;
      t.addEventListener("dragover", (ev) => {
        if (!ev.dataTransfer.types.includes("text/agent-id")) return;
        ev.preventDefault();
        ev.dataTransfer.dropEffect = "copy";
        t.classList.add("member-drop");
      });
      t.addEventListener("dragleave", () => t.classList.remove("member-drop"));
      t.addEventListener("drop", async (ev) => {
        t.classList.remove("member-drop");
        const agentId = ev.dataTransfer.getData("text/agent-id");
        if (!agentId || !state.currentRoomId) return;
        ev.preventDefault();
        const rid = state.currentRoomId;
        try {
          if (agentId === HUMAN) {
            await api("PATCH", `/api/rooms/${rid}`, { humanMember: true });
          } else {
            const room = state.rooms.find((r) => r.id === rid);
            if (room?.memberAgentIds?.includes(agentId)) return; // schon Mitglied
            await api("POST", `/api/rooms/${rid}/members`, { agentId });
          }
          await loadRooms();
          await selectRoom(rid);
        } catch (err) { toast(err.message); }
      });
    });
  }

  // ═════════════════════════════════════════════════════════════════
  // Roster (Team) — Drag-Quellen / Rückgabe-Ziele
  // ═════════════════════════════════════════════════════════════════
  function renderRoster() {
    const list = $("#rosterList");
    if (!list) return;
    list.innerHTML = [
      `<span class="roster-chip" data-agent="${HUMAN}" draggable="true" title="In einen Raum ziehen (du) oder hierher zurück = Raum verlassen">
        <span class="avatar" style="background:var(--ok)">G</span>du<span class="presence on"></span></span>`,
      ...state.agents.map((a) => {
        // Sprint-4 (Matrix-4/5): Presence-3-Zustand + Avatar-Dimming
        const p = agentPresence(a);
        return `
        <span class="roster-chip ${p}" draggable="true" data-agent="${a.id}" title="${esc(a.name)}: ${p === "online" ? "online" : p === "idle" ? "idle" : "aus"}">
          <span class="avatar p-${p}" style="background:${esc(a.color)}">${esc(a.emoji)}</span>${esc(a.name)}<span class="presence ${p === "off" ? "" : p}"></span>
        </span>`;
      }),
    ].join("");
    list.querySelectorAll('[draggable="true"]').forEach((chip) => {
      chip.addEventListener("dragstart", (ev) => {
        ev.dataTransfer.setData("text/agent-id", chip.dataset.agent);
        ev.dataTransfer.effectAllowed = "copyMove";
      });
      chip.addEventListener("dragover", (ev) => { ev.preventDefault(); chip.classList.add("dragover"); });
      chip.addEventListener("dragleave", () => chip.classList.remove("dragover"));
      chip.addEventListener("drop", async (ev) => {
        ev.preventDefault(); chip.classList.remove("dragover");
        const agentId = ev.dataTransfer.getData("text/agent-id");
        if (!agentId || agentId === chip.dataset.agent || !state.currentRoomId) return;
        try {
          if (agentId === HUMAN) {
            await api("PATCH", `/api/rooms/${state.currentRoomId}`, { humanMember: false });
            state.currentRoomId = null;
            $("#activeRoomName").textContent = "Kein Raum gewählt";
            $("#memberChips").innerHTML = "";
          } else {
            await api("DELETE", `/api/rooms/${state.currentRoomId}/members/${agentId}`);
            await selectRoom(state.currentRoomId);
          }
          await loadRooms();
        } catch (err) { toast(err.message); }
      });
    });
  }

  // ═════════════════════════════════════════════════════════════════
  // Agents-View
  // ═════════════════════════════════════════════════════════════════
  async function loadRoutines() {
    state.routines = await api("GET", "/api/routines").catch(() => []);
    if (state.view === "settings") renderRoutinesPanel();
  }

  async function loadSkills() {
    state.skills = await api("GET", "/api/skills").catch(() => []);
    if (state.view === "agents") renderSkillsPanel();
  }

  async function loadAgents() {
    state.agents = await api("GET", "/api/agents");
    if (state.view === "agents") renderAgents();
    renderRoster();
    if (state.currentRoomId) renderMemberChips(state.rooms.find((r) => r.id === state.currentRoomId));
  }

  // Sprint-2 (Matrix-14, Paperclip lib/status-colors.ts als EINE Quelle):
  // Status-Registry — badge/dot/text je Zustand, überall aus dieser Funktion
  // bedient (Board-Tiles, Agent-Cards, Run-Zustände, Conn-Dot).
  // Farben: ok/warn/accent aus dem bestehenden Token-Satz (kein neues Rad).
  const STATUS_REGISTRY = {
    todo:  { label: "Zu tun",   dot: "var(--ink-4)", badge: "var(--s3)",  text: "var(--ink-3)" },
    doing: { label: "In Arbeit", dot: "var(--warn)",  badge: "rgba(245,158,11,.14)", text: "var(--warn)" },
    done:  { label: "Fertig",    dot: "var(--ok)",   badge: "rgba(34,197,94,.14)",  text: "var(--ok)" },
    error: { label: "Fehler",    dot: "var(--accent)", badge: "rgba(255,92,92,.14)", text: "var(--accent-hover)" },
    active: { label: "Aktiv",  dot: "var(--ok)",   badge: "rgba(34,197,94,.14)",  text: "var(--ok)" },
    off:    { label: "Aus",     dot: "var(--ink-4)", badge: "var(--s3)",  text: "var(--ink-4)" },
    streaming: { label: "Streamt", dot: "var(--accent)", badge: "rgba(255,92,92,.14)", text: "var(--accent-hover)" },
  };
  function statusBadge(status) {
    const s = STATUS_REGISTRY[status] || STATUS_REGISTRY.todo;
    return `<span class="badge-pill" style="color:${s.text};background:${s.badge}" title="${s.label}">● ${s.label}</span>`;
  }
  function statusDot(status) {
    const s = STATUS_REGISTRY[status] || STATUS_REGISTRY.todo;
    return `<span class="status-dot" style="background:${s.dot}" title="${s.label}"></span>`;
  }

  function renderAgents() {
    const el = $("#agentCards");
    if (!state.agents.length) {
      el.innerHTML = `<div class="empty-state" style="grid-column:1/-1"><div class="big">🤖</div>Noch keine Agents.<div class="hint">Lege den ersten an — Adapter <code>echo</code> funktioniert sofort.</div></div>`;
      return;
    }
    el.innerHTML = state.agents.map((a) => `
      <div class="card${a.enabled ? "" : " is-disabled"}">
        <div class="row">
          <span class="emoji">${esc(a.emoji)}</span>
          <div><div class="name">${esc(a.name)}</div>
            <div class="sub">${esc(a.adapter)}${a.model ? " · " + esc(a.model) : ""}</div></div>
          ${statusBadge(a.enabled ? "active" : "off")}
        </div>
        ${a.systemPrompt ? `<div class="desc">${esc(a.systemPrompt.slice(0, 130))}${a.systemPrompt.length > 130 ? "…" : ""}</div>` : ""}
        <div class="actions">
          <button class="btn" data-edit="${a.id}">Bearbeiten</button>
          <button class="btn danger" data-del="${a.id}">Löschen</button>
        </div>
      </div>`).join("");
    el.querySelectorAll("[data-edit]").forEach((b) => b.addEventListener("click", () => agentModal(b.dataset.edit)));
    renderAgentsBadge();
    renderSkillsPanel();
    el.querySelectorAll("[data-del]").forEach((b) =>
      b.addEventListener("click", async () => {
        const a = state.agents.find((x) => x.id === b.dataset.del);
        if (!confirm(`Agent „${a?.name}" löschen? (Räume verlieren ihn)`)) return;
        await api("DELETE", `/api/agents/${b.dataset.del}`);
        await loadAgents(); await loadRooms();
      }));
  }

  function agentModal(id) {
    const a = id ? state.agents.find((x) => x.id === id) : null;
    openModal(a ? "Agent bearbeiten" : "Neuer Agent", [
      { label: "Name", name: "name", type: "text", value: a?.name, required: true },
      { label: "Emoji", name: "emoji", type: "text", value: a?.emoji ?? "🤖" },
      { label: "Farbe (Chat-Zuordnung)", name: "color", type: "color", value: a?.color ?? "#5e6ad2" },
      { label: "Adapter", name: "adapter", type: "select", value: a?.adapter ?? "echo",
        options: state.adapters.map((x) => ({ value: x, label: x === "echo" ? "echo (Demo)" : x === "openai" ? "openai — OpenAI-kompatibel (OpenClaw /v1, Ollama, …)" : x === "openclaw" ? "openclaw — nativ (Gateway-WS)" : x })) },
      { label: "Modell / Agent-Ziel  ·  z. B. openclaw/coordinator", name: "model", type: "text", value: a?.model ?? "" },
      { label: "Basis-URL (leer = globale Einstellung)", name: "baseUrl", type: "text", value: a?.baseUrl ?? "" },
      { label: "API-Key (write-only — leer = unverändert)", name: "apiKey", type: "password", value: "" },
      { label: "System-Prompt", name: "systemPrompt", type: "textarea", value: a?.systemPrompt ?? "" },
      { label: "Aktiv", name: "enabled", type: "checkbox", checked: a ? !!a.enabled : true },
      { label: "Approval-Gate (Antworten brauchen Freigabe)", name: "approvalGate", type: "checkbox", checked: a ? !!a.approvalGate : false },
    ], async (form) => {
      const data = Object.fromEntries(new FormData(form).entries());
      data.enabled = form.enabled.checked;
      data.approvalGate = form.approvalGate?.checked || false;
      if (form.apiKey && !form.apiKey.value) {
        const { apiKey, ...rest } = data; data = rest; // write-only: nie leeren
      }
      if (id) await api("PATCH", `/api/agents/${id}`, data);
      else await api("POST", "/api/agents", data);
      await loadAgents(); await loadRooms();
      return true;
    });
  }

  // ═════════════════════════════════════════════════════════════════
  // Projekte (Launcher) + Board-Tab im Raum
  // ═════════════════════════════════════════════════════════════════
  const COLUMNS = [["todo", "Zu tun"], ["doing", "In Arbeit"], ["done", "Fertig"]];

  async function loadProjectsView() {
    state.projects = await api("GET", "/api/projects");
    state.tasks = await api("GET", "/api/tasks");
    const el = $("#projectList");
    if (!state.projects.length) {
      el.innerHTML = `<div class="empty-state" style="grid-column:1/-1"><div class="big">🗂</div>Noch keine Projekte.<div class="hint">Ein Projekt-Raum ist Chat + Board in einem.</div></div>`;
      return;
    }
    el.innerHTML = state.projects.map((p) => {
      const room = state.rooms.find((r) => r.projectId === p.id);
      const tasks = state.tasks.filter((t) => t.projectId === p.id);
      const doing = tasks.filter((t) => t.status === "doing").length;
      const done = tasks.filter((t) => t.status === "done").length;
      const preview = tasks.filter((t) => t.status !== "done").slice(0, 3);
      const members = (room?.memberAgentIds || []).map((aid) => state.agents.find((a) => a.id === aid)).filter(Boolean);
      return `
      <button class="proj-tile" data-open="${room?.id ?? ""}">
        <span class="tile-head"><span class="p-dot" style="background:${esc(p.color)};width:10px;height:10px;border-radius:50%"></span>
          <span class="tile-name">${esc(p.name)}</span>
          <span class="tile-meta" style="margin-left:auto"><span class="mono">${doing ? `${doing} aktiv · ` : ""}${done} fertig</span></span></span>
        <span class="tile-meta">${esc(p.description || "—")}</span>
        <span class="tile-preview">${preview.map((t) => `<span class="tp ${t.status === "doing" ? "tp-doing" : ""}">${esc(t.title)}${t.status === "doing" ? statusDot("doing") : ""}</span>`).join("") || '<span class="tile-meta">keine offenen Tasks</span>'}</span>
        <span class="tiles-members">${members.map((m) => `<span class="swatch" style="background:${esc(m.color)};width:10px;height:10px;border-radius:50%;display:inline-block;margin-right:-4px"></span>`).join("")}</span>
      </button>`;
    }).join("");
    el.querySelectorAll("[data-open]").forEach((b) =>
      b.addEventListener("click", () => { if (b.dataset.open) selectRoom(b.dataset.open); }));
  }

  function newProjectModal() {
    openModal("Neuer Projekt-Raum", [
      { label: "Name", name: "name", type: "text", required: true },
      { label: "Beschreibung", name: "description", type: "textarea" },
      { label: "Farbe", name: "color", type: "color", value: "#5e6ad2" },
      { label: "Agents im Raum (Mehrfachauswahl)", name: "agentIds", type: "select", options: [] },
    ], async (form) => {
      const data = Object.fromEntries(new FormData(form).entries());
      data.kind = "project";
      data.agentIds = [...form.agentIds.selectedOptions].map((o) => o.value);
      const room = await api("POST", "/api/rooms", data); // autolink erzeugt das Projekt
      await loadRooms(); await loadAgents();
      selectRoom(room.id);
      return true;
    });
    const agentSel = $("#modal [name=agentIds]");
    agentSel.setAttribute("multiple", "multiple");
    agentSel.size = Math.min(6, Math.max(3, state.agents.length));
    agentSel.innerHTML = state.agents.map((a) =>
      `<option value="${a.id}">${esc(a.emoji)} ${esc(a.name)}</option>`).join("");
  }

  // Board-Tab im Raum
  function showTab(tab) {
    state.activeTab = tab;
    const msgs = $("#messages"), board = $("#inlineBoard"), composer = $("#composer");
    if (tab === "board") {
      msgs.hidden = true; composer.style.display = "none"; board.hidden = false;
      renderInlineBoard();
    } else {
      board.hidden = true; msgs.hidden = false; composer.style.display = "flex";
    }
  }

  async function renderInlineBoard() {
    const room = state.rooms.find((r) => r.id === state.currentRoomId);
    state.currentInlinePid = room?.projectId ?? null;
    state.projects = await api("GET", "/api/projects");
    state.tasks = await api("GET", `/api/tasks${state.currentInlinePid ? `?projectId=${state.currentInlinePid}` : ""}`);
    const el = $("#inlineBoard");
    const projects = state.projects.filter((p) => !state.currentInlinePid || p.id === state.currentInlinePid);
    if (!projects.length) {
      el.innerHTML = `<div class="empty-state"><div class="big">🗂</div>Kein Projekt verknüpft.</div>`;
      return;
    }
    // P2#6: Swimlane-Filter (Chips über dem Board; state.boardFilter)
    const filter = state.boardFilter || "all";
    const chipRow = `<div class="board-filters">${[["all","Alles"],["todo","Zu tun"],["doing","In Arbeit"],["done","Fertig"]].map(([k, l]) =>
      `<button type="button" class="bf ${filter === k ? "on" : ""}" data-bfilter="${k}">${l}</button>`).join("")}</div>`;
    el.innerHTML = chipRow + `<div class="board-wrap">${projects.map((p) => {
      const tasksRaw = state.tasks.filter((t) => t.projectId === p.id);
      const tasks = filter === "all" ? tasksRaw : tasksRaw.filter((t) => t.status === filter);
      return `
      <div class="project">
        <div class="p-head">
          <span class="p-dot" style="background:${esc(p.color)};width:10px;height:10px;border-radius:50%"></span>
          <h3>${esc(p.name)}</h3>
        </div>
        <div class="columns">
          ${COLUMNS.map(([key, label]) => `
            <div class="column" data-col-key="${key}">
              <h4>${label} <span class="col-count">${tasks.filter((t) => t.status === key).length}</span></h4>
              ${tasks.filter((t) => t.status === key).map((t) => `
                <div class="task ${t.status === "done" ? "done" : ""}" draggable="true" data-taskdrag="${t.id}">
                  ${statusBadge(t.status)}
                  <span>${esc(t.title)}</span>
                  <span class="who">${t.assignee ? `@${esc(assigneeName(t.assignee))}` : ""}</span>
                  <span class="t-actions">
                    ${t.status !== "todo" ? `<button data-tmove="${t.id}:${prevStatus(t.status)}" title="zurück">←</button>` : ""}
                    ${t.status !== "done" ? `<button data-tmove="${t.id}:${nextStatus(t.status)}" title="weiter">→</button>` : ""}
                    <button data-tedit="${t.id}" title="Bearbeiten">✎</button>
                    <button data-tchat="${t.id}" title="Chatter anzeigen">💬</button>
                    <button data-tdel="${t.id}" title="löschen">✕</button>
                  </span>
                </div>`).join("") || `<div class="hint" style="padding:4px 2px">—</div>`}
            </div>`).join("")}
        </div>
        <button class="add-task" data-taskadd="${p.id}">＋ Aufgabe</button>
      </div>`;
    }).join("")}</div>`;
    el.querySelectorAll("[data-bfilter]").forEach((b) => b.addEventListener("click", () => {
      state.boardFilter = b.dataset.bfilter;
      renderInlineBoard();
    }));
        el.querySelectorAll("[data-taskadd]").forEach((b) => b.addEventListener("click", () => taskModal(b.dataset.taskadd)));
    el.querySelectorAll("[data-tmove]").forEach((b) => b.addEventListener("click", async () => {
      const [id, st] = b.dataset.tmove.split(":");
      await api("PATCH", `/api/tasks/${id}`, { status: st });
      renderInlineBoard();
    }));
    el.querySelectorAll("[data-tdel]").forEach((b) => b.addEventListener("click", async () => {
      await api("DELETE", `/api/tasks/${b.dataset.tdel}`);
      renderInlineBoard();
    }));
    el.querySelectorAll("[data-tedit]").forEach((b) => b.addEventListener("click", () => taskEditModal(b.dataset.tedit)));
    el.querySelectorAll("[data-tchat]").forEach((b) => b.addEventListener("click", () => toggleTaskChat(b.dataset.tchat)));
    // Lane 2 (Matrix-24): Spalten als Drop-Zonen — Task-Karten per HTML5-DnD
    // zwischen Status-Spalten; Anker: dataTransfer (real drag), Fallback
    // __lastTaskDrag (headless-DragEvent-Synthese ohne dataTransfer-Übernahme).
    el.querySelectorAll(".column").forEach((col) => {
      col.addEventListener("dragover", (ev) => { ev.preventDefault(); col.classList.add("drop-col"); });
      col.addEventListener("dragleave", () => col.classList.remove("drop-col"));
      col.addEventListener("drop", async (ev) => {
        ev.preventDefault(); col.classList.remove("drop-col");
        let tid = "";
        try { tid = ev.dataTransfer?.getData("text/task-id") || ""; } catch {}
        if (!tid) tid = window.__lastTaskDrag || "";
        if (!tid) return;
        const st = col.getAttribute("data-col-key");
        if (!st) return;
        try {
          await api("PATCH", `/api/tasks/${tid}`, { status: st });
          window.__lastTaskDrag = "";
          renderInlineBoard();
        } catch (err) { toast(err.message); }
      });
    });
    el.querySelectorAll("[data-taskdrag]").forEach((b) =>
      b.addEventListener("dragstart", (ev) => {
        ev.dataTransfer.setData("text/task-id", b.dataset.taskdrag);
        ev.dataTransfer.effectAllowed = "move";
        window.__lastTaskDrag = b.dataset.taskdrag;
      }));
    renderOpenChatPanels(el);
  }

  // P2#1: Chatter-Panel toggle + render (Liste + Input, WS-fed)
  async function toggleTaskChat(taskId) {
    state.chatTaskId = state.chatTaskId === taskId ? null : taskId;
    await renderInlineBoard();
  }
  async function renderOpenChatPanels(el) {
    if (!state.chatTaskId) return;
    const t = state.tasks.find((x) => x.id === state.chatTaskId);
    if (!t) { state.chatTaskId = null; return; }
    const card = el.querySelector(`.task .t-actions [data-tchat="${t.id}"]`)?.closest(".task");
    if (!card) return;
    const panel = document.createElement("div");
    panel.className = "task-chat";
    panel.dataset.forTask = t.id;
    const comments = await api("GET", `/api/tasks/${t.id}/comments`).catch(() => []);
    const items = (comments || []).map((c) => `
      <div class="tc-item ${c.authorKind}">
        <span class="tc-who">${c.authorKind === "agent" ? "🤖" : "🧑"} ${esc(c.authorName)}</span>
        <span class="tc-text">${esc(c.content)}</span>
        <span class="tc-time">${esc(fmtTime(c.createdAt))}</span>
      </div>`).join("") || `<div class="hint" style="padding:2px 0">— noch keine Kommentare —</div>`;
    panel.innerHTML = `
      <div class="task-chat-list">${items}</div>
      <form class="task-chat-form">
        <input type="text" name="content" placeholder="Kommentar… (Enter = posten)" aria-label="Kommentar" />
        <button type="submit" class="btn primary">↑</button>
      </form>`;
    panel.querySelector(".task-chat-form").addEventListener("submit", async (ev) => {
      ev.preventDefault();
      const inp = ev.target.querySelector("input");
      const v = inp.value.trim();
      if (!v) return;
      inp.value = "";
      await api("POST", `/api/tasks/${t.id}/comments`, { content: v });
      // Panel live: WS task.comment.created refresht; Fallback hier direkt
      const list = panel.querySelector(".task-chat-list");
      list.insertAdjacentHTML("beforeend", `<div class="tc-item human"><span class="tc-who">🧑 ${esc(humanName())}</span><span class="tc-text">${esc(v)}</span><span class="tc-time">jetzt</span></div>`);
      list.scrollTop = list.scrollHeight;
    });
    panel.querySelector("input")?.focus?.();
    card.after(panel);
  }

  function taskEditModal(taskId) {
    const t = state.tasks.find((x) => x.id === taskId);
    if (!t) return;
    openModal("Aufgabe bearbeiten", [
      { label: "Titel", name: "title", type: "text", value: t.title, required: true },
      { label: "Notizen", name: "notes", type: "textarea", value: t.notes ?? "" },
      { label: "Status", name: "status", type: "select", value: t.status, options: COLUMNS.map(([v, l]) => ({ value: v, label: l })) },
      { label: "Agent", name: "assignee", type: "select", value: t.assignee ?? "", options: [
        { value: "", label: "— niemand —" },
        ...state.agents.map((a) => ({ value: a.id, label: `${a.emoji} ${a.name}` })),
      ]},
    ], async (form) => {
      const data = Object.fromEntries(new FormData(form).entries());
      await api("PATCH", `/api/tasks/${taskId}`, data);
      renderInlineBoard();
      return true;
    });
  }

  function assigneeName(assignee) {
    const a = state.agents.find((x) => x.id === assignee || x.name === assignee);
    return a ? a.name : String(assignee);
  }
  function nextStatus(s) { return s === "todo" ? "doing" : "done"; }
  function prevStatus(s) { return s === "done" ? "doing" : "todo"; }

  function taskModal(projectId) {
    openModal("Neue Aufgabe", [
      { label: "Titel", name: "title", type: "text", required: true },
      { label: "Notizen", name: "notes", type: "textarea" },
      { label: "Status", name: "status", type: "select", value: "todo",
        options: COLUMNS.map(([v, l]) => ({ value: v, label: l })) },
      { label: "Agent (Name)", name: "assignee", type: "text" },
    ], async (form) => {
      const data = Object.fromEntries(new FormData(form).entries());
      data.projectId = projectId;
      await api("POST", "/api/tasks", data);
      renderInlineBoard();
      return true;
    });
  }

  function projectModalEdit() { /* Project-Tile klick → Raum (Implementierung oben) */ }

  // ═════════════════════════════════════════════════════════════════
  // Einstellungen
  // ═════════════════════════════════════════════════════════════════
  async function renderSettingsForm() {
    state.settings = await api("GET", "/api/settings");
    const f = $("#settingsForm");
    f.relayName.value = state.settings.relayName ?? "";
    f.openclawBaseUrl.value = state.settings.openclawBaseUrl ?? "";
    f.openclawToken.value = ""; // write-only: nie den Wert zurückspielen
    $("#brandName").textContent = state.settings.relayName || "Agent Relay";
    renderRoutinesPanel(); // P2#4
  }

  $("#settingsForm").addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const f = ev.target;
    const patch = { relayName: f.relayName.value, openclawBaseUrl: f.openclawBaseUrl.value };
    if (f.openclawToken.value) patch.openclawToken = f.openclawToken.value; // nur bei Eingabe
    const saved = await api("PATCH", "/api/settings", patch);
    state.settings = saved;
    $("#brandName").textContent = saved.relayName || "Agent Relay";
    $("#settingsSaved").textContent = "✓ gespeichert";
    setTimeout(() => { $("#settingsSaved").textContent = ""; }, 2400);
  });

  // ═════════════════════════════════════════════════════════════════
  // Modal + Popover + Toast
  // ═════════════════════════════════════════════════════════════════
  function openModal(title, fields, onSubmit) {
    const wrap = $("#modalWrap"), dlg = $("#modal");
    wrap.hidden = false;
    dlg.innerHTML = `
      <h3>${esc(title)}</h3>
      <form novalidate>
        ${fields.map((f) => f.type === "select"
          ? `<label>${esc(f.label)}<select name="${f.name}">${f.options.map((o) =>
              `<option value="${esc(o.value)}" ${o.value === f.value ? "selected" : ""}>${esc(o.label)}</option>`).join("")}</select></label>`
          : f.type === "checkbox"
          ? `<label class="checkrow"><input type="checkbox" name="${f.name}" ${f.checked ? "checked" : ""}/>${esc(f.label)}</label>`
          : f.type === "textarea"
          ? `<label>${esc(f.label)}<textarea name="${f.name}" rows="${f.rows ?? 3}">${esc(f.value ?? "")}</textarea></label>`
          : `<label>${esc(f.label)}<input type="${f.type}" name="${f.name}" value="${esc(f.value ?? "")}" ${f.required ? "required" : ""}/></label>`).join("")}
        <div class="modal-actions">
          <button type="button" class="btn" data-cancel>Abbrechen</button>
          <button type="submit" class="btn primary">OK</button>
        </div>
      </form>`;
    dlg.querySelector("[data-cancel]").addEventListener("click", () => closeModal());
    const form = dlg.querySelector("form");
    form.addEventListener("submit", (ev) => {
      ev.preventDefault();
      if (!form.reportValidity()) return;
      Promise.resolve(onSubmit(form))
        .then((ok) => { if (ok !== false) closeModal(); })
        .catch((err) => toast(err.message));
    });
    dlg.querySelector("input,select,textarea")?.focus();
  }
  function closeModal() { $("#modalWrap").hidden = true; $("#modal").innerHTML = ""; }
  $("#modalWrap").addEventListener("mousedown", (ev) => {
    if (ev.target === $("#modalWrap")) closeModal();
  });

  let popEl = null;
  function openPopover(anchor, title, items) {
    closePopover();
    popEl = document.createElement("div");
    popEl.className = "pop";
    popEl.innerHTML = `${title ? `<div class="pop-head">${esc(title)}</div>` : ""}${items.map((it, i) =>
      `<button data-pi="${i}" class="${it.danger ? "danger" : ""}">${it.label}</button>`).join("")}`;
    document.body.appendChild(popEl);
    const r = anchor.getBoundingClientRect();
    const pw = popEl.offsetWidth, ph = popEl.offsetHeight;
    let x = Math.min(r.right - pw, window.innerWidth - pw - 8);
    let y = r.bottom + 6;
    if (y + ph > window.innerHeight - 8) y = r.top - ph - 6;
    popEl.style.left = Math.max(8, x) + "px";
    popEl.style.top = Math.max(8, y) + "px";
    popEl.querySelectorAll("[data-pi]").forEach((b) =>
      b.addEventListener("click", () => { closePopover(); items[Number(b.dataset.pi)].run(); }));
    setTimeout(() => document.addEventListener("mousedown", popDismiss), 0);
  }
  function popDismiss(ev) {
    if (popEl && !popEl.contains(ev.target)) closePopover();
  }
  function closePopover() {
    if (popEl) { popEl.remove(); popEl = null; document.removeEventListener("mousedown", popDismiss); }
  }

  function toast(msg) {
    const t = document.createElement("div");
    t.className = "toastbox";
    t.textContent = msg;
    document.body.appendChild(t);
    setTimeout(() => t.classList.add("out"), 2600);
    setTimeout(() => t.remove(), 3000);
  }

  // ═════════════════════════════════════════════════════════════════
  // Menü-Handler (Member-Add, Raummenü, Mobile-Drawer)
  // ═════════════════════════════════════════════════════════════════
  $("#memberAdd").addEventListener("click", () => {
    const room = state.rooms.find((r) => r.id === state.currentRoomId);
    if (!room) return toast("Erst einen Raum wählen");
    const outside = state.agents.filter((a) => !(room.memberAgentIds || []).includes(a.id));
    if (!outside.length) return toast("Alle Agents sind schon im Raum");
    openPopover($("#memberAdd"), "Agent einladen", outside.map((a) => ({
      label: `${a.emoji} ${a.name}`,
      run: async () => {
        await api("POST", `/api/rooms/${room.id}/members`, { agentId: a.id });
        await loadRooms(); await selectRoom(room.id);
      },
    })));
  });

  $("#roomMenu").addEventListener("click", () => {
    const room = state.rooms.find((r) => r.id === state.currentRoomId);
    if (!room) return toast("Erst einen Raum wählen");
    openPopover($("#roomMenu"), room.name, [
      { label: "✏️ Umbenennen", run: () => renameRoomModal(room) },
      { label: room.pinnedMessageId ? "📌 Lospinen" : "📌 Letzte Nachricht anpinnen", run: async () => {
        const lastId = Object.values(state.messages)
          .filter((m) => m.roomId === room.id && m.senderKind !== "system")
          .sort((x, y) => String(y.createdAt).localeCompare(String(x.createdAt)))[0]?.id || "";
        const id = room.pinnedMessageId ? "" : lastId;
        if (!room.pinnedMessageId && !lastId) { toast("Keine Nachricht zum Anpinnen"); return; }
        try {
          await api("PATCH", `/api/rooms/${room.id}`, { pinnedMessageId: id });
          await loadRooms();
          renderPinnedBanner(room.id);
        } catch (err) { toast(err.message); }
      } },
      { label: room.kind === "project" ? "💬 In Chat-Raum umwandeln" : "🗂 In Projekt-Raum umwandeln",
        run: async () => {
          await api("PATCH", `/api/rooms/${room.id}`, { kind: room.kind === "project" ? "chat" : "project" });
          await loadRooms(); await selectRoom(room.id);
        } },
      { label: "🗑 Raum löschen", danger: true, run: async () => {
        if (!confirm(`Raum „${room.name}" inklusive Verlauf löschen?`)) return;
        await api("DELETE", `/api/rooms/${room.id}`);
        state.currentRoomId = null;
        await loadRooms();
        if (state.rooms.length) selectRoom(state.rooms[0].id);
        else $("#activeRoomName").textContent = "Kein Raum gewählt";
      } },
    ]);
  });

  function renameRoomModal(room) {
    openModal("Raum umbenennen", [
      { label: "Name", name: "name", type: "text", value: room.name, required: true },
      { label: "Thema (optional)", name: "topic", type: "text", value: room.topic || "" },
    ], async (form) => {
      await api("PATCH", `/api/rooms/${room.id}`, { name: form.name.value, topic: form.topic?.value || "" });
      await loadRooms(); await selectRoom(room.id);
      return true;
    });
  }

  $("#burger").addEventListener("click", () =>
    document.body.classList.toggle("sidebar-open"));

  // Mobile-Back (GaRoN-Feedback 09:38): ← in Vollansichten = zurück zum Chat.
  // Desktop-Verhalten identisch (konsistente Navigation, kein Sackgassen-Layout).
  document.querySelectorAll(".view-back").forEach((b) =>
    b.addEventListener("click", () => switchView("chat")));

  // ── P3#4 Audit-Register (paperclip audit / buzz "receipts") ──
  // Register = Beweis-Schicht: agent.turn.* + relay.genesis als hash-chained
  // Kette; verify() rechnet die Kette nach (Beweis statt Behauptung).
  const AUDIT_ACTIONS = {
    "relay.genesis":       { ico: "🧾", label: "Register-Genesis" },
    "agent.turn.started":  { ico: "▶️", label: "Turn gestartet" },
    "agent.turn.finished": { ico: "✔️", label: "Turn beendet" },
    "agent.turn.error":    { ico: "⚠️", label: "Turn-Fehler" },
    "approval.granted":    { ico: "✅", label: "Approval erteilt" },
    "approval.denied":     { ico: "⛔", label: "Approval abgelehnt" },
  };
  function auditMeta(ev) { return AUDIT_ACTIONS[ev.action] || { ico: "•", label: ev.action }; }
  function auditDetail(d, key) { try { return JSON.parse(d || "{}")[key] ?? ""; } catch { return ""; } }
  async function loadAudit() {
    const data = await api("GET", "/api/audit?limit=100").catch(() => null);
    if (Array.isArray(data)) state.audit = data;
    if (state.view === "audit") renderAudit();
  }
  function renderAudit() {
    const list = $("#auditList");
    if (!list) return;
    if (!state.audit.length) {
      list.innerHTML = `<div class="audit-empty">🧾 Register leer — Agent-Turns erscheinen hier mit Beweis-Kette.</div>`;
      return;
    }
    list.innerHTML = state.audit.map((ev) => {
      const m = auditMeta(ev);
      const t = fmtTime(ev.createdAt);
      const bytes = auditDetail(ev.detail, "bytes");
      const detail = ev.action === "agent.turn.finished" && bytes !== ""
        ? `${esc(String(bytes))} B Antwort` : esc(auditDetail(ev.detail, "phase") || auditDetail(ev.detail, "adapter"));
      return `<div class="audit-row${ev.action.endsWith(".error") ? " err" : ""}">
        <span class="audit-ico">${m.ico}</span>
        <span class="audit-body"><b>${esc(m.label)}</b>
          <span class="a-sub">${esc(ev.actor)} · ${esc(ev.subject)}${detail ? " · " + detail : ""}</span></span>
        <span class="audit-right"><span class="a-seq" title="seq">№${ev.seq}</span><span class="a-time">${t}</span></span>
      </div>`;
    }).join("");
  }
  async function verifyAudit() {
    const out = $("#auditVerifyOut");
    if (!out) return;
    out.textContent = "…";
    const v = await api("GET", "/api/audit/verify").catch(() => null);
    if (!v) { out.textContent = "⚠️ verify fehlgeschlagen"; return; }
    out.innerHTML = v.ok
      ? `<span class="a-ok">✔ Kette intakt</span> — ${v.entries} Einträge geprüft`
      : `<span class="a-bad">✘ KETTE BRUCH bei ${esc(v.brokenAt)}</span> (${v.entries} Einträge vor Bruch verifiziert)`;
  }
  $("#auditVerifyBtn")?.addEventListener("click", verifyAudit);


  // ── Sprint-5a: Shortcuts (Matrix-19, buzz keyboard-shortcuts.ts) ──
  // Escape = Raum als gelesen (unread dot erlischt); ⇧Escape = alle;
  // Ctrl/⌘K = Suche; ? = Cheatsheet. Reihenfolge: mentionPop > modal >
  // globale Shortcuts (nicht in Inputs fangen ausser Ctrl-K/?).
  const SHORTCUTS = [
    { id: "esc-read", label: "Raum als gelesen", keys: "Esc" },
    { id: "esc-all", label: "Alle Räume als gelesen", keys: "⇧ Esc" },
    { id: "search", label: "Chat-Suche fokussieren", keys: "Ctrl/⌘ K" },
    { id: "help", label: "Diese Übersicht", keys: "?" },
    { id: "nav-chat", label: "Zu den Räumen", keys: "g c" },
    { id: "nav-agents", label: "Zu den Agents", keys: "g a" },
    { id: "nav-projects", label: "Zu den Projekten", keys: "g p" },
  ];
  let gPrefix = false;
  document.addEventListener("keydown", (ev) => {
    // Vorrang: mention-pop (eigener handler) und offenes modal (verarbeitet
    // Escape separat via #modal-keydown unten)
    const modalOpen = !$("#modalWrap")?.hidden;
    const inField = ["INPUT", "TEXTAREA"].includes(document.activeElement?.tagName);
    // Ctrl/⌘K: auch im Input erlauben (Standard-Such-Hotkey)
    if ((ev.ctrlKey || ev.metaKey) && ev.key.toLowerCase() === "k") {
      ev.preventDefault();
      switchView("chat");
      $("#searchInput")?.focus();
      return;
    }
    if (modalOpen) return;
    if (ev.key === "Escape" && !inField) {
      if (ev.shiftKey) {
        // alle Räume gelesen
        for (const r of state.rooms) {
          r.unread = 0;
          state.lastRead[r.id] = Date.now();
          try { localStorage.setItem(`relay.lastRead.${r.id}`, String(Date.now())); } catch {}
        }
        renderRoomList();
      } else if (state.currentRoomId) {
        const r = state.rooms.find((x) => x.id === state.currentRoomId);
        if (r) { delete r.unread; state.lastRead[r.id] = Date.now();
          try { localStorage.setItem(`relay.lastRead.${r.id}`, String(Date.now())); } catch {}
          api("POST", `/api/rooms/${r.id}/read`).catch(() => {}); // P1#10
        }
        renderRoomList();
      }
      return;
    }
    if (inField) {
      // "?" im Composer = normales Zeichen, NICHT shortcut (nur global ausserhalb Felder)
      return;
    }
    // g-Prefix Navigation (buzz/gmail-Stil)
    if (gPrefix) {
      gPrefix = false;
      const map = { c: "chat", a: "agents", p: "projects" };
      const v = map[ev.key.toLowerCase()];
      if (v) { ev.preventDefault(); switchView(v); return; }
    }
    if (ev.key === "g") { gPrefix = true; setTimeout(() => { gPrefix = false; }, 1200); return; }
    if (ev.key === "?") {
      ev.preventDefault();
      openModal("Tastenkürzel", [], () => {});
      // inject shortcut- list ins modal (openModal ohne Felder rendert nur form)
      const dlg = $("#modal");
      const list = document.createElement("div");
      list.className = "shortcut-list";
      list.innerHTML = SHORTCUTS.map((s) =>
        `<div class="shortcut-row"><span>${esc(s.label)}</span><kbd>${esc(s.keys)}</kbd></div>`).join("");
      dlg.querySelector("form").before(list);
      // submit mit leeren feldern: modal schließt via OK/Abbrechen/Escape; ok-callback nichts tun
      return;
    }
  });

  // quick-win 2: Chat-Suche mit live-Ergebnis-Popover
  function wireSearch() {
    const inp = $("#searchInput");
    if (!inp) return;   /* guard: Element landet erst mit dem HTML-Zug (R11-Koordination) */
    inp.addEventListener("input", async () => {
      const q = inp.value.trim();
      if (!q || !state.currentRoomId) return;
      try {
        const hits = await api("GET", `/api/rooms/${state.currentRoomId}/search?q=${encodeURIComponent(q)}`);
        openPopover(inp, `Suche "${q}" (${hits.length})`, hits.slice(0, 8).map((m) => ({
          label: `${m.senderName}: ${m.content.slice(0, 60)}`,
          run: () => {  // zum Treffer scrollen
            const el = document.querySelector(`[data-mid="${m.id}"]`);
            if (el) { el.scrollIntoView({ behavior: "smooth", block: "center" }); el.style.outline = "2px solid var(--accent-hover)"; setTimeout(() => el.style.outline = "", 1600); }
          },
        })));
      } catch { /* silent */ }
    });
    inp.addEventListener("keydown", (ev) => { if (ev.key === "Escape") inp.value = ""; });
  }
  wireSearch();

  // User-Dropdown unten (Identity-Card)
  (() => {
    const card = $("#identityCard"), menu = $("#userMenu");
    if (!card || !menu) return;
    const close = () => { menu.hidden = true; card.setAttribute("aria-expanded", "false"); };
    card.addEventListener("click", (e) => { e.stopPropagation(); const open = menu.hidden; if (open) { menu.hidden = false; card.setAttribute("aria-expanded", "true"); } else close(); });
    menu.querySelectorAll("[data-view]").forEach((b) => b.addEventListener("click", () => { close(); switchView(b.dataset.view); }));
    menu.querySelectorAll("[data-user-act]").forEach((b) => b.addEventListener("click", () => {
      close();
      const act = b.dataset.userAct;
      if (act === "newRoom") openNewRoom();
      else if (act === "newAgent") agentModal();
      else if (act === "notify") toggleNotify();
      else if (act) switchView(act);
    }));
    document.addEventListener("mousedown", (ev) => { if (!menu.hidden && !card.contains(ev.target) && !menu.contains(ev.target)) close(); });
    document.addEventListener("keydown", (ev) => { if (ev.key === "Escape") close(); });
  })();
  function openNewRoom() {
    openModal("Neuer Raum", [
      { label: "Raumname", name: "name", type: "text", required: true },
      { label: "Typ", name: "kind", type: "select", value: "chat", options: [{ value: "chat", label: "💬 Chat-Raum" },{ value: "project", label: "🗂 Projekt-Raum (Chat + Board)" }]},
      { label: "Agents im Raum (später auch per Drag&Drop)", name: "agentIds", type: "select", options: [] },
    ], async (form) => {
      const data = { name: form.name.value, kind: form.kind.value };
      data.agentIds = [...form.agentIds.selectedOptions].map((o) => o.value);
      const room = await api("POST", "/api/rooms", data);
      await loadRooms(); await loadAgents(); selectRoom(room.id); return true;
    });
    const sel = $("#modal [name=agentIds]"); sel.setAttribute("multiple","multiple"); sel.size = Math.min(6, Math.max(3, state.agents.length)); sel.innerHTML = state.agents.map((a) => `<option value="${a.id}">${esc(a.emoji)} ${esc(a.name)}</option>`).join("");
  }
  $("#newRoomBtn")?.addEventListener("click", () => openNewRoom());
  $("#newRoomBtnLegacy")?.addEventListener("click", () => {
    openModal("Neuer Raum", [
      { label: "Raumname", name: "name", type: "text", required: true },
      { label: "Typ", name: "kind", type: "select", value: "chat", options: [
        { value: "chat", label: "💬 Chat-Raum" },
        { value: "project", label: "🗂 Projekt-Raum (Chat + Board)" },
      ]},
      { label: "Agents im Raum (später auch per Drag&Drop)", name: "agentIds", type: "select", options: [] },
    ], async (form) => {
      const data = { name: form.name.value, kind: form.kind.value };
      data.agentIds = [...form.agentIds.selectedOptions].map((o) => o.value);
      const room = await api("POST", "/api/rooms", data);
      await loadRooms(); await loadAgents();
      selectRoom(room.id);
      return true;
    });
    const sel = $("#modal [name=agentIds]");
    sel.setAttribute("multiple", "multiple");
    sel.size = Math.min(6, Math.max(3, state.agents.length));
    sel.innerHTML = state.agents.map((a) =>
      `<option value="${a.id}">${esc(a.emoji)} ${esc(a.name)}</option>`).join("");
  });

  $("#newAgentBtn").addEventListener("click", () => agentModal());
  document.getElementById("newAgentQuick")?.addEventListener("click", () => agentModal());
  $("#newProjectBtn").addEventListener("click", () => newProjectModal());

  document.querySelectorAll(".nav-btn").forEach((b) =>
    b.addEventListener("click", () => switchView(b.dataset.view)));
  document.querySelectorAll(".room-tab").forEach((t) =>
    t.addEventListener("click", () => showTab(t.dataset.tab)));

  // ═════════════════════════════════════════════════════════════════
  // Boot
  // ═════════════════════════════════════════════════════════════════
  async function boot() {
    try {
      const h = await api("GET", "/api/health");
      state.adapters = h.adapters || ["echo"];
      $("#brandName").textContent = h.name || "Agent Relay";
    } catch { state.adapters = ["echo"]; }
    restoreLastRead();
    updateNotifyLabel();

    // P2#2: Pendings beim Boot laden (sonst ist das Banner nach Reload leer,
    // obwohl WS-Events während der Abwesenheit pendende Freigaben hinterließen)
    api("GET", "/api/approvals?status=pending").then((aps) => {
      pendingApprovals = aps || [];
      renderApprovalBanner();
    }).catch(() => {});
    await Promise.all([loadRooms(), loadAgents(), loadSkills(), loadRoutines()]);
    updateDashBadge();
    // P1#10 (authoritativ): server-Wahrheit NACH rooms-Load anwenden —
    // Reload-feste Unread-Badges (client-only Zähler starben beim Reload).
    await applyServerUnreads();
    if (state.rooms.length) selectRoom(state.rooms[0].id);
    connectWS();
  }
  async function applyServerUnreads() {
    try {
      const m = await api("GET", "/api/unread");
      for (const [rid, c] of Object.entries(m || {})) {
        if (c <= 0) continue;
        const r = state.rooms.find((x) => x.id === rid);
        if (!r) continue;
        // ehrliche Mischung: WS-geborene client-Zähler (frisch) schlagen
        // server nicht unter; Boot-Fälle (kein client-Zähler) = server
        r.unread = Math.max(r.unread || 0, c);
      }
      renderRoomList();
    } catch {}
  }
// ── P2#3: Skills-Panel (Agents-View-Sektion; paperclip company-skills) ──
  function renderSkillsPanel() {
    // Section unter den Agent-Karten (eigenes grid)
    let sec = $("#skillsPanel");
    if (!sec) {
      sec = document.createElement("div");
      sec.id = "skillsPanel";
      sec.className = "skills-panel";
      $("#agentCards").after(sec);
    }
    const skills = state.skills || [];
    if (!skills.length) { sec.innerHTML = "<h3><span class='sk-emoji'>🧩</span> Skills</h3><div class='hint' style='padding:6px 2px'>— noch keine Skills (wiederverwendbare Instruction-Bausteine) —</div>"; return; }
    sec.innerHTML = `<h3><span class='sk-emoji'>🧩</span> Skills <button id="newSkillBtn" class="btn primary" style="margin-left:auto">＋ Skill</button></h3>
      <div class="skills-grid">
        ${skills.map((sk) => `
          <div class="skill-card" data-skill="${esc(sk.id)}">
            <div class="row">
              <strong>${esc(sk.name)}</strong>
              <span class="badge-pill" style="color:var(--ink-3);background:var(--s3)">v${sk.Version}</span>
            </div>
            <div class="desc">${esc((sk.instructions || "").slice(0, 120))}${(sk.instructions || "").length > 120 ? "…" : ""}</div>
            <div class="sk-agents">${(sk.agentIds || []).length ? "gebunden: " + sk.agentIds.map((id) => esc((state.agents.find((a) => a.id === id) || {}).name || id)).join(", ") : "— keine Agents —"}</div>
            <div class="actions">
              <button class="btn" data-skedit="${esc(sk.id)}">Bearbeiten</button>
              <button class="btn" data-skassign="${esc(sk.id)}">Agenten binden</button>
              <button class="btn danger" data-skdel="${esc(sk.id)}">Löschen</button>
            </div>
          </div>`).join("")}
      </div>`;
    $("#newSkillBtn")?.addEventListener("click", () => skillModal());
    sec.querySelectorAll("[data-skedit]").forEach((b) => b.addEventListener("click", () => skillModal(b.dataset.skedit)));
    sec.querySelectorAll("[data-skassign]").forEach((b) => b.addEventListener("click", () => skillAssignModal(b.dataset.skassign)));
    sec.querySelectorAll("[data-skdel]").forEach((b) => b.addEventListener("click", async () => {
      if (!confirm("Skill löschen? (Versionen bleiben restorebar)")) return;
      await api("DELETE", `/api/skills/${b.dataset.skdel}`);
      await loadSkills();
    }));
  }

  function skillModal(id) {
    const sk = id ? state.skills.find((x) => x.id === id) : null;
    openModal(sk ? "Skill bearbeiten (neue Version)" : "Neuer Skill", [
      { label: "Name", name: "name", type: "text", value: sk?.name || "", required: true },
      { label: "Instructions", name: "instructions", type: "textarea", value: sk?.instructions || "", rows: 6 },
    ], async (form) => {
      const data = Object.fromEntries(new FormData(form).entries());
      if (sk) {
        await api("PATCH", `/api/skills/${sk.id}`, data); // neue Version
      } else {
        await api("POST", "/api/skills", data);
      }
      await loadSkills();
      return true;
    });
  }

  function skillAssignModal(skillId) {
    const sk = state.skills.find((x) => x.id === skillId);
    if (!sk) return;
    openModal(`Skill binden: ${sk.name}`, [
      { label: "Agents (Mehrfachauswahl, Ctrl/⌘-Klick)", name: "agentIds", type: "select",
        value: sk.agentIds || [], options: state.agents.map((a) => ({ value: a.id, label: `${a.emoji} ${a.name}` })) },
    ], async (form) => {
      const sel = form.querySelector('[name="agentIds"]');
      sel.setAttribute("multiple", "multiple");
      const ids = [...sel.selectedOptions].map((o) => o.value);
      await api("PUT", `/api/skills/${skillId}/agents`, { agentIds: ids });
      await loadSkills();
      return true;
    });
    // multiple nach render setzen (select-Render-Vertrag)
    const sel = $("#modal [name=agentIds]");
    if (sel) { sel.setAttribute("multiple", "multiple"); sel.size = Math.min(6, Math.max(3, state.agents.length));
      // vor-selektieren (bestehende Bindungen)
      (sk?.agentIds || []).forEach((id) => { const o = sel.querySelector(`option[value="${id}"]`); if (o) o.selected = true; });
    }
  }

  // ── P2#4: Routines-Panel (Settings-View; paperclip routines.ts) ──
  function renderRoutinesPanel() {
    let sec = $("#routinesPanel");
    if (!sec) {
      sec = document.createElement("div");
      sec.id = "routinesPanel";
      sec.className = "routines-panel";
      $("#settingsForm").after(sec);
    }
    const rs = state.routines || [];
    const roomOpt = (rid) => { const r = state.rooms.find((x) => x.id === rid); return r ? r.name : (rid || "—"); };
    const agentOpt = (aid) => { const a = state.agents.find((x) => x.id === aid); return a ? `${a.emoji} ${a.name}` : "alle Member"; };
    sec.innerHTML = `<h3><span class="sk-emoji">⏰</span> Routines <button id="newRoutineBtn" class="btn primary" style="margin-left:auto">＋ Routine</button></h3>
      ${rs.length ? `<div class="routines-grid">
        ${rs.map((t) => `
          <div class="routine-card" data-rt="${esc(t.id)}">
            <div class="row">
              <strong>${esc(t.name)}</strong>
              <code class="mono" style="font-size:11px;color:var(--ink-3)">${esc(t.cron)}</code>
              <span class="rt-toggle ${t.enabled ? "on" : ""}" data-rtoggle="${esc(t.id)}" title="an/aus">${t.enabled ? "an" : "aus"}</span>
            </div>
            <div class="rt-prompt">${esc(t.prompt.slice(0, 110))}${t.prompt.length > 110 ? "…" : ""}</div>
            <div class="rt-meta">Raum: ${esc(roomOpt(t.roomId))} · Agent: ${esc(agentOpt(t.agentId))} · nächster Lauf: ${esc((t.nextRunAt || "").slice(11, 16))}</div>
            <div class="actions">
              <button class="btn" data-rtedit="${esc(t.id)}">Bearbeiten</button>
              <button class="btn danger" data-rtdel="${esc(t.id)}">Löschen</button>
            </div>
          </div>`).join("")}
      </div>` : `<div class="hint" style="padding:4px 0">— keine Routines —</div>`}`;
    $("#newRoutineBtn")?.addEventListener("click", () => routineModal());
    sec.querySelectorAll("[data-rtoggle]").forEach((b) => b.addEventListener("click", async () => {
      const t = state.routines.find((x) => x.id === b.dataset.rtoggle);
      if (!t) return;
      await api("PATCH", `/api/routines/${t.id}`, { enabled: !t.enabled });
      await loadRoutines();
    }));
    sec.querySelectorAll("[data-rtedit]").forEach((b) => b.addEventListener("click", () => routineModal(b.dataset.rtedit)));
    sec.querySelectorAll("[data-rtdel]").forEach((b) => b.addEventListener("click", async () => {
      if (!confirm("Routine löschen?")) return;
      await api("DELETE", `/api/routines/${b.dataset.rtdel}`);
      await loadRoutines();
    }));
  }

  function routineModal(id) {
    const t = id ? state.routines.find((x) => x.id === id) : null;
    openModal(t ? "Routine bearbeiten" : "Neue Routine", [
      { label: "Name", name: "name", type: "text", value: t?.name || "", required: true },
      { label: "Cron (minute hour dom month dow; z.B. '30 9 * * 1-5')", name: "cron", type: "text", value: t?.cron || "0 9 * * *", required: true },
      { label: "Prompt (an den Agent)", name: "prompt", type: "textarea", value: t?.prompt || "", rows: 4 },
      { label: "Ziel-Raum", name: "roomId", type: "select", value: t?.roomId || "", options: state.rooms.map((r) => ({ value: r.id, label: r.name })) },
      { label: "Agent (leer = alle im Raum)", name: "agentId", type: "select", value: t?.agentId || "", options: [{ value: "", label: "— alle Member —" }, ...state.agents.map((a) => ({ value: a.id, label: `${a.emoji} ${a.name}` }))] },
    ], async (form) => {
      const sel = form.querySelector('[name="roomId"]');
      sel.removeAttribute("multiple");
      const data = Object.fromEntries(new FormData(form).entries());
      if (t) {
        await api("PATCH", `/api/routines/${t.id}`, data);
      } else {
        await api("POST", "/api/routines", data);
      }
      await loadRoutines();
      return true;
    });
  }
  // ── P2#9: Dashboard (paperclip status-cards-Vertrag) ──
  async function renderDashboard() {
    const d = await api("GET", "/api/dashboard").catch(() => null);
    const el = $("#dashGrid");
    if (!el) return;
    if (!d) { el.innerHTML = `<div class="empty-state"><div class="big">📊</div>Übersicht nicht ladbar.</div>`; return; }
    const needCards = [];
    // needs-you: approvals mit Buttons (identische Karte wie Banner — Wiederverwendung!)
    for (const ap of (d.pendingApprovals || [])) {
      const ag = state.agents.find((x) => x.id === ap.agentId);
      needCards.push(`
        <div class="dash-card need" data-apid="${esc(ap.id)}">
          <div class="dc-head"><span class="badge-pill ap-pill">● braucht dich</span><strong>${esc(ag ? ag.name : ap.agentId)}</strong></div>
          <div class="dc-body">${esc(ap.reason || "")}</div>
          <div class="ap-actions">
            <button class="btn primary" data-apact="granted" data-apid="${esc(ap.id)}">Genehmigen</button>
            <button class="btn danger" data-apact="denied" data-apid="${esc(ap.id)}">Ablehnen</button>
          </div>
        </div>`);
    }
    if ((d.unreadTotal || 0) > 0) {
      needCards.push(`<div class="dash-card need"><div class="dc-head"><span class="badge-pill ap-pill">● ${d.unreadTotal} ungelesen</span></div><div class="dc-body">Ungelesene Nachrichten über Räume verteilt — Sidebar-Dots sind die Liste.</div></div>`);
    }
    // what-is-happening: agents-Trio + projects-Rows + next routine
    const ag = d.agents || {};
    const projRows = (d.projects || []).map((p) => `
      <div class="dash-proj">
        <span class="p-dot" style="background:${esc(p.color)}"></span>
        <span class="dp-name">${esc(p.name)}</span>
        <span class="dp-nums mono">${p.todo}/${p.doing}/${p.done}</span>
        ${p.openTotal ? `<span class="dp-open">${p.openTotal} offen</span>` : `<span class="dp-ok">✅</span>`}
      </div>`).join("") || `<div class="hint">— keine Projekte —</div>`;
    const nextRt = d.nextRoutineAt ? new Date(d.nextRoutineAt).toLocaleString("de-DE", { hour: "2-digit", minute: "2-digit", day: "2-digit", month: "2-digit" }) : "—";
    el.innerHTML = `
      ${needCards.length ? `<div class="dash-need-grid">${needCards.join("")}</div>` : `<div class="dash-allclear">✅ Alles im Grün — nichts braucht dich.</div>`}
      <div class="dash-card"><div class="dc-head"><strong>Agents</strong><span class="mono">${ag.online ?? 0} online · ${ag.off ?? 0} aus</span></div></div>
      <div class="dash-card"><div class="dc-head"><strong>Projekte</strong></div><div class="dash-projs">${projRows}</div></div>
      <div class="dash-card"><div class="dc-head"><strong>Routines</strong><span class="mono">${d.routinesActive ?? 0} aktiv</span></div><div class="dc-body">Nächster Lauf: ${esc(nextRt)}</div></div>`;
    $("#dashStamp").textContent = "Stand " + new Date().toLocaleTimeString("de-DE", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
    // approval-Buttons im Dashboard = gleiche decision-Route (Wiederverwendung)
    el.querySelectorAll("[data-apact]").forEach((b) =>
      b.addEventListener("click", async () => {
        try {
          await api("POST", `/api/approvals/${b.dataset.apid}/${b.dataset.apact === "granted" ? "grant" : "deny"}`);
          renderDashboard();
        } catch (err) { toast(err.message); }
      }));
  }
  // ── P2#8: Rollen am Member-Chip (Klick-Cycle '' → admin → '') ──
  function memberRole(aid) { return state.roomRoles?.[aid] || ""; }


  // ── Demo-Feedback (GaRoN 14:07): Theme-Switch + Sidebar-Collapse ──
  function applyTheme() {
    const pref = (function () { try { return localStorage.getItem("relay.theme") || "dark"; } catch (e) { return "dark"; } })();
    document.documentElement.setAttribute("data-theme", pref);
    const btn = $("#themeToggle");
    if (btn) btn.textContent = pref === "light" ? "☀️" : "🌙";
  }
  function applySideCollapse() {
    const coll = (function () { try { return localStorage.getItem("relay.sideCollapsed") === "1"; } catch (e) { return false; } })();
    const app = document.querySelector("#app");
    if (app) app.classList.toggle("side-collapsed", coll);
    const btn = $("#sideCollapse");
    if (btn) { btn.textContent = coll ? "›" : "‹"; btn.title = coll ? "Seitenleiste auf" : "Seitenleiste zu"; }
  }
  $("#themeToggle")?.addEventListener("click", () => {
    const now = document.documentElement.getAttribute("data-theme");
    const next = now === "light" ? "dark" : "light";
    try { localStorage.setItem("relay.theme", next); } catch (e) {}
    applyTheme();
  });
  $("#sideCollapse")?.addEventListener("click", () => {
    const now = document.querySelector("#app").classList.contains("side-collapsed");
    try { localStorage.setItem("relay.sideCollapsed", now ? "0" : "1"); } catch (e) {}
    applySideCollapse();
  });
  applyTheme();
  applySideCollapse();

  // P2#7: Decision-Queue-Badge am Übersicht-Button (pending+unread live)
  async function updateDashBadge() {
    const d = await api("GET", "/api/dashboard").catch(() => null);
    const b = $("#dashBadge");
    if (!b) return;
    const n = ((d?.pendingApprovals || []).length) + (d?.unreadTotal || 0);
    b.hidden = n === 0;
    b.textContent = String(n);
  }
  

  boot();
})();
