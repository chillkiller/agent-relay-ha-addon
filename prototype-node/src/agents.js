// Agent runtime: registry + multi-agent turn scheduling for rooms.

const store = require("./store");
const echoAdapter = require("./adapters/echo");
const openaiAdapter = require("./adapters/openai");

const ADAPTERS = {
  [echoAdapter.id]: echoAdapter,
  [openaiAdapter.id]: openaiAdapter,
};

function listAdapters() {
  return Object.keys(ADAPTERS);
}

function getAdapter(id) {
  return ADAPTERS[id] || null;
}

function uid(prefix) {
  return `${prefix}_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`;
}

// ── mention parsing ─────────────────────────────────────────────────
function parseMentions(text, agents) {
  const t = String(text).toLowerCase();
  const found = [];
  for (const a of agents) {
    const names = [a.name, a.id].map((n) => String(n).toLowerCase()).filter(Boolean);
    for (const n of names) {
      const re = new RegExp(`@${n.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}(?![\\w-])`);
      if (re.test(t)) {
        found.push(a);
        break;
      }
    }
  }
  return found;
}

// ── turn scheduling ─────────────────────────────────────────────────
let emitEvent = () => {};

function bindEmitter(fn) {
  emitEvent = fn;
}

async function runAgentsForMessage(message) {
  const room = store.getRoom(message.roomId);
  if (!room) return;
  const agents = store.getRoomAgents(message.roomId).filter((a) => a.enabled);
  if (!agents.length) return;

  const mentioned = parseMentions(message.content, agents);
  const targets = mentioned.length ? mentioned : agents;

  targets.forEach((agent, i) => {
    setTimeout(() => {
      triggerAgentTurn(agent, message).catch((err) => {
        console.error(`[agents] turn failed for ${agent.name}:`, err.message);
        emitEvent({ type: "system", roomId: room.id, text: `⚠️ ${agent.name}: ${err.message}` });
      });
    }, i * 900);
  });
}

async function triggerAgentTurn(agent, userMessage) {
  const { relay } = require("./relay");
  const settings = store.getSettings();
  const adapter = getAdapter(agent.adapter) || echoAdapter;

  const msgId = uid("msg");
  store.insertMessage({
    id: msgId,
    roomId: userMessage.roomId,
    agentId: agent.id,
    senderName: agent.name,
    senderKind: "agent",
    content: "",
    status: "streaming",
  });
  emitEvent({
    type: "agent.started",
    message: store.getMessage(msgId),
    agentName: agent.name,
  });
  emitEvent({ type: "typing", roomId: userMessage.roomId, agentId: agent.id, agentName: agent.name });

  const history = store
    .listMessages(userMessage.roomId, 40)
    .filter((m) => m.id !== msgId && m.sender_kind !== "system" && m.content)
    .map((m) => ({
      role: m.sender_kind === "human" ? "user" : "assistant",
      content: m.content,
      name: m.sender_kind === "human" ? "user" : m.sender_name,
    }));

  if (!history.length || history[history.length - 1].content !== userMessage.content) {
    history.push({ role: "user", content: userMessage.content, name: "user" });
  }

  let acc = "";
  try {
    const stream = adapter.stream({
      agent,
      settings,
      history,
      systemPrompt: agent.system_prompt || `Du bist ${agent.name}, ein hilfreicher Agent im Raum "${store.getRoom(userMessage.roomId)?.name}".`,
    });
    for await (const token of stream) {
      if (!token) continue;
      acc += token;
      emitEvent({ type: "agent.token", messageId: msgId, delta: token });
    }
    const final = acc.trim();
    if (!final) throw new Error("Leere Antwort vom Adapter");
    store.updateMessage(msgId, { content: final, status: "complete" });
    emitEvent({ type: "agent.finished", message: store.getMessage(msgId) });
  } catch (err) {
    const partial = acc.trim() ? acc : "";
    store.updateMessage(msgId, { content: partial || `⚠️ ${err.message}`, status: "error" });
    emitEvent({ type: "agent.finished", message: store.getMessage(msgId) });
  }
}

module.exports = { listAdapters, getAdapter, runAgentsForMessage, triggerAgentTurn, bindEmitter, uid };