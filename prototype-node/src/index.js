// Agent Relay server entrypoint.
const path = require("node:path");
const os = require("node:os");
const { createRelay } = require("./relay");

const PORT = Number(process.env.PORT || process.env.RELAY_PORT || 8099);
const HOST = process.env.HOST || "0.0.0.0";
// SQLite needs proper fcntl locks — network mounts (SMB/NFS) don't provide them.
// Default: store under the user's home dir (local FS). In the HA add-on,
// run.sh sets RELAY_DATA_DIR to the persistent /data volume.
const DATA_DIR =
  process.env.RELAY_DATA_DIR ||
  process.env.DATA_DIR ||
  path.join(os.homedir(), ".agent-relay");

const { server } = createRelay({ dataDir: DATA_DIR, port: PORT, host: HOST });

function shutdown() {
  console.log("[relay] shutting down…");
  server.close(() => process.exit(0));
  setTimeout(() => process.exit(0), 1500);
}
process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);