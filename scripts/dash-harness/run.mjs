// DASH harness driver: plays Evey's manifest.mpd in a REAL player (Shaka and
// dash.js), and its master.m3u8 in hls.js, inside a headless, muted Chrome,
// and checks what the browser did: cold start, seek, rung switch, audio
// switch, end of stream, resume. The arbiter is MSE
// (the SourceBuffers' buffered ranges and the playhead), not the manifest.
//
// Chrome, not Playwright's Chromium: the latter has no H.264/AAC.
//
//   EVEY_CHROME=/path/to/google-chrome EVEY_USER=... EVEY_PASS=... \
//     node scripts/dash-harness/run.mjs <mediaId>[:<query>] ...
//
// <query> is added to the manifest request, e.g. vd=transcode forces the
// ffmpeg ladder on a copy-eligible title. EVEY_URL defaults to
// http://localhost:9091. Exit code 1 when any check fails.
import { createServer } from 'node:http';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const require = createRequire(join(here, '../../tests/e2e/package.json'));
const { chromium } = require('playwright');

const base = process.env.EVEY_URL || 'http://localhost:9091';
const chrome = process.env.EVEY_CHROME;
if (!chrome) throw new Error('EVEY_CHROME must point at a Google Chrome binary (H.264/AAC)');
const players = (process.env.DASH_PLAYERS || 'shaka,dashjs,hlsjs').split(',');
const targets = process.argv.slice(2);
if (targets.length === 0) throw new Error('usage: run.mjs <mediaId>[:<query>] ...');

const login = await fetch(`${base}/api/v1/auth/login`, {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: process.env.EVEY_USER, password: process.env.EVEY_PASS }),
});
const token = (await login.json()).accessToken;
if (!token) throw new Error('login failed');

const html = readFileSync(join(here, 'harness.html'));
const server = createServer((_, res) => { res.setHeader('Content-Type', 'text/html'); res.end(html); });
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const page0 = `http://127.0.0.1:${server.address().port}/`;

const browser = await chromium.launch({ executablePath: chrome, headless: true, args: ['--mute-audio', '--autoplay-policy=no-user-gesture-required'] });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
let failed = 0;
const report = [];

function check(ctx, name, ok, detail) {
  if (!ok) failed++;
  report.push({ ...ctx, check: name, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'} ${ctx.media} ${ctx.player} ${name} ${detail ? JSON.stringify(detail) : ''}`);
}

async function until(page, cond, timeoutMs) {
  const end = Date.now() + timeoutMs;
  let s;
  while (Date.now() < end) {
    s = await page.evaluate(() => window.H.stats());
    if (cond(s)) return s;
    await sleep(250);
  }
  return s;
}

// The first buffered start of the video and audio SourceBuffers: where MSE
// placed each track's first frame.
function starts(s) {
  const pick = (re) => s.buffers.filter((b) => re.test(b.mime)).map((b) => b.buffered[0] && b.buffered[0][0]).find((x) => x !== undefined);
  return { video: pick(/^video/), audio: pick(/^audio/) };
}

async function open(player, url, startTime) {
  const page = await browser.newPage();
  page.on('pageerror', (e) => console.log('pageerror', e.message));
  page.on('crash', () => console.log('page crashed (renderer)'));
  if (process.env.DASH_DEBUG) page.on('console', (m) => console.log('console', m.type(), m.text().slice(0, 300)));
  await page.goto(`${page0}?player=${player}`);
  await page.waitForFunction(() => window.__ready === true);
  await page.evaluate(([u, t]) => window.H.load(u, t), [url, startTime]);
  return page;
}

for (const target of targets) {
  const [media, extra] = target.split(':');
  const q = new URLSearchParams(extra || '');
  q.set('videoCodecs', q.get('videoCodecs') || 'h264');
  q.set('audioCodecs', q.get('audioCodecs') || 'aac');
  q.set('containers', q.get('containers') || 'mp4');
  q.set('token', token);
  const mpdURL = `${base}/api/v1/media/${media}/hls/manifest.mpd?${q}`;
  const hlsURL = `${base}/api/v1/media/${media}/hls/master.m3u8?${q}`;
  const mpd = await fetch(mpdURL);
  const dash = mpd.status === 200;
  if (!dash) {
    console.log(`INFO ${media} manifest.mpd ${mpd.status}: ${(await mpd.text()).slice(0, 120)} - HLS only`);
  }
  for (const player of players) {
    const ctx = { media, player };
    if (player !== 'hlsjs' && !dash) continue;
    const url = player === 'hlsjs' ? hlsURL : mpdURL;
    const page = await open(player, url, 0);
    try {
      // Cold start: the playhead moves, both tracks buffered from the start.
      let s = await until(page, (x) => x.t >= 4 || x.errors.length > 0, 30000);
      check(ctx, 'cold start plays', s.t >= 4 && s.errors.length === 0, { t: s.t, errors: s.errors });
      const st = starts(s);
      check(ctx, 'first frames placed at 0', st.video !== undefined && Math.abs(st.video) < 0.05 && (st.audio === undefined || Math.abs(st.audio) < 0.05), st);

      // Seek far ahead: plays from there, video and audio buffered at the target.
      await page.evaluate(() => window.H.seek(60));
      s = await until(page, (x) => x.t >= 63 || x.errors.length > 0, 30000);
      check(ctx, 'seek 60 plays', s.t >= 63 && s.errors.length === 0, { t: s.t, errors: s.errors });
      const around = s.buffers.map((b) => ({ mime: b.mime, holds60: b.buffered.some(([a, e]) => a <= 60.1 && e >= 62) }));
      check(ctx, 'seek 60 buffered on every track', around.every((b) => b.holds60), around);

      // Rung switch, when the ladder has more than one.
      const heights = await page.evaluate(() => window.H.videoHeights());
      if (heights.length > 1) {
        for (const h of [heights[0], heights[heights.length - 1]]) {
          await page.evaluate((x) => window.H.selectHeight(x), h);
          const from = (await page.evaluate(() => window.H.stats())).t;
          s = await until(page, (x) => (x.active && x.active.height === h && x.t >= from + 6) || x.errors.length > 0, 40000);
          check(ctx, `switch to ${h}p plays`, s.active && s.active.height === h && s.t >= from + 6 && s.errors.length === 0, { t: s.t, active: s.active, errors: s.errors, decoded: s.decoded, dropped: s.dropped });
        }
      } else {
        check(ctx, 'rung switch', true, { skipped: `one rung (${heights})` });
      }

      // Audio switch, when there is more than one rendition.
      const langs = await page.evaluate(() => window.H.audioLangs());
      if (langs.length > 1) {
        const other = langs[langs.length - 1];
        await page.evaluate((l) => window.H.selectAudio(l), other);
        const from = (await page.evaluate(() => window.H.stats())).t;
        s = await until(page, (x) => (x.active && x.active.audio === other && x.t >= from + 5) || x.errors.length > 0, 30000);
        check(ctx, `audio switch to ${other} plays`, s.active && s.active.audio === other && s.t >= from + 5 && s.errors.length === 0, { t: s.t, active: s.active, errors: s.errors });
      }
      s = await page.evaluate(() => window.H.stats());
      check(ctx, 'no dropped-frame storm', s.dropped <= Math.max(10, s.decoded / 20), { decoded: s.decoded, dropped: s.dropped });
    } finally {
      await page.close();
    }

    // End of stream: loaded 8 s before the end, the player must reach it.
    // A declared segment nobody produces leaves it waiting forever.
    const probe = await open(player, url, 0);
    const d = (await until(probe, (x) => x.duration > 0 || x.errors.length > 0, 30000)).duration;
    await probe.close();
    const eos = await open(player, url, Math.max(0, d - 8));
    try {
      const s = await until(eos, (x) => x.ended || x.errors.length > 0, 60000);
      check(ctx, 'plays to the end', s.ended && s.errors.length === 0, { t: s.t, duration: s.duration, errors: s.errors });
    } finally {
      await eos.close();
    }

    // Resume: a fresh load starting at 45 s.
    const resume = await open(player, url, 45);
    try {
      const s = await until(resume, (x) => x.t >= 48 || x.errors.length > 0, 30000);
      const st = starts(s);
      check(ctx, 'resume at 45 plays', s.t >= 48 && s.errors.length === 0, { t: s.t, starts: st, errors: s.errors });
    } finally {
      await resume.close();
    }
  }
}

await browser.close();
server.close();
console.log(JSON.stringify({ failed, checks: report.length }));
process.exit(failed ? 1 : 0);
