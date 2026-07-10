// Weather plugin v2 - backend.
//
// Two jobs:
//   1. gRPC lifecycle stub so the host keeps the plugin "running" and the
//      frontend slot active.
//   2. Sun-image updater: fetches the current NASA/SDO full-disc image for the
//      day's channel and writes it into web_root (dist/) so the frontend can
//      display the LIVE sun, not just the bundled snapshot. Runs server-side,
//      so NASA's lack of CORS headers is irrelevant here; the frontend then
//      loads the file same-origin via the host asset route.
//
// The live asset (dist/sun-live-aia171.jpg) is SEEDED at build time with the
// bundled snapshot, so the frontend's asset request always resolves (200) -
// there is no 404 in the browser console, even on the very first load before
// any fetch. On a failed fetch the previous good image is kept (the filename is
// stable, so yesterday's live image is naturally reused).

const grpc = require("@grpc/grpc-js");
const protoLoader = require("@grpc/proto-loader");
const path = require("path");
const fs = require("fs");

// ---- Sun image updater ----------------------------------------------------

// Must stay in sync with src/domain/services/sunModel.ts (same ids). The plugin
// ships a single clean AIA 171 skin: AIA 193 was dropped because its large dark
// coronal holes read as a "moth-eaten" black blotch on the disc.
const SUN_MODELS = [
  { id: "aia171", code: "0171" },
];

const SDO_HOST = "sdo.gsfc.nasa.gov";
const REFRESH_MS = 3 * 60 * 60 * 1000;   // re-fetch every 3h (catches the day flip + fresher "latest")
const DIST_DIR = path.join(__dirname, "dist");

function modelForToday() {
  const dayIndex = Math.floor(Date.now() / 86_400_000);
  const i = ((dayIndex % SUN_MODELS.length) + SUN_MODELS.length) % SUN_MODELS.length;
  return SUN_MODELS[i];
}

// GET an https URL into a Buffer through the host Fetch broker. The plugin has
// no direct network access (the sandbox unshares the net namespace); the host
// enforces the external_fetch allow-list, an SSRF-safe dialer, redirect
// re-validation and a size cap, then hands back the body. `host` is assigned
// below once the proto is loaded, before any RPC (updateSunImage) fires.
function fetchBuffer(url) {
  return new Promise((resolve, reject) => {
    host.Fetch({ url, method: "GET" }, (err, res) => {
      if (err) {
        reject(err);
        return;
      }
      const status = Number(res.status_code);
      if (status !== 200) {
        reject(new Error("HTTP " + status));
        return;
      }
      // res.body is a Buffer (proto bytes). The host already caps size; treat a
      // truncated body as a failure so a half-image never gets written.
      if (res.truncated) {
        reject(new Error("image too large"));
        return;
      }
      resolve(Buffer.isBuffer(res.body) ? res.body : Buffer.from(res.body || []));
    });
  });
}

async function updateSunImage() {
  const model = modelForToday();
  const url = `https://${SDO_HOST}/assets/img/latest/latest_1024_${model.code}.jpg`;
  try {
    const buf = await fetchBuffer(url);
    // Cheap JPEG sanity check (SOI marker) so a captive-portal HTML page or an
    // error body never gets written as sun-live.
    if (buf.length < 1024 || buf[0] !== 0xff || buf[1] !== 0xd8) {
      throw new Error("not a JPEG");
    }
    fs.mkdirSync(DIST_DIR, { recursive: true });
    // Atomic write: temp then rename, so the asset route never serves a
    // half-written file. On any failure the previous good image is left in
    // place (stable filename → yesterday's live image is reused).
    const finalPath = path.join(DIST_DIR, `sun-live-${model.id}.jpg`);
    const tmpPath = path.join(DIST_DIR, `.sun-live-${model.id}.jpg.tmp`);
    fs.writeFileSync(tmpPath, buf);
    fs.renameSync(tmpPath, finalPath);
    console.error(`weather-v2: sun image updated (${model.id}, ${buf.length} bytes)`);
  } catch (e) {
    // Non-fatal: the frontend falls back to the seeded/previous snapshot.
    console.error(`weather-v2: sun image update failed (${model.id}): ${e.message}`);
  }
}

let sunTimer = null;
function startSunUpdater() {
  void updateSunImage();
  sunTimer = setInterval(() => void updateSunImage(), REFRESH_MS);
  if (sunTimer.unref) sunTimer.unref(); // don't keep the process alive just for this
}

// ---- gRPC lifecycle stub --------------------------------------------------

const PROTO_PATH = path.join(__dirname, "plugin_api.proto");
const pkgDef = protoLoader.loadSync(PROTO_PATH, { keepCase: true });
const proto = grpc.loadPackageDefinition(pkgDef).plugin.v1;

// HostService client (plugin -> host) for the brokered Fetch. SO_PEERCRED on the
// unix socket authenticates the plugin process; no token needed.
const HOST_SOCK = process.env.EVEY_HOST_SOCK || "/tmp/evey/plugins/evey-host.sock";
const host = new proto.HostService(`unix://${HOST_SOCK}`, grpc.credentials.createInsecure());

const PLUGIN_SOCK = process.env.EVEY_PLUGIN_SOCK;
if (!PLUGIN_SOCK) {
  console.error("EVEY_PLUGIN_SOCK not set");
  process.exit(1);
}

const server = new grpc.Server();
server.addService(proto.PluginService.service, {
  Initialize(_call, cb) { startSunUpdater(); cb(null, { success: true }); },
  Shutdown(_call, cb) { if (sunTimer) clearInterval(sunTimer); cb(null, {}); setTimeout(() => process.exit(0), 200); },
  OnEvent(_call, cb) { cb(null, { handled: false }); },
  HealthCheck(_call, cb) { cb(null, { healthy: true, status: "ok" }); },
  GetCapabilities(_call, cb) { cb(null, { api_version: "1", capabilities: [] }); },
});

server.bindAsync(`unix://${PLUGIN_SOCK}`, grpc.ServerCredentials.createInsecure(), (err) => {
  if (err) { console.error("bind failed:", err); process.exit(1); }
  console.error(`weather-v2 listening on ${PLUGIN_SOCK}`);
});
