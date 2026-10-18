import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import {
  View,
  Text,
  Pressable,
  StatusBar,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useLocalSearchParams, router } from "expo-router";
import { VideoView } from "expo-video";
import { useSourcedVideoPlayer } from "@evey/client/native/useSourcedVideoPlayer";
import Ionicons from "@expo/vector-icons/Ionicons";
import * as ScreenOrientation from "expo-screen-orientation";
import { useTranslation } from "react-i18next";
import { formatTime } from "@evey/api";
import type { V1StreamInfo } from "@evey/api";
import { useTheme } from "../../src/providers/ThemeProvider";
import { useServerUrl } from "../../src/hooks/useServerUrl";
import { buildStreamUrl } from "../../src/hooks/usePlaySession";
import { useSyncPlay } from "../../src/hooks/useSyncPlay";
import { SyncPlayBar } from "../../src/components/SyncPlayBar";
import { useBarGesture } from "../../src/hooks/useBarGesture";
import { readGuestSession, clearGuestSession } from "../../src/features/syncplay/guestSession";

// GuestWatch is the PUBLIC (unauthenticated) "watch together" guest player. A
// guest reaches it after entering a room code on the login page: it plays the
// shared media with the media-scoped guest stream token (streaming-only, forced
// transcode - the guest can't run the JWT-gated GetStreamInfo, so we skip the
// decision and let the server transcode), and joins the sync room via the guest
// ws path. It is intentionally bare - no back button, no download, no episodes,
// no navigation: the guest is confined to this one title. Mirror of the web
// GuestWatch (watch.$mediaId.lazy.tsx).
export default function GuestWatch() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const mediaId = id ?? "";
  const theme = useTheme();
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();
  const serverUrl = useServerUrl();
  const guest = useMemo(() => readGuestSession(mediaId), [mediaId]);
  const joinedRef = useRef(false);
  const mountedRef = useRef(true);

  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  const [isBuffering, setIsBuffering] = useState(false);
  const [showControls, setShowControls] = useState(true);
  const seekingRef = useRef(false);
  const [seekPreviewPct, setSeekPreviewPct] = useState<number | null>(null);
  const seekPctRef = useRef(0);
  const seekBar = useBarGesture((pct) => {
    seekPctRef.current = pct;
    setSeekPreviewPct(pct);
  });

  // A guest can't reach GetStreamInfo, so force the transcode path (always
  // works): the HLS master serves the whole timeline and the guest token
  // authenticates every segment fetch. buildStreamUrl only reads playMethod.
  const streamInfo = useMemo(() => ({ playMethod: "PLAY_METHOD_TRANSCODE" }) as V1StreamInfo, []);
  const videoSource = useMemo(() => {
    if (!guest || !serverUrl) return null;
    const uri = buildStreamUrl(serverUrl, mediaId, streamInfo, guest.token);
    return { uri, contentType: "hls" as const, headers: { Authorization: `Bearer ${guest.token}` } };
  }, [guest, serverUrl, mediaId, streamInfo]);

  // No guest token or server yet means no native player: see
  // useSourcedVideoPlayer for why a sourceless one is unsafe.
  const player = useSourcedVideoPlayer(videoSource, (p) => {
    p.timeUpdateEventInterval = 0.5;
    p.play();
  });
  const playerRef = useRef(player);
  useEffect(() => { playerRef.current = player; }, [player]);

  const syncPlay = useSyncPlay({
    guestToken: guest?.token,
    onPlay: (posMs) => { const p = playerRef.current; if (p) { p.currentTime = posMs / 1000; p.play(); } },
    onPause: (posMs) => { const p = playerRef.current; if (p) { p.currentTime = posMs / 1000; p.pause(); } },
    onSeek: (posMs) => { const p = playerRef.current; if (p) { p.currentTime = posMs / 1000; } },
  });

  // No guest session for this media (deep link, app restart dropped the in-memory
  // token): there is nothing to play, send them back to the login to re-enter the
  // code.
  useEffect(() => {
    if (!guest) {
      router.replace("/login");
      return;
    }
    // Join exactly once - a second connect() would close the first ws before it
    // finished handshaking.
    if (joinedRef.current) return;
    joinedRef.current = true;
    syncPlay.joinRoom(guest.roomId);
  }, [guest, syncPlay]);

  useEffect(() => {
    ScreenOrientation.unlockAsync().catch((err) => console.warn("orientation unlock failed", err));
    return () => {
      ScreenOrientation.lockAsync(ScreenOrientation.OrientationLock.DEFAULT)
        .catch((err) => console.warn("orientation restore failed", err));
    };
  }, []);

  useEffect(() => { return () => { mountedRef.current = false; }; }, []);

  useEffect(() => {
    if (!player) return;
    const timeSub = player.addListener("timeUpdate", (ev) => {
      if (!mountedRef.current || seekingRef.current) return;
      setCurrentTime(ev.currentTime);
      try { setDuration(player.duration); } catch (err) { console.warn("released", err); }
    });
    const statusSub = player.addListener("statusChange", (ev) => {
      if (!mountedRef.current) return;
      try { setIsPlaying(player.playing); setDuration(player.duration); } catch (err) { console.warn("released", err); }
      setIsBuffering(ev.status === "loading");
    });
    const playSub = player.addListener("playingChange", () => {
      if (!mountedRef.current) return;
      try { setIsPlaying(player.playing); } catch (err) { console.warn("released", err); }
    });
    return () => { timeSub.remove(); statusSub.remove(); playSub.remove(); };
  }, [player]);

  const togglePlayPause = useCallback(() => {
    if (!player) return;
    if (player.playing) {
      player.pause();
      if (syncPlay.state.connected) syncPlay.sendPause(player.currentTime * 1000);
    } else {
      player.play();
      if (syncPlay.state.connected) syncPlay.sendPlay(player.currentTime * 1000);
    }
  }, [player, syncPlay]);

  const seekTo = useCallback((pct: number) => {
    if (!player || duration <= 0) return;
    const targetSec = pct * duration;
    setCurrentTime(targetSec);
    player.currentTime = targetSec;
    if (syncPlay.state.connected) syncPlay.sendSeek(targetSec * 1000);
  }, [player, duration, syncPlay]);

  const handleLeave = useCallback(() => {
    void syncPlay.leaveRoom();
    clearGuestSession();
    router.replace("/login");
  }, [syncPlay]);

  if (!guest || !player) return null;

  const progress = duration > 0 ? (seekPreviewPct ?? currentTime / duration) : 0;

  return (
    <View style={styles.root}>
      <StatusBar hidden />
      <Pressable style={StyleSheet.absoluteFill} onPress={() => setShowControls((s) => !s)}>
        {videoSource && (
          <VideoView
            style={StyleSheet.absoluteFill}
            player={player}
            contentFit="contain"
            nativeControls={false}
          />
        )}
      </Pressable>

      {isBuffering && (
        <View style={styles.centerOverlay} pointerEvents="none">
          <ActivityIndicator size="large" color="#fff" />
        </View>
      )}

      {showControls && (
        <>
          <View style={[styles.centerOverlay]} pointerEvents="box-none">
            <Pressable
              onPress={togglePlayPause}
              hitSlop={16}
              accessibilityRole="button"
              accessibilityLabel={isPlaying ? t("player.controls.pause", "Pause") : t("player.controls.play", "Play")}
              style={styles.playButton}
            >
              <Ionicons name={isPlaying ? "pause" : "play"} size={40} color="#fff" />
            </Pressable>
          </View>

          <View style={[styles.bottomBlock, { paddingBottom: insets.bottom }]} pointerEvents="box-none">
            <View style={styles.seekRow}>
              <Text style={styles.time}>{formatTime(currentTime)}</Text>
              <View
                style={styles.seekBar}
                {...seekBar.barProps}
                onResponderGrant={(e) => {
                  seekingRef.current = true;
                  seekBar.barProps.onResponderGrant(e);
                }}
                onResponderRelease={(e) => {
                  seekTo(seekBar.fractionAt(e) ?? seekPctRef.current);
                  setSeekPreviewPct(null);
                  seekingRef.current = false;
                }}
                onResponderTerminate={() => { setSeekPreviewPct(null); seekingRef.current = false; }}
              >
                <View style={styles.seekTrack}>
                  <View style={[styles.seekFill, { flex: progress, backgroundColor: theme.colors.accent }]} />
                  <View style={{ flex: 1 - progress }} />
                </View>
                <View style={[styles.seekThumb, { left: `${progress * 100}%`, backgroundColor: theme.colors.accent }]} />
              </View>
              <Text style={styles.time}>{formatTime(duration)}</Text>
            </View>
          </View>
        </>
      )}

      {syncPlay.state.roomId && (
        <View style={{ position: "absolute", left: 0, right: 0, top: insets.top }} pointerEvents="box-none">
          <SyncPlayBar
            roomCode={syncPlay.state.roomCode}
            isHost={syncPlay.state.isHost}
            members={syncPlay.state.members}
            connected={syncPlay.state.connected}
            onLeave={handleLeave}
          />
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: "#000" },
  centerOverlay: { ...StyleSheet.absoluteFillObject, alignItems: "center", justifyContent: "center" },
  playButton: {
    width: 72,
    height: 72,
    borderRadius: 36,
    backgroundColor: "rgba(0,0,0,0.45)",
    alignItems: "center",
    justifyContent: "center",
  },
  bottomBlock: { position: "absolute", left: 0, right: 0, bottom: 0, paddingHorizontal: 16 },
  seekRow: { flexDirection: "row", alignItems: "center", gap: 8 },
  time: { color: "#fff", fontSize: 12, fontVariant: ["tabular-nums"], minWidth: 44, textAlign: "center" },
  seekBar: { flex: 1, height: 32, justifyContent: "center", overflow: "visible" },
  seekTrack: { height: 4, borderRadius: 2, backgroundColor: "rgba(255,255,255,0.3)", flexDirection: "row", overflow: "hidden" },
  seekFill: {},
  seekThumb: { position: "absolute", top: 10, width: 14, height: 14, borderRadius: 7, marginLeft: -7 },
});
