/**
 * Monkey Testing Engine
 *
 * Concept: pool of atomic "actions" tagged by context (page type).
 * The runner picks a starting page, detects context, picks a valid action,
 * executes it, detects new context, repeats N times.
 *
 * Each action is a function that:
 * - Takes a page
 * - Does 1 micro-interaction (click, fill, navigate, etc.)
 * - Returns the new context (or null if unknown)
 *
 * The runner collects JS errors, 5xx, unexpected API 4xx (malformed requests
 * from UI actions - 400/405/422), blank pages, and untranslated i18n keys. Each
 * action is bounded by ACTION_TIMEOUT_MS so one wedged interaction can't blow
 * the run's budget.
 */

import type { Page } from "@playwright/test";

// ─── SEEDED PRNG (reproducible monkey runs) ──────────────────────────
// Raw rng() made a failed run impossible to replay across 800 runs.
// seedRng() swaps in a deterministic mulberry32 stream; runMonkey reseeds it per
// run from MONKEY_SEED, and the seed is surfaced on the result so a red run can
// be reproduced in isolation: MONKEY_SEED=<seed> MONKEY_RUNS=1.
let _rng: () => number = Math.random;
export function seedRng(seed: number): void {
  let a = seed >>> 0;
  _rng = () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}
function rng(): number {
  return _rng();
}

export type Context =
  | "home" | "media-detail" | "person" | "player"
  | "library" | "favorites" | "collections" | "playlists" | "music" | "photos"
  | "search" | "profile" | "settings" | "history" | "notifications"
  | "admin-dashboard" | "admin-libraries" | "admin-users" | "admin-plugins"
  | "admin-feeds" | "admin-metadata" | "admin-webhooks" | "admin-audit"
  | "admin-activity" | "admin-sessions" | "admin-analytics" | "admin-health"
  | "admin-config" | "admin-logs"
  | "unknown";

export interface MonkeyAction {
  name: string;
  contexts: Context[];  // which pages this action is valid on
  weight: number;       // higher = more likely to be picked
  fn: (page: Page) => Promise<Context | null>;
}

/** Detect current page context from URL */
export function detectContext(url: string): Context {
  const path = new URL(url).pathname;
  if (path === "/home" || path === "/") return "home";
  if (path.startsWith("/player/")) return "player";
  if (path.match(/^\/media\/[^/]+$/)) return "media-detail";
  if (path.startsWith("/person/")) return "person";
  if (path.startsWith("/library/") || path === "/library") return "library";
  if (path === "/favorites") return "favorites";
  if (path.startsWith("/collections")) return "collections";
  if (path.startsWith("/playlists")) return "playlists";
  if (path === "/music") return "music";
  if (path === "/photos") return "photos";
  if (path === "/search") return "search";
  if (path === "/profile") return "profile";
  if (path === "/profile/settings") return "settings";
  if (path === "/profile/history") return "history";
  if (path === "/profile/notifications") return "notifications";
  if (path === "/admin") return "admin-dashboard";
  if (path === "/admin/libraries") return "admin-libraries";
  if (path === "/admin/users") return "admin-users";
  if (path === "/admin/plugins") return "admin-plugins";
  if (path === "/admin/feeds") return "admin-feeds";
  if (path === "/admin/metadata") return "admin-metadata";
  if (path === "/admin/webhooks") return "admin-webhooks";
  if (path === "/admin/audit") return "admin-audit";
  if (path === "/admin/activity") return "admin-activity";
  if (path === "/admin/sessions") return "admin-sessions";
  if (path === "/admin/analytics") return "admin-analytics";
  if (path === "/admin/system-health") return "admin-health";
  if (path === "/admin/server-config") return "admin-config";
  if (path === "/admin/logs") return "admin-logs";
  return "unknown";
}

// ─── ACTION POOL ────────────────────────────────────────────────────

const everywhere: Context[] = [
  "home", "media-detail", "person", "library", "favorites", "collections",
  "playlists", "music", "photos", "search", "profile", "settings", "history",
  "notifications", "admin-dashboard", "admin-libraries", "admin-users",
  "admin-plugins", "admin-feeds", "admin-metadata", "admin-webhooks",
  "admin-audit", "admin-activity", "admin-sessions", "admin-analytics",
  "admin-health", "admin-config", "admin-logs",
];

export const ACTIONS: MonkeyAction[] = [
  // ── Navigation ──
  {
    name: "click random media card",
    contexts: ["home", "library", "favorites", "collections", "person"],
    weight: 10,
    fn: async (page) => {
      const cards = page.locator('[data-testid="media-card"]');
      const count = await cards.count();
      if (count === 0) return null;
      const idx = Math.floor(rng() * Math.min(count, 10));
      try { await cards.nth(idx).click({ timeout: 3_000 }); } catch { return null; }
      await page.waitForTimeout(1_000);
      return detectContext(page.url());
    },
  },
  {
    name: "click random link in page",
    contexts: everywhere,
    weight: 5,
    fn: async (page) => {
      const links = page.locator("a[href]:visible");
      const count = await links.count();
      if (count === 0) return null;
      const idx = Math.floor(rng() * Math.min(count, 15));
      try { await links.nth(idx).click({ timeout: 2_000 }); } catch { return null; }
      await page.waitForTimeout(800);
      return detectContext(page.url());
    },
  },
  {
    name: "go back",
    contexts: everywhere,
    weight: 3,
    fn: async (page) => {
      try { await page.goBack({ timeout: 3_000 }); } catch { return null; }
      await page.waitForTimeout(500);
      return detectContext(page.url());
    },
  },
  {
    name: "navigate to home",
    contexts: everywhere,
    weight: 2,
    fn: async (page) => {
      await page.goto("/home", { waitUntil: "domcontentloaded", timeout: 5_000 });
      return "home";
    },
  },
  {
    name: "navigate to favorites",
    contexts: everywhere,
    weight: 2,
    fn: async (page) => {
      await page.goto("/favorites", { waitUntil: "domcontentloaded", timeout: 5_000 });
      return "favorites";
    },
  },
  {
    name: "navigate to admin",
    contexts: everywhere,
    weight: 2,
    fn: async (page) => {
      await page.goto("/admin", { waitUntil: "domcontentloaded", timeout: 5_000 });
      return "admin-dashboard";
    },
  },

  // ── Search ──
  {
    name: "search random query",
    contexts: everywhere,
    weight: 4,
    fn: async (page) => {
      const input = page.locator('[data-testid="topbar-search-input"]');
      if (!(await input.isVisible({ timeout: 1_000 }).catch(() => false))) return null;
      const queries = ["the", "star", "love", "night", "dark", "world", "man", "king"];
      await input.fill(queries[Math.floor(rng() * queries.length)]);
      await page.waitForTimeout(800);
      const item = page.locator("[data-search-item]").first();
      if (await item.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await item.click();
        await page.waitForTimeout(1_000);
      } else {
        await page.keyboard.press("Escape");
      }
      return detectContext(page.url());
    },
  },

  // ── Buttons / CTAs ──
  {
    name: "click random visible button",
    contexts: everywhere,
    weight: 3,
    fn: async (page) => {
      const btns = page.locator("button:visible");
      const count = await btns.count();
      if (count === 0) return null;
      const idx = Math.floor(rng() * Math.min(count, 10));
      try { await btns.nth(idx).click({ timeout: 2_000 }); } catch {}
      await page.waitForTimeout(500);
      return detectContext(page.url());
    },
  },
  {
    name: "toggle favorite on detail",
    contexts: ["media-detail"],
    weight: 8,
    fn: async (page) => {
      const btn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
      if (await btn.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await btn.click();
        await page.waitForTimeout(500);
      }
      return "media-detail";
    },
  },
  {
    name: "toggle watched on detail",
    contexts: ["media-detail"],
    weight: 8,
    fn: async (page) => {
      const btn = page.locator('[data-testid="mark-watched-btn"], [data-testid="mark-unwatched-btn"]').first();
      if (await btn.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await btn.click();
        await page.waitForTimeout(500);
      }
      return "media-detail";
    },
  },
  {
    name: "click play button",
    contexts: ["media-detail"],
    weight: 5,
    fn: async (page) => {
      const btn = page.locator('[data-testid="play-button"]');
      if (await btn.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await btn.click();
        await page.waitForTimeout(2_000);
        return "player";
      }
      return null;
    },
  },
  {
    name: "open edit mode",
    contexts: ["media-detail"],
    weight: 5,
    fn: async (page) => {
      const btn = page.locator('[data-testid="media-edit-toggle-btn"]').first();
      if (await btn.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await btn.click();
        await page.waitForTimeout(500);
      }
      return "media-detail";
    },
  },
  {
    name: "right-click context menu on card",
    contexts: ["home", "library", "favorites", "collections", "person"],
    weight: 4,
    fn: async (page) => {
      const card = page.locator('[data-testid="media-card"]').first();
      if (await card.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await card.click({ button: "right" });
        await page.waitForTimeout(300);
        await page.keyboard.press("Escape");
      }
      return null;
    },
  },
  {
    name: "click watched badge on card",
    contexts: ["home", "library", "favorites"],
    weight: 6,
    fn: async (page) => {
      const badge = page.locator('[data-testid="watched-badge"]').first();
      if (await badge.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await badge.click();
        await page.waitForTimeout(500);
      }
      return null;
    },
  },
  {
    name: "click cast member",
    contexts: ["media-detail"],
    weight: 6,
    fn: async (page) => {
      const link = page.locator('[data-testid="cast-section"] a').first();
      if (await link.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await link.click();
        await page.waitForTimeout(1_000);
        return "person";
      }
      return null;
    },
  },

  // ── Player ──
  {
    name: "player: press Space",
    contexts: ["player"],
    weight: 5,
    fn: async (page) => {
      await page.keyboard.press("Space");
      await page.waitForTimeout(300);
      return "player";
    },
  },
  {
    name: "player: press M (mute)",
    contexts: ["player"],
    weight: 3,
    fn: async (page) => {
      await page.keyboard.press("m");
      await page.waitForTimeout(200);
      return "player";
    },
  },
  {
    name: "player: press ArrowRight",
    contexts: ["player"],
    weight: 3,
    fn: async (page) => {
      await page.keyboard.press("ArrowRight");
      await page.waitForTimeout(500);
      return "player";
    },
  },
  {
    name: "player: exit (Escape)",
    contexts: ["player"],
    weight: 4,
    fn: async (page) => {
      await page.keyboard.press("Escape");
      await page.waitForTimeout(1_000);
      return detectContext(page.url());
    },
  },

  // ── Admin ──
  {
    name: "admin: click random admin nav link",
    contexts: ["admin-dashboard", "admin-libraries", "admin-users", "admin-plugins", "admin-feeds", "admin-metadata", "admin-webhooks", "admin-audit", "admin-activity", "admin-sessions", "admin-analytics", "admin-health", "admin-config", "admin-logs"],
    weight: 5,
    fn: async (page) => {
      const links = page.locator("nav a[href*='/admin']");
      const count = await links.count();
      if (count === 0) return null;
      const idx = Math.floor(rng() * count);
      try { await links.nth(idx).click({ timeout: 2_000 }); } catch { return null; }
      await page.waitForTimeout(800);
      return detectContext(page.url());
    },
  },

  // ── Misc ──
  {
    name: "scroll down",
    contexts: everywhere,
    weight: 2,
    fn: async (page) => {
      await page.evaluate(() => window.scrollBy(0, 400));
      await page.waitForTimeout(200);
      return null;
    },
  },
  {
    name: "press Escape",
    contexts: everywhere,
    weight: 2,
    fn: async (page) => {
      await page.keyboard.press("Escape");
      await page.waitForTimeout(200);
      return null;
    },
  },
];

// ─── RUNNER ─────────────────────────────────────────────────────────

function pickAction(context: Context): MonkeyAction {
  const valid = ACTIONS.filter((a) => a.contexts.includes(context) || a.contexts === everywhere);
  const totalWeight = valid.reduce((sum, a) => sum + a.weight, 0);
  let roll = rng() * totalWeight;
  for (const action of valid) {
    roll -= action.weight;
    if (roll <= 0) return action;
  }
  return valid[valid.length - 1];
}

export interface MonkeyResult {
  steps: string[];
  jsErrors: string[];
  serverErrors: string[];
  blankPage: boolean;
  untranslatedKeys: string[];
  // Unexpected 4xx on API calls: a UI action that fires a 400/405/422 against
  // /api/ is almost always a real client bug (malformed payload, wrong method,
  // unparseable param) - exactly the class of drift that made several specs pass
  // while silently no-opping. Distinct from the noisy 4xx a random walk
  // legitimately produces (401 auth, 403 RBAC, 404 missing resource, 409
  // conflict, 429 rate-limit), which we deliberately do NOT flag.
  apiErrors: string[];
  seed?: number;
}

// 4xx statuses that, on an /api/ request, indicate the *client* sent something
// wrong rather than a legitimate "no/denied/conflict" outcome.
const API_4XX_FLAG = new Set([400, 405, 422]);

// Known-legitimate flagged 4xx that a random walk can trip without it being a
// bug. Keep this tight and annotate every entry - an over-broad allowlist
// re-blinds the monkey. (Empty until a real false positive is observed.)
const API_4XX_ALLOWLIST: Array<{ method?: string; urlIncludes: string; status?: number }> = [];

function isAllowlisted4xx(method: string, url: string, status: number): boolean {
  return API_4XX_ALLOWLIST.some(
    (a) =>
      url.includes(a.urlIncludes) &&
      (a.method === undefined || a.method === method) &&
      (a.status === undefined || a.status === status),
  );
}

function stripQuery(url: string): string {
  const q = url.indexOf("?");
  return q === -1 ? url : url.slice(0, q);
}

// Hard ceiling per action. A single chaotic action occasionally wedges on a
// Playwright default timeout (~30s) and two of those blow the 60s test budget,
// flaking the whole run. Bound each action so one hang costs seconds, not the
// run. The wedged operation isn't cancelled - it just stops blocking the walk.
const ACTION_TIMEOUT_MS = 6_000;

export async function runMonkey(page: Page, startUrl: string, numSteps: number, seed?: number): Promise<MonkeyResult> {
  // Reseed the action/choice RNG so this run is deterministic and replayable.
  if (seed !== undefined) seedRng(seed);
  const result: MonkeyResult = {
    steps: [],
    jsErrors: [],
    serverErrors: [],
    blankPage: false,
    untranslatedKeys: [],
    apiErrors: [],
    seed,
  };

  // The step in flight when a response arrives - lets us point at the action
  // that triggered a bad request for replay (responses land async, usually
  // during or just after the step that fired them).
  let currentStep = "START";

  page.on("pageerror", (e) => result.jsErrors.push(e.message));
  page.on("response", (r) => {
    const status = r.status();
    const url = r.url();
    if (status >= 500) {
      result.serverErrors.push(`${status} ${url}  @ ${currentStep}`);
      return;
    }
    if (API_4XX_FLAG.has(status) && url.includes("/api/")) {
      const method = r.request().method();
      if (!isAllowlisted4xx(method, url, status)) {
        result.apiErrors.push(`${status} ${method} ${stripQuery(url)}  @ ${currentStep}`);
      }
    }
  });

  await page.goto(startUrl, { waitUntil: "domcontentloaded", timeout: 10_000 });
  let context = detectContext(page.url());
  result.steps.push(`START: ${startUrl} (${context})`);

  for (let i = 0; i < numSteps; i++) {
    const action = pickAction(context);
    currentStep = `${i + 1}. [${context}] ${action.name}`;
    result.steps.push(currentStep);
    try {
      const newCtx = await Promise.race([
        action.fn(page),
        new Promise<never>((_, reject) =>
          setTimeout(() => reject(new Error(`action timed out after ${ACTION_TIMEOUT_MS}ms`)), ACTION_TIMEOUT_MS),
        ),
      ]);
      if (newCtx) context = newCtx;
      else context = detectContext(page.url());
    } catch (e) {
      result.steps.push(`   ERROR: ${e instanceof Error ? e.message : String(e)}`);
    }
  }
  // Let any in-flight request triggered by the last action settle so its
  // response is captured before we return.
  await page.waitForTimeout(300);

  // Final checks
  const bodyLen = await page.evaluate(() => document.body?.innerText?.length ?? 0);
  result.blankPage = bodyLen < 10;

  const bodyText = await page.evaluate(() => document.body?.innerText ?? "");
  const i18nPattern = /\b(?:media|common|player|admin|auth|errors)\.[a-z]+\.[a-zA-Z]+/g;
  const matches = bodyText.match(i18nPattern) ?? [];
  result.untranslatedKeys = matches.filter((k) => !k.includes("http") && !k.includes("api"));

  // Dedup the bad-request signal: a stuck control clicked repeatedly yields the
  // same 400 N times; one line is enough to act on.
  result.apiErrors = [...new Set(result.apiErrors)];

  return result;
}
