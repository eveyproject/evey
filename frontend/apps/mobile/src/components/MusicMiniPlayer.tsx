// Globally-mounted music player. Subscribes to useMusicQueueStore and
// drives an expo-audio AudioPlayer for the active track. Background
// playback comes from the audio session configured at boot
// (configureAudioSession + iOS UIBackgroundModes:["audio"]).
//
// Web has GlobalAudioPlayer.tsx; this is the RN equivalent. expo-av's
// ExponentAV native module is gone from Expo Go on SDK 55, so we use
// expo-audio's hook-based API instead.
import { memo, useCallback, useEffect, useRef } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { useAudioPlayer, useAudioPlayerStatus } from "expo-audio";
import { useAuthStore } from "@evey/store";
import { useMusicQueueStore } from "../stores/musicQueue";
import { useTheme } from "../providers/ThemeProvider";
import { useServerUrl } from "../hooks/useServerUrl";

function MusicMiniPlayerImpl() {
  const theme = useTheme();
  const serverUrl = useServerUrl();
  const accessToken = useAuthStore((s) => s.accessToken);

  const queue = useMusicQueueStore((s) => s.queue);
  const currentIndex = useMusicQueueStore((s) => s.currentIndex);
  const isPlayingState = useMusicQueueStore((s) => s.isPlaying);
  const togglePlay = useMusicQueueStore((s) => s.togglePlay);
  const next = useMusicQueueStore((s) => s.next);
  const previous = useMusicQueueStore((s) => s.previous);
  const clear = useMusicQueueStore((s) => s.clear);

  const track = currentIndex >= 0 && currentIndex < queue.length ? queue[currentIndex] : null;

  // Build the streaming URI for the active track. Token is appended as a
  // query param because the audio fetch path doesn't carry our Authorization
  // header through to ffmpeg/the streamer.
  const streamUri = (track && serverUrl && accessToken)
    ? `${serverUrl}/api/v1/music/tracks/${track.id}/stream?token=${encodeURIComponent(accessToken)}`
    : null;

  // Single AudioPlayer for the entire queue lifetime. We swap its source via
  // .replace() on track change; creating a new player per track would
  // tear down the audio session and clobber the iOS now-playing card.
  const player = useAudioPlayer(streamUri ?? undefined, { updateInterval: 500 });
  const status = useAudioPlayerStatus(player);

  // Track which URL is currently loaded so we don't redundantly call replace.
  const loadedUriRef = useRef<string | null>(null);

  useEffect(() => {
    if (!streamUri) return;
    if (loadedUriRef.current === streamUri) return;
    loadedUriRef.current = streamUri;
    try {
      player.replace(streamUri);
      if (isPlayingState) player.play();
    } catch {
      // Player not ready yet — the next render will retry.
    }
  }, [streamUri, player, isPlayingState]);

  // Mirror queue store play/pause onto the player.
  useEffect(() => {
    if (!streamUri) return;
    try {
      if (isPlayingState) player.play();
      else player.pause();
    } catch { /* released */ }
  }, [isPlayingState, player, streamUri]);

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

  const onClose = useCallback(() => {
    try { player.pause(); } catch { /* released */ }
    loadedUriRef.current = null;
    clear();
  }, [clear, player]);

  if (!track) return null;

  const duration = (status.duration ?? track.duration ?? 0);
  const position = status.currentTime ?? 0;
  const progress = duration > 0 ? position / duration : 0;
  const artUri = serverUrl
    ? `${serverUrl}/api/v1/music/tracks/${track.id}/art?token=${encodeURIComponent(accessToken ?? "")}`
    : undefined;

  return (
    <View
      style={[styles.root, { backgroundColor: theme.colors.surfaceRaised, borderTopColor: theme.colors.border }]}
      accessibilityLabel="Music player"
    >
      <View style={[styles.progressTrack, { backgroundColor: theme.colors.border }]}>
        <View style={[styles.progressFill, { width: `${Math.max(0, Math.min(100, progress * 100))}%`, backgroundColor: theme.colors.accent }]} />
      </View>
      <View style={styles.row}>
        <View style={styles.cover}>
          {artUri ? (
            <Image source={{ uri: artUri }} style={{ width: 40, height: 40, borderRadius: 4 }} contentFit="cover" />
          ) : (
            <Ionicons name="musical-notes" size={20} color={theme.colors.textMuted} />
          )}
        </View>
        <View style={{ flex: 1, marginLeft: 10, minWidth: 0 }}>
          <Text numberOfLines={1} style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>{track.title || "—"}</Text>
          <Text numberOfLines={1} style={{ color: theme.colors.textMuted, fontSize: 11 }}>
            {[track.artist, track.album].filter(Boolean).join(" · ") || "—"}
          </Text>
        </View>
        <Pressable onPress={previous} hitSlop={8} accessibilityLabel="Previous track" accessibilityRole="button">
          <Ionicons name="play-skip-back" size={22} color={theme.colors.text} />
        </Pressable>
        <Pressable
          onPress={togglePlay}
          hitSlop={8}
          accessibilityLabel={isPlayingState ? "Pause" : "Play"}
          accessibilityRole="button"
          style={{ marginHorizontal: 10 }}
        >
          <Ionicons name={isPlayingState ? "pause" : "play"} size={26} color={theme.colors.text} />
        </Pressable>
        <Pressable onPress={next} hitSlop={8} accessibilityLabel="Next track" accessibilityRole="button">
          <Ionicons name="play-skip-forward" size={22} color={theme.colors.text} />
        </Pressable>
        <Pressable onPress={onClose} hitSlop={8} accessibilityLabel="Close player" accessibilityRole="button" style={{ marginLeft: 10 }}>
          <Ionicons name="close" size={20} color={theme.colors.textMuted} />
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
  progressTrack: {
    height: 2,
    borderRadius: 1,
    overflow: "hidden",
    marginBottom: 6,
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
