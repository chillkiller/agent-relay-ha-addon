// Echo adapter: deterministic demo agent with streaming chunks.
// Useful to prove the multi-agent room flow without any external service.

const REPLIES = [
  "Verstanden — ich habe deine Nachricht im Raum gesehen.",
  "Guter Punkt. Falls du mich direkt ansprechen willst: @echo vor die Nachricht setzen.",
  "Ich bin der Demo-Agent (`echo`). Lege in Einstellungen einen OpenAI-Agent an, um echte Antworten zu bekommen.",
];

function pick(text) {
  let h = 0;
  for (const ch of String(text)) h = (h * 31 + ch.codePointAt(0)) >>> 0;
  return REPLIES[h % REPLIES.length];
}

async function* stream({ history }) {
  const last = history.filter((m) => m.role === "user").at(-1);
  const text = `✅ ${pick(last ? last.content : "")}

> Du schriebst: „${last ? String(last.content).slice(0, 200) : ""}"`;

  const chunks = text.match(/[\s\S]{1,18}/g) || [];
  for (const c of chunks) {
    await new Promise((r) => setTimeout(r, 40));
    yield c;
  }
}

module.exports = { id: "echo", stream };