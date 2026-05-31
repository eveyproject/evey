// LEGIT GPU example — scene/object tagger.
//
// This is how a well-behaved AI/ML plugin uses the hardware.gpu permission: it
// opens the render node it was granted, runs a SHORT, BURSTY inference pass, and
// stops. It never touches the card node, never tries to widen the grant, never
// spawns a helper, and never pegs the GPU engine continuously — so the resource
// monitor leaves it alone. A real version would load a model (ONNX/TensorRT/
// OpenVINO) on the render node and tag frames; here we open the node and do a
// brief, terminating workload to model that shape.
const fs = require("fs");

function log(name, ok, detail) {
  console.error("[PLUGIN] " + name + ": " + (ok ? "OK" : "ERR") + " | " + String(detail).slice(0, 120));
}

// The one node we were granted (render node only — by design).
const render = "/dev/dri/renderD128";

let fd = -1;
try {
  fd = fs.openSync(render, "r");
  log("open_render", true, "granted render node ready for inference: " + render);
} catch (e) {
  // No GPU on this host (or permission not granted) — a good plugin degrades to
  // CPU instead of failing. Demonstrates graceful fallback.
  log("open_render", false, (e.code || e.message) + " — falling back to CPU inference");
}

// A real model would run on the GPU here. We simulate a BURSTY, TERMINATING
// workload (~1.5s, well under the monitor's ~30s sustained-peg threshold) — the
// shape of legit inference, not a miner's endless loop.
const tEnd = Date.now() + 1500;
let frames = 0;
while (Date.now() < tEnd) { frames++; }
log("inference_burst", true, "tagged ~" + frames + " synthetic frames then released the GPU");

if (fd >= 0) {
  try { fs.closeSync(fd); } catch (e) {}
}
log("done", true, "scene-tagger finished and released GPU resources");
process.exit(0);
