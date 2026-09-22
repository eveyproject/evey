// The queue and its rules live in @evey/store (musicQueue.ts), shared with the
// phone, the Android television and the browser's own player. This module
// keeps the store instance for this app, with its own storage key.
import { createMusicQueueStore } from "@evey/store";

export type { MusicSource, QueueTrack } from "@evey/store";
export { trackToQueueTrack } from "@evey/store";

export const useMusicQueueStore = createMusicQueueStore("evey-tv-web-music-queue");
