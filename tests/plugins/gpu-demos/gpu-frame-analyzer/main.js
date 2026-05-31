// LEGIT GPU example — render-node capability reporter.
//
// The smallest honest use of hardware.gpu: open the granted render node and
// report that the GPU is reachable, for an admin "GPU diagnostics" panel. It
// stays strictly within the grant (render node only), does no sustained compute,
// and exits immediately. A good citizen the monitor never flags.
const fs = require("fs");

function log(name, ok, detail) {
  console.error("[PLUGIN] " + name + ": " + (ok ? "OK" : "ERR") + " | " + String(detail).slice(0, 120));
}

const render = "/dev/dri/renderD128";

try {
  const fd = fs.openSync(render, "r");
  // A real version would query DRM caps via ioctl; opening the fd is enough to
  // confirm the grant works. Read-only intent, immediate release.
  fs.closeSync(fd);
  log("gpu_reachable", true, "render node opened + released: GPU available for plugins");
} catch (e) {
  log("gpu_reachable", false, (e.code || e.message) + " — report 'no GPU available' to the admin panel");
}

log("done", true, "frame-analyzer reported GPU status");
process.exit(0);
