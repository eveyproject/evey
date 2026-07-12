// RED-TEAM - grant-widening probe. The plugin legitimately holds hardware.gpu
// (so the render node opens), then tries every way to widen that single-node
// grant into the privileged/compute GPU surface and to spawn vendor tools.
// Everything except the render node must stay BLOCKED - proving hardware.gpu is
// the render node ALONE and cannot be escalated from inside the sandbox.
const fs = require("fs");

function report(name, bypassed, detail) {
  console.error("[ESCAPE] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 120));
}

const SEP = "/";
const dev = SEP + "dev";
const dri = [dev, "d" + "ri"].join(SEP);

function probeOpen(name, p) {
  try { const fd = fs.openSync(p, "r"); fs.closeSync(fd); report(name, true, "opened " + p); }
  catch (e) { report(name, false, (e.code || e.message) + " " + p); }
}

// Control: the granted render node opens (expected BYPASSED - that is the grant).
probeOpen("control_render", [dri, "render" + "D128"].join(SEP));

// Escalation attempts - all expected BLOCKED.
probeOpen("widen_card", [dri, "card" + "0"].join(SEP));            // privileged KMS node
probeOpen("widen_control", [dri, "control" + "D64"].join(SEP));    // legacy control node
probeOpen("widen_dxg", [dev, "d" + "xg"].join(SEP));               // WSL paravirt GPU
probeOpen("widen_nvidia0", [dev, "nv" + "idia0"].join(SEP));       // NVIDIA compute
probeOpen("widen_nvidiactl", [dev, "nv" + "idiactl"].join(SEP));
probeOpen("widen_kfd", [dev, "k" + "fd"].join(SEP));               // AMD KFD compute (VRAM leak path)

// Try to enumerate /dev to discover other nodes - also blocked (no /dev dir grant).
try { const d = fs.readdirSync(dev); report("widen_list_dev", true, d.length + " entries in /dev"); }
catch (e) { report("widen_list_dev", false, e.code || e.message); }

// Try to spawn a GPU tool to bypass the device-node restriction - blocked by the
// LD_PRELOAD execve shim (no subprocess permission).
try {
  const cp = require("child" + "_pro" + "cess");
  cp.execFileSync([SEP + "usr", "bin", "ff" + "mpeg"].join(SEP), ["-hide_banner", "-encoders"], { timeout: 3000 });
  report("widen_spawn_ffmpeg", true, "spawned ffmpeg");
} catch (e) { report("widen_spawn_ffmpeg", false, e.code || e.message); }

report("DONE", true, "escalation probes complete");
process.exit(0);
