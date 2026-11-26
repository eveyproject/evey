import type { BrowserContext, Page } from "@playwright/test";
import { test, expect } from "./fixtures/auth";
import { apiGet, getToken } from "./scenarios/shared/helpers";

// One viewing = one play session. Every way into the player opens exactly one,
// every way out closes exactly that one, and the layout around the player
// never mounts it a second time (a second mount opened a second session that
// was stopped at once: the admin counted two plays for one).
//
// MEDIA_ID pins the film, EPISODE_ID an episode that has a next one; otherwise
// both are picked from the library and the case is skipped when it has none.

interface Sessions {
  started: string[];
  stopped: string[];
  startRequests: number;
}

function trackSessions(page: Page): Sessions {
  const sessions: Sessions = { started: [], stopped: [], startRequests: 0 };
  page.on("request", (request) => {
    if (request.method() !== "POST") return;
    const path = new URL(request.url()).pathname;
    if (path === "/api/v1/playback/start") sessions.startRequests += 1;
    if (path === "/api/v1/playback/stop") {
      try {
        const body = JSON.parse(request.postData() ?? "{}") as { sessionId?: string };
        if (body.sessionId) sessions.stopped.push(body.sessionId);
      } catch {
        // Not a stop this spec can read: the assertions below will say so.
      }
    }
  });
  page.on("response", async (response) => {
    if (new URL(response.url()).pathname !== "/api/v1/playback/start") return;
    const body = (await response.json().catch(() => null)) as { id?: string } | null;
    if (body?.id) sessions.started.push(body.id);
  });
  return sessions;
}

async function expectStarted(sessions: Sessions, count: number) {
  await expect.poll(() => sessions.started.length, { timeout: 20_000 }).toBe(count);
}

// Every session opened was closed, once, and nothing else was: no second
// session behind the viewer's back, no session left running.
async function expectAllClosed(sessions: Sessions, count: number) {
  await expect.poll(() => [...sessions.stopped].sort(), { timeout: 10_000 }).toEqual([...sessions.started].sort());
  // A late duplicate would land after the poll above went green.
  await new Promise((resolve) => setTimeout(resolve, 1_500));
  expect(sessions.startRequests).toBe(count);
  expect(sessions.started).toHaveLength(count);
  expect([...sessions.stopped].sort()).toEqual([...sessions.started].sort());
}

const inPlayer = (page: Page) => page.locator("#main-content");

// Firefox and WebKit abort a load that a redirect of the app overtakes (the
// landing page settling on its route): the load is asked again.
async function visit(page: Page, path: string) {
  for (let attempt = 0; ; attempt += 1) {
    try {
      await page.goto(path, { waitUntil: "domcontentloaded" });
      return;
    } catch (err) {
      if (attempt >= 2 || !/NS_BINDING_ABORTED|NS_ERROR_FAILURE|interrupted by another navigation/.test(String(err))) throw err;
      await page.waitForTimeout(300);
    }
  }
}

// The server's own list of running sessions (the admin's Now Playing). A page
// that is being unloaded sends its stop too late for the test to see the
// request go, so there the server is asked.
const probes = new WeakMap<BrowserContext, Page>();

// Asked from a page of its own, so the question survives whatever happens to
// the page under test (left, reloaded, sent elsewhere).
async function runningOnServer(page: Page, token: string, sessions: Sessions): Promise<string[]> {
  const context = page.context();
  let probe = probes.get(context);
  if (!probe) {
    probe = await context.newPage();
    await probe.goto("/login", { waitUntil: "commit" });
    probes.set(context, probe);
  }
  const body = await probe.evaluate(async (bearer) => {
    const response = await fetch("/api/v1/admin/sessions/active", { headers: { Authorization: `Bearer ${bearer}` } });
    return response.ok ? ((await response.json()) as { sessions?: { sessionId?: string }[] }) : null;
  }, token);
  expect(body, "the account running this spec reads the admin's running sessions").not.toBeNull();
  const running = new Set((body?.sessions ?? []).map((session) => session.sessionId));
  return sessions.started.filter((id) => running.has(id));
}

async function expectNoneRunningOnServer(page: Page, token: string, sessions: Sessions) {
  await expect.poll(() => runningOnServer(page, token, sessions), { timeout: 10_000 }).toEqual([]);
}

async function pickMovie(page: Page): Promise<string> {
  if (process.env.MEDIA_ID) return process.env.MEDIA_ID;
  const result = (await apiGet(page, "/api/v1/media?pagination.pageSize=20&type=MEDIA_ITEM_TYPE_MOVIE")) as {
    body?: { items?: { id?: string }[] };
  };
  return result?.body?.items?.[0]?.id ?? "";
}

// An episode that is not the last of its season, so the player offers a next one.
async function pickEpisodeWithNext(page: Page): Promise<string> {
  if (process.env.EPISODE_ID) return process.env.EPISODE_ID;
  const result = (await apiGet(page, "/api/v1/media?pagination.pageSize=200&type=MEDIA_ITEM_TYPE_EPISODE")) as {
    body?: { items?: { id?: string; parentId?: string; indexNumber?: number }[] };
  };
  const bySeason = new Map<string, { id: string; index: number }[]>();
  for (const item of result?.body?.items ?? []) {
    if (!item.id || !item.parentId) continue;
    const season = bySeason.get(item.parentId) ?? [];
    season.push({ id: item.id, index: item.indexNumber ?? 0 });
    bySeason.set(item.parentId, season);
  }
  for (const season of bySeason.values()) {
    if (season.length >= 2) return season.sort((a, b) => a.index - b.index)[0].id;
  }
  return "";
}

async function openFromMediaPage(page: Page, mediaId: string) {
  await visit(page, `/media/${mediaId}`);
  await page.locator('[data-testid="play-button"]').first().click({ timeout: 15_000 });
  await page.waitForURL(/\/player\//, { timeout: 15_000 });
}

test.describe("Player - one play session per viewing", () => {
  let movieId = "";
  let episodeId = "";

  test.beforeEach(async ({ page }) => {
    await visit(page, "/home");
    movieId ||= await pickMovie(page);
    episodeId ||= await pickEpisodeWithNext(page);
  });

  test("closing the player ends its one session and brings the layout back", async ({ page }) => {
    test.skip(!movieId, "No film in the library");
    const sessions = trackSessions(page);
    await openFromMediaPage(page, movieId);
    await expectStarted(sessions, 1);
    await expect(inPlayer(page)).toHaveCount(0);
    await expect(page.locator("video").first()).toBeAttached();
    const token = await getToken(page);
    // The server sees it running: what the checks on the way out compare with.
    await expect.poll(() => runningOnServer(page, token, sessions), { timeout: 10_000 }).toEqual(sessions.started);

    await page.keyboard.press("Escape");
    await page.waitForURL(/\/media\//, { timeout: 10_000 });
    await expectAllClosed(sessions, 1);
    await expectNoneRunningOnServer(page, token, sessions);
    await expect(inPlayer(page)).toBeVisible();
    await expect(page.locator("video")).toHaveCount(0);
  });

  test("the browser's back button ends its one session", async ({ page }) => {
    test.skip(!movieId, "No film in the library");
    const sessions = trackSessions(page);
    await openFromMediaPage(page, movieId);
    await expectStarted(sessions, 1);

    await page.goBack();
    await page.waitForURL(/\/media\//, { timeout: 10_000 });
    await expectAllClosed(sessions, 1);
    await expect(inPlayer(page)).toBeVisible();
  });

  test("back then forward is a second viewing: a second session, and only one", async ({ page }) => {
    test.skip(!movieId, "No film in the library");
    const sessions = trackSessions(page);
    await openFromMediaPage(page, movieId);
    await expectStarted(sessions, 1);
    await page.goBack();
    await page.waitForURL(/\/media\//, { timeout: 10_000 });
    await expectAllClosed(sessions, 1);

    await page.goForward();
    await page.waitForURL(/\/player\//, { timeout: 10_000 });
    await expectStarted(sessions, 2);
    await expect(inPlayer(page)).toHaveCount(0);
    await page.keyboard.press("Escape");
    await page.waitForURL(/\/media\//, { timeout: 10_000 });
    await expectAllClosed(sessions, 2);
  });

  test("a player opened by its address ends its one session on the way out", async ({ page }) => {
    test.skip(!movieId, "No film in the library");
    const sessions = trackSessions(page);
    await visit(page, `/player/${movieId}`);
    await expectStarted(sessions, 1);
    await expect(inPlayer(page)).toHaveCount(0);

    await page.keyboard.press("Escape");
    await page.waitForURL(/\/(media|home)/, { timeout: 10_000 });
    await expectAllClosed(sessions, 1);
    await expect(inPlayer(page)).toBeVisible();
  });

  test("leaving before the session has opened leaves none running", async ({ page }) => {
    test.skip(!movieId, "No film in the library");
    const sessions = trackSessions(page);
    await openFromMediaPage(page, movieId);
    // No wait: the start may still be in flight, or not even sent. Back, not
    // Escape: the player's own keys are not listening yet.
    await page.goBack();
    await page.waitForURL(/\/media\//, { timeout: 10_000 });
    await new Promise((resolve) => setTimeout(resolve, 3_000));
    expect(sessions.startRequests).toBeLessThanOrEqual(1);
    expect([...sessions.stopped].sort()).toEqual([...sessions.started].sort());
    await expectNoneRunningOnServer(page, await getToken(page), sessions);
  });

  test("opening and closing three times in a row is three sessions", async ({ page }) => {
    test.skip(!movieId, "No film in the library");
    const sessions = trackSessions(page);
    for (let viewing = 1; viewing <= 3; viewing += 1) {
      await openFromMediaPage(page, movieId);
      await expectStarted(sessions, viewing);
      await page.keyboard.press("Escape");
      await page.waitForURL(/\/media\//, { timeout: 10_000 });
      await expectAllClosed(sessions, viewing);
    }
  });

  test("leaving the site from the player ends its session", async ({ page }) => {
    test.skip(!movieId, "No film in the library");
    const sessions = trackSessions(page);
    await openFromMediaPage(page, movieId);
    await expectStarted(sessions, 1);
    const token = await getToken(page);
    await expect.poll(() => runningOnServer(page, token, sessions), { timeout: 10_000 }).toEqual(sessions.started);

    // Another document, as a bookmark or a typed address would load.
    await page.goto("data:text/html,<title>elsewhere</title>");
    await expectNoneRunningOnServer(page, token, sessions);
    expect(sessions.startRequests).toBe(1);
  });

  test("reloading the player leaves one session running, and none once it is left", async ({ page }) => {
    test.skip(!movieId, "No film in the library");
    const sessions = trackSessions(page);
    await openFromMediaPage(page, movieId);
    await expectStarted(sessions, 1);

    const token = await getToken(page);
    const first = sessions.started[0];
    await expect.poll(() => runningOnServer(page, token, sessions), { timeout: 10_000 }).toEqual([first]);

    await page.reload({ waitUntil: "domcontentloaded" });
    await expectStarted(sessions, 2);
    // One session runs for this player: a new one when the first was closed on
    // the way out, the first one taken back by the server otherwise.
    await expect
      .poll(async () => [...new Set(await runningOnServer(page, token, sessions))], { timeout: 10_000 })
      .toEqual([sessions.started[1]]);
    await page.keyboard.press("Escape");
    await page.waitForURL(/\/(media|home)/, { timeout: 10_000 });
    await expectNoneRunningOnServer(page, token, sessions);
    expect(sessions.startRequests).toBe(2);
  });

  test("the next episode is a second session, one per episode", async ({ page }) => {
    test.skip(!episodeId, "No season with two episodes in the library");
    const sessions = trackSessions(page);
    await visit(page, `/player/${episodeId}`);
    await expectStarted(sessions, 1);

    // The controls show on pointer movement.
    await page.mouse.move(400, 400);
    await page.mouse.move(640, 500);
    await page.locator("button:has(svg.lucide-skip-forward)").first().click({ timeout: 10_000 });
    await page.waitForURL((url) => url.pathname.startsWith("/player/") && !url.pathname.endsWith(episodeId), { timeout: 10_000 });
    await expectStarted(sessions, 2);
    await expect(inPlayer(page)).toHaveCount(0);

    await page.keyboard.press("Escape");
    await page.waitForURL(/\/(media|home)/, { timeout: 10_000 });
    await expectAllClosed(sessions, 2);
  });

  test("on a phone-width screen the layout comes back with its menu button", async ({ page }) => {
    test.skip(!movieId, "No film in the library");
    await page.setViewportSize({ width: 390, height: 844 });
    const sessions = trackSessions(page);
    await openFromMediaPage(page, movieId);
    await expectStarted(sessions, 1);
    await expect(page.locator('[data-testid="open-sidebar-btn"]')).toHaveCount(0);

    await page.goBack();
    await page.waitForURL(/\/media\//, { timeout: 10_000 });
    await expectAllClosed(sessions, 1);
    await expect(page.locator('[data-testid="open-sidebar-btn"]')).toBeVisible();
  });
});
