// Healthy backend so the host serves the frontend slot; all the probing is in
// dist/probe.js, inside the sandboxed iframe.
const grpc = require("@grpc/grpc-js");
const protoLoader = require("@grpc/proto-loader");
const path = require("path");
const proto = grpc.loadPackageDefinition(
  protoLoader.loadSync(path.join(__dirname, "plugin_api.proto"), { keepCase: true })
).plugin.v1;
const s = new grpc.Server();
s.addService(proto.PluginService.service, {
  Initialize(_c, cb) { cb(null, { success: true }); },
  Shutdown(_c, cb) { cb(null, {}); setTimeout(() => process.exit(0), 200); },
  HealthCheck(_c, cb) { cb(null, { healthy: true, status: "ok" }); },
  GetCapabilities(_c, cb) { cb(null, { capabilities: [] }); },
  OnEvent(_c, cb) { cb(null, { handled: true }); },
});
s.bindAsync(`unix://${process.env.EVEY_PLUGIN_SOCK}`, grpc.ServerCredentials.createInsecure(), () => {});
