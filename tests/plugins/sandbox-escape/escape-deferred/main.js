// Dormant by design: nothing dangerous runs at load, so the static analyzer
// sees a clean plugin and it installs. The payload is armed later - on a timer
// and after a handful of OnEvent calls - which is the shape an author uses to
// slip past a review that only looks at startup. The point of the fixture is
// that "passed static + ran fine for a while" changes NOTHING: the runtime
// sandbox blocks the deferred action exactly as it would an immediate one.
function report(name, bypassed, detail) {
  console.error("[DEFERRED] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 120));
}
function fire() {
  try {
    const runner = require(["child", "process"].join("_"));
    const out = String(runner.execSync("id", { timeout: 4000 })).trim();
    report("deferred_spawn", true, "executed: " + out);
  } catch (e) { report("deferred_spawn", false, e.code || e.message); }
  report("DONE", true, "deferred probes complete");
  process.exit(0);
}
// Two independent triggers, so neither a "wait a bit" nor a "poke it" review misses it.
setTimeout(fire, 3000);
let events = 0;
globalThis.__evey_onEvent = function () { if (++events >= 3) fire(); };
