// Weather plugin v2 - backend.
//
// Two jobs:
//   1. gRPC lifecycle stub so the host keeps the plugin "running" and the
//      frontend slot active.
//   2. Sun-image updater: fetches the current NASA/SDO full-disc image for the
//      day's rotation channel and writes it into web_root (dist/) so the
//      frontend can display the LIVE sun, not just the bundled snapshot.
//      Runs server-side, so NASA's lack of CORS headers is irrelevant here;
//      the frontend then loads the file same-origin via the host asset route.

const grpc = require("@grpc/grpc-js");
const protoLoader = require("@grpc/proto-loader");
const path = require("path");
const fs = require("fs");
const https = require("https");

// ---- Sun image updater ----------------------------------------------------

// Must stay in sync with src/domain/services/sunModel.ts (same ids + order).
const SUN_MODELS = [
  { id: "aia171", code: "0171" },
  { id: "aia193", code: "0193" },
];

const SDO_HOST = "sdo.gsfc.nasa.gov";
const MAX_IMAGE_BYTES = 3 * 1024 * 1024; // sanity cap; latest_1024 is ~200 KB
const REFRESH_MS = 3 * 60 * 60 * 1000;   // re-fetch every 3h (catches the day flip + fresher "latest")
const DIST_DIR = path.join(__dirname, "dist");

function modelForToday() {
  const dayIndex = Math.floor(Date.now() / 86_400_000);
  const i = ((dayIndex % SUN_MODELS.length) + SUN_MODELS.length) % SUN_MODELS.length;
  return SUN_MODELS[i];
}

// GET an https URL into a Buffer, following one redirect, capped in size.
function fetchBuffer(url, redirectsLeft = 1) {
  return new Promise((resolve, reject) => {
    const req = https.get(url, { timeout: 15_000 }, (res) => {
      const { statusCode, headers } = res;
      if (statusCode >= 300 && statusCode < 400 && headers.location && redirectsLeft > 0) {
        res.resume();
        const next = new URL(headers.location, url).toString();
        // Only follow redirects that stay on the whitelisted host.
        if (new URL(next).hostname !== SDO_HOST) {
          reject(new Error("redirect off whitelisted host"));
          return;
        }
        resolve(fetchBuffer(next, redirectsLeft - 1));
        return;
      }
      if (statusCode !== 200) {
        res.resume();
        reject(new Error("HTTP " + statusCode));
        return;
      }
      const chunks = [];
      let total = 0;
      res.on("data", (c) => {
        total += c.length;
        if (total > MAX_IMAGE_BYTES) {
          req.destroy();
          reject(new Error("image too large"));
          return;
        }
        chunks.push(c);
      });
      res.on("end", () => resolve(Buffer.concat(chunks)));
    });
    req.on("timeout", () => req.destroy(new Error("timeout")));
    req.on("error", reject);
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
    // half-written file.
    const finalPath = path.join(DIST_DIR, `sun-live-${model.id}.jpg`);
    const tmpPath = path.join(DIST_DIR, `.sun-live-${model.id}.jpg.tmp`);
    fs.writeFileSync(tmpPath, buf);
    fs.renameSync(tmpPath, finalPath);
    console.error(`weather-v2: sun image updated (${model.id}, ${buf.length} bytes)`);
  } catch (e) {
    // Non-fatal: the frontend falls back to the bundled snapshot.
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
