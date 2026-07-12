// Weather plugin - backend lifecycle stub.
// All weather fetching happens in front.js (CSP connect-src is whitelisted
// via external_fetch in plugin.yaml). The backend just stays alive so the
// host treats the plugin as "running" and keeps the frontend slot active.

const grpc = require("@grpc/grpc-js");
const protoLoader = require("@grpc/proto-loader");
const path = require("path");

const PROTO_PATH = path.join(__dirname, "plugin_api.proto");
const pkgDef = protoLoader.loadSync(PROTO_PATH, { keepCase: true });
const proto = grpc.loadPackageDefinition(pkgDef).plugin.v1;

const PLUGIN_SOCK = process.env.EVEY_PLUGIN_SOCK;
if (!PLUGIN_SOCK) {
  console.error("EVEY_PLUGIN_SOCK not set");
  process.exit(1);
}

const server = new grpc.Server();
server.addService(proto.PluginService.service, {
  Initialize(_call, cb) { cb(null, { success: true }); },
  Shutdown(_call, cb) { cb(null, {}); setTimeout(() => process.exit(0), 200); },
  OnEvent(_call, cb) { cb(null, { handled: false }); },
  HealthCheck(_call, cb) { cb(null, { healthy: true, status: "ok" }); },
  GetCapabilities(_call, cb) { cb(null, { api_version: "1", capabilities: [] }); },
});

server.bindAsync(`unix://${PLUGIN_SOCK}`, grpc.ServerCredentials.createInsecure(), (err) => {
  if (err) { console.error("bind failed:", err); process.exit(1); }
  console.error(`weather listening on ${PLUGIN_SOCK}`);
});
