/**
 * relay-dna — Overlay-Kern (relay-server injiziert dieses Script in den Stamp).
 *
 * DNA-Quellen (Feature-Matrix, Beweise in audits/raw/2026-10-24-*):
 *   buzz:       Räume-Liste + Presence/Typing-Verträge + Mention-Lane
 *   Hermes:     Fleet-Rail-Organisation (GW-Gruppen) + Projekte-Dimension
 *   Paperclip:  Board/Tasks + Approvals + Heartbeat/Routines-Anzeigen
 *
 * Verhalten: respektvoll zum SPA (kein Fork) — Panel links unten als
 * eigenes Dock; wenn das SPA-Shell-Selbst-Layout greift, bleibt es nicht
 * störend (Toggle-Button + localStorage-Position). Kein SPA-Code wird
 * verändert (nur eigene DOM-Insel via custom element).
 */
(function () {
  "use strict";
  if (window.__RELAY_DNA__) return;
  window.__RELAY_DNA__ = true;
  // Fehlerkanal SOFORT (vor jedem Import-Zug) — Diagnose-Beweis-Lane:
  window.addEventListener("error", function (ev) {
    (window.__relayErrors = window.__relayErrors || []).push(String(ev.message || ev).slice(0, 300));
  });
  try { window.addEventListener("unhandledrejection", function (ev) {
    (window.__relayErrors = window.__relayErrors || []).push("promise: " + String(ev.reason && (ev.reason.message || ev.reason)).slice(0, 300));
  }); } catch {}

  const API = "";
  const state = { rooms: [], tasks: [], agents: [], gateway: null, tab: "rooms" };

  function fetchJSON(url, opts) {
    return fetch(url, opts).then((r) => r.json());
  }
  async function sync() {
    try {
      const [h, rooms, tasks, reg] = await Promise.all([
        fetchJSON(API + "/api/health"),
        fetchJSON(API + "/api/rooms"),
        fetchJSON(API + "/api/tasks"),
        fetchJSON(API + "/api/registry"),
      ]);
      state.rooms = rooms.rooms || [];
      state.tasks = tasks.tasks || [];
      state.agents = reg.agents || [];
      state.gateway = reg.gateway || null;
      state.health = h;
      render();
    } catch (e) { /* stille Nacht */ }
  }

  const css = `
  .relay-dna-root{position:fixed;bottom:12px;left:12px;z-index:99998;font:12px/1.4 ui-sans-serif,system-ui;
    color:#bcbcc0;}
  .relay-dna-panel{width:340px;max-height:52vh;overflow:auto;background:#161920;border:1px solid #2a2d37;
    border-radius:10px;box-shadow:0 12px 32px #0007;backdrop-filter:blur(6px)}
  .relay-dna-head{display:flex;gap:6px;align-items:center;padding:8px 10px;border-bottom:1px solid #2a2d37;
    position:sticky;top:0;background:#161920}
  .relay-dna-head b{color:#f4f4f5;font-size:12px;margin-right:auto}
  .relay-dna-tab{cursor:pointer;padding:3px 9px;border-radius:8px;background:#1d2029;color:#8b8b94;border:0;font:inherit}
  .relay-dna-tab.on{background:#3a2a2a;color:#ff7070}
  .relay-dna-row{display:flex;gap:8px;align-items:baseline;padding:6px 10px;border-bottom:1px solid #22242e}
  .relay-dna-row:last-child{border-bottom:0}
  .relay-dna-name{color:#f4f4f5;font-weight:600}
  .relay-dna-sub{color:#8b8b94;font-size:11px}
  .relay-dna-badge{font-size:10px;padding:1px 7px;border-radius:6px;background:#22242e;color:#bcbcc0}
  .relay-dna-dot{width:7px;height:7px;border-radius:50%;background:#4ade80;display:inline-block;margin-right:5px}
  .relay-dna-toggle{cursor:pointer;background:#161920cc;color:#ff5c5c;border:1px solid #3a2a2a;border-radius:10px;
    padding:7px 11px;font:inherit;font-weight:600}
  .relay-dna-hidden{display:none}
  `;

  function item(r) {
    return '<div class="relay-dna-row"><span class="relay-dna-dot"></span><span class="relay-dna-name">'
      + escapeHTML(r.name) + '</span><span class="relay-dna-badge">' + escapeHTML(r.kind)
      + '</span>' + (r.topic ? '<span class="relay-dna-sub">' + escapeHTML(r.topic) + "</span>" : "") + "</div>";
  }
  function task(t) {
    return '<div class="relay-dna-row"><span class="relay-dna-badge">' + escapeHTML(t.status)
      + '</span><span class="relay-dna-name">' + escapeHTML(t.title) + "</span></div>";
  }
  function agent(a) {
    const m = a.model && (a.model.primary || a.model);
    return '<div class="relay-dna-row"><span class="relay-dna-name">' + (a.identity ? a.identity + " " : "")
      + escapeHTML(a.name) + '</span><span class="relay-dna-sub">' + escapeHTML(String(m || "")) + "</span></div>";
  }
  function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  }

  function render() {
    const root = document.getElementById("relay-dna-root") || mount();
    const panel = root.querySelector(".relay-dna-panel");
    const body = root.querySelector(".relay-dna-body");
    if (!root || !panel || !body) return;
    panel.classList.toggle("relay-dna-hidden", root.dataset.collapsed === "1");
    let html = "";
    if (state.tab === "rooms") html = state.rooms.map(item).join("") || '<div class="relay-dna-row relay-dna-sub">Noch keine Räume</div>';
    if (state.tab === "tasks") html = state.tasks.map(task).join("") || '<div class="relay-dna-row relay-dna-sub">Noch keine Tasks</div>';
    if (state.tab === "agents") html = (state.agents || []).map(agent).join("") || '<div class="relay-dna-row relay-dna-sub">Roster wartet…</div>';
    body.innerHTML = html;
    const g = state.gateway || {};
    root.querySelector(".relay-dna-state").textContent =
      (g.connected ? "GW " + (g.serverVersion || "") + " · " + (state.agents.length) + " Agents" : "GW verbindet…");
  }

  function mount() {
    const style = document.createElement("style");
    style.textContent = css;
    document.head.appendChild(style);
    const root = document.createElement("div");
    root.id = "relay-dna-root";
    root.className = "relay-dna-root";
    root.dataset.collapsed = localStorage.getItem("relay-dna.collapsed") || "0";
    root.innerHTML =
      '<button class="relay-dna-toggle" id="relay-dna-toggle">🧩 Relay</button>' +
      '<div class="relay-dna-panel' + (root.dataset.collapsed === "1" ? " relay-dna-hidden" : "") + '">' +
      '<div class="relay-dna-head"><b>🧩 Agent Relay</b>' +
      '<span class="relay-dna-state relay-dna-sub">…</span>' +
      '<button class="relay-dna-tab" data-tab="rooms">Räume</button>' +
      '<button class="relay-dna-tab" data-tab="tasks">Tasks</button>' +
      '<button class="relay-dna-tab" data-tab="agents">Agents</button>' +
      "</div>" +
      '<div class="relay-dna-body"></div>' +
      "</div>";
    (document.body || document.documentElement).appendChild(root);
    // Fehlerkanal für CDP-Diagnose (Beweis-Lane):
    window.addEventListener("error", function (ev) {
      (window.__relayErrors = window.__relayErrors || []).push(String(ev.message || ev).slice(0, 300));
    });
    root.querySelector("#relay-dna-toggle").addEventListener("click", () => {
      root.dataset.collapsed = root.dataset.collapsed === "1" ? "0" : "1";
      localStorage.setItem("relay-dna.collapsed", root.dataset.collapsed);
      render();
    });
    root.querySelectorAll(".relay-dna-tab").forEach((b) => {
      b.addEventListener("click", () => {
        state.tab = b.dataset.tab;
        root.querySelectorAll(".relay-dna-tab").forEach((x) => x.classList.toggle("on", x === b));
        render();
      });
    });
    return root;
  }

  function boot() {
    mount();
    sync();
    setInterval(sync, 10000);
  }

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", boot);
  else boot();
})();