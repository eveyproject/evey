// Globally-mounted TV music player. Subscribes to useMusicQueueStore and drives
// an expo-audio AudioPlayer for the active track. This is the TV equivalent of
// the mobile MusicMiniPlayer / web GlobalAudioPlayer: the search screen's audio
// scope enqueues tracks here. Controls are focusable for D-pad navigation.
import { memo, useCallback, useEffect, useRef, useState } from "react";
import { View, Text, Pressable, Image, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useAudioPlayer, useAudioPlayerStatus } from "expo-audio";
import { activateKeepAwakeAsync, deactivateKeepAwake } from "expo-keep-awake";
import { useTranslation } from "react-i18next";
import { getFreshAccessToken } from "@evey/api";
import { useMusicQueueStore } from "../stores/musicQueue";
import { useTheme } from "../providers/ThemeProvider";
import { useServerUrl } from "../hooks/useServerUrl";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";
import { coverArtUri } from "../lib/artwork";

// Music has no video surface to hold the screen the way expo-video does, so
// without this the box sees an idle user, starts its screensaver and then
// sleeps, and the HDMI output (and the sound with it) goes away mid-album.
const KEEP_AWAKE_TAG = "tv-music";

// The bar's height (padding 11, progress 3 + 10, row 56, padding 11, hairline),
// what the screen above it and the home shortcut keep clear while it shows.
// It floats over everything at the bottom edge: without the inset the home
// button sat on the cover art and an album's last rows hid under it.
export const TV_MUSIC_BAR_HEIGHT = 92;

function FocusCtrl({
  name,
  size,
  onPress,
  label,
  hasTVPreferredFocus,
  active,
}: {
  name: keyof typeof Ionicons.glyphMap;
  size: number;
  onPress: () => void;
  label: string;
  hasTVPreferredFocus?: boolean;
  // A toggle that is on (shuffle): drawn in the accent when not focused.
  active?: boolean;
}) {
  const theme = useTheme();
  const [focused, setFocused] = useState(false);
  return (
    <Pressable
      onPress={onPress}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
      hasTVPreferredFocus={hasTVPreferredFocus}
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={active === undefined ? undefined : { selected: active }}
      style={{
        padding: 10,
        marginHorizontal: 6,
        borderRadius: 8,
        backgroundColor: focused ? theme.colors.accent : "transparent",
      }}
    >
      <Ionicons name={name} size={size} color={focused ? theme.colors.onAccent : active ? theme.colors.accent : theme.colors.text} />
    </Pressable>
  );
}

function TVMusicPlayerImpl() {
  const theme = useTheme();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();

  const queue = useMusicQueueStore((s) => s.queue);
  const currentIndex = useMusicQueueStore((s) => s.currentIndex);
  const isPlayingState = useMusicQueueStore((s) => s.isPlaying);
  const togglePlay = useMusicQueueStore((s) => s.togglePlay);
  const shuffle = useMusicQueueStore((s) => s.shuffle);
  const toggleShuffle = useMusicQueueStore((s) => s.toggleShuffle);
  const next = useMusicQueueStore((s) => s.next);
  const previous = useMusicQueueStore((s) => s.previous);
  const clear = useMusicQueueStore((s) => s.clear);

  const track = currentIndex >= 0 && currentIndex < queue.length ? queue[currentIndex] : null;
  const trackId = track?.id ?? null;

  // One player for the queue's lifetime; every track goes through replace().
  const player = useAudioPlayer(undefined, { updateInterval: 500 });
  const status = useAudioPlayerStatus(player);

  // The stream token is fetched fresh at every track change. The store's
  // token is what the last API call left behind, and an album is long enough
  // for it to expire between two tracks with nobody touching the remote: the
  // next track's stream then 401s and the music stops (prod logs, 21-09,
  // `auth rejected invalid_token` on /music/tracks/{id}/stream by bursts of
  // six ExoPlayer retries). The player is fed from the promise, imperatively:
  // measured on the emulator, a render queued from that promise while the
  // app is in the background only commits on resume, and the next track
  // stayed silent until then. The pinned copy only feeds the cover art. A
  // refresh mid-track never touches the source, so the song never restarts.
  const [pinned, setPinned] = useState<{ trackId: string; token: string } | null>(null);
  useEffect(() => {
    if (!trackId || !serverUrl) return;
    let live = true;
    getFreshAccessToken().then((token) => {
      if (!live || !token) return;
      setPinned({ trackId, token });
      try {
        player.replace(`${serverUrl}/api/v1/music/tracks/${trackId}/stream?token=${encodeURIComponent(token)}`);
        if (useMusicQueueStore.getState().isPlaying) player.play();
      } catch { /* released */ }
    }).catch(() => { /* signed out: the 401 path signs the app out */ });
    return () => { live = false; };
  }, [trackId, serverUrl, player]);
  const token = pinned && pinned.trackId === trackId ? pinned.token : null;

  useEffect(() => {
    try {
      // No track means the queue was cleared - by the close button, or by a
      // film opening (the video player clears the queue). The player keeps
      // its last source, so this is the pause that actually silences it:
      // left to the store alone, the bar vanished and the album played on
      // under the film.
      if (!trackId || !isPlayingState) player.pause();
      else player.play();
    } catch { /* released */ }
  }, [isPlayingState, player, trackId]);

  // Follows the bytes actually flowing (paused or finished releases the
  // screen), not the store's intention.
  const audible = !!status.playing;
  useEffect(() => {
    if (!audible) return;
    activateKeepAwakeAsync(KEEP_AWAKE_TAG).catch(() => { /* module absent */ });
    return () => { deactivateKeepAwake(KEEP_AWAKE_TAG).catch(() => { /* module absent */ }); };
  }, [audible]);

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
    clear();
  }, [clear, player]);

  if (!track) return null;

  const duration = (status.duration ?? track.duration ?? 0);
  const position = status.currentTime ?? 0;
  const progress = duration > 0 ? position / duration : 0;
  // 56px slot, 2x: a display-sized WebP, not the full cover embedded in the
  // file (the server resizes and caches it).
  const artUri = (serverUrl && token)
    ? coverArtUri(serverUrl, `/api/v1/music/tracks/${track.id}/art?token=${encodeURIComponent(token)}`, 112)
    : undefined;

  return (
    <View style={[styles.root, { backgroundColor: theme.colors.surfaceRaised, borderTopColor: theme.colors.border }]}>
      <View style={[styles.progressTrack, { backgroundColor: theme.colors.border }]}>
        <View style={[styles.progressFill, { width: `${Math.max(0, Math.min(100, progress * 100))}%`, backgroundColor: theme.colors.accent }]} />
      </View>
      <View style={styles.row}>
        <View style={[styles.cover, { backgroundColor: theme.colors.surface }]}>
          {artUri ? (
            <Image source={{ uri: artUri }} style={{ width: 56, height: 56, borderRadius: 6 }} resizeMode="cover" />
          ) : (
            <Ionicons name="musical-notes" size={ICON_SIZE.xl} color={theme.colors.textMuted} />
          )}
        </View>
        <View style={{ flex: 1, marginLeft: 16, minWidth: 0 }}>
          <Text numberOfLines={1} style={{ color: theme.colors.text, fontSize: FONT_SIZE.lg, fontWeight: "600" }}>{track.title || "-"}</Text>
          <Text numberOfLines={1} style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.sm }}>
            {[track.artist, track.album].filter(Boolean).join(" · ") || "-"}
          </Text>
        </View>
        {/* Shuffle, the mode the phone and the browser already had. */}
        <FocusCtrl name="shuffle" size={ICON_SIZE.xl} onPress={toggleShuffle} label={t("media.music.shuffle", "Shuffle")} active={shuffle} />
        <FocusCtrl name="play-skip-back" size={ICON_SIZE.xl} onPress={previous} label={t("common.previous")} />
        <FocusCtrl name={isPlayingState ? "pause" : "play"} size={ICON_SIZE["2xl"]} onPress={togglePlay} label={isPlayingState ? t("common.pause") : t("common.play")} hasTVPreferredFocus />
        <FocusCtrl name="play-skip-forward" size={ICON_SIZE.xl} onPress={next} label={t("common.next")} />
        <FocusCtrl name="close" size={ICON_SIZE.xl} onPress={onClose} label={t("common.close")} />
      </View>
    </View>
  );
}

export const TVMusicPlayer = memo(TVMusicPlayerImpl);

const styles = StyleSheet.create({
  root: {
    position: "absolute",
    bottom: 0,
    left: 0,
    right: 0,
    borderTopWidth: StyleSheet.hairlineWidth,
    paddingHorizontal: 32,
    paddingVertical: 11,
  },
  progressTrack: { height: 3, borderRadius: 2, overflow: "hidden", marginBottom: 10 },
  progressFill: { height: 3, borderRadius: 2 },
  row: { flexDirection: "row", alignItems: "center" },
  cover: { width: 56, height: 56, borderRadius: 6, alignItems: "center", justifyContent: "center" },
});
