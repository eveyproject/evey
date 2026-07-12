// RED-TEAM - DISGUISED miner #2, the HARDEST case. It passes as a legitimate
// GPU plugin: an "AI upscaler" that genuinely needs hardware.gpu, so an admin
// reasonably grants it. It then ABUSES the grant. This tests the controls that
// must hold even AFTER the GPU is legitimately granted:
//   1) the grant is the render node ALONE - escalating to card0/dxg/nvidia* must
//      still be BLOCKED (the plugin cannot widen its own access);
//   2) sustained GPU/CPU pegging (mining instead of bursty upscaling) is the
//      miner signature the resource monitor must flag/kill.
//
// NOTE: a real miner would submit endless GPU kernels on the render node, which
// the monitor's DRM-engine watcher (/proc/<pid>/fdinfo drm-engine-*) kills after
// ~30s sustained. Plain Node can't submit GPU work, so here the "mining" shows
// as a sustained CPU peg (caught by the CPU monitor) - the GPU-engine kill is
// exercised by the unit logic in runner_resmon.go.
const fs = require("fs");

function report(name, bypassed, detail) {
  console.error("[MINER] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 120));
}

const SEP = "/";
const dri = [SEP + "dev", "d" + "ri"].join(SEP);

// 1. Use the legitimately-granted render node (this SHOULD succeed - it is the
//    cover that makes the plugin look benign).
try {
  const fd = fs.openSync([dri, "render" + "D128"].join(SEP), "r");
  fs.closeSync(fd);
  report("granted_render_open", true, "opened the granted render node (legit-looking)");
} catch (e) { report("granted_render_open", false, (e.code || e.message)); }

// 2. ESCALATE beyond the grant - these must all stay BLOCKED.
function probe(name, p) {
  try { const fd = fs.openSync(p, "r"); fs.closeSync(fd); report(name, true, "WIDENED grant to " + p); }
  catch (e) { report(name, false, (e.code || e.message) + " " + p); }
}
probe("escalate_card", [dri, "card" + "0"].join(SEP));
probe("escalate_dxg", [SEP + "dev", "d" + "xg"].join(SEP));
probe("escalate_nvidiactl", [SEP + "dev", "nv" + "idiactl"].join(SEP));
probe("escalate_kfd", [SEP + "dev", "k" + "fd"].join(SEP));

// 3. MINE: peg compute continuously instead of doing bursty upscaling. On a real
//    GPU this would be endless kernels; the monitor flags sustained occupancy.
const tEnd = Date.now() + 10000;
let acc = 0;
while (Date.now() < tEnd) { acc += Math.sqrt(acc + 1) | 0; }
report("sustained_mine", true, "pegged compute ~10s (acc=" + acc + ") - monitor should kill a real GPU peg");

report("DONE", true, "fake-upscaler probes complete");
process.exit(0);
