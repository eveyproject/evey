/**
 * Everything the error screens need to know about a failed lazy chunk, written
 * so that NOTHING here ever dereferences the caught value.
 *
 * Measured against the deployed bundle on 2026-08-24 (Firefox, Playwright): when
 * a lazily imported route chunk fails to load, the value React hands to an error
 * boundary is `undefined`, not an Error. `error.name === "ChunkLoadError"` in
 * componentDidCatch therefore threw `TypeError: can't access property "name", t
 * is undefined` - the boundary crashed inside its own handler, never rendered
 * its fallback, and the TypeError escaped to the route errorComponent, which
 * showed it to the user. The recovery written for exactly this failure was the
 * line that broke, so a stale chunk always ended as "Une erreur est survenue".
 *
 * Two triggers produce it in production, both routine:
 *  - a network hiccup while a route chunk is being fetched;
 *  - a deployment: an open tab still asks for the previous build's hashed
 *    chunks, and the SPA fallback answers 200 text/html, which the module
 *    loader rejects.
 */

// The wording differs per engine: "error loading dynamically imported module"
// (Firefox), "Failed to fetch dynamically imported module" (Chromium),
// "Importing a module script failed" (WebKit), "Loading chunk N failed"
// (webpack-era bundlers, still emitted by some plugin bundles).
const CHUNK_ERROR_MESSAGE =
  /loading chunk|dynamically imported module|importing a module script failed/i;

const RELOAD_KEY = "evey_chunk_reload";

// How long a `vite:preloadError` stays usable as evidence. The boundary catches
// in the same tick as the rejection, so this only has to survive React's error
// replay - it is deliberately short so an unrelated crash minutes later is not
// mistaken for a chunk failure.
const PRELOAD_ERROR_TTL_MS = 10_000;

let lastPreloadErrorAt = 0;
let installed = false;

/**
 * Vite's preload helper dispatches `vite:preloadError` with the REAL failure as
 * payload before rethrowing, so the event survives even when what reaches React
 * is an opaque `undefined`. Recording it gives the boundary a second, shape-free
 * signal to recognise a chunk failure by.
 */
export function installPreloadErrorTracking(): void {
  if (installed || typeof window === "undefined") return;
  installed = true;
  window.addEventListener("vite:preloadError", () => {
    lastPreloadErrorAt = Date.now();
    // A route chunk that fails during NAVIGATION never reaches an error
    // boundary: the router aborts the transition and the page simply stays,
    // so every home title reads as "unclickable" in a tab opened before a
    // redeploy (prod, 2026-08-25). Vite's documented remedy is to reload on
    // this very event; the once-per-session guard below keeps a genuinely
    // broken build from looping.
    reloadOnceForChunkError();
  });
}

// Auto-install: a chunk can fail before any component mounts (the very first
// route import), so this cannot wait for a React effect to register it.
installPreloadErrorTracking();

/** For tests only - forget the recorded preload failure AND re-arm the
 * once-per-session reload guard, so each test starts from a fresh session. */
export function resetPreloadErrorForTests(): void {
  lastPreloadErrorAt = 0;
  try {
    sessionStorage.removeItem(RELOAD_KEY);
  } catch {
    // Storage refused: nothing recorded, nothing to clear.
  }
}

function readString(value: unknown, key: "name" | "message"): string {
  if (typeof value !== "object" || value === null) return "";
  const field = (value as Record<string, unknown>)[key];
  return typeof field === "string" ? field : "";
}

/** The error's message, or "" for anything that does not carry one. */
export function errorMessage(value: unknown): string {
  return readString(value, "message");
}

/**
 * True when the caught value is (or accompanies) a failed chunk load. Accepts
 * `unknown` on purpose: the whole point is that the caller has no idea what it
 * caught.
 */
export function isChunkLoadError(value: unknown): boolean {
  if (readString(value, "name") === "ChunkLoadError") return true;
  if (CHUNK_ERROR_MESSAGE.test(errorMessage(value))) return true;
  // Opaque value (undefined, null, a string, a stripped Error): it carries no
  // shape to test, so fall back on Vite's own event. Anything that DID carry a
  // name or a message has already been judged on its own evidence above.
  if (readString(value, "name") !== "" || errorMessage(value) !== "") return false;
  return Date.now() - lastPreloadErrorAt < PRELOAD_ERROR_TTL_MS;
}

/**
 * Reload once per session to pick up the new build. Returns true when a reload
 * was started, false when this session already tried one - the second failure
 * is a real one and must be shown instead of looping.
 */
export function reloadOnceForChunkError(): boolean {
  let alreadyTried = false;
  try {
    alreadyTried = sessionStorage.getItem(RELOAD_KEY) !== null;
    if (alreadyTried) sessionStorage.removeItem(RELOAD_KEY);
    else sessionStorage.setItem(RELOAD_KEY, "1");
  } catch {
    // Storage refused (private mode, cookies blocked). Reading it is what the
    // loop guard needs, so without it the safe move is to NOT reload: a reload
    // loop is worse than one error screen.
    return false;
  }
  if (alreadyTried) return false;
  window.location.reload();
  return true;
}
