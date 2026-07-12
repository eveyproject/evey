// RED-TEAM - DISGUISED miner #1. Ships as a benign "subtitle enhancer" (the
// manifest looks innocent: media.read + api.library, no hardware.gpu), but at
// runtime it covertly tries to reach the GPU to mine and falls back to CPU
// mining. This is the realistic supply-chain case: a useful-looking plugin that
// turns malicious. Everything sensitive is assembled at runtime so the static
// analyzer sees only string fragments, not flagged paths.
//
// Expected outcome - the sandbox defeats the disguise regardless of the friendly
// manifest: every GPU node BLOCKED (no hardware.gpu was granted), exec BLOCKED
// (no subprocess), and the CPU mining flagged by the monitor.
const fs = require("fs");

function report(name, bypassed, detail) {
  console.error("[MINER] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 120));
}

// Pretend to do subtitle work first (cover traffic).
report("subtitle_scan", true, "scanning subtitles (cover for the real payload)");

const SEP = "/";
const dri = [SEP + "dev", "d" + "ri"].join(SEP);
function probeOpen(name, p) {
  try { const fd = fs.openSync(p, "r"); fs.closeSync(fd); report(name, true, "opened " + p); }
  catch (e) { report(name, false, (e.code || e.message) + " " + p); }
}
probeOpen("covert_open_render", [dri, "render" + "D128"].join(SEP));
probeOpen("covert_open_card", [dri, "card" + "0"].join(SEP));

// Try to spawn a miner binary (also covert).
try {
  const cp = require("child" + "_pro" + "cess");
  cp.execFileSync([SEP + "usr", "bin", "n" + "ode"].join(SEP), ["-e", "0"], { timeout: 2000 });
  report("covert_spawn_helper", true, "spawned helper");
} catch (e) { report("covert_spawn_helper", false, e.code || e.message); }

// Fall back to CPU mining behind the "enhancement" label.
const tEnd = Date.now() + 6000;
let acc = 0;
while (Date.now() < tEnd) { acc += Math.sqrt(acc + 1) | 0; }
report("covert_cpu_mine", true, "mined on CPU ~6s under the subtitle-enhancer label");

report("DONE", true, "disguised-miner probes complete");
process.exit(0);
