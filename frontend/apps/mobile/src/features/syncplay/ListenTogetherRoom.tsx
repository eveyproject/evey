import { useEffect, useRef, useState, type MutableRefObject } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useAudioPlayer, useAudioPlayerStatus, type AudioPlayer } from "expo-audio";
import { getFreshAccessToken, musicServiceGetTrack } from "@evey/api";
import type { RoomQueueItem, SyncPlayState } from "@evey/client";
import { useTheme } from "../../providers/ThemeProvider";
import { useServerUrl } from "../../hooks/useServerUrl";

export interface ListenTogetherControls {
  sendPlay: (ms: number) => void;
  sendPause: (ms: number) => void;
  sendNext: () => void;
  sendPrev: () => void;
  sendSetIndex: (index: number) => void;
}

// The phone's "listen together" room: one expo-audio player on the queue's
// current track. The host drives (play/pause, previous/next, pick in the
// queue, auto-advance at the end of a track) and every command goes through
// the room, so the phone's own player moves only when the room says so. A
// member follows: the screen's room callbacks move this player through
// playerRef.
export function ListenTogetherRoom({
  state,
  controls,
  playerRef,
  trackMeta,
  onLeave,
}: {
  state: SyncPlayState;
  controls: ListenTogetherControls;
  playerRef: MutableRefObject<AudioPlayer | null>;
  trackMeta: MutableRefObject<Map<string, RoomQueueItem>>;
  onLeave: () => void;
}) {
  const theme = useTheme();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const trackId = state.queue[state.trackIndex] ?? "";
  // The room's send functions are rebuilt on every render; effects read them
  // through a ref so a re-render never replays a command.
  const controlsRef = useRef(controls);
  controlsRef.current = controls;

  // The stream token rides the URL (the audio fetch sends no header); it is
  // fetched fresh per track and pinned so a refresh never restarts the song.
  const [pinned, setPinned] = useState<{ trackId: string; token: string } | null>(null);
  useEffect(() => {
    if (!trackId) return;
    let live = true;
    getFreshAccessToken().then((token) => {
      if (live && token) setPinned({ trackId, token });
    }).catch(() => { /* signed out: the 401 path signs the app out */ });
    return () => { live = false; };
  }, [trackId]);
  const streamUri = trackId && serverUrl && pinned?.trackId === trackId
    ? `${serverUrl}/api/v1/music/tracks/${trackId}/stream?token=${encodeURIComponent(pinned.token)}`
    : null;

  const player = useAudioPlayer(streamUri ?? undefined, { updateInterval: 500 });
  const status = useAudioPlayerStatus(player);
  useEffect(() => {
    playerRef.current = player;
    return () => { playerRef.current = null; };
  }, [player, playerRef]);

  // Load the new track when the room moves; the host keeps the room playing
  // across a track change (auto-advance or next/previous).
  const loadedRef = useRef<string | null>(null);
  const wasPlayingRef = useRef(false);
  useEffect(() => {
    if (!streamUri || loadedRef.current === streamUri) return;
    const changed = loadedRef.current !== null;
    loadedRef.current = streamUri;
    player.replace(streamUri);
    if (changed && state.isHost && wasPlayingRef.current) {
      player.play();
      controlsRef.current.sendPlay(0);
    }
  }, [streamUri, player, state.isHost]);
  useEffect(() => { wasPlayingRef.current = !!status.playing; }, [status.playing]);

  // Auto-advance once per finished track. didJustFinish stays true until the
  // next track loads, and the room echoes "next" first: the index moves while
  // the flag is still up, and a second "next" would skip a track. Re-armed
  // once the finish is over.
  const finishHandledRef = useRef(false);
  useEffect(() => {
    if (!status.didJustFinish) {
      finishHandledRef.current = false;
      return;
    }
    if (!state.isHost || finishHandledRef.current) return;
    if (state.trackIndex >= state.queue.length - 1) return;
    finishHandledRef.current = true;
    controlsRef.current.sendNext();
  }, [status.didJustFinish, state.isHost, state.trackIndex, state.queue.length]);

  // Title/artist: the host's picked queue first, then the server by id.
  const [, setMetaTick] = useState(0);
  useEffect(() => {
    if (!trackId || trackMeta.current.has(trackId)) return;
    let live = true;
    musicServiceGetTrack(trackId).then((res) => {
      const tr = res.track;
      if (!live || !tr) return;
      trackMeta.current.set(trackId, { id: trackId, title: tr.title ?? "", artist: tr.artist ?? "" });
      setMetaTick((n) => n + 1);
    }).catch(() => undefined);
    return () => { live = false; };
  }, [trackId, trackMeta]);
  const meta = trackMeta.current.get(trackId);

  const positionMs = Math.floor((status.currentTime ?? 0) * 1000);
  const togglePlay = () => {
    if (status.playing) {
      player.pause();
      controls.sendPause(positionMs);
    } else {
      player.play();
      controls.sendPlay(positionMs);
    }
  };

  return (
    <View style={{ gap: 16 }}>
      <View style={{ gap: 4 }}>
        <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "700" }} numberOfLines={1}>
          {meta?.title || t("media.syncplay.sharedTrack")}
        </Text>
        {!!meta?.artist && <Text style={{ color: theme.colors.textMuted }} numberOfLines={1}>{meta.artist}</Text>}
        {state.isHost && state.roomCode && (
          <Text style={{ color: theme.colors.accent, fontSize: 24, fontWeight: "700", letterSpacing: 4, fontFamily: "monospace" }} testID="room-code">
            {state.roomCode}
          </Text>
        )}
      </View>

      {state.isHost ? (
        <View style={styles.controls}>
          <Pressable onPress={() => controls.sendPrev()} disabled={state.trackIndex <= 0} accessibilityRole="button" accessibilityLabel={t("media.player.previous", "Previous")} hitSlop={8}>
            <Ionicons name="play-skip-back" size={28} color={state.trackIndex <= 0 ? theme.colors.border : theme.colors.text} />
          </Pressable>
          <Pressable onPress={togglePlay} accessibilityRole="button" accessibilityLabel={status.playing ? t("media.player.pause", "Pause") : t("media.player.play", "Play")} testID="room-play-pause" hitSlop={8}>
            <Ionicons name={status.playing ? "pause-circle" : "play-circle"} size={56} color={theme.colors.accent} />
          </Pressable>
          <Pressable onPress={() => controls.sendNext()} disabled={state.trackIndex >= state.queue.length - 1} accessibilityRole="button" accessibilityLabel={t("media.player.next", "Next")} hitSlop={8}>
            <Ionicons name="play-skip-forward" size={28} color={state.trackIndex >= state.queue.length - 1 ? theme.colors.border : theme.colors.text} />
          </Pressable>
        </View>
      ) : (
        <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("media.syncplay.followingHost")}</Text>
      )}

      {state.queue.length > 0 && (
        <View style={{ gap: 2 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
            {t("media.syncplay.queue")} - {state.trackIndex + 1} / {state.queue.length}
          </Text>
          {state.queue.map((id, idx) => {
            const m = trackMeta.current.get(id);
            const current = idx === state.trackIndex;
            return (
              <Pressable
                key={`${id}-${idx}`}
                onPress={() => state.isHost && controls.sendSetIndex(idx)}
                disabled={!state.isHost}
                accessibilityRole="button"
                style={[styles.queueRow, current && { backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm }]}
              >
                <Text style={{ color: theme.colors.textMuted, width: 24, fontSize: 12 }}>{idx + 1}</Text>
                <Text style={{ color: current ? theme.colors.accent : theme.colors.text, flex: 1, fontWeight: current ? "600" : "400" }} numberOfLines={1}>
                  {m?.title || id}
                </Text>
              </Pressable>
            );
          })}
        </View>
      )}

      <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
        <Ionicons name="people" size={16} color={theme.colors.textMuted} />
        <Text style={{ color: theme.colors.textMuted, flex: 1 }}>{state.members.length + 1} {t("media.syncplay.members")}</Text>
        <View style={{ width: 8, height: 8, borderRadius: 4, backgroundColor: state.connected ? theme.colors.success : theme.colors.error }} />
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{state.connected ? t("media.syncplay.connected") : t("media.syncplay.connecting")}</Text>
      </View>

      <Pressable onPress={onLeave} accessibilityRole="button" testID="room-leave" style={[styles.leave, { borderColor: theme.colors.border, borderRadius: theme.radii.md }]}>
        <Ionicons name="exit-outline" size={18} color={theme.colors.text} />
        <Text style={{ color: theme.colors.text }}>{t("media.syncplay.leave")}</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  controls: { flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 32 },
  queueRow: { flexDirection: "row", alignItems: "center", gap: 8, paddingVertical: 6, paddingHorizontal: 6 },
  leave: { flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 8, paddingVertical: 12, borderWidth: StyleSheet.hairlineWidth },
});
