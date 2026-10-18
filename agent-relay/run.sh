#!/usr/bin/env bash
# Agent Relay HA Add-on — entrypoint.
# Reads standard HA add-on options and starts the Go binary with env vars.
set -euo pipefail

OPTIONS_FILE="/data/options.json"
opt()           { jq -r ".${1} // empty"    "$OPTIONS_FILE" 2>/dev/null || echo ""; }
opt_bool_str()  { jq -r ".${1} // empty"    "$OPTIONS_FILE" 2>/dev/null || echo ""; }

export PORT=8099
export HOST=0.0.0.0
# /data is the app-private persistent volume (SQLite needs real fcntl locks —
# never put this on a network mount).
export RELAY_DATA_DIR=/data
export RELAY_USER_NAME="$(opt relay_user_name)"
export RELAY_OPENAI_BASE_URL="$(opt openclaw_base_url)"
export RELAY_OPENAI_API_KEY="$(opt openclaw_token)"
export RELAY_BEARER_TOKEN="$(opt api_bearer_token)"

echo "[relay] starting agentrelay (user=$(id -un), data=$RELAY_DATA_DIR)"
if [ -n "$RELAY_OPENAI_BASE_URL" ]; then
  echo "[relay] LLM base URL: $RELAY_OPENAI_BASE_URL"
fi

# healthcheck helper (the Supervisor also probes /api/health)
if [ "${1:-}" = "--health" ]; then
  curl -fsS "http://127.0.0.1:8099/api/health" > /dev/null
  exit $?
fi

exec /usr/bin/agentrelay