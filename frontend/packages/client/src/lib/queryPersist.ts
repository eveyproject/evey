import { dehydrate, hydrate, type DehydratedState, type Query, type QueryClient } from "@tanstack/react-query";

// A cold start used to wait for the network before showing anything: the
// home's rows, the library list, the profile - each a round trip, on a
// television several seconds (measured 2026-10-25). The screens a viewer
// opens first are written to storage as they land and handed back to React
// Query at the next start: the app opens on the last session's data and
// refreshes it behind (stale data refetches on mount as usual).

export interface PersistStorage {
  read(): Promise<string | null>;
  write(value: string): Promise<void>;
  remove(): Promise<void>;
}

// Bump to drop every stored cache (a response shape that changed).
const FORMAT = 1;
// Older than this, a stored cache is not worth showing.
const MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000;
// Writes are batched: a home opening lands a dozen queries in a second.
const WRITE_DELAY_MS = 2_000;
// A cache past this size is not written: parsing it at start would cost more
// than the round trips it saves.
const MAX_BYTES = 1_500_000;

// The first screens: the home's rows and what decides them, the libraries,
// the viewer. List pages are kept only from their first page.
const KEPT_PREFIXES = [
  "/api/v1/feeds/",
  "/api/v1/libraries",
  "/api/v1/playback/resume",
  "/api/v1/playback/next-up",
  "/api/v1/popular",
  "/api/v1/collections",
  "/api/v1/media",
  "/api/v1/plugins",
  "/api/v1/users/",
  "/api/v1/favorites",
];

/** The cache belongs to one server and one profile; nothing is kept signed out. */
export function queryCacheScope(serverId: string | null | undefined, profileId: string | null | undefined): string {
  return serverId && profileId ? `${serverId}|${profileId}` : "";
}

interface Stored {
  format: number;
  scope: string;
  savedAt: number;
  state: DehydratedState;
}

export function isKeptQuery(query: Pick<Query, "queryKey" | "state">): boolean {
  if (query.state.status !== "success") return false;
  const [path, params] = query.queryKey as [unknown, unknown];
  if (typeof path !== "string" || !KEPT_PREFIXES.some((p) => path.startsWith(p))) return false;
  // A later page of a list is only reached by scrolling to it.
  return !(params && typeof params === "object" && "pagination.cursor" in params);
}

/**
 * Hands the stored cache back to the client when it was written for this
 * scope (server and profile: a kids profile never starts on an adult's rows).
 * Resolves once done; never throws - a bad cache is a cache miss.
 */
export async function restoreQueryCache(client: QueryClient, storage: PersistStorage, scope: string): Promise<boolean> {
  try {
    const raw = await storage.read();
    if (!raw) return false;
    const stored = JSON.parse(raw) as Stored;
    if (stored.format !== FORMAT || stored.scope !== scope || Date.now() - stored.savedAt > MAX_AGE_MS) {
      await storage.remove();
      return false;
    }
    hydrate(client, stored.state);
    return true;
  } catch {
    await storage.remove().catch(() => {});
    return false;
  }
}

/**
 * Writes the kept queries as they change, a batch at a time, under `scope()`
 * (read at write time: the profile can change under a running app; an empty
 * scope - signed out - writes nothing). Returns the unsubscribe.
 */
export function persistQueryCache(client: QueryClient, storage: PersistStorage, scope: () => string): () => void {
  let timer: ReturnType<typeof setTimeout> | null = null;
  const write = () => {
    timer = null;
    const current = scope();
    if (!current) return;
    const stored: Stored = {
      format: FORMAT,
      scope: current,
      savedAt: Date.now(),
      state: dehydrate(client, { shouldDehydrateQuery: isKeptQuery }),
    };
    const json = JSON.stringify(stored);
    if (json.length > MAX_BYTES) return;
    void storage.write(json).catch(() => {});
  };
  const unsubscribe = client.getQueryCache().subscribe((event) => {
    if (event.type !== "updated" || event.action.type !== "success") return;
    if (!isKeptQuery(event.query)) return;
    if (!timer) timer = setTimeout(write, WRITE_DELAY_MS);
  });
  return () => {
    unsubscribe();
    if (timer) clearTimeout(timer);
  };
}
