// Globally-mounted music player. Subscribes to useMusicQueueStore and uses
// expo-av's Audio.Sound to play the current track. configureAudioSession
// (called at app boot from RootLayout) sets staysActiveInBackground=true so
// the audio keeps playing once the user locks the screen or switches apps.
//
// Web has GlobalAudioPlayer.tsx; this is the RN equivalent. Native modules
// only — Expo Go on Android plays audio while the app is in the background
// out of the box once the audio session is configured.
import { useCallback, useEffect, useRef, useState } from "react";
import { View, Text, Pressable, StyleSheet, ActivityIndicator } from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { useAuthStore } from "@evey/store";
import { useMusicQueueStore } from "../stores/musicQueue";
import { useTheme } from "../providers/ThemeProvider";
import { useServerUrl } from "../hooks/useServerUrl";

interface AudioModule {
  Sound: {
    createAsync: (
      source: { uri: string },
      initial?: { shouldPlay?: boolean; volume?: number },
      onStatus?: (status: PlaybackStatus) => void,
    ) => Promise<{ sound: Sound }>;
  };
}

interface PlaybackStatus {
  isLoaded?: boolean;
  isPlaying?: boolean;
  didJustFinish?: boolean;
  positionMillis?: number;
  durationMillis?: number;
  error?: string;
}

interface Sound {
  unloadAsync: () => Promise<void>;
  playAsync: () => Promise<void>;
  pauseAsync: () => Promise<void>;
  setStatusAsync: (status: { positionMillis?: number; volume?: number; shouldPlay?: boolean }) => Promise<void>;
  setOnPlaybackStatusUpdate: (cb: (status: PlaybackStatus) => void) => void;
}

let cachedAudio: AudioModule | null = null;
function loadAudio(): AudioModule | null {
  if (cachedAudio) return cachedAudio;
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const mod = require("expo-av") as { Audio?: AudioModule };
    if (mod.Audio) {
      cachedAudio = mod.Audio;
      return cachedAudio;
    }
  } catch {
    // not installed (test env)
  }
  return null;
}

export function MusicMiniPlayer() {
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

  const soundRef = useRef<Sound | null>(null);
  const trackIdRef = useRef<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [position, setPosition] = useState(0);
  const [duration, setDuration] = useState(0);

  const track = currentIndex >= 0 && currentIndex < queue.length ? queue[currentIndex] : null;

  // Load + play whenever the active track changes.
  useEffect(() => {
    if (!track || !serverUrl || !accessToken) return;
    if (trackIdRef.current === track.id) return;
    trackIdRef.current = track.id;
    const Audio = loadAudio();
    if (!Audio) return;

    let cancelled = false;
    setLoading(true);
    setPosition(0);
    setDuration((track.duration ?? 0) * 1000);

    (async () => {
      try {
        if (soundRef.current) {
          try { await soundRef.current.unloadAsync(); } catch { /* ignore */ }
          soundRef.current = null;
        }
        const tokenParam = `?token=${encodeURIComponent(accessToken)}`;
        const uri = `${serverUrl}/api/v1/music/tracks/${track.id}/stream${tokenParam}`;
        const { sound } = await Audio.Sound.createAsync(
          { uri },
          { shouldPlay: true, volume: 1 },
        );
        if (cancelled) {
          try { await sound.unloadAsync(); } catch { /* ignore */ }
          return;
        }
        soundRef.current = sound;
        sound.setOnPlaybackStatusUpdate((status) => {
          if (!status.isLoaded) return;
          if (typeof status.positionMillis === "number") setPosition(status.positionMillis);
          if (typeof status.durationMillis === "number" && status.durationMillis > 0) {
            setDuration(status.durationMillis);
          }
          if (status.didJustFinish) next();
        });
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => { cancelled = true; };
  }, [track, serverUrl, accessToken, next]);

  // Mirror isPlaying state changes onto the sound.
  useEffect(() => {
    const sound = soundRef.current;
    if (!sound) return;
    (async () => {
      try {
        if (isPlayingState) await sound.playAsync();
        else await sound.pauseAsync();
      } catch { /* sound unloaded — next track-change effect will replace it */ }
    })();
  }, [isPlayingState]);

  // Unload on unmount.
  useEffect(() => {
    return () => {
      const sound = soundRef.current;
      if (sound) { sound.unloadAsync().catch(() => undefined); }
      soundRef.current = null;
    };
  }, []);

  const onClose = useCallback(() => {
    const sound = soundRef.current;
    if (sound) { sound.unloadAsync().catch(() => undefined); }
    soundRef.current = null;
    trackIdRef.current = null;
    clear();
  }, [clear]);

  if (!track) return null;
  const progress = duration > 0 ? position / duration : 0;
  const artUri = serverUrl
    ? `${serverUrl}/api/v1/music/tracks/${track.id}/art?token=${encodeURIComponent(accessToken ?? "")}`
    : undefined;

  return (
    <View style={[styles.root, { backgroundColor: theme.colors.surfaceRaised, borderTopColor: theme.colors.border }]} accessibilityLabel="Music player">
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
        <Pressable onPress={togglePlay} hitSlop={8} accessibilityLabel={isPlayingState ? "Pause" : "Play"} accessibilityRole="button" style={{ marginHorizontal: 10 }}>
          {loading ? (
            <ActivityIndicator size="small" color={theme.colors.text} />
          ) : (
            <Ionicons name={isPlayingState ? "pause" : "play"} size={26} color={theme.colors.text} />
          )}
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
