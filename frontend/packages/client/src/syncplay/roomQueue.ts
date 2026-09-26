import { musicServiceListTracks, playlistServiceGetPlaylistItems, collectAllPages, EXPORT_PAGE_SIZE } from "@evey/api";

// The queue a host assembles before creating a "listen together" room, shared
// by the web and phone room builders.

export interface RoomQueueItem {
  id: string;
  title: string;
  artist: string;
}

export const MAX_ROOM_QUEUE = 500;

// addToRoomQueue appends items not already queued, up to MAX_ROOM_QUEUE.
export function addToRoomQueue(prev: RoomQueueItem[], items: RoomQueueItem[]): RoomQueueItem[] {
  const seen = new Set(prev.map((q) => q.id));
  const next = [...prev];
  for (const it of items) {
    if (it.id && !seen.has(it.id) && next.length < MAX_ROOM_QUEUE) {
      seen.add(it.id);
      next.push(it);
    }
  }
  return next;
}

export function removeFromRoomQueue(prev: RoomQueueItem[], id: string): RoomQueueItem[] {
  return prev.filter((q) => q.id !== id);
}

// moveInRoomQueue swaps an item with its neighbour; out of bounds is a no-op.
export function moveInRoomQueue(prev: RoomQueueItem[], idx: number, dir: -1 | 1): RoomQueueItem[] {
  const j = idx + dir;
  if (j < 0 || j >= prev.length) return prev;
  const next = [...prev];
  [next[idx], next[j]] = [next[j], next[idx]];
  return next;
}

// albumRoomQueueItems pulls EVERY page of an album: ListTracks defaults to 50
// rows, and a 60-track box set used to become a 50-track room silently.
export async function albumRoomQueueItems(libraryId: string, albumId: string): Promise<RoomQueueItem[]> {
  const collected = await collectAllPages<{ id?: string; title?: string; artist?: string }>(
    (pageIndex) =>
      musicServiceListTracks({
        libraryId,
        albumId,
        limit: EXPORT_PAGE_SIZE,
        offset: pageIndex * EXPORT_PAGE_SIZE,
      }).then((r) => ({ rows: r.tracks ?? [] })),
  );
  return collected.rows.map((tr) => ({ id: tr.id ?? "", title: tr.title ?? "", artist: tr.artist ?? "" }));
}

export async function playlistRoomQueueItems(playlistId: string): Promise<RoomQueueItem[]> {
  const res = await playlistServiceGetPlaylistItems(playlistId);
  return (res.items ?? []).map((it) => ({ id: it.mediaItemId ?? "", title: it.title ?? "", artist: "" }));
}
