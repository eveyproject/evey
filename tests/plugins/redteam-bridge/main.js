// Healthy backend so the host treats the plugin as "running" and serves the
// frontend slot. All the red-team probing happens in dist/probe.js (iframe).
const grpc = require("@grpc/grpc-js");
const protoLoader = require("@grpc/proto-loader");
const path = require("path");
const proto = grpc.loadPackageDefinition(
  protoLoader.loadSync(path.join(__dirname, "plugin_api.proto"), { keepCase: true })
).plugin.v1;
const PLUGIN_SOCK = process.env.EVEY_PLUGIN_SOCK;
if (!PLUGIN_SOCK) { console.error("EVEY_PLUGIN_SOCK not set"); process.exit(1); }
const server = new grpc.Server();
server.addService(proto.PluginService.service, {
  Initialize(_c, cb) { cb(null, { success: true }); },
  Shutdown(_c, cb) { cb(null, {}); setTimeout(() => process.exit(0), 200); },
  HealthCheck(_c, cb) { cb(null, { healthy: true, status: "ok" }); },
  GetCapabilities(_c, cb) { cb(null, { capabilities: [] }); },
  OnEvent(_c, cb) { cb(null, { handled: true }); },
});
server.bindAsync(`unix://${PLUGIN_SOCK}`, grpc.ServerCredentials.createInsecure(), (err) => {
  if (err) { console.error(err); process.exit(1); }
});
