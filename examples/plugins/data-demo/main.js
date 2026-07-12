/**
 * Evey example plugin - data store demo.
 *
 * On startup it increments a persistent "boot_count" using the per-plugin data
 * store (HostService GetData/SetData). The store needs NO permission: a plugin
 * owns its own namespace, and the rows are wiped automatically when the plugin
 * is uninstalled. Values are opaque strings - encode JSON yourself if needed.
 */
const grpc = require("@grpc/grpc-js");
const protoLoader = require("@grpc/proto-loader");
const path = require("path");

const HOST_SOCK = process.env.EVEY_HOST_SOCK || "/tmp/evey/plugins/evey-host.sock";

const PROTO_PATH = path.join(__dirname, "plugin_api.proto"); // copied next to main.js by the installer
const pkgDef = protoLoader.loadSync(PROTO_PATH, {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true,
});
const proto = grpc.loadPackageDefinition(pkgDef).plugin.v1;

const host = new proto.HostService(`unix://${HOST_SOCK}`, grpc.credentials.createInsecure());

const log = (level, message, fields = {}) => host.Log({ level, message, fields }, () => {});

// Promisified data-store helpers.
const getData = (key) => new Promise((res) => host.GetData({ key }, (e, r) => res(e ? null : r)));
const setData = (key, value) => new Promise((res) => host.SetData({ key, value }, () => res()));

async function bumpBootCount() {
  const r = await getData("boot_count");
  const next = (r && r.found ? parseInt(r.value, 10) || 0 : 0) + 1;
  await setData("boot_count", String(next));
  log("LOG_LEVEL_INFO", `data-demo started - boot #${next}`);
}

const pluginImpl = {
  Initialize(call, callback) {
    bumpBootCount().catch((err) => log("LOG_LEVEL_ERROR", `data store error: ${err}`));
    callback(null, { success: true });
  },
  Shutdown(_call, callback) {
    callback(null, {});
    setTimeout(() => process.exit(0), 200);
  },
  HealthCheck(_call, callback) {
    callback(null, { healthy: true, status: "running" });
  },
  GetCapabilities(_call, callback) {
    callback(null, { capabilities: [] });
  },
  OnEvent(_call, callback) {
    callback(null, { handled: true });
  },
};

const server = new grpc.Server();
server.addService(proto.PluginService.service, pluginImpl);
const sock = process.env.EVEY_PLUGIN_SOCK || "/tmp/evey/plugins/data-demo.sock";
server.bindAsync(`unix://${sock}`, grpc.ServerCredentials.createInsecure(), () => {
  log("LOG_LEVEL_INFO", "data-demo plugin listening");
});
