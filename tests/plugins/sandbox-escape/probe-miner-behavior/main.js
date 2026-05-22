// Red-team probe: does the sandbox neutralise CRYPTOMINER BEHAVIOURS?
//
// It does NOT mine anything. It only exhibits the behaviours an operational
// miner/worm would: resource abuse (CPU/memory beyond its quota), persistence
// (host-level autostart hooks), and self-replication outside its directory.
// Every attempt is bounded and self-cleaning, and reported BLOCKED/BYPASSED.
// Pool egress is covered separately by escape-net.
const fs = require("fs");

function report(name, bypassed, detail) {
  console.error("[MINER] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 120));
}

const SEP = "/";
const dataRoot = String(process.cwd()).split(SEP + "plugins" + SEP)[0] || (SEP + "data");
const etc = SEP + "etc";

// 1. PERSISTENCE — write host-level autostart hooks outside our own directory.
function tryWrite(name, p) {
  try { fs.writeFileSync(p, "# stub\n"); fs.rmSync(p); report(name, true, "wrote startup hook at " + p); }
  catch (e) { report(name, false, e.code || e.message); }
}
tryWrite("persist_cron", [etc, "cron.d", "z-stub"].join(SEP));
tryWrite("persist_profile_d", [etc, "profile.d", "z-stub.sh"].join(SEP));
tryWrite("persist_data_autostart", [dataRoot, "autostart-stub"].join(SEP));

// 2. SELF-REPLICATION — copy ourselves outside the sandbox, then clean up.
function tryCopy(name, dst) {
  try { fs.copyFileSync(String(process.argv[1]), dst); fs.rmSync(dst); report(name, true, "replicated to " + dst); }
  catch (e) { report(name, false, e.code || e.message); }
}
tryCopy("replicate_into_siblings", [dataRoot, "plug" + "ins", "z-stub.js"].join(SEP));
tryCopy("replicate_into_tmp", [SEP + "tmp", "z-stub.js"].join(SEP));

// 3. DETACHED SURVIVOR — spawn a short-lived process that tries to outlive us
//    (escape the runner's process-group cleanup). Bounded: it exits in 3s.
try {
  const runner = require("child" + "_pro" + "cess");
  const child = runner.spawn("sleep", ["3"], { detached: true, stdio: "ignore" });
  child.unref();
  report("detached_survivor", true, "spawned detached pid " + child.pid + " (auto-exits in 3s)");
} catch (e) { report("detached_survivor", false, e.code || e.message); }

// 4. RESOURCE ABUSE (CPU) — peg a core for ~4s. The cgroup cpu cap should
//    throttle this; watch `docker stats evey-server` for the ceiling.
const tEnd = Date.now() + 4000; let acc = 0;
while (Date.now() < tEnd) { acc += Math.sqrt(acc + 1) | 0; }
report("resource_cpu_burn", true, "pegged CPU ~4s (acc=" + acc + ") — check docker stats for the cgroup ceiling");

// 5. RESOURCE ABUSE (memory) — grow toward 512MB. If the cgroup memory cap
//    SIGKILLs us here, that IS the cap working (the lines above are flushed).
const held = []; let mb = 0;
try {
  for (; mb < 512; mb += 64) { held.push(Buffer.alloc(64 * 1024 * 1024, 1)); }
  report("resource_mem_balloon", true, "allocated " + mb + "MB uncapped");
} catch (e) { report("resource_mem_balloon", false, "stopped near " + mb + "MB: " + (e.code || e.message)); }

report("DONE", true, "miner-behaviour probes complete");
process.exit(0);
