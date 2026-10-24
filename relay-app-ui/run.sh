#!/usr/bin/env bash
# Agent Relay UI HA Add-on — entrypoint.
# Startet den eigenständigen Front-Server (relay-server/) mit der geklonten Control-UI.
set -euo pipefail

OPTIONS_FILE="/data/options.json"
opt() { jq -r ".${1} // empty" "$OPTIONS_FILE" 2>/dev/null || echo ""; }

export PORT=8099
export HOST=0.0.0.0
# Der OpenClaw-Gateway in der HA-Addon-Welt: 45b315ab_openclaw_ha_addon — das Addon
# teilt dessen addon_configs-Volume? (addon_shared-Map) — sonst: env.
export GW_HOST="${GW_HOST:-$(opt gw_host)}"
export GW_PORT="${GW_PORT:-$(opt gw_port)}"
[ -n "$GW_PORT" ] || GW_PORT=18789
# Gateway-Token: MASKIEREND setzen — nie als Literal in Dateien:
# 1) Option openclaw_token (HA-Options)  2) addon_configs/openclaw.json (Beweis-Lane)
export OPENCLAW_GATEWAY_TOKEN="$(opt openclaw_token)"
if [ -z "$OPENCLAW_GATEWAY_TOKEN" ]; then
  for cfg in \
    /mnt/data/addon_configs/45b315ab_openclaw_ha_addon/.openclaw/openclaw.json \
    /config/openclaw/openclaw.json \
    /share/openclaw/openclaw.json; do
    if [ -f "$cfg" ]; then
      tok=$(jq -r '.gateway.auth.token // empty' "$cfg" 2>/dev/null || echo "")
      if [ -n "$tok" ]; then export OPENCLAW_GATEWAY_TOKEN="$tok"; echo "[relay-ui] Gateway-Token aus $cfg geladen (nicht geloggt)"; break; fi
    fi
  done
fi
# Launch-Lane (SPA-Token im Fragment) — nur wenn Token geladen:
if [ -n "$OPENCLAW_GATEWAY_TOKEN" ]; then
  export RELAY_LAUNCH_TOKEN="$OPENCLAW_GATEWAY_TOKEN"
  export RELAY_LAUNCH_GW_URL="${GW_HOST}:${GW_PORT}"
  export RELAY_OVERLAY=1
fi
export RELAY_DATA_DIR=/data

echo "[relay-ui] starting relay-server (host=$GW_HOST port=$GW_PORT overlay=${RELAY_OVERLAY:-0} data=$RELAY_DATA_DIR)"

if [ "${1:-}" = "--health" ]; then
  curl -fsS "http://127.0.0.1:8099/api/health" > /dev/null
  exit $?
fi

exec node /app/server/index.js