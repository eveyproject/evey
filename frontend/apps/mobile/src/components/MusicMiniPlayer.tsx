// Globally-mounted music player. Subscribes to useMusicQueueStore and
// drives an expo-audio AudioPlayer for the active track. Background
// playback comes from the audio session configured at boot
// (configureAudioSession + iOS UIBackgroundModes:["audio"]).
//
// Web has GlobalAudioPlayer.tsx; this is the RN equivalent. expo-av's
// ExponentAV native module is gone from Expo Go on SDK 55, so we use
// expo-audio's hook-based API instead.
import { memo, useCallback, useEffect, useRef, useState } from "react";
import { View, Text, Pressable, StyleSheet, AppState, Platform, type GestureResponderEvent } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useAudioPlayer, useAudioPlayerStatus } from "expo-audio";
import { useTranslation } from "react-i18next";
import { router } from "expo-router";
import { eveyFetch, formatTime, getFreshAccessToken } from "@evey/api";
import { toast } from "@evey/store";
import { useMusicQueueStore } from "../stores/musicQueue";
import { useTheme } from "../providers/ThemeProvider";
import { useServerUrl } from "../hooks/useServerUrl";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";
import { shouldRecordScrobble } from "../lib/musicScrobble";
import { coverArtUri } from "../lib/coverArtUri";

// One nowplaying "session" per mini-player mount (not one per track), so the
// admin "En cours de lecture" panel shows a single updating tile instead of a
// new row every time the track changes - mirrors the web GlobalAudioPlayer.
const nowPlayingSessionId = `music-mobile-${
  typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
    ? crypto.randomUUID()
    : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`
}`;

function nowPlayingDeviceName(): string {
  if (Platform.OS === "ios") return "iPhone";
  if (Platform.OS === "android") return "Android Phone";
  return "Mobile";
}

function MusicMiniPlayerImpl() {
  const theme = useTheme();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();

  const queue = useMusicQueueStore((s) => s.queue);
  const currentIndex = useMusicQueueStore((s) => s.currentIndex);
  const isPlayingState = useMusicQueueStore((s) => s.isPlaying);
  const shuffle = useMusicQueueStore((s) => s.shuffle);
  const source = useMusicQueueStore((s) => s.source);
  const togglePlay = useMusicQueueStore((s) => s.togglePlay);
  const toggleShuffle = useMusicQueueStore((s) => s.toggleShuffle);
  const next = useMusicQueueStore((s) => s.next);
  const previous = useMusicQueueStore((s) => s.previous);
  const clear = useMusicQueueStore((s) => s.clear);

  const track = currentIndex >= 0 && currentIndex < queue.length ? queue[currentIndex] : null;
  const trackId = track?.id ?? null;

  // Build the streaming URI for the active track. Token is appended as a
  // query param because the audio fetch path doesn't carry our Authorization
  // header through to ffmpeg/the streamer. It is fetched fresh and pinned at
  // track change: the store's token is what the last API call left, and an
  // album is long enough for it to expire between two tracks with the phone
  // in a pocket - the next stream would 401 and the music stop. Pinned, not
  // subscribed, so a refresh mid-track never restarts the song.
  const [pinned, setPinned] = useState<{ trackId: string; token: string } | null>(null);
  useEffect(() => {
    if (!trackId) return;
    let live = true;
    getFreshAccessToken().then((token) => {
      if (live && token) setPinned({ trackId, token });
    }).catch(() => { /* signed out: the 401 path signs the app out */ });
    return () => { live = false; };
  }, [trackId]);
  const accessToken = pinned && pinned.trackId === trackId ? pinned.token : null;

  const streamUri = (trackId && serverUrl && accessToken)
    ? `${serverUrl}/api/v1/music/tracks/${trackId}/stream?token=${encodeURIComponent(accessToken)}`
    : null;

  // Single AudioPlayer for the entire queue lifetime. We swap its source via
  // .replace() on track change; creating a new player per track would
  // tear down the audio session and clobber the iOS now-playing card.
  const player = useAudioPlayer(streamUri ?? undefined, { updateInterval: 500 });
  const status = useAudioPlayerStatus(player);

  // Track which URL is currently loaded so we don't redundantly call replace.
  const loadedUriRef = useRef<string | null>(null);

  // Tap/drag-to-seek state: measured bar width, the fraction tracked during
  // the responder gesture, and the value shown while actively scrubbing
  // (falls back to the real playback progress once released).
  const seekBarWidthRef = useRef(0);
  const seekPctRef = useRef(0);
  const [scrubPct, setScrubPct] = useState<number | null>(null);

  useEffect(() => {
    if (!streamUri) return;
    if (loadedUriRef.current === streamUri) return;
    loadedUriRef.current = streamUri;
    try {
      player.replace(streamUri);
      if (isPlayingState) player.play();
    } catch {
      // Player not ready yet - the next render will retry.
    }
  }, [streamUri, player, isPlayingState]);

  // Mirror queue store play/pause onto the player. No track means the queue
  // was cleared - by the close button, or by the video player opening - and
  // the player still holds its last source: this pause is what silences it.
  useEffect(() => {
    try {
      if (!trackId || !isPlayingState) player.pause();
      else if (streamUri) player.play();
    } catch { /* released */ }
    // The same track played again later must start over, not resume the
    // paused source the player still holds.
    if (!trackId) loadedUriRef.current = null;
  }, [isPlayingState, player, streamUri, trackId]);

  // Auto-advance when the current track ends.
  const lastDidFinishRef = useRef(false);
  useEffect(() => {
    const finished = !!status.didJustFinish;
    if (finished && !lastDidFinishRef.current) {
      lastDidFinishRef.current = true;
      next();
    } else if (!finished) {
      lastDidFinishRef.current = false;
    }
  }, [status.didJustFinish, next]);

  // Scrobble a finished track once per playthrough - feeds the playlist
  // generator's taste signal, mirrors the web GlobalAudioPlayer. The dedup
  // decision itself lives in the pure shouldRecordScrobble (musicScrobble.ts)
  // so it's unit-tested; this effect only wires it to the network call.
  const lastRecordedTrackIdRef = useRef<string | null>(null);
  useEffect(() => {
    const trackId = track?.id ?? null;
    if (!shouldRecordScrobble({ didJustFinish: !!status.didJustFinish, trackId, lastRecordedTrackId: lastRecordedTrackIdRef.current })) return;
    lastRecordedTrackIdRef.current = trackId;
    void eveyFetch({ url: `/api/v1/music/tracks/${trackId}/played`, method: "POST" }).catch(() => {});
  }, [status.didJustFinish, track]);

  // Detect a track that never loads (dead transcode, expired token, 404) and
  // skip it instead of leaving the mini-player stuck silent - expo-audio's
  // AudioStatus carries no explicit error field, so a load-grace-period is
  // the only cross-platform failure signal available. Counts consecutive
  // failures so a systemic outage (every track dead) stops and surfaces one
  // error instead of cycling the queue forever.
  const statusRef = useRef(status);
  statusRef.current = status;
  const isPlayingRef = useRef(isPlayingState);
  isPlayingRef.current = isPlayingState;
  const failureCountRef = useRef(0);

  useEffect(() => {
    if (status.isLoaded) failureCountRef.current = 0;
  }, [status.isLoaded]);

  useEffect(() => {
    if (!streamUri) return;
    const timer = setTimeout(() => {
      if (!isPlayingRef.current) return;
      const s = statusRef.current;
      if (s.isLoaded || s.playing) return;
      failureCountRef.current += 1;
      if (failureCountRef.current >= Math.max(queue.length, 1)) {
        failureCountRef.current = 0;
        toast.error(t("common.error"));
        togglePlay();
        return;
      }
      next();
    }, 8000);
    return () => clearTimeout(timer);
    // Timer is keyed per-track by streamUri only; queue/next/togglePlay/t are
    // read at fire time via refs/closures, not tracked as effect deps.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [streamUri]);

  const onClose = useCallback(() => {
    try { player.pause(); } catch { /* released */ }
    loadedUriRef.current = null;
    clear();
  }, [clear, player]);

  // Report this playback to the admin "En cours de lecture" panel every ~10s,
  // mirroring the web GlobalAudioPlayer, and apply any remoteCommand it
  // queues back (play/pause/stop). Foreground-only: the audio session
  // survives backgrounding, but beating from the background would keep
  // spending battery/data on a session the admin can't meaningfully act on
  // anymore, so heartbeating stops there and resumes when the app returns.
  useEffect(() => {
    if (!track) return;
    const trackId = track.id;
    let cancelled = false;
    let intervalId: ReturnType<typeof setInterval> | null = null;

    async function beat() {
      try {
        const data = await eveyFetch<{ remoteCommand?: string }>({
          url: "/api/v1/music/nowplaying",
          method: "POST",
          data: {
            sessionId: nowPlayingSessionId,
            trackId,
            deviceName: nowPlayingDeviceName(),
            positionMs: Math.floor((statusRef.current.currentTime ?? 0) * 1000),
            isPaused: !statusRef.current.playing,
          },
        });
        if (cancelled) return;
        const cmd = data?.remoteCommand;
        if (!cmd) return;
        if (cmd === "pause") {
          if (isPlayingRef.current) togglePlay();
        } else if (cmd === "play") {
          if (!isPlayingRef.current) togglePlay();
        } else if (cmd === "stop") {
          onClose();
        }
      } catch { /* best-effort telemetry, never disrupt playback */ }
    }

    function start() {
      if (intervalId != null) return;
      void beat();
      intervalId = setInterval(() => void beat(), 10_000);
    }
    function stop() {
      if (intervalId != null) {
        clearInterval(intervalId);
        intervalId = null;
      }
    }

    if (AppState.currentState === "active") start();
    const sub = AppState.addEventListener("change", (state) => {
      if (state === "active") start();
      else stop();
    });

    return () => {
      cancelled = true;
      stop();
      sub.remove();
    };
    // Keyed by track id only, mirroring the web GlobalAudioPlayer's
    // [playing?.ID] heartbeat effect - togglePlay/onClose are stable store
    // actions, read here via closure rather than restarting the interval.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [track?.id]);

  // Carry WHICH source, not just "the music tab": landing on an unfiltered
  // library is not going back to the album you were listening to. The music
  // screen reads these params and restores the matching filter.
  const goToSource = useCallback(() => {
    if (!source) return;
    const params =
      source.type === "playlist"
        ? { playlistId: source.id }
        : source.type === "album"
          ? { album: source.id, albumLabel: source.label }
          : { artist: source.id };
    router.push({ pathname: "/(auth)/(tabs)/music", params });
  }, [source]);

  if (!track) return null;

  const duration = (status.duration ?? track.duration ?? 0);
  const position = status.currentTime ?? 0;
  const progress = duration > 0 ? position / duration : 0;
  const displayedProgress = scrubPct ?? progress;
  // 40dp slot at the device's density - a display-sized WebP, not the full
  // cover embedded in the file (the server resizes and caches it).
  const artUri = (serverUrl && accessToken)
    ? coverArtUri(serverUrl, `/api/v1/music/tracks/${track.id}/art?token=${encodeURIComponent(accessToken)}`, 40)
    : undefined;

  const pctFromX = (x: number) => {
    const w = seekBarWidthRef.current;
    if (w <= 0) return 0;
    return Math.max(0, Math.min(1, x / w));
  };

  const commitSeek = (pct: number) => {
    if (duration <= 0) return;
    try { void player.seekTo(pct * duration); } catch { /* released */ }
  };

  const titleNode = (
    <Text numberOfLines={1} style={{ color: theme.colors.text, fontSize: FONT_SIZE.xs, fontWeight: "600" }}>
      {track.title || "-"}
    </Text>
  );

  return (
    <View
      style={[styles.root, { backgroundColor: theme.colors.surfaceRaised, borderTopColor: theme.colors.border }]}
      accessibilityLabel={t("media.music.miniPlayer")}
    >
      <View style={styles.seekRow}>
        <Text style={[styles.timeLabel, { color: theme.colors.textMuted }]}>{formatTime(position)}</Text>
        <View
          style={styles.seekBarContainer}
          accessible
          accessibilityRole="adjustable"
          accessibilityLabel={t("media.music.seekPosition")}
          accessibilityValue={{ min: 0, max: 100, now: Math.round(progress * 100) }}
          onLayout={(e) => { seekBarWidthRef.current = e.nativeEvent.layout.width; }}
          onStartShouldSetResponder={() => true}
          onMoveShouldSetResponder={() => true}
          onResponderGrant={(e: GestureResponderEvent) => {
            const pct = pctFromX(e.nativeEvent.locationX);
            seekPctRef.current = pct;
            setScrubPct(pct);
          }}
          onResponderMove={(e: GestureResponderEvent) => {
            const pct = pctFromX(e.nativeEvent.locationX);
            seekPctRef.current = pct;
            setScrubPct(pct);
          }}
          onResponderRelease={() => {
            commitSeek(seekPctRef.current);
            setScrubPct(null);
          }}
        >
          <View style={[styles.progressTrack, { backgroundColor: theme.colors.border }]}>
            <View style={[styles.progressFill, { width: `${Math.max(0, Math.min(100, displayedProgress * 100))}%`, backgroundColor: theme.colors.accent }]} />
          </View>
        </View>
        <Text style={[styles.timeLabel, { color: theme.colors.textMuted }]}>{duration > 0 ? formatTime(duration) : "-"}</Text>
      </View>
      <View style={styles.row}>
        <View style={styles.cover}>
          {artUri ? (
            <Image source={{ uri: artUri }} style={{ width: 40, height: 40, borderRadius: 4 }} contentFit="cover" />
          ) : (
            <Ionicons name="musical-notes" size={ICON_SIZE.lg} color={theme.colors.textMuted} />
          )}
        </View>
        <View style={{ flex: 1, marginLeft: 10, minWidth: 0 }}>
          {source ? (
            <Pressable onPress={goToSource} accessibilityRole="button" accessibilityLabel={source.label}>
              {titleNode}
            </Pressable>
          ) : titleNode}
          <Text numberOfLines={1} style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>
            {[track.artist, track.album].filter(Boolean).join(" · ") || "-"}
          </Text>
        </View>
        <Pressable
          onPress={toggleShuffle}
          hitSlop={8}
          accessibilityLabel={t("media.music.shuffle")}
          accessibilityRole="button"
          accessibilityState={{ selected: shuffle }}
          style={{ marginRight: 4 }}
        >
          <Ionicons name="shuffle" size={ICON_SIZE.md} color={shuffle ? theme.colors.accent : theme.colors.textMuted} />
        </Pressable>
        <Pressable onPress={previous} hitSlop={8} accessibilityLabel={t("mobile.music.previousTrack")} accessibilityRole="button">
          <Ionicons name="play-skip-back" size={ICON_SIZE.xl} color={theme.colors.text} />
        </Pressable>
        <Pressable
          onPress={togglePlay}
          hitSlop={8}
          accessibilityLabel={isPlayingState ? t("player.controls.pause") : t("player.controls.play")}
          accessibilityRole="button"
          style={{ marginHorizontal: 10 }}
        >
          <Ionicons name={isPlayingState ? "pause" : "play"} size={ICON_SIZE.xl} color={theme.colors.text} />
        </Pressable>
        <Pressable onPress={next} hitSlop={8} accessibilityLabel={t("mobile.music.nextTrack")} accessibilityRole="button">
          <Ionicons name="play-skip-forward" size={ICON_SIZE.xl} color={theme.colors.text} />
        </Pressable>
        <Pressable onPress={onClose} hitSlop={8} accessibilityLabel={t("mobile.music.closePlayer")} accessibilityRole="button" style={{ marginLeft: 10 }}>
          <Ionicons name="close" size={ICON_SIZE.lg} color={theme.colors.textMuted} />
        </Pressable>
      </View>
    </View>
  );
}

// memo: no props; subscribes to music queue store via selectors which
// already gate renders. Memo guards against root-layout re-renders
// from unrelated state changes elsewhere.
export const MusicMiniPlayer = memo(MusicMiniPlayerImpl);

const styles = StyleSheet.create({
  root: {
    position: "absolute",
    bottom: 56,
    left: 0,
    right: 0,
    borderTopWidth: StyleSheet.hairlineWidth,
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  seekRow: {
    flexDirection: "row",
    alignItems: "center",
    marginBottom: 2,
  },
  seekBarContainer: {
    flex: 1,
    height: 16,
    justifyContent: "center",
  },
  timeLabel: {
    fontSize: FONT_SIZE.xs,
    minWidth: 34,
    textAlign: "center",
  },
  progressTrack: {
    height: 2,
    borderRadius: 1,
    overflow: "hidden",
  },
  progressFill: {
    height: 2,
    borderRadius: 1,
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
  },
  cover: {
    width: 40,
    height: 40,
    borderRadius: 4,
    alignItems: "center",
    justifyContent: "center",
  },
});
