// How much of a film the native player may hold in memory ahead of the
// playhead, in bytes. ExoPlayer's default byte target for video is 128 MiB,
// the whole Java heap of a small television: a 1080p source filled it within
// a minute and the blocking GC pauses that followed froze the picture. A
// quarter of a 128 MiB heap was still too much: a 7 Mb/s film held its 32 MiB
// on top of the app, the heap stood at 105 of 128 MiB, and back-to-back
// collections of up to 2.5 s dropped frames and starved the audio. The cap is
// an eighth of the heap the platform grants, never under 16 MiB; a heap of
// 512 MiB or more keeps the player's own sizing, so the cap stays under 64 MiB.

const MIB = 1024 * 1024;
export const PLAYER_BUFFER_MIN_BYTES = 16 * MIB;
/** From this heap class up, the player's default sizing is left alone. */
export const PLAYER_BUFFER_UNCAPPED_HEAP_MB = 512;

export interface PlayerBufferOptions {
  maxBufferBytes: number;
  /** Size wins over duration: the byte cap must hold whatever the bitrate. */
  prioritizeTimeOverSizeThreshold: false;
}

export function playerBufferOptions(profile: { memoryClassMb: number } | null | undefined): PlayerBufferOptions | undefined {
  if (!profile || !(profile.memoryClassMb > 0) || profile.memoryClassMb >= PLAYER_BUFFER_UNCAPPED_HEAP_MB) return undefined;
  const eighth = Math.floor((profile.memoryClassMb * MIB) / 8);
  return {
    maxBufferBytes: Math.max(PLAYER_BUFFER_MIN_BYTES, eighth),
    prioritizeTimeOverSizeThreshold: false,
  };
}
