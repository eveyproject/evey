import { useEffect, useMemo } from "react";
import { startPlayback } from "./hlsPlayerUtils";

export interface MediaSessionParams {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  /** Display title (episode/movie). No metadata is published without it. */
  title?: string;
  /** Optional artwork URL (poster). Best-effort - a 401/404 just shows no art. */
  artworkUrl?: string;
  playing: boolean;
  /** Absolute position/duration on the media timeline (transcode-offset aware). */
  position: number;
  duration: number;
  playbackRate: number;
  /** Offset between the <video> element clock and the absolute timeline. In
   *  transcode mode the element plays a window starting at transcodeOffset, so
   *  an absolute seekTo must subtract it before touching video.currentTime. */
  transcodeOffset: number;
  skipForwardSeconds: number;
  skipBackwardSeconds: number;
  /** Episode list (ordered) + current id, for previoustrack / nexttrack on the
   *  lock screen. Only wired for series; absent ⇒ those controls stay hidden. */
  episodes?: ReadonlyArray<{ id: string }>;
  currentEpisodeId?: string;
  /** Navigate to an episode - reuses the player's existing episode switch. */
  onPlayEpisode?: (episodeId: string) => void;
  /** The film is playing on ANOTHER device (Cast session or Remote Playback
   *  fling). The OS transport then controls nothing here - see below. */
  remoteActive?: boolean;
}

const hasMediaSession = () =>
  typeof navigator !== "undefined" && "mediaSession" in navigator;

// useMediaSession mirrors the player onto the OS Media Session API so the
// lock screen / notification shade / macOS Now Playing shows the title +
// artwork and its play / pause / seek controls drive the <video> element.
// Entirely feature-detected and additive: on a browser without Media Session
// support every effect is a no-op. Seeks go through video.currentTime - the
// same mechanism as the in-player scrub bar - so they behave identically in
// direct-play and transcode mode (the player's seeking listeners react the
// same way), with the transcodeOffset applied for absolute seeks.
export function useMediaSession({
  videoRef,
  title,
  artworkUrl,
  playing,
  position,
  duration,
  playbackRate,
  transcodeOffset,
  skipForwardSeconds,
  skipBackwardSeconds,
  episodes,
  currentEpisodeId,
  onPlayEpisode,
  remoteActive = false,
}: MediaSessionParams) {
  // Resolve neighbours as primitives so the handler effect only re-binds when
  // the actual prev/next changes, not on every episodes-array re-identity.
  const prevEpisodeId = useMemo(() => {
    if (!episodes || !currentEpisodeId) return undefined;
    const i = episodes.findIndex((e) => e.id === currentEpisodeId);
    return i > 0 ? episodes[i - 1].id : undefined;
  }, [episodes, currentEpisodeId]);
  const nextEpisodeId = useMemo(() => {
    if (!episodes || !currentEpisodeId) return undefined;
    const i = episodes.findIndex((e) => e.id === currentEpisodeId);
    return i >= 0 && i < episodes.length - 1 ? episodes[i + 1].id : undefined;
  }, [episodes, currentEpisodeId]);

  // Metadata (title + artwork).
  useEffect(() => {
    if (!hasMediaSession() || !title) return;
    try {
      navigator.mediaSession.metadata = new MediaMetadata({
        title,
        artwork: artworkUrl
          ? [{ src: artworkUrl, sizes: "512x512", type: "image/jpeg" }]
          : [],
      });
    } catch {
      // MediaMetadata unavailable (older Safari) - leave it unset.
    }
    return () => {
      if (hasMediaSession()) navigator.mediaSession.metadata = null;
    };
  }, [title, artworkUrl]);

  // Action handlers. Re-bound when the skip sizes / offset change so the
  // closures read fresh values.
  useEffect(() => {
    if (!hasMediaSession()) return;
    const ms = navigator.mediaSession;
    const video = () => videoRef.current;
    const set = (
      action: MediaSessionAction,
      handler: MediaSessionActionHandler | null,
    ) => {
      try {
        ms.setActionHandler(action, handler);
      } catch {
        // Unsupported action on this browser - ignore.
      }
    };
    const actions: MediaSessionAction[] = [
      "play",
      "pause",
      "seekbackward",
      "seekforward",
      "seekto",
      "previoustrack",
      "nexttrack",
    ];

    // The picture is on another device: this element is not what is playing,
    // so the OS transport must reach none of it. A media key used to call
    // startPlayback() here behind the cast's back, and HLSPlayer's structural
    // guard re-paused it a moment later - an audible, visible blip. Claiming
    // no handler is the honest state, and it is also what stops the blip.
    if (remoteActive) {
      actions.forEach((a) => set(a, null));
      return;
    }

    set("play", () => {
      const v = video();
      if (v) void startPlayback(v);
    });
    set("pause", () => video()?.pause());
    set("seekbackward", (d) => {
      const v = video();
      if (!v) return;
      v.currentTime = Math.max(
        0,
        v.currentTime - (d.seekOffset ?? skipBackwardSeconds),
      );
    });
    set("seekforward", (d) => {
      const v = video();
      if (!v) return;
      v.currentTime = v.currentTime + (d.seekOffset ?? skipForwardSeconds);
    });
    set("seekto", (d) => {
      const v = video();
      if (!v || d.seekTime == null) return;
      // seekTime is on the absolute timeline; the element clock is offset by
      // transcodeOffset in transcode mode (0 in direct play).
      v.currentTime = Math.max(0, d.seekTime - transcodeOffset);
    });
    // null handler ⇒ the OS hides the prev/next control, so they only appear
    // when there actually is a neighbouring episode.
    set(
      "previoustrack",
      prevEpisodeId && onPlayEpisode ? () => onPlayEpisode(prevEpisodeId) : null,
    );
    set(
      "nexttrack",
      nextEpisodeId && onPlayEpisode ? () => onPlayEpisode(nextEpisodeId) : null,
    );

    return () => {
      actions.forEach((a) => set(a, null));
    };
  }, [
    videoRef,
    remoteActive,
    transcodeOffset,
    skipForwardSeconds,
    skipBackwardSeconds,
    prevEpisodeId,
    nextEpisodeId,
    onPlayEpisode,
  ]);

  // Play/pause state for the OS widget.
  useEffect(() => {
    if (!hasMediaSession()) return;
    navigator.mediaSession.playbackState = playing ? "playing" : "paused";
  }, [playing]);

  // Position state drives the scrubber on the lock screen.
  useEffect(() => {
    if (
      !hasMediaSession() ||
      typeof navigator.mediaSession.setPositionState !== "function"
    ) {
      return;
    }
    if (!duration || !isFinite(duration) || duration <= 0) return;
    try {
      navigator.mediaSession.setPositionState({
        duration,
        position: Math.min(Math.max(0, position), duration),
        playbackRate: playbackRate || 1,
      });
    } catch {
      // Invalid state (e.g. position > duration mid-update) - skip this tick.
    }
  }, [duration, position, playbackRate]);
}
