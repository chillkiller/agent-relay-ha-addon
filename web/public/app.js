// Agent Relay SPA — no build step, plain ES module in classic script form.
(() => {
  "use strict";

  // ── state ──────────────────────────────────────────────────────────
  const HUMAN = "human"; // pseudo-agent id for the drag chip "du"
  const state = {
    view: "chat",
    rooms: [],
    agents: [],
    projects: [],
    tasks: [],
    currentRoomId: null,
    messages: {},
    settings: {},
    ws: null,
  };

  const $ = (sel) => document.querySelector(sel);
  const esc = (s) =>
    String(s ?? "").replace(/[&<>"']/g, (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

  // Base prefix for HA Ingress: the app is served under e.g.
  // /api/hassio_ingress/<token>/ — API and WS paths must be relative to it.
  const BASE = location.pathname.endsWith("/")
    ? location.pathname.slice(0, -1)
    : location.pathname.replace(/\/[^/]*$/, "");
  const url = (p) => `${BASE}${p}`;

  // optional API bearer (LAN gate): asked once on 401, kept in sessionStorage
  function bearer() { return sessionStorage.getItem("relayBearer") || ""; }

  async function api(method, path, body) {
    const headers = body ? { "Content-Type": "application/json" } : {};
    if (bearer()) headers["Authorization"] = `Bearer ${bearer()}`;
    const res = await fetch(url(path), {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });
    if (res.status === 401) {
      const tok = prompt("API-Token erforderlich (api_bearer_token):");
      if (tok) { sessionStorage.setItem("relayBearer", tok); return api(method, path, body); }
    }
    if (res.status === 204) return null;
    const json = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(json.error || res.statusText);
    return json;
  }

  // streamed token buffers per message id (survive re-renders)
  const streamBuf = {};

  // ── mobile drawer ─────────────────────────────────────────────────
  const drawer = () => document.body.classList.contains("sidebar-open");
  $("#burger").addEventListener("click", () =>
    document.body.classList.toggle("sidebar-open"));

  // ── websocket ──────────────────────────────────────────────────────
  function connectWS() {
    const proto = location.protocol === "https:" ? "wss" : "ws";
        const tokQ = bearer() ? `?token=${encodeURIComponent(bearer())}` : "";
    const ws = new WebSocket(`${proto}://${location.host}${url("/ws")}${tokQ}`);
    state.ws = ws;
    ws.onopen = () => { attempts = 0; setConn(true); };
    let attempts = 0;
    ws.onclose = () => {
      setConn(false);
      const delay = Math.min(15000, 500 * Math.pow(2, attempts++)); // 0.5,1,2,4…15s (review-2)
      setTimeout(connectWS, delay);
    };
    ws.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch { return; }
      handleEvent(msg);
    };
  }

  function setConn(on) {
    $("#connDot").classList.toggle("on", on);
    $("#connText").textContent = on ? "verbunden" : "nicht verbunden";
  }

  const typingTimers = {};
  function handleEvent(e) {
    switch (e.type) {
      case "message.created": {
        if (!state.currentRoomId || e.message.roomId !== state.currentRoomId) break;
        state.messages[e.message.id] = e.message;
        renderMessages();
        break;
      }
      case "agent.token": {
        streamBuf[e.messageId] = (streamBuf[e.messageId] ?? "") + e.delta;
        const msgEl = document.querySelector(`[data-mid="${e.messageId}"] .content`);
        if (msgEl) msgEl.textContent = streamBuf[e.messageId];
        break;
      }
      case "agent.started": {
        if (!e.message) break;
        if (state.currentRoomId === e.message.roomId) {
          state.messages[e.message.id] = e.message;
          streamBuf[e.message.id] = "";
          renderMessages();
        }
        break;
      }
      case "agent.finished": {
        if (!e.message) break;
        delete streamBuf[e.message.id];
        if (state.messages[e.message.id]) {
          state.messages[e.message.id] = e.message;
          renderMessages();
        }
        const t = typingTimers[e.message.agentId];
        if (t) { clearTimeout(t); delete typingTimers[e.message.agentId]; renderTyping(); }
        break;
      }
      case "typing": {
        const key = e.agentId;
        renderTyping(e.agentName);
        clearTimeout(typingTimers[key]);
        typingTimers[key] = setTimeout(() => { delete typingTimers[key]; renderTyping(); }, 12000);
        break;
      }
      case "rooms.changed": loadRooms(); break;
      case "system": {
        if (e.roomId && e.roomId === state.currentRoomId) {
          pushLocalSystem(e.text);
        }
        break;
      }
    }
  }

  function pushLocalSystem(text) {
    const wrap = document.createElement("div");
    wrap.className = "empty-state";
    wrap.style.color = "var(--err)";
    wrap.textContent = text;
    $("#messages").appendChild(wrap);
  }

  function renderTyping(name) {
    $("#typingBar").textContent = name ? `⌨️ ${name} schreibt…` : "";
  }

  // ── views ──────────────────────────────────────────────────────────
  function switchView(view) {
    state.view = view;
    for (const v of ["chat", "agents", "projects", "settings"]) {
      $(`#view-${v}`).hidden = v !== view;
    }
    document.querySelectorAll(".nav-btn").forEach((b) =>
      b.classList.toggle("active", b.dataset.view === view));
    if (view === "projects") loadBoard();
    if (view === "settings") renderSettings();
  }

  // ── rooms ──────────────────────────────────────────────────────────
  async function loadRooms() {
    state.rooms = await api("GET", "/api/rooms");
    renderRoomList();
  }

  function renderRoomList() {
    const el = $("#roomList");
    const chats = state.rooms.filter((r) => (r.kind || "chat") === "chat");
    const projRooms = state.rooms.filter((r) => r.kind === "project");
    const item = (r) => `
      <button class="room-item ${r.id === state.currentRoomId ? "active" : ""}" data-room="${r.id}" data-kind="${esc(r.kind || "chat")}">
        <span>${r.kind === "project" ? "🗂" : "#"}</span><span style="flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(r.name)}</span>
        <span class="badge">${r.messageCount || ""}</span>
      </button>`;
    const group = (label, rooms) => rooms.length ? `
      <div class="room-group">${label} <span class="badge">${rooms.length}</span></div>
      ${rooms.map(item).join("")}` : "";
    el.innerHTML = group("RÄUME", chats) + group("PROJEKTE", projRooms);
    el.querySelectorAll("[data-room]").forEach((b) => {
      b.addEventListener("click", () => selectRoom(b.dataset.room));
      // drop target: agent-chip here = join room
      b.addEventListener("dragover", (ev) => { ev.preventDefault(); b.classList.add("dragover"); });
      b.addEventListener("dragleave", () => b.classList.remove("dragover"));
      b.addEventListener("drop", async (ev) => {
        ev.preventDefault(); b.classList.remove("dragover");
        const agentId = ev.dataTransfer.getData("text/agent-id");
        if (!agentId || agentId === HUMAN) return;
        await api("POST", `/api/rooms/${b.dataset.room}/members`, { agentId });
        await loadRooms();
        if (b.dataset.room === state.currentRoomId) await selectRoom(state.currentRoomId);
      });
    });
  }

  let activeTab = "chat"; // per room: "chat" | "board" (project rooms)

  async function selectRoom(id) {
    document.body.classList.remove("sidebar-open");
    state.currentRoomId = id;
    activeTab = "chat";
    renderRoomList();
    switchView("chat");
    const room = state.rooms.find((r) => r.id === id);
    $("#activeRoomName").textContent = room ? `${room.kind === "project" ? "🗂" : "#"} ${room.name}` : "";
    const members = (room?.memberAgentIds || [])
      .map((aid) => state.agents.find((a) => a.id === aid))
      .filter(Boolean);
    $("#roomMembers").innerHTML = members.map((m) =>
      `<span class="chip"><span class="emoji">${esc(m.emoji)}</span>${esc(m.name)}</span>`).join("");
    // tabs only for project rooms
    const isProject = room?.kind === "project";
    $("#roomTabs").style.display = isProject ? "inline-flex" : "none";
    document.querySelector("#view-chat .messages").style.display = "flex";
    $("#composer").style.display = "flex";
    document.querySelectorAll(".room-tab").forEach((t) =>
      t.classList.toggle("active", t.dataset.tab === "chat"));
    await loadMessages();
  }

  document.querySelectorAll(".room-tab").forEach((t) =>
    t.addEventListener("click", async () => {
      activeTab = t.dataset.tab;
      document.querySelectorAll(".room-tab").forEach((x) =>
        x.classList.toggle("active", x === t));
      showTab();
    }));

  function showTab() {
    const msgs = document.querySelector("#view-chat .messages");
    const composer = $("#composer");
    const board = $("#inlineBoard");
    if (activeTab === "board") {
      msgs.style.display = "none";
      composer.style.display = "none";
      if (!board) {
        const el = document.createElement("div");
        el.id = "inlineBoard";
        document.querySelector("#view-chat").insertBefore(el, msgs);
      }
      loadInlineBoard();
    } else {
      msgs.style.display = "flex";
      composer.style.display = "flex";
      if (board) board.remove();
    }
  }

  let inlineBoardRoomId = null;
  state.currentInlinePid = null;
  async function loadInlineBoard(forceRefresh) {
    const room = state.rooms.find((r) => r.id === state.currentRoomId);
    if (room?.projectId) state.currentInlinePid = room.projectId;
    if (!forceRefresh && inlineBoardRoomId === state.currentRoomId) return;
    inlineBoardRoomId = state.currentRoomId;
    const b = $("#inlineBoard");
    if (b && window.__renderBoardInto) await window.__renderBoardInto(b, state.currentInlinePid);
  }

  async function loadMessages() {
    if (!state.currentRoomId) return;
    state.messages = {};
    Object.keys(streamBuf).forEach((k) => delete streamBuf[k]); // review M-3
    const list = await api("GET", `/api/rooms/${state.currentRoomId}/messages`);
    for (const m of list) state.messages[m.id] = m;
    renderMessages(true);
  }

  function renderMessages(force = false) {
    const wrap = $("#messages");
    const msgs = Object.values(state.messages)
      .filter((m) => m.roomId === state.currentRoomId)
      .sort((a, b) => a.createdAt.localeCompare(b.createdAt));
    stickBottom(wrap, force, () => {
      wrap.innerHTML = msgs.length ? msgs.map(msgHtml).join("") :
        `<div class="empty-state"><p>Noch keine Nachrichten in diesem Raum.</p></div>`;
      wrap.querySelectorAll("[data-mid]").forEach(annotateMsg);
    });
  }

  function stickBottom(wrap, force, fill) {
    const near = force || (wrap.scrollHeight - wrap.scrollTop - wrap.clientHeight) < 120;
    fill();
    if (near) wrap.scrollTop = wrap.scrollHeight;
  }

  function agentOf(m) {
    return state.agents.find((a) => a.id === m.agentId);
  }

  function msgHtml(m) {
    const who = m.senderKind === "human" ? "du" : m.senderName;
    const dot = m.agentId && agentOf(m)
      ? `<span class="dot" style="background:${agentOf(m).color}"></span>` : "";
    return `
      <div class="msg ${m.senderKind} ${m.status} ${m.status === "error" ? "error" : ""}" data-mid="${m.id}">
        <div class="meta">${dot}<strong>${esc(who)}</strong><span>${esc(fmtTime(m.createdAt))}</span></div>
        <div class="content">${esc(m.status === "streaming" && streamBuf[m.id] !== undefined ? streamBuf[m.id] : m.content)}</div>
      </div>`;
  }

  function annotateMsg(el) {
    const m = state.messages[el.dataset.mid];
    if (m) el.classList.toggle("streaming", m.status === "streaming");
  }

  function fmtTime(iso) {
    const d = new Date(iso);
    return d.toLocaleTimeString("de-DE", { hour: "2-digit", minute: "2-digit" });
  }

  // ── agents ─────────────────────────────────────────────────────────
  async function loadAgents() {
    state.agents = await api("GET", "/api/agents");
    if (state.view === "agents") renderAgents();
    renderRoster();
  }

  function renderAgents() {
    $("#agentCards").innerHTML = state.agents.map((a) => `
      <div class="card">
        <div class="row">
          <span class="emoji">${esc(a.emoji)}</span>
          <div><div class="name">${esc(a.name)}</div>
          <div class="sub">${esc(a.adapter)}${a.model ? " · " + esc(a.model) : ""}</div></div>
          <span class="chip" style="margin-left:auto;border-color:${esc(a.color)}">${a.enabled ? "aktiv" : "aus"}</span>
        </div>
        <div class="sub">${esc((a.systemPrompt || "").slice(0, 110))}${(a.systemPrompt || "").length > 110 ? "…" : ""}</div>
        <div class="actions">
          <button class="btn" data-edit="${a.id}">Bearbeiten</button>
          <button class="btn danger" data-del="${a.id}">Löschen</button>
        </div>
      </div>`).join("");
    $("#agentCards").querySelectorAll("[data-edit]").forEach((b) =>
      b.addEventListener("click", () => agentModal(b.dataset.edit)));
    $("#agentCards").querySelectorAll("[data-del]").forEach((b) =>
      b.addEventListener("click", async () => {
        if (!confirm("Agent wirklich löschen?")) return;
        await api("DELETE", `/api/agents/${b.dataset.del}`);
        await loadAgents(); await loadRooms();
      }));
  }

  function agentModal(id) {
    const a = id ? state.agents.find((x) => x.id === id) : null;
    openModal(a ? "Agent bearbeiten" : "Neuer Agent", [
      { label: "Name", name: "name", type: "text", value: a?.name, required: true },
      { label: "Emoji", name: "emoji", type: "text", value: a?.emoji ?? "🤖" },
      { label: "Farbe", name: "color", type: "color", value: a?.color ?? "#6ea8fe" },
      { label: "Adapter", name: "adapter", type: "select", value: a?.adapter ?? "echo",
        options: state.adapters.map((x) => ({ value: x, label: x })) },
      { label: "Modell (z. B. openclaw/coordinator)", name: "model", type: "text", value: a?.model ?? "" },
      { label: "Basis-URL (leer = globale Einstellung)", name: "baseUrl", type: "text", value: a?.baseUrl ?? "" },
      { label: "API-Key (leer = globales Token)", name: "apiKey", type: "password", value: a?.apiKey ?? "" },
      { label: "System-Prompt", name: "systemPrompt", type: "textarea", value: a?.systemPrompt ?? "" },
      { label: "Aktiv", name: "enabled", type: "checkbox", checked: a ? !!a.enabled : true },
    ], async (form) => {
      const data = Object.fromEntries(new FormData(form).entries());
      data.enabled = form.enabled.checked;
      // write-only apiKey: only send when the user typed something — an empty
      // field must NOT clear the stored key (key never renders back).
      if (!data.apiKey) {
        const { apiKey, ...rest } = data;
        data = rest;
      }
      if (id) await api("PATCH", `/api/agents/${id}`, data);
      else await api("POST", "/api/agents", data);
      await loadAgents(); await loadRooms();
      return true;
    });
  }

  // ── projects + tasks ───────────────────────────────────────────────
  async function loadBoard() {
    state.projects = await api("GET", "/api/projects");
    state.tasks = await api("GET", "/api/tasks");
    renderBoard();
  }

  const COLUMNS = [["todo", "Zu tun"], ["doing", "In Arbeit"], ["done", "Fertig"]];

  // Ein Renderer für beide Flächen: Projekte-View (#projectBoard) und Board-Tab im Raum (#inlineBoard)
  function boardHtml(forRoom, pid) {
    const projects = pid ? state.projects.filter((p) => p.id === pid) : state.projects;
    if (!projects.length) return `<div class="empty-state"><p>Noch keine Projekte.</p></div>`;
    return projects.map((p) => {
      const tasks = state.tasks.filter((t) => t.projectId === p.id);
      return `
      <div class="project">
        <div class="p-head">
          <span class="p-dot" style="background:${esc(p.color)}"></span>
          <h3>${esc(p.name)}</h3>
          ${forRoom ? "" : `<button class="btn danger" data-pdel="${p.id}" style="margin-left:auto">Löschen</button>`}
        </div>
        <div class="p-desc">${esc(p.description)}</div>
        <div class="columns">
          ${COLUMNS.map(([key, label]) => `
            <div class="column">
              <h4>${label}</h4>
              ${tasks.filter((t) => t.status === key).map((t) => `
                <div class="task ${t.status === "done" ? "done" : ""}">
                  <span>${esc(t.title)}</span>
                  <span class="who">${t.assignee ? `@${esc(assigneeName(t.assignee))}` : ""}</span>
                  <span class="t-actions">
                    ${t.status !== "todo" ? `<button data-tmove="${t.id}:${prevStatus(t.status)}" title="zurück">←</button>` : ""}
                    ${t.status !== "done" ? `<button data-tmove="${t.id}:${nextStatus(t.status)}" title="weiter">→</button>` : ""}
                    <button data-tdel="${t.id}">✕</button>
                  </span>
                </div>`).join("")}
            </div>`).join("")}
        </div>
        <button class="add-task" data-taskadd="${p.id}">＋ Aufgabe</button>
      </div>`;
    }).join("");
  }

  function bindBoardHandlers(el) {
    el.querySelectorAll("[data-pdel]").forEach((b) => b.addEventListener("click", async () => {
      if (!confirm("Projekt löschen (zugeordnete Aufgaben auch, Räume bleiben)")) return;
      await api("DELETE", `/api/projects/${b.dataset.pdel}`);
      loadBoard();
    }));
    el.querySelectorAll("[data-taskadd]").forEach((b) => b.addEventListener("click", () => taskModal(b.dataset.taskadd)));
    el.querySelectorAll("[data-tmove]").forEach((b) => b.addEventListener("click", async () => {
      const [id, st] = b.dataset.tmove.split(":");
      await api("PATCH", `/api/tasks/${id}`, { status: st });
      loadBoard(); loadInlineBoard(true);
    }));
    el.querySelectorAll("[data-tdel]").forEach((b) => b.addEventListener("click", async () => {
      await api("DELETE", `/api/tasks/${b.dataset.tdel}`);
      loadBoard(); loadInlineBoard(true);
    }));
  }

  window.__renderBoardInto = async function (el, pid) {
    state.projects = await api("GET", "/api/projects");
    state.tasks = await api("GET", "/api/tasks", null).catch(() => api("GET", "/api/tasks"));
    el.innerHTML = boardHtml(true, pid);
    bindBoardHandlers(el);
  };

  async function loadInlineBoardRefresh() {
    return loadInlineBoard(true);
  }
  function renderBoard() {
    const el = $("#projectBoard");
    if (!state.projects.length) {
      el.innerHTML = `<div class="empty-state"><p>Noch keine Projekte.</p></div>`;
      return;
    }
    el.innerHTML = state.projects.map((p) => {
      const tasks = state.tasks.filter((t) => t.projectId === p.id);
      return `
      <div class="project">
        <div class="p-head">
          <span class="p-dot" style="background:${esc(p.color)}"></span>
          <h3>${esc(p.name)}</h3>
          <button class="btn danger" data-pdel="${p.id}" style="margin-left:auto">Löschen</button>
        </div>
        <div class="p-desc">${esc(p.description)}</div>
        <div class="columns">
          ${COLUMNS.map(([key, label]) => `
            <div class="column">
              <h4>${label}</h4>
              ${tasks.filter((t) => t.status === key).map((t) => `
                <div class="task ${t.status === "done" ? "done" : ""}">
                  <span>${esc(t.title)}</span>
                  <span class="who">${t.assignee ? `@${esc(assigneeName(t.assignee))}` : ""}</span>
                  <span class="t-actions">
                    ${t.status !== "todo" ? `<button data-tmove="${t.id}:${prevStatus(t.status)}" title="zurück">←</button>` : ""}
                    ${t.status !== "done" ? `<button data-tmove="${t.id}:${nextStatus(t.status)}" title="weiter">→</button>` : ""}
                    <button data-tdel="${t.id}">✕</button>
                  </span>
                </div>`).join("")}
            </div>`).join("")}
        </div>
        <button class="add-task" data-taskadd="${p.id}">＋ Aufgabe</button>
      </div>`;
    }).join("");

    el.querySelectorAll("[data-pdel]").forEach((b) => b.addEventListener("click", async () => {
      if (!confirm("Projekt löschen (zugeordnete Aufgaben auch, Räume bleiben)")) return;
      await api("DELETE", `/api/projects/${b.dataset.pdel}`);
      loadBoard();
    }));
    el.querySelectorAll("[data-taskadd]").forEach((b) => b.addEventListener("click", () => taskModal(b.dataset.taskadd)));
    el.querySelectorAll("[data-tmove]").forEach((b) => b.addEventListener("click", async () => {
      const [id, st] = b.dataset.tmove.split(":");
      await api("PATCH", `/api/tasks/${id}`, { status: st });
      loadBoard();
    }));
    el.querySelectorAll("[data-tdel]").forEach((b) => b.addEventListener("click", async () => {
      await api("DELETE", `/api/tasks/${b.dataset.tdel}`);
      loadBoard();
    }));
  }

  function assigneeName(assignee) {
    const a = typeof assignee === "number"
      ? state.agents[assignee - 1]
      : state.agents.find((x) => x.id === assignee);
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
      { label: "Agent (Nr. oder leer)", name: "assignee", type: "text" },
    ], async (form) => {
      const data = Object.fromEntries(new FormData(form).entries());
      data.projectId = projectId;
      await api("POST", "/api/tasks", data);
      loadBoard();
      return true;
    });
  }

  function projectModal(id) {
    const p = id ? state.projects.find((x) => x.id === id) : null;
    openModal(p ? "Projekt bearbeiten" : "Neues Projekt", [
      { label: "Name", name: "name", type: "text", value: p?.name, required: true },
      { label: "Beschreibung", name: "description", type: "textarea", value: p?.description ?? "" },
      { label: "Farbe", name: "color", type: "color", value: p?.color ?? "#8a7ff0" },
    ], async (form) => {
      const data = Object.fromEntries(new FormData(form).entries());
      if (id) await api("PATCH", `/api/projects/${id}`, data);
      else await api("POST", "/api/projects", data);
      loadBoard();
      return true;
    });
  }

  // ── settings ───────────────────────────────────────────────────────
  async function renderSettings() {
    state.settings = await api("GET", "/api/settings");
    const f = $("#settingsForm");
    f.relayName.value = state.settings.relayName ?? "";
    f.openclawBaseUrl.value = state.settings.openclawBaseUrl ?? "";
    f.openclawToken.value = state.settings.openclawToken ?? "";
    $("#brandName").textContent = state.settings.relayName || "Agent Relay";
  }

  $("#settingsForm").addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const f = ev.target;
    const saved = await api("PATCH", "/api/settings", {
      relayName: f.relayName.value,
      openclawBaseUrl: f.openclawBaseUrl.value,
      openclawToken: f.openclawToken.value,
    });
    state.settings = saved;
    $("#brandName").textContent = saved.relayName || "Agent Relay";
    $("#settingsSaved").textContent = "✓ gespeichert";
    setTimeout(() => { $("#settingsSaved").textContent = ""; }, 2500);
  });

  // ── modal helper ───────────────────────────────────────────────────
  function openModal(title, fields, onSubmit) {
    const dlg = $("#modal");
    dlg.innerHTML = `
      <h3>${esc(title)}</h3>
      <form method="dialog" novalidate>
        ${fields.map((f) => {
          if (f.type === "select") {
            return `<label>${esc(f.label)}<select name="${f.name}">
              ${f.options.map((o) => `<option value="${esc(o.value)}" ${o.value === f.value ? "selected" : ""}>${esc(o.label)}</option>`).join("")}
            </select></label>`;
          }
          if (f.type === "checkbox") {
            return `<label class="checkrow"><input type="checkbox" name="${f.name}" ${f.checked ? "checked" : ""}/> ${esc(f.label)}</label>`;
          }
          if (f.type === "textarea") {
            return `<label>${esc(f.label)}<textarea name="${f.name}">${esc(f.value ?? "")}</textarea></label>`;
          }
          return `<label>${esc(f.label)}<input type="${f.type}" name="${f.name}" value="${esc(f.value ?? "")}" /></label>`;
        }).join("")}
        <div class="modal-actions">
          <button type="button" class="btn" value="cancel">Abbrechen</button>
          <button type="submit" class="btn accent" value="ok">OK</button>
        </div>
      </form>`;
    const form = dlg.querySelector("form");
    const cancel = form.querySelector("[value=cancel]");
    cancel.addEventListener("click", () => dlg.close());
    form.addEventListener("submit", (ev) => {
      ev.preventDefault();
      Promise.resolve(onSubmit(form))
        .then((ok) => { if (ok !== false) dlg.close(); })
        .catch((err) => alert(err.message));
    });
    dlg.showModal();
  }

  // ── modals wiring ──────────────────────────────────────────────────
  $("#newRoomBtn").addEventListener("click", () => {
    openModal("Neuer Raum", [
      { label: "Raumname", name: "name", type: "text", required: true },
      { label: "Typ", name: "kind", type: "select", value: "chat", options: [
        { value: "chat", label: "💬 Chat-Raum" },
        { value: "project", label: "🗂 Projekt-Raum (mit Task-Board)" },
      ]},
      { label: "Agents im Raum (Mehrfachauswahl, oder später reinziehen)", name: "agentIds", type: "select", options: [] },
      { label: "Projekt (nur für chat-Kind, optional)", name: "projectId", type: "select", options: [] },
    ], async (form) => {
      const data = { name: form.name.value, kind: form.kind.value };
      data.agentIds = [...form.agentIds.selectedOptions].map((o) => o.value);
      data.projectId = form.projectId.value || null;
      const room = await api("POST", "/api/rooms", data);
      await loadRooms();
      await loadBoard().catch(() => {});
      selectRoom(room.id);
      return true;
    });
    const agentSel = $("#modal [name=agentIds]");
    agentSel.setAttribute("multiple", "multiple");
    agentSel.size = Math.max(3, state.agents.length);
    agentSel.innerHTML = state.agents.map((a) =>
      `<option value="${a.id}">${esc(a.emoji)} ${esc(a.name)} (${esc(a.adapter)})</option>`).join("");
    const projSel = $("#modal [name=projectId]");
    projSel.innerHTML = `<option value="">— kein Projekt —</option>` +
      state.projects.map((p) => `<option value="${p.id}">${esc(p.name)}</option>`).join("");
  });

  $("#newAgentBtn").addEventListener("click", () => agentModal());
  $("#newProjectBtn").addEventListener("click", () => projectModal());

  document.querySelectorAll(".nav-btn").forEach((b) =>
    b.addEventListener("click", () => switchView(b.dataset.view)));

  // ── composer ───────────────────────────────────────────────────────
  $("#composer").addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const input = $("#composerInput");
    const content = input.value.trim();
    document.body.classList.remove("sidebar-open"); // hide keyboard-friendly drawer
    if (!content || !state.currentRoomId) return;
    input.value = "";
    try {
      await api("POST", `/api/rooms/${state.currentRoomId}/messages`, { content });
    } catch (err) {
      pushLocalSystem(`${err.message}`);
      input.value = content;
    }
  });

  $("#composerInput").addEventListener("keydown", (ev) => {
    if (ev.key === "Enter" && !ev.shiftKey) {
      ev.preventDefault();
      $("#composer").requestSubmit();
    }
  });

  // ── roster (drag&drop agents in/out of rooms) ──────────────────────
  function renderRoster() {
    const list = $("#rosterList");
    if (!list) return;
    const human = `<span class="roster-chip" data-agent="${HUMAN}"><span class="swatch" style="background:#27a644"></span>du</span>`;
    list.innerHTML = human + state.agents.map((a) => `
      <span class="roster-chip" draggable="true" data-agent="${a.id}">
        <span class="swatch" style="background:${esc(a.color)}"></span>${esc(a.emoji)} ${esc(a.name)}
      </span>`).join("");
    list.querySelectorAll('.roster-chip[draggable="true"]').forEach((chip) => {
      chip.addEventListener("dragstart", (ev) => {
        ev.dataTransfer.setData("text/agent-id", chip.dataset.agent);
        ev.dataTransfer.effectAllowed = "copyMove";
      });
      // drop chip back onto roster = remove from current room
      chip.addEventListener("dragover", (ev) => { ev.preventDefault(); chip.classList.add("dragover"); });
      chip.addEventListener("dragleave", () => chip.classList.remove("dragover"));
      chip.addEventListener("drop", async (ev) => {
        ev.preventDefault(); chip.classList.remove("dragover");
        const agentId = ev.dataTransfer.getData("text/agent-id");
        if (!agentId || agentId === HUMAN || !state.currentRoomId) return;
        await api("DELETE", `/api/rooms/${state.currentRoomId}/members/${agentId}`);
        await loadRooms();
        await selectRoom(state.currentRoomId);
      });
    });
  }
  // ── boot ───────────────────────────────────────────────────────────
  async function boot() {
    try {
      const health = await api("GET", "/api/health");
      state.adapters = health.adapters || ["echo"];
      $("#brandName").textContent = health.name || "Agent Relay";
    } catch { state.adapters = ["echo"]; }
    await loadRooms();
    await loadAgents();
    state.projects = await api("GET", "/api/projects").catch(() => []);
    connectWS();
    if (state.rooms.length) selectRoom(state.rooms[0].id);
  }

  boot();
})();