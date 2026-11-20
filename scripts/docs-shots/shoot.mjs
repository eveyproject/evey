// Takes the documentation screenshots from a running demo instance.
//
// Usage: node scripts/docs-shots/shoot.mjs <outdir> [name ...]
//   BASE (default http://localhost:9292), EVEY_USER / EVEY_PASS (demo / demo).
// Writes <outdir>/<name>.png, one per image of docs/images it knows; no names
// means all of them. Playwright comes from tests/e2e (npm ci there first).
import { createRequire } from "node:module";
import { mkdirSync } from "node:fs";

const require = createRequire(new URL("../../tests/e2e/package.json", import.meta.url));
const { chromium } = require("@playwright/test");

const BASE = process.env.BASE || "http://localhost:9292";
const USER = process.env.EVEY_USER || "demo";
const PASS = process.env.EVEY_PASS || "demo";
const [out, ...only] = process.argv.slice(2);
if (!out) {
  console.error("usage: shoot.mjs <outdir> [name ...]");
  process.exit(2);
}
mkdirSync(out, { recursive: true });
const wanted = (name) => only.length === 0 || only.includes(name);

const CONTEXT = { viewport: { width: 1600, height: 1000 }, deviceScaleFactor: 2, locale: "en-US", colorScheme: "dark" };

async function login(browser) {
  const ctx = await browser.newContext(CONTEXT);
  const page = await ctx.newPage();
  await page.goto(BASE + "/login");
  await page.fill("#username", USER);
  await page.fill("#password", PASS);
  await page.keyboard.press("Enter");
  await page.waitForURL((u) => !u.pathname.startsWith("/login"), { timeout: 30000 });
  await page.waitForTimeout(1500);
  return page;
}

// Content area of an admin page: right of the admin sidebar, below the top bar.
async function contentClip(page) {
  return page.evaluate(() => {
    const back = [...document.querySelectorAll("a,button")].find((e) => e.textContent.trim() === "Back to app");
    let n = back;
    while (n && n.getBoundingClientRect().height < window.innerHeight * 0.9) n = n.parentElement;
    const left = n ? n.getBoundingClientRect().right : 0;
    const top = document.querySelector("header")?.getBoundingClientRect().bottom ?? 0;
    return { x: left, y: top, width: window.innerWidth - left, height: window.innerHeight - top };
  });
}

// From the element whose text starts with `from` to the one containing `to`, full content width.
function between(from, to, pad = 16) {
  return async (page) => {
    const c = await contentClip(page);
    const r = await page.evaluate(([f, t]) => {
      // Leaves only: a parent card also "contains" the text and would widen the crop.
      const leaves = [...document.querySelectorAll("body *")].filter((e) => e.children.length === 0);
      const a = leaves.find((e) => e.textContent.trim().toLowerCase().startsWith(f.toLowerCase()));
      const b = leaves.find((e) => e.textContent.includes(t));
      if (!a || !b) return null;
      return { top: a.getBoundingClientRect().top, bottom: b.getBoundingClientRect().bottom };
    }, [from, to]);
    if (!r) throw new Error(`crop: "${from}" or "${to}" is no longer on the page`);
    return { x: c.x, y: r.top - pad, width: c.width, height: r.bottom - r.top + 2 * pad };
  };
}

// The page as the demo administrator sees it, before anything plays.
const PAGES = [
  { name: "home", path: "/home" },
  { name: "admin-dashboard", path: "/admin" },
  { name: "admin-libraries", path: "/admin/libraries", clip: contentClip },
  { name: "admin-users", path: "/admin/users", clip: contentClip },
  { name: "admin-remote-access", path: "/admin/remote-access", clip: contentClip },
  { name: "admin-transcoding-bitrate", path: "/admin/transcoding", clip: between("Video bitrate ceiling", "Video bitrate ceiling, remote viewers", 18) },
  { name: "admin-system-health", path: "/admin/system-health", clip: contentClip },
  { name: "admin-plugins", path: "/admin/plugins", clip: contentClip },
  { name: "admin-webhooks", path: "/admin/webhooks", clip: contentClip },
  {
    name: "admin-backup", path: "/admin/backup", clip: contentClip,
    setup: async (page) => {
      const box = page.getByLabel("Enabled");
      // The demo locks its settings: the page is shot as the demo shows it.
      if ((await box.isChecked()) || (await box.isDisabled())) return;
      await box.check();
      await page.getByRole("button", { name: /^Save$/ }).click();
      // Let the "Schedule saved" toast go away before the shot.
      await page.waitForTimeout(6000);
    },
  },
];

// The id of a film, from the server's list of films.
async function mediaId(page, title) {
  const id = await page.evaluate(async (t) => {
    const auth = JSON.parse(localStorage.getItem("evey-auth") || "{}");
    const res = await fetch("/api/v1/media?type=MEDIA_ITEM_TYPE_MOVIE&summary=true&pagination.page_size=200", {
      headers: { Authorization: "Bearer " + auth?.state?.accessToken },
    });
    if (!res.ok) return null;
    const hit = ((await res.json()).items || []).find((i) => i.title === t);
    return hit ? hit.id : null;
  }, title);
  if (!id) throw new Error(`no media found for "${title}"`);
  return id;
}

// A viewer: the admin connects as another account, takes its first profile
// and plays a film from `seek` seconds, muted.
async function watch(browser, account, id, seek) {
  const page = await login(browser);
  await page.goto(BASE + "/admin/users", { waitUntil: "networkidle" });
  await page.locator("tr", { hasText: account }).getByRole("button", { name: "Connect as" }).click();
  await page.waitForTimeout(2500);
  const profile = page.locator("[data-testid='profile-card'], button:has(img), [role='button']:has(img)").first();
  if (page.url().includes("profile") && (await profile.count())) {
    await profile.click();
    await page.waitForTimeout(1500);
  }
  await page.goto(BASE + "/player/" + id, { waitUntil: "load" });
  await page.waitForSelector("video", { timeout: 30000 });
  await page.evaluate(async (s) => {
    const el = document.querySelector("video");
    el.muted = true;
    try { await el.play(); } catch {}
    if (s) el.currentTime = s;
  }, seek);
  await page.waitForTimeout(8000);
  return page;
}

// Shows the player controls the way a viewer moving the mouse sees them.
async function wakeControls(page) {
  await page.mouse.move(800, 500);
  await page.waitForTimeout(300);
  await page.mouse.move(820, 900);
  await page.waitForTimeout(700);
}

async function shot(page, name, clip) {
  await page.screenshot({ path: `${out}/${name}.png`, clip });
  console.log("shot", name);
}

// A page that moved under a crop must not cost the other images: its failure
// is reported, the run goes on, and the exit code says so at the end.
const failed = [];
async function attempt(name, fn) {
  try {
    await fn();
  } catch (err) {
    failed.push(name);
    console.error(`FAILED ${name}: ${String(err.message || err).split("\n")[0]}`);
  }
}

// The demo is photographed once it has nothing left to do on its own: the
// scans, the keyframe index (a film without it is transcoded, not remuxed),
// the artwork. The dashboard's background work panel says "all idle" then;
// three readings in a row, a task can start right after another ends.
async function waitIdle(page) {
  await page.goto(BASE + "/admin", { waitUntil: "load" });
  const panel = page.locator("[data-testid='background-tasks-panel']");
  let calm = 0;
  for (let i = 0; i < 180 && calm < 3; i++) {
    await page.waitForTimeout(10000);
    calm = (await panel.innerText().catch(() => "")).includes("all idle") ? calm + 1 : 0;
  }
  if (calm < 3) throw new Error("the demo was still busy after 30 minutes");
  console.log("demo idle");
}

const browser = await chromium.launch({ args: ["--mute-audio", "--autoplay-policy=no-user-gesture-required"] });
try {
  const admin = await login(browser);
  await waitIdle(admin);
  for (const p of PAGES) {
    if (!wanted(p.name)) continue;
    await attempt(p.name, async () => {
      await admin.goto(BASE + p.path, { waitUntil: "networkidle" }).catch(() => {});
      await admin.waitForTimeout(2000);
      if (p.setup) await p.setup(admin);
      await shot(admin, p.name, p.clip ? await p.clip(admin) : undefined);
    });
  }

  const showcase = ["showcase-media", "showcase-player", "player-stream-info", "showcase-dashboard"];
  // The viewers play in sequence and the dashboard needs both: one failure
  // here costs the four images together.
  if (showcase.some(wanted)) await attempt(showcase.filter(wanted).join(", "), async () => {
    const spring = await mediaId(admin, "Spring");
    const sprite = await mediaId(admin, "Sprite Fright");

    if (wanted("showcase-media")) {
      await admin.goto(BASE + "/media/" + sprite, { waitUntil: "load" });
      await admin.waitForTimeout(3500);
      await shot(admin, "showcase-media");
    }

    const first = await watch(browser, "famille@demo.invalid", spring, 160);
    if (wanted("showcase-player")) {
      await wakeControls(first);
      await shot(first, "showcase-player");
    }
    const second = await watch(browser, "invite@demo.invalid", sprite, 160);
    if (wanted("player-stream-info")) {
      await wakeControls(second);
      await second.getByRole("button", { name: /stream info/i }).first().click();
      await second.waitForTimeout(1500);
      // The panel, with as much of the picture to its left as it is wide.
      const clip = await second.evaluate(() => {
        const leaf = [...document.querySelectorAll("body *")].find((e) => e.children.length === 0 && e.textContent.trim() === "Method");
        let n = leaf;
        while (n && n.getBoundingClientRect().height < 400) n = n.parentElement;
        const r = n.getBoundingClientRect();
        const x = Math.max(0, r.left - r.width);
        return { x, y: Math.max(0, r.top - 100), width: r.right + 16 - x, height: Math.min(window.innerHeight, r.bottom + 16) - Math.max(0, r.top - 100) };
      });
      await shot(second, "player-stream-info", clip);
      await second.keyboard.press("Escape");
    }

    if (wanted("showcase-dashboard")) {
      await admin.goto(BASE + "/admin", { waitUntil: "load" });
      await admin.waitForTimeout(10000);
      // The dashboard refreshes every 3 s and the Active streams tile lags the
      // cards: wait until both films show before the shot.
      for (let i = 0; i < 30; i++) {
        const playing = await admin.evaluate(() => document.body.innerText.includes("Sprite Fright") && document.body.innerText.includes("Spring"));
        if (playing) break;
        await admin.waitForTimeout(3000);
      }
      // The header and the now playing cards: the rest is the page above.
      await shot(admin, "showcase-dashboard", { x: 0, y: 0, width: 1600, height: 705 });
    }
  });
} finally {
  await browser.close();
}
if (failed.length) {
  console.error(`${failed.length} image(s) not taken: ${failed.join(", ")}`);
  process.exit(1);
}
