/**
 * Evey plugin example (Node.js).
 *
 * Listens for media events and logs them via the host Log RPC.
 * Demonstrates PluginService (host→plugin) and HostService (plugin→host).
 */
const grpc = require("@grpc/grpc-js");
const protoLoader = require("@grpc/proto-loader");
const path = require("path");

const PLUGIN_ID = process.env.EVEY_PLUGIN_ID || "unknown";
const PLUGIN_SOCK = process.env.EVEY_PLUGIN_SOCK || "/tmp/evey/plugins/nodejs-hello.sock";
const HOST_SOCK = process.env.EVEY_HOST_SOCK || "/tmp/evey/plugins/evey-host.sock";

// Load proto — adjust path to where proto files are mounted
const PROTO_PATH = path.join(__dirname, "../../proto/plugin/v1/plugin_api.proto");
const pkgDef = protoLoader.loadSync(PROTO_PATH, {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true,
  includeDirs: [path.join(__dirname, "../../proto")],
});
const proto = grpc.loadPackageDefinition(pkgDef).plugin.v1;

// Host client
const hostClient = new proto.HostService(
  `unix://${HOST_SOCK}`,
  grpc.credentials.createInsecure()
);

function hostLog(level, message, fields = {}) {
  hostClient.Log({ level, message, fields }, () => {});
}

// Plugin server implementation
const pluginImpl = {
  Initialize(call, callback) {
    hostLog("LOG_LEVEL_INFO", `plugin initialized (host ${call.request.host_version})`);
    callback(null, { success: true });
  },

  Shutdown(call, callback) {
    hostLog("LOG_LEVEL_INFO", `shutting down: ${call.request.reason}`);
    callback(null, {});
    setTimeout(() => process.exit(0), 500);
  },

  OnEvent(call, callback) {
    hostLog("LOG_LEVEL_INFO", `event received: ${call.request.event_type}`, {
      source: call.request.source || "",
    });
    callback(null, { handled: true });
  },

  HealthCheck(call, callback) {
    callback(null, { healthy: true, status: "running" });
  },

  GetCapabilities(call, callback) {
    callback(null, {
      api_version: "1.0.0",
      capabilities: [
        {
          name: "media.read",
          description: "Logs media events",
          event_types: ["media.added", "media.updated", "media.deleted"],
        },
      ],
    });
  },
};

const server = new grpc.Server();
server.addService(proto.PluginService.service, pluginImpl);
server.bindAsync(
  `unix://${PLUGIN_SOCK}`,
  grpc.ServerCredentials.createInsecure(),
  (err) => {
    if (err) {
      console.error(`[nodejs-hello] bind failed: ${err.message}`);
      process.exit(1);
    }
    console.error(`[nodejs-hello] listening on ${PLUGIN_SOCK}`);
  }
);
