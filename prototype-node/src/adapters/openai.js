// OpenAI-compatible chat adapter.
// Works with the OpenClaw gateway (POST {base}/v1/chat/completions with agent
// target models like "openclaw/<agentId>"), Ollama, llama.cpp, vLLM, etc.

function defaultBaseUrl(settings) {
  const fromSettings = settings?.openclawBaseUrl;
  if (fromSettings) return String(fromSettings).replace(/\/+$/, "");
  if (process.env.RELAY_OPENAI_BASE_URL) return process.env.RELAY_OPENAI_BASE_URL;
  if (process.env.OPENCLAW_GATEWAY_URL) {
    return String(process.env.OPENCLAW_GATEWAY_URL).replace(/\/+$/, "") + "/v1";
  }
  return "http://127.0.0.1:18789/v1";
}

function defaultToken(settings) {
  if (settings?.openclawToken) return String(settings.openclawToken);
  if (process.env.RELAY_OPENAI_API_KEY) return process.env.RELAY_OPENAI_API_KEY;
  if (process.env.OPENCLAW_GATEWAY_TOKEN) return process.env.OPENCLAW_GATEWAY_TOKEN;
  if (process.env.OPENAI_API_KEY) return process.env.OPENAI_API_KEY;
  return "";
}

function completionsUrl(baseUrl) {
  const b = String(baseUrl).replace(/\/+$/, "");
  if (/\/chat\/completions$/.test(b)) return b;
  if (/\/v\d+$/.test(b)) return `${b}/chat/completions`;
  return `${b}/v1/chat/completions`;
}

async function* stream({ agent, settings, history, systemPrompt }) {
  const baseUrl = agent.base_url || defaultBaseUrl(settings);
  const apiKey = agent.api_key || defaultToken(settings);
  const model = agent.model || "openclaw";

  const messages = [];
  if (agent.system_prompt || systemPrompt) {
    messages.push({ role: "system", content: agent.system_prompt || systemPrompt });
  }
  for (const m of history) {
    messages.push({
      role: m.role,
      content: m.content,
      ...(m.role === "user" && m.name ? { name: m.name } : {}),
    });
  }

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 180_000);
  let resp;
  try {
    resp = await fetch(completionsUrl(baseUrl), {
      method: "POST",
      signal: controller.signal,
      headers: {
        "Content-Type": "application/json",
        ...(apiKey ? { Authorization: `Bearer ${apiKey}` } : {}),
      },
      body: JSON.stringify({
        model,
        messages,
        stream: true,
        user: agent.id ? `relay-agent-${agent.id}` : undefined,
      }),
    });
  } catch (err) {
    clearTimeout(timer);
    throw new Error(
      `Adapter-Verbindung fehlgeschlagen (${baseUrl}): ${err.name === "AbortError" ? "Timeout" : err.message}`
    );
  }
  if (!resp.ok || !resp.body) {
    clearTimeout(timer);
    const body = await resp.text().catch(() => "");
    throw new Error(`LLM-Endpoint ${resp.status}: ${body.slice(0, 400) || "keine Antwort"}`);
  }

  // SSE stream: lines "data: {...}"
  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      const lines = buf.split("\n");
      buf = lines.pop() ?? "";
      for (const line of lines) {
        const t = line.trim();
        if (!t.startsWith("data:")) continue;
        const payload = t.slice(5).trim();
        if (payload === "[DONE]") return;
        let json;
        try {
          json = JSON.parse(payload);
        } catch {
          continue;
        }
        const delta = json.choices?.[0]?.delta?.content;
        if (delta) yield delta;
      }
    }
  } finally {
    clearTimeout(timer);
    try {
      reader.releaseLock();
    } catch {}
  }
}

module.exports = { id: "openai", stream };