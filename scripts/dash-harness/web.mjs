// The Evey web player's own journey with the dev DASH flag on (devDash.ts):
// log in through the form, open /player/:id, and check what the page did -
// which manifest it loaded (manifest.mpd, or master.m3u8 when the server
// offers no DASH), start, seek, a quality and an audio switch through the
// player's settings menu, resume from the saved progress, end of stream.
// Headless, muted Google Chrome (EVEY_CHROME), as run.mjs.
//
//   EVEY_CHROME=... EVEY_USER=... EVEY_PASS=... node scripts/dash-harness/web.mjs \
//     <mediaId>[:hls] ...     (":hls" = a title the server serves as HLS only)
//
// EVEY_WEB defaults to http://localhost:5173, EVEY_URL (the API, for the
// watched-state reset) to http://localhost:9091. Exit code 1 when a check fails.
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const require = createRequire(join(here, '../../tests/e2e/package.json'));
const { chromium } = require('playwright');

const web = process.env.EVEY_WEB || 'http://localhost:5173';
const api = process.env.EVEY_URL || 'http://localhost:9091';
const apiToken = (await (await fetch(`${api}/api/v1/auth/login`, {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: process.env.EVEY_USER, password: process.env.EVEY_PASS }),
})).json()).accessToken;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
let failed = 0;
const check = (media, name, ok, detail) => {
  if (!ok) failed++;
  console.log(`${ok ? 'PASS' : 'FAIL'} ${media} web ${name} ${detail ? JSON.stringify(detail) : ''}`);
};

const browser = await chromium.launch({ executablePath: process.env.EVEY_CHROME, headless: true, args: ['--mute-audio', '--autoplay-policy=no-user-gesture-required'] });
const ctx = await browser.newContext({ baseURL: web, locale: 'en-US' });
const page = await ctx.newPage();
await page.goto('/login');
await page.fill('#username', process.env.EVEY_USER);
await page.fill('#password', process.env.EVEY_PASS);
await page.click('button[type="submit"]');
await page.waitForURL('**/home', { timeout: 20000 });
await page.evaluate(() => localStorage.setItem('evey.dev.dash', '1'));

let endedAt = null;
await page.exposeFunction('__eveyEnded', (t) => { endedAt = t; });
const video = () => page.locator('video').first();
const state = () => video().evaluate((v) => ({ t: +v.currentTime.toFixed(2), h: v.videoHeight, paused: v.paused, ended: v.ended, d: v.duration }));
async function until(cond, ms) {
  const end = Date.now() + ms;
  let s;
  while (Date.now() < end) {
    s = await state().catch(() => null);
    if (s && cond(s)) return s;
    await sleep(300);
  }
  return s;
}
async function openSettings() {
  await page.mouse.move(400, 300);
  await page.mouse.move(420, 320);
  // The controls auto-hide: the button stays in the DOM, faded out.
  await page.locator('[data-testid="player-settings"]').first().click({ timeout: 5000, force: true });
  await sleep(300);
}

for (const target of process.argv.slice(2)) {
  const [media, mode] = target.split(':');
  const expectDash = mode !== 'hls';
  const seen = { mpd: [], master: 0, audioSeg: new Set() };
  const onResp = (r) => {
    const u = r.url();
    if (u.includes('/manifest.mpd')) seen.mpd.push(r.status());
    if (u.includes('/master.m3u8')) seen.master++;
    const m = u.match(/\/audio_(\d+)_\d+\.m4s/);
    if (m) seen.audioSeg.add(m[1]);
  };
  page.on('response', onResp);
  // A title a previous run watched to its end resumes at 0 (played): start clean.
  await fetch(`${api}/api/v1/playback/${media}/unwatched`, { method: 'POST', headers: { Authorization: `Bearer ${apiToken}` } });
  await page.goto(`/player/${media}?restart=1`);
  await video().waitFor({ state: 'attached', timeout: 30000 });
  await page.locator('body').click({ position: { x: 400, y: 300 } }).catch(() => {});
  await video().evaluate((v) => { v.muted = true; void v.play().catch(() => {}); });
  let s = await until((x) => x.t >= 4, 40000);
  check(media, 'start plays', s && s.t >= 4, s);
  const durationSec = s ? s.d : 0;
  if (expectDash) {
    check(media, 'played as DASH', seen.mpd.includes(200) && seen.master === 0, { mpd: seen.mpd, master: seen.master });
  } else {
    check(media, 'fell back to HLS', seen.master > 0, { mpd: seen.mpd, master: seen.master });
  }

  await video().evaluate((v) => { v.currentTime = 60; });
  s = await until((x) => x.t >= 63, 40000);
  check(media, 'seek 60 plays', s && s.t >= 63, s);

  // Quality through the settings menu: the lowest rung, then back up.
  try {
    await openSettings();
    const rungs = await page.locator('button', { hasText: /^\s*\d{3,4}p\s*$/ }).allInnerTexts();
    const heights = rungs.map((r) => parseInt(r, 10)).filter((h) => h > 0).sort((a, b) => a - b);
    if (heights.length > 1) {
      await page.locator('button', { hasText: new RegExp(`^\\s*${heights[0]}p\\s*$`) }).first().click();
      s = await until((x) => x.h === heights[0], 30000);
      check(media, `quality ${heights[0]}p`, s && s.h === heights[0], s);
      await openSettings();
      await page.locator('button', { hasText: new RegExp(`^\\s*${heights[heights.length - 1]}p\\s*$`) }).first().click();
      s = await until((x) => x.h === heights[heights.length - 1], 30000);
      check(media, `quality ${heights[heights.length - 1]}p`, s && s.h === heights[heights.length - 1], s);
    } else {
      check(media, 'quality', true, { skipped: `rungs ${rungs}` });
    }
  } catch (e) {
    check(media, 'quality menu', false, String(e).slice(0, 200));
  }
  await page.keyboard.press('Escape').catch(() => {});

  // Audio through the settings menu: the second track, then its segments.
  try {
    await openSettings();
    const audioRow = page.locator('[data-testid="player-settings-audio"]').first();
    if (await audioRow.count()) {
      await audioRow.click();
      await sleep(300);
      const options = page.locator('[role="dialog"] button, button').filter({ hasText: /fra|fran/i });
      if (await options.count()) {
        const before = new Set(seen.audioSeg);
        await options.first().click();
        const from = (await state()).t;
        s = await until((x) => x.t >= from + 5, 30000);
        const fresh = [...seen.audioSeg].filter((a) => !before.has(a));
        check(media, 'audio switch plays the other rendition', s && s.t >= from + 5 && (fresh.length > 0 || seen.audioSeg.size > 1), { t: s && s.t, audioRenditionsFetched: [...seen.audioSeg] });
      } else {
        check(media, 'audio switch', true, { skipped: 'one track' });
      }
    } else {
      check(media, 'audio switch', true, { skipped: 'no audio menu' });
    }
  } catch (e) {
    check(media, 'audio menu', false, String(e).slice(0, 200));
  }
  await page.keyboard.press('Escape').catch(() => {});

  // Resume: leave the player past 60 s, come back through its own progress -
  // kept only for a title over the server's 5 min floor (no progress row below).
  s = await until((x) => x.t >= 66, 20000);
  const resumable = durationSec >= 300;
  await page.goto('/home');
  await sleep(1500);
  seen.mpd.length = 0;
  await page.goto(`/player/${media}`);
  await video().waitFor({ state: 'attached', timeout: 30000 });
  await video().evaluate((v) => { v.muted = true; void v.play().catch(() => {}); });
  // The first position the element reaches: the resume point, not a replay.
  s = await until((x) => x.t > 0.5, 40000);
  if (resumable) check(media, 'resume from the saved progress', s && s.t >= 55, s);
  else check(media, 'resume', true, { skipped: 'under the 5 min progress floor' });

  // End of stream.
  // The element's own ended event (the app may leave the player on it).
  await until((x) => Number.isFinite(x.d) && x.d > 0, 30000);
  endedAt = null;
  await video().evaluate((v) => {
    v.addEventListener('ended', () => window.__eveyEnded(v.currentTime), { once: true });
    v.currentTime = Math.max(0, v.duration - 6);
  });
  for (let i = 0; i < 150 && endedAt === null; i++) await sleep(300);
  check(media, 'plays to the end (ended)', endedAt !== null, { endedAt });
  page.off('response', onResp);
}

await browser.close();
console.log(JSON.stringify({ failed }));
process.exit(failed ? 1 : 0);
