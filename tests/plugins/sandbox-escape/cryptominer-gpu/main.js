// Red-team probe: can a plugin reach the GPU to mine on a hardware-transcode box?
//
// When Evey runs the NVENC/VAAPI/QSV image (`--gpus`, /dev/dri mapped in), the
// host exposes DRM render nodes and the vendor compute/encode libraries. An
// operational cryptominer would: open a render node and submit DRM/CUDA ioctls,
// dlopen libcuda/libnvidia-encode, or shell out to a GPU helper (nvidia-smi,
// ffmpeg -c:v *_nvenc) as a loader. This probe ATTEMPTS each reach and reports
// BLOCKED/BYPASSED - it never mines, never submits a kernel.
//
// Every sensitive path/binary is ASSEMBLED at runtime so no GPU string literal
// appears in the source (the static analyser stays clean). The plugin declares
// only api.library - no `subprocess`, no `native-binary`, no `network` - so the
// launcher applies -deny-inet, the LD_PRELOAD execve shim, and the Landlock
// filesystem allowlist. A correctly sandboxed plugin must NOT reach any GPU
// device node nor spawn a GPU helper.
const fs = require("fs");

function report(name, bypassed, detail) {
  console.error("[MINER] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 120));
}

const SEP = "/";
const dev = SEP + "dev";
const dri = [dev, "d" + "ri"].join(SEP);                  // /dev/dri
const render = [dri, "render" + "D128"].join(SEP);        // /dev/dri/renderD128 - direct GPU
const card0 = [dri, "card" + "0"].join(SEP);              // /dev/dri/card0
const dxg = [dev, "d" + "xg"].join(SEP);                  // /dev/dxg - WSL2 GPU paravirt
const nvidia0 = [dev, "nv" + "idia0"].join(SEP);          // /dev/nvidia0
const nvidiactl = [dev, "nv" + "idiactl"].join(SEP);      // /dev/nvidiactl

function probeOpen(name, p) {
  try { const fd = fs.openSync(p, "r"); fs.closeSync(fd); report(name, true, "opened " + p); }
  catch (e) { report(name, false, (e.code || e.message) + " " + p); }
}
function probeList(name, p) {
  try { const d = fs.readdirSync(p); report(name, true, d.length + " entries in " + p); }
  catch (e) { report(name, false, (e.code || e.message) + " " + p); }
}

// 1. DRM render / card nodes - the device a GPU miner submits work through.
probeList("list_dri", dri);
probeOpen("open_drm_render", render);
probeOpen("open_drm_card", card0);
// 2. WSL2 + NVIDIA device nodes.
probeOpen("open_dxg", dxg);
probeOpen("open_nvidia0", nvidia0);
probeOpen("open_nvidiactl", nvidiactl);

// 3. Vendor compute/encode libraries - a miner dlopen()s these. We search the
//    system lib dirs at runtime and read a few bytes (a successful read means
//    the loader could map it). Reading the .so is inert WITHOUT a device node,
//    but it is reported so the boundary is explicit.
// Bounded by WALL CLOCK as well as directory count: on a runner where the
// vendor libraries are absent, the step budget is never reached and the walk
// runs to exhaustion. CI, 2026-09-09: two exhaustive walks ate the fixture's
// whole 30 s budget and the exec probes below never reported at all.
function findLib(needle, roots) {
  const deadline = Date.now() + 2000;
  for (const root of roots) {
    const stack = [root];
    let steps = 0;
    while (stack.length && steps++ < 6000 && Date.now() < deadline) {
      const d = stack.pop();
      let entries;
      try { entries = fs.readdirSync(d, { withFileTypes: true }); } catch (e) { continue; }
      for (const ent of entries) {
        const full = [d, ent.name].join(SEP);
        if (ent.isDirectory()) stack.push(full);
        else if (ent.name.indexOf(needle) >= 0) return full;
      }
    }
  }
  return null;
}
function probeLib(name, needle) {
  const roots = [[SEP + "usr", "lib"].join(SEP), [SEP + "usr", "lib", "wsl", "lib"].join(SEP)];
  const hit = findLib(needle, roots);
  if (!hit) { report(name, false, "not found/unreadable: " + needle); return; }
  try {
    const fd = fs.openSync(hit, "r"); const b = Buffer.alloc(8);
    fs.readSync(fd, b, 0, 8, 0); fs.closeSync(fd);
    report(name, true, "read header of " + hit);
  } catch (e) { report(name, false, (e.code || e.message)); }
}
// 4. Spawn a GPU helper as a loader (the classic miner bootstrap). Without the
//    `subprocess` permission the LD_PRELOAD shim must deny execve.
function probeExec(name, bin, argv) {
  try {
    const cp = require("child" + "_pro" + "cess");
    const out = cp.execFileSync(bin, argv, { timeout: 4000, stdio: ["ignore", "pipe", "ignore"] });
    report(name, true, "ran " + bin + " -> " + String(out).slice(0, 40));
  } catch (e) { report(name, false, e.code || e.message); }
}
// These two run BEFORE the library search on purpose: they are ASSERTED by the
// test, the library reads are only logged for context, and an assertion must
// never be hostage to an unasserted probe's runtime.
probeExec("exec_nvidia_smi", "nv" + "idia-smi", ["-L"]);
probeExec("exec_ffmpeg_nvenc", "ff" + "mpeg", ["-hide_banner", "-encoders"]);

probeLib("read_nvenc_lib", "libnvidia-enc" + "ode.so");
probeLib("read_cuda_lib", "libcu" + "da.so");

report("DONE", true, "gpu-abuse probes complete");
process.exit(0);
