import { useEffect, useMemo, useRef } from "react";
import { createVideoPlayer, type VideoPlayer, type VideoSource } from "expo-video";

// A source with something to play: expo-video's VideoSource minus the "nothing"
// values. The screens build one from the stream URL and token.
export type PlayableVideoSource = Exclude<VideoSource, null | undefined>;

/**
 * A native video player that exists only once there is something to play.
 *
 * expo-video's useVideoPlayer always builds a player, source or not. With no
 * source, Android prepares an empty ExoPlayer, which reports ENDED at once, and
 * expo-video emits playToEnd for it: every listener installed on that phantom
 * sees a film "end" before the stream URL is even known (mobile beta.80 left
 * every title on it before its first byte, 2026-10-16). Here a null source
 * means no player at all - nothing to listen to, nothing to release.
 *
 * Lifecycle, as expo's own hook: the player is created when the source arrives
 * and `setup` runs on it once; it is replaced when the source changes by value
 * (not by object identity, the screens rebuild the object every render) and the
 * previous one is released after the commit, never during a render; it is
 * released on unmount, but kept across a fast refresh.
 */
export function useSourcedVideoPlayer(
  source: PlayableVideoSource | null,
  setup?: (player: VideoPlayer) => void,
): VideoPlayer | null {
  const key = source === null ? null : JSON.stringify(source);
  const playerRef = useRef<VideoPlayer | null>(null);
  const toRelease = useRef<VideoPlayer | null>(null);
  // undefined until the first render: a first null source must still "apply".
  const appliedKey = useRef<string | null | undefined>(undefined);
  const isFastRefresh = useRef(false);

  const player = useMemo(() => {
    // Same source again: a fast refresh dropped the memo, not the source.
    if (appliedKey.current !== undefined && appliedKey.current === key) return playerRef.current;
    toRelease.current = playerRef.current;
    let next: VideoPlayer | null = null;
    if (source !== null) {
      next = createVideoPlayer(source);
      setup?.(next);
    }
    playerRef.current = next;
    appliedKey.current = key;
    return next;
    // `source` and `setup` are read only when `key` changes: the key IS the source.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  // The previous player is released after the commit that replaced it, never
  // during a render.
  useEffect(() => {
    if (toRelease.current) {
      toRelease.current.release();
      toRelease.current = null;
    }
  }, [player]);

  useMemo(() => {
    isFastRefresh.current = true;
  }, []);

  useEffect(() => {
    isFastRefresh.current = false;
    return () => {
      // Runs on every fast refresh and on unmount: only the unmount releases.
      if (!isFastRefresh.current && playerRef.current) {
        playerRef.current.release();
        playerRef.current = null;
      }
    };
  }, []);

  return player;
}
