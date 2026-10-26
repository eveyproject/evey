// How much of a film the native player may hold in memory ahead of the
// playhead, in bytes, on this device.
//
// ExoPlayer's default byte target for video is 128 MiB, which is the whole
// Java heap of a small television (heapgrowthlimit 128m on a Chromecast HD):
// playing a 1080p source there, the heap climbed from 21 to 121 MiB in a
// minute, the runtime ended in blocking "GC Alloc" pauses on the main and
// player threads and an OutOfMemory, and the picture froze for a moment at
// each pause (2026-10-26). The cap is a quarter of the heap the platform
// grants the app, between 16 and 64 MiB; a device with a large heap keeps
// the player's own sizing.

const MIB = 1024 * 1024;
export const PLAYER_BUFFER_MIN_BYTES = 16 * MIB;
export const PLAYER_BUFFER_MAX_BYTES = 64 * MIB;
/** From this heap class up, the player's default sizing is left alone. */
export const PLAYER_BUFFER_UNCAPPED_HEAP_MB = 512;

export interface PlayerBufferOptions {
  maxBufferBytes: number;
  /** Size wins over duration: the byte cap must hold whatever the bitrate. */
  prioritizeTimeOverSizeThreshold: false;
}

export function playerBufferOptions(profile: { memoryClassMb: number } | null | undefined): PlayerBufferOptions | undefined {
  if (!profile || !(profile.memoryClassMb > 0) || profile.memoryClassMb >= PLAYER_BUFFER_UNCAPPED_HEAP_MB) return undefined;
  const quarter = Math.floor((profile.memoryClassMb * MIB) / 4);
  return {
    maxBufferBytes: Math.min(PLAYER_BUFFER_MAX_BYTES, Math.max(PLAYER_BUFFER_MIN_BYTES, quarter)),
    prioritizeTimeOverSizeThreshold: false,
  };
}
