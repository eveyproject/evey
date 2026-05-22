// Red-team probe: is the plugin isolated from other processes?
//
// Landlock does NOT provide PID-namespace isolation (that needs namespaces =
// bubblewrap = the mount syscall, blocked under default Docker). So this probe
// is expected to surface a real gap: a plugin shares the container's PID
// namespace with the server. Method names are assembled at runtime so static
// analysis sees no flagged "process.kill(1" / "/proc/<pid>/environ" literal.
const fs = require("fs");

function report(name, bypassed, detail) {
  console.error("[ESCAPE] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 120));
}

const SEP = "/";
const procRoot = SEP + "pr" + "oc";

// 1. Can we signal PID 1 (the server)? Signal 0 is a CAPABILITY PROBE — it does
//    NOT kill anything; success means we COULD send SIGKILL (a DoS vector).
try {
  process["ki" + "ll"](1, 0);
  report("signal_pid1", true, "can signal PID 1 — could DoS the server");
} catch (e) { report("signal_pid1", false, e.code || e.message); }

// 2. Enumerate other processes via /proc.
try {
  const pids = fs.readdirSync(procRoot).filter((x) => /^\d+$/.test(x));
  report("proc_enumeration", true, pids.length + " processes visible");
} catch (e) { report("proc_enumeration", false, e.code || e.message); }

// 3. Read PID 1's environment — would leak the server's secrets (DB password,
//    master key) if /proc were reachable.
try {
  const env = fs.readFileSync([procRoot, "1", "envi" + "ron"].join(SEP), "utf8");
  report("steal_server_env", true, env.length + "B of server env leaked");
} catch (e) { report("steal_server_env", false, e.code || e.message); }

report("DONE", true, "proc probes complete");
process.exit(0);
