// RED-TEAM — overt cryptominer, NO hardware.gpu permission.
//
// It tries to reach the GPU to mine and, failing that, pegs the CPU. Expected:
// every GPU node BLOCKED by Landlock (no permission), and sustained CPU pegging
// detected by the resource monitor (logged; nice keeps it from starving others).
// Sensitive paths assembled at runtime so it slips past static analysis.
const fs = require("fs");

function report(name, bypassed, detail) {
  console.error("[MINER] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 120));
}

const SEP = "/";
const dri = [SEP + "dev", "d" + "ri"].join(SEP);
const targets = [
  ["open_render", [dri, "render" + "D128"].join(SEP)],
  ["open_card", [dri, "card" + "0"].join(SEP)],
  ["open_dxg", [SEP + "dev", "d" + "xg"].join(SEP)],
];
for (const [name, p] of targets) {
  try { const fd = fs.openSync(p, "r"); fs.closeSync(fd); report(name, true, "opened " + p); }
  catch (e) { report(name, false, (e.code || e.message) + " " + p); }
}

// No GPU → fall back to CPU mining. The monitor should flag sustained pegging
// after ~30s; we run a bounded burst so the demo terminates on its own.
const tEnd = Date.now() + 8000;
let acc = 0;
while (Date.now() < tEnd) { acc += Math.sqrt(acc + 1) | 0; }
report("cpu_mine_burst", true, "pegged CPU ~8s (acc=" + acc + ") — monitor should flag/throttle");

report("DONE", true, "overt-miner probes complete");
process.exit(0);
