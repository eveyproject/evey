// Red-team probe: can a plugin WITHOUT the "network" permission open TCP
// connections? On Landlock ABI v4 the launcher passes -restrict-net with no
// allowed ports, so every TCP connect (even to loopback) should be denied.
// Targets: the internal Postgres / Meilisearch / server ports, and an external
// host. The variable is not named "socket", so the analyzer's network rule
// (which keys on `socket.connect`) does not match node's `net.connect`.
const tcp = require("net");

function report(name, bypassed, detail) {
  console.error("[ESCAPE] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 120));
}

const targets = [
  ["postgres", "127.0.0.1", 5432],
  ["meilisearch", "127.0.0.1", 7700],
  ["server_http", "127.0.0.1", 9091],
  ["external_https", "1.1.1.1", 443],
  ["external_dns_tcp", "1.1.1.1", 53],
];

let pending = targets.length;
function settle() { if (--pending <= 0) { report("DONE", true, "net probes complete"); process.exit(0); } }

targets.forEach(([label, host, port]) => {
  const name = "connect_" + label + "_" + port;
  const conn = tcp.connect({ host: host, port: port });
  conn.setTimeout(2500);
  conn.on("connect", () => { report(name, true, "connected to " + host + ":" + port); conn.destroy(); settle(); });
  conn.on("timeout", () => { report(name, false, "timeout (no route / dropped)"); conn.destroy(); settle(); });
  conn.on("error", (e) => { report(name, false, e.code || e.message); settle(); });
});

// Safety net: never hang the runner.
setTimeout(() => { report("WATCHDOG", true, "forced exit"); process.exit(0); }, 7000);
