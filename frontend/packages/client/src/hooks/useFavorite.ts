import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  favoriteServiceAddFavorite,
  favoriteServiceBatchIsFavorite,
  favoriteServiceListFavorites,
  favoriteServiceRemoveFavorite,
  getFavoriteServiceListFavoritesQueryKey,
  getFavoriteServiceIsFavoriteQueryKey,
  collectAllPages,
  EXPORT_PAGE_SIZE,
  type V1FavoriteProto,
} from "@evey/api";

// Must not exceed the server's maxBatchIsFavoriteIDs (grpcapi/favorite_handler.go):
// past it BatchIsFavorite refuses the whole call rather than truncating.
const MAX_BATCH = 200;

type FavoriteState = { isFavorite: boolean };
type Waiter = { resolve: (v: FavoriteState) => void; reject: (e: unknown) => void };

// Every card on a screen asks about its own media id, but they all ask within
// the same commit. Collect the ids raised during that turn and answer them with
// ONE request instead of one per card: a 226-card screen used to fire 226
// GET /api/v1/favorites/{id} and, on its own, saturate the 300 req/min limiter
// - the page's real calls were then refused as collateral damage (2026-08-15).
const waiting = new Map<string, Waiter[]>();
let flushScheduled = false;

function flushFavoriteBatch() {
  flushScheduled = false;
  const batch = new Map(waiting);
  waiting.clear();
  const ids = [...batch.keys()];
  for (let i = 0; i < ids.length; i += MAX_BATCH) {
    const chunk = ids.slice(i, i + MAX_BATCH);
    favoriteServiceBatchIsFavorite({ mediaItemIds: chunk })
      .then((res) => {
        const favorites = new Set(res.favoriteMediaItemIds ?? []);
        for (const id of chunk) {
          for (const w of batch.get(id) ?? []) w.resolve({ isFavorite: favorites.has(id) });
        }
      })
      .catch((err) => {
        // Reject rather than default to false: a swallowed error would paint an
        // empty heart on a real favorite, which is exactly the silent lie this
        // whole change exists to remove.
        for (const id of chunk) {
          for (const w of batch.get(id) ?? []) w.reject(err);
        }
      });
  }
}

function loadFavoriteState(mediaId: string): Promise<FavoriteState> {
  return new Promise<FavoriteState>((resolve, reject) => {
    const waiters = waiting.get(mediaId);
    if (waiters) waiters.push({ resolve, reject });
    else waiting.set(mediaId, [{ resolve, reject }]);
    if (!flushScheduled) {
      flushScheduled = true;
      // A microtask, not a timer: it runs once React has committed the whole
      // list, and stays deterministic under the fake timers half this suite uses.
      queueMicrotask(flushFavoriteBatch);
    }
  });
}

export function useIsFavorite(mediaId: string | undefined) {
  return useQuery({
    // Deliberately the generated single-item key: useToggleFavorite's optimistic
    // write and every screen that invalidates a favourite already target it.
    queryKey: getFavoriteServiceIsFavoriteQueryKey(mediaId ?? ""),
    enabled: !!mediaId,
    staleTime: 30_000,
    queryFn: () => loadFavoriteState(mediaId!),
    select: (d: FavoriteState | { isFavorite?: boolean }) => d.isFavorite ?? false,
  });
}

/**
 * Flips the favourite. `mutate(currentIsFavorite)` says which way from what
 * the caller shows; a bare `mutate()` reads the cached state instead, which is
 * what a screen whose button has no local copy of the state does.
 */
export function useToggleFavorite(mediaId: string | undefined) {
  const qc = useQueryClient();
  const key = () => getFavoriteServiceIsFavoriteQueryKey(mediaId ?? "");
  const currentIsFavorite = (explicit: boolean | undefined) =>
    explicit ?? qc.getQueryData<{ isFavorite?: boolean }>(key())?.isFavorite ?? false;
  return useMutation({
    mutationFn: async (prevIsFavorite?: boolean) => {
      if (!mediaId) return { isFavorite: false };
      // onMutate has already flipped the cache by the time this runs, so the
      // cached value is the DESIRED state, not the previous one.
      const wasFavorite = prevIsFavorite ?? !currentIsFavorite(undefined);
      if (wasFavorite) {
        await favoriteServiceRemoveFavorite(mediaId);
        return { isFavorite: false };
      }
      const res = await favoriteServiceAddFavorite(mediaId, {});
      return { isFavorite: res.isFavorite ?? true };
    },
    onMutate: async (prevIsFavorite?: boolean) => {
      if (!mediaId) return { prev: undefined };
      await qc.cancelQueries({ queryKey: key() });
      const prev = qc.getQueryData<{ isFavorite?: boolean }>(key());
      qc.setQueryData(key(), { isFavorite: !currentIsFavorite(prevIsFavorite) });
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev && mediaId) qc.setQueryData(key(), ctx.prev);
    },
    onSettled: () => {
      if (mediaId) qc.invalidateQueries({ queryKey: key() });
      qc.invalidateQueries({ queryKey: getFavoriteServiceListFavoritesQueryKey() });
    },
  });
}

/**
 * Marks every id a favourite, one call at a time (there is no batch write),
 * then refreshes the list once - the library's multi-select action.
 */
export function useBulkFavorite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (ids: string[]) => {
      for (const id of ids) {
        await favoriteServiceAddFavorite(id, {});
      }
    },
    onSettled: (_res, _err, ids) => {
      for (const id of ids) qc.invalidateQueries({ queryKey: getFavoriteServiceIsFavoriteQueryKey(id) });
      qc.invalidateQueries({ queryKey: getFavoriteServiceListFavoritesQueryKey() });
    },
  });
}

export interface FavoriteItem {
  mediaItemId: string;
  createdAt: string;
  title?: string;
  type?: string;
  year?: number;
  communityRating?: number;
}

// The favourites page sorts, filters and searches over this list on the
// client, so a partial list is not a shorter page - it is a wrong answer:
// searching for a favourite the server holds returns "no results". The hook
// used to send no pagination at all, which lands on the handler's default of
// 50 (clampLimitOffset, grpcapi/favorite_handler.go), and ListFavoritesResponse
// carries only `items` - no total, no cursor - so neither the screen nor the
// user could tell the list had been cut. Walk the pages instead, with the
// shared collector that stops on a short page and is bounded by its ceiling.
export function useFavoritesList() {
  return useQuery({
    // Deliberately the generated key: several screens already invalidate it
    // after toggling a favourite, and they must keep hitting this query.
    queryKey: getFavoriteServiceListFavoritesQueryKey(),
    staleTime: 30_000,
    queryFn: async ({ signal }) => {
      const collected = await collectAllPages<V1FavoriteProto>(
        (pageIndex) =>
          favoriteServiceListFavorites(
            { limit: EXPORT_PAGE_SIZE, offset: pageIndex * EXPORT_PAGE_SIZE },
            signal,
          ).then((r) => ({ rows: r.items ?? [] })),
      );
      return collected.rows.map<FavoriteItem>((f) => ({
        mediaItemId: f.mediaItemId ?? "",
        createdAt: f.createdAt ?? "",
        title: f.title,
        type: f.type,
        year: f.year,
        communityRating: f.communityRating,
      }));
    },
  });
}
