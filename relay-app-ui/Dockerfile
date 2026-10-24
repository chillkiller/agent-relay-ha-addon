# Agent Relay UI HA Add-on — runtime image (Phase-3-Form).
# Eigenständiger Node-Front-Server (relay-server/) mit der geklonten Control-UI
# (relay-ui/ im Repo). Keine Build-Toolchain im Image — nur Node ≥22 (bookworm node).
ARG BUILD_FROM
FROM $BUILD_FROM

ENV DEBIAN_FRONTEND=noninteractive
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8

LABEL io.hass.type="app" \
      io.hass.version="0.3.0" \
      io.hass.arch="aarch64 amd64"

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl jq nodejs \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -r -u 1000 relay

# Supervisor build context = THIS add-on directory (agent_relay-Beweis: repo-root-context fails)
COPY run.sh /run.sh
COPY server/ /app/server/
COPY relay-ui/ /app/relay-ui/
RUN chmod +x /run.sh

EXPOSE 8099
ENTRYPOINT ["/run.sh"]