// Config Demo Plugin — minimal stub
// The interesting part is in plugin.yaml (config_schema) and widget.bundle.js

const grpc = require("@grpc/grpc-js");
const protoLoader = require("@grpc/proto-loader");
const path = require("path");

const PROTO_PATH = path.join(__dirname, "plugin_api.proto");
const pkgDef = protoLoader.loadSync(PROTO_PATH, { keepCase: true });
const proto = grpc.loadPackageDefinition(pkgDef).plugin.v1;

const PLUGIN_PORT = process.env.EVEY_PLUGIN_PORT || "50052";

const server = new grpc.Server();
server.addService(proto.PluginService.service, {
  Initialize(call, cb) { cb(null, { success: true }); },
  Shutdown(call, cb) { cb(null, {}); setTimeout(() => process.exit(0), 500); },
  OnEvent(call, cb) { cb(null, { handled: false }); },
  HealthCheck(_call, cb) { cb(null, { healthy: true, status: "ok" }); },
  GetCapabilities(_call, cb) { cb(null, { api_version: "1", capabilities: [] }); },
});

server.bindAsync("0.0.0.0:" + PLUGIN_PORT, grpc.ServerCredentials.createInsecure(), (err) => {
  if (err) { console.error(err); process.exit(1); }
  console.log("config-demo listening on port %s", PLUGIN_PORT);
});
