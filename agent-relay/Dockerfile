# Agent Relay HA Add-on — runtime image.
# The Go binary is built by build.sh (or the GitHub Actions workflow) and
# shipped pre-built per arch; the Supervisor builds the container from the
# matching arch folder. No Rust/npm toolchain in the image — distroless-style.
ARG ARCH
FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8

# HA OCI labels (required when not using the GitHub builder flow)
LABEL io.hass.type="app" \
      io.hass.version="0.1.0" \
      io.hass.arch="aarch64 amd64"

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl jq \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -r -u 1000 relay

# app binary + static assets are copied by the Supervisor build (build.yaml)
ARG TARGETARCH
COPY agent-relay/run.sh /run.sh
COPY dist/agentrelay-${TARGETARCH:-amd64} /usr/bin/agentrelay
RUN chmod +x /run.sh /usr/bin/agentrelay

EXPOSE 8099
ENTRYPOINT ["/run.sh"]