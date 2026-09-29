// directplay-harness: every title the server decides to DIRECT PLAY is played
// for real, muted, in each browser engine, and must advance.
//
// Why: the gates compare the probe with ffprobe and the master with the bytes
// served, but nothing checked the last link - that a file the decision hands
// over raw actually decodes in the engine it was decided for. Elephants Dream
// (VP9 Profile 3, 4:4:4 10-bit) was announced DIRECT_PLAY to browsers that
// only decode profiles 0 and 2 (2026-09-29); this run would have failed on it.
//
// For each engine: the page computes its OWN capabilities the way the web
// client does (mirror of frontend/packages/client/src/web/codec.ts and
// hlsCodecParams.ts - keep them in step), stream-info is asked with them, and
// each DIRECT_PLAY title is loaded raw in a <video> and played SECONDS long.
// Pass: no media error, readyState >= 2, and playback advanced at least 60%
// of the wall time. Titles decided otherwise are listed, not played (the HLS
// paths have their own harnesses). Exit 1 on any failure.
//
//   EVEY_USER=demo EVEY_PASS=demo node scripts/directplay-harness/run.mjs
//
// EVEY_URL (default http://localhost:9091), SECONDS (default 6), ENGINES
// (default "chromium,firefox"; Playwright's builds, no H.264/AAC in either),
// EVEY_CHROME=/path/to/google-chrome adds a real Chrome ("chrome") that has
// H.264/AAC. Dev tool, not part of the CI gate (no browser there).
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const require = createRequire(join(here, '../../tests/e2e/package.json'));
const pw = require('playwright');

const base = process.env.EVEY_URL || 'http://localhost:9091';
const seconds = Number(process.env.SECONDS || 6);
const engines = (process.env.ENGINES || 'chromium,firefox').split(',').map((s) => s.trim()).filter(Boolean);
if (process.env.EVEY_CHROME && !engines.includes('chrome')) engines.push('chrome');

async function api(path, token, init = {}) {
  const r = await fetch(`${base}${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}), ...(init.headers || {}) },
  });
  if (!r.ok) throw new Error(`${init.method || 'GET'} ${path}: ${r.status} ${(await r.text()).slice(0, 120)}`);
  return r.json();
}

const login = await api('/api/v1/auth/login', null, {
  method: 'POST',
  body: JSON.stringify({ username: process.env.EVEY_USER, password: process.env.EVEY_PASS }),
});
const token = login.accessToken;
if (!token) throw new Error('login failed');

// Every playable video title: films, and the episodes of the series libraries.
const { libraries = [] } = await api('/api/v1/libraries', token);
const titles = [];
for (const lib of libraries) {
  const types = { LIBRARY_TYPE_MOVIE: ['MEDIA_ITEM_TYPE_MOVIE'], LIBRARY_TYPE_TVSHOW: ['MEDIA_ITEM_TYPE_EPISODE'] }[lib.type]
    ?? (/movie/i.test(lib.type || '') ? ['MEDIA_ITEM_TYPE_MOVIE'] : /tv|show|series/i.test(lib.type || '') ? ['MEDIA_ITEM_TYPE_EPISODE'] : []);
  for (const type of types) {
    const q = new URLSearchParams({ libraryId: lib.id, type, 'pagination.pageSize': '500' });
    const { items = [] } = await api(`/api/v1/media?${q}`, token);
    for (const it of items) titles.push({ id: it.id, title: it.title, library: lib.name });
  }
}
if (titles.length === 0) throw new Error('no video title visible to this account');

// Mirror of getBrowserCapabilities + canWasmDecodeAC3 + getFmp4AudioPassthrough.
function pageCapabilities() {
  const v = document.createElement('video');
  const canPlay = (t) => v.canPlayType(t) !== '';
  const MS = window.MediaSource;
  const mse = (t) => !!MS && typeof MS.isTypeSupported === 'function' && MS.isTypeSupported(t);
  const videoCodecs = [];
  if (canPlay('video/mp4; codecs="avc1.42E01E"')) videoCodecs.push('h264');
  if (mse('video/mp4; codecs="hvc1.1.6.L93.B0"') || mse('video/mp4; codecs="hev1.1.6.L93.B0"')) videoCodecs.push('hevc');
  if (canPlay('video/webm; codecs="vp9"')) videoCodecs.push('vp9');
  if (canPlay('video/mp4; codecs="av01.0.00M.08"')) videoCodecs.push('av1');
  const audioCodecs = ['aac', 'mp3', 'opus'];
  if (canPlay('audio/mp4; codecs="ac-3"') || canPlay('video/mp4; codecs="ac-3"')) audioCodecs.push('ac3');
  if (canPlay('audio/mp4; codecs="ec-3"') || canPlay('video/mp4; codecs="ec-3"')) audioCodecs.push('eac3');
  if (canPlay('audio/flac')) audioCodecs.push('flac');
  const containers = [];
  if (canPlay('video/mp4')) containers.push('mp4');
  if (canPlay('video/webm')) containers.push('webm');
  if (canPlay('video/ogg')) containers.push('ogg');
  const nativeAC3 = audioCodecs.includes('ac3') || audioCodecs.includes('eac3');
  const passthrough = [];
  if (mse('audio/mp4; codecs="mp4a.6B"')) passthrough.push('mp3');
  if (mse('audio/mp4; codecs="opus"')) passthrough.push('opus');
  return { videoCodecs, audioCodecs, containers, wasmAudio: typeof WebAssembly !== 'undefined' && !nativeAC3, passthrough };
}

// Plays one raw URL muted for `secs` and reports what the element did.
async function playRaw({ url, secs }) {
  const v = document.createElement('video');
  v.muted = true;
  v.preload = 'auto';
  document.body.appendChild(v);
  let error = '';
  v.addEventListener('error', () => { error = v.error ? `${v.error.code}:${v.error.message}` : 'error'; });
  v.src = url;
  try { await v.play(); } catch (e) { error = error || `play(): ${e.name}`; }
  const t0 = performance.now();
  const start = v.currentTime;
  await new Promise((r) => setTimeout(r, secs * 1000));
  const wall = (performance.now() - t0) / 1000;
  const out = { advanced: +(v.currentTime - start).toFixed(2), wall: +wall.toFixed(2), readyState: v.readyState, error };
  v.pause();
  v.removeAttribute('src');
  v.load();
  v.remove();
  return out;
}

const launchers = {
  chromium: () => pw.chromium.launch({ headless: true, args: ['--mute-audio', '--autoplay-policy=no-user-gesture-required'] }),
  chrome: () => pw.chromium.launch({ executablePath: process.env.EVEY_CHROME, headless: true, args: ['--mute-audio', '--autoplay-policy=no-user-gesture-required'] }),
  firefox: () => pw.firefox.launch({ headless: true, firefoxUserPrefs: { 'media.autoplay.default': 0, 'media.autoplay.blocking_policy': 0, 'media.volume_scale': '0.0' } }),
};

let failures = 0;
for (const engine of engines) {
  const launch = launchers[engine];
  if (!launch) throw new Error(`unknown engine ${engine}`);
  const browser = await launch();
  const page = await browser.newPage();
  await page.goto('about:blank');
  const caps = await page.evaluate(pageCapabilities);
  console.log(`\n== ${engine}  video=${caps.videoCodecs} audio=${caps.audioCodecs} containers=${caps.containers} wasm=${caps.wasmAudio} passthrough=${caps.passthrough}`);
  const params = new URLSearchParams();
  for (const c of caps.videoCodecs) params.append('supportedCodecs', c);
  for (const c of caps.audioCodecs) params.append('supportedAudioCodecs', c);
  for (const c of caps.containers) params.append('supportedContainers', c);
  for (const c of caps.passthrough) params.append('fmp4AudioPassthrough', c);
  params.set('supportsHdr', 'false');
  params.set('wasmAudio', String(caps.wasmAudio));

  for (const t of titles) {
    const info = await api(`/api/v1/playback/${t.id}/stream-info?${params}`, token);
    const method = String(info.playMethod || '').replace('PLAY_METHOD_', '');
    if (method !== 'DIRECT_PLAY') {
      console.log(`  skip  ${method.padEnd(13)} ${t.title}`);
      continue;
    }
    // A fresh stream token per title: they expire in minutes, a long run outlives one.
    const st = await api(`/api/v1/media/${t.id}/stream-token`, token, { method: 'POST', body: '{}' });
    const raw = info.directPlayUrl || `/api/v1/media/${t.id}/stream`;
    const url = `${base}${raw}${raw.includes('?') ? '&' : '?'}token=${encodeURIComponent(st.token)}`;
    const r = await page.evaluate(playRaw, { url, secs: seconds }).catch((e) => ({ error: String(e), advanced: 0, wall: seconds, readyState: 0 }));
    const ok = !r.error && r.readyState >= 2 && r.advanced >= 0.6 * r.wall;
    if (!ok) failures++;
    console.log(`  ${ok ? 'PASS' : 'FAIL'}  DIRECT_PLAY   ${t.title}  advanced=${r.advanced}s/${r.wall}s rs=${r.readyState}${r.error ? ` error=${r.error}` : ''}`);
  }
  await browser.close();
}

console.log(failures === 0 ? '\ndirect play: every title the server hands over raw plays in every engine' : `\ndirect play: ${failures} FAILURE(S)`);
process.exit(failures === 0 ? 0 : 1);
