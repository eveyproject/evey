import { useEffect, useMemo, useRef, useState } from "react";
import { View, Text, Pressable, Modal, ScrollView, ActivityIndicator, StyleSheet } from "react-native";
import { useVideoPlayer, VideoView, type VideoPlayerStatus } from "expo-video";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  apiErrorMessage,
  getAdminMediaServiceGetMediaRepairHistoryQueryKey,
  getAdminMediaServiceListMediaRepairsQueryKey,
  SEEK_INDEX_SCAN_QUERY_BASE,
  useAdminMediaServiceApplyAudioShift,
  useLibraryServiceGetMediaItem,
  type V1SeekIndexItem,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../../../providers/ThemeProvider";
import { useServerUrl } from "../../../hooks/useServerUrl";

// Mobile port of the web SyncPreviewPanel: watch a server-rendered clip to
// judge an A/V desync by ear, try a candidate audio offset, apply it. The clip
// is rendered server-side (4K HEVC/MKV) and streamed to the native player with
// the admin bearer as a header (a plain <video src> couldn't send it on web -
// on mobile expo-video can, so no blob round-trip). Continuous sliders are
// replaced by nudge buttons + the file's measured offsets as one-click starts.

const SHIFT_MAX_SEC = 60;
const CLIP_LENGTHS_SEC = [10, 20, 45];
const SPEEDS = [0.25, 0.5, 1];
const AT_NUDGES = [-60, -10, -1, 1, 10, 60];
const SHIFT_NUDGES = [-1, -0.1, -0.05, 0.05, 0.1, 1];
const FALLBACK_FPS = 25;

const clamp = (v: number, lo: number, hi: number) => (v < lo ? lo : v > hi ? hi : v);
const signed = (v: number) => `${v > 0 ? "+" : ""}${Number(v.toFixed(2))}`;
function hhmmss(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), r = s % 60;
  const mm = String(m).padStart(2, "0"), ss = String(r).padStart(2, "0");
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

export function suggestedShifts(item: Pick<V1SeekIndexItem, "audioTracksLagSec" | "audioDelaySec" | "audioShortfallSec">): number[] {
  const raw = [Number(item.audioTracksLagSec ?? 0), Number(item.audioDelaySec ?? 0), Number(item.audioShortfallSec ?? 0)];
  const out: number[] = [];
  for (const v of raw) {
    const mag = Math.round(Math.abs(v) * 100) / 100;
    if (mag < 0.05 || mag > SHIFT_MAX_SEC) continue;
    for (const c of [-mag, mag]) if (!out.includes(c)) out.push(c);
  }
  return out.sort((a, b) => a - b);
}

export function SyncPreviewModal({ item, title, onClose }: { item: V1SeekIndexItem; title: string; onClose: () => void }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const s = styles(theme);
  const mediaId = item.mediaId ?? "";
  const serverUrl = useServerUrl();
  const accessToken = useAuthStore((st) => st.accessToken);

  const [atSec, setAtSec] = useState(600);
  const [lenSec, setLenSec] = useState(20);
  const [track, setTrack] = useState(0);
  const [shift, setShift] = useState(0);
  const [speed, setSpeed] = useState(1);
  const [rendering, setRendering] = useState(false);
  const [shown, setShown] = useState<{ at: number; shift: number; track: number } | null>(null);
  const [error, setError] = useState<string | null>(null);

  const queryClient = useQueryClient();
  // The failure is said inline below, so the global net must not toast it too.
  // A shift is a repair: the file's history, the library journal and the
  // seek-index row all changed.
  const applyShift = useAdminMediaServiceApplyAudioShift({
    mutation: {
      meta: { silentError: true },
      onSuccess: () => {
        void queryClient.invalidateQueries({ queryKey: getAdminMediaServiceGetMediaRepairHistoryQueryKey(mediaId) });
        void queryClient.invalidateQueries({ queryKey: getAdminMediaServiceListMediaRepairsQueryKey() });
        void queryClient.invalidateQueries({ queryKey: [SEEK_INDEX_SCAN_QUERY_BASE] });
      },
    },
  });
  const { data: media } = useLibraryServiceGetMediaItem(mediaId, { query: { enabled: mediaId !== "", staleTime: 5 * 60_000, retry: false } });
  const durationSec = Math.floor(Number(media?.durationMs ?? 0) / 1000);
  const audioStreams = useMemo(() => (media?.streams ?? []).filter((x) => x.type === "MEDIA_STREAM_TYPE_AUDIO"), [media]);
  const videoFps = useMemo(() => {
    const v = (media?.streams ?? []).find((x) => x.type === "MEDIA_STREAM_TYPE_VIDEO");
    const fps = Number(v?.frameRate ?? 0);
    return fps > 0 ? fps : FALLBACK_FPS;
  }, [media]);
  const suggestions = useMemo(() => suggestedShifts(item), [item]);
  const trackCount = audioStreams.length > 0 ? audioStreams.length : 4;

  const player = useVideoPlayer(null, (p) => { p.pause(); });

  useEffect(() => { if (durationSec > 0 && atSec > durationSec) setAtSec(Math.floor(durationSec / 2)); }, [durationSec, atSec]);
  useEffect(() => { try { player.playbackRate = speed; } catch { /* released */ } }, [speed, player]);

  // On Android replaceAsync resolves as soon as the source is handed to
  // ExoPlayer, before a byte of the clip is fetched: a refused render (4xx/5xx
  // from /sync-preview) only ever arrives here, as an "error" status. The
  // spinner therefore stays up until the player says it loaded or failed.
  const settle = (status: VideoPlayerStatus) => {
    if (status === "readyToPlay") setRendering(false);
    if (status === "error") {
      setRendering(false);
      setShown(null);
      setError(t("admin.syncPreview.renderFailed", "Could not render the clip"));
    }
  };
  const settleRef = useRef(settle);
  settleRef.current = settle;
  useEffect(() => {
    const sub = player.addListener("statusChange", (ev) => settleRef.current(ev.status));
    return () => sub.remove();
  }, [player]);

  async function render() {
    if (!mediaId) return;
    setRendering(true);
    setError(null);
    const params = new URLSearchParams({ at: String(atSec), len: String(lenSec), track: String(track), shift: String(shift) });
    const uri = `${serverUrl}/api/v1/admin/media/${encodeURIComponent(mediaId)}/sync-preview?${params.toString()}`;
    try {
      await player.replaceAsync({ uri, headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : undefined });
      setShown({ at: atSec, shift, track });
      try { player.play(); } catch { /* released */ }
      // iOS may have settled before the promise did; its statusChange is gone.
      settle(player.status);
    } catch (e) {
      setError(apiErrorMessage(e as Error, t, t("admin.syncPreview.renderFailed", "Could not render the clip")));
      setRendering(false);
    }
  }

  const stepFrame = (dir: 1 | -1) => { try { player.pause(); player.currentTime = clamp(player.currentTime + dir / videoFps, 0, lenSec); } catch { /* released */ } };
  const stale = shown !== null && (shown.at !== atSec || shown.shift !== shift || shown.track !== track);

  const trackLabel = (n: number) => {
    const st = audioStreams[n];
    if (!st) return t("admin.syncPreview.trackN", { n: n + 1 });
    const bits = [st.language, st.codec?.toUpperCase(), st.channels ? `${st.channels}ch` : "", st.title].filter(Boolean).join(" · ");
    return bits ? `${n + 1} - ${bits}` : t("admin.syncPreview.trackN", { n: n + 1 });
  };

  const nudge = (label: string, onPress: () => void) => (
    <Pressable key={label} onPress={onPress} style={[s.nudge, { borderColor: theme.colors.border }]}>
      <Text style={{ color: theme.colors.text, fontSize: 12, fontVariant: ["tabular-nums"] }}>{label}</Text>
    </Pressable>
  );
  const chip = (label: string, active: boolean, onPress: () => void) => (
    <Pressable key={label} onPress={onPress} style={{ paddingHorizontal: 10, paddingVertical: 5, borderRadius: theme.radii.sm, borderWidth: 1, borderColor: active ? theme.colors.accent : theme.colors.border, backgroundColor: active ? theme.colors.accent : "transparent" }}>
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 12, fontWeight: "600" }}>{label}</Text>
    </Pressable>
  );

  return (
    <Modal visible transparent animationType="slide" onRequestClose={onClose}>
      <Pressable style={{ flex: 1, backgroundColor: "rgba(0,0,0,0.5)" }} onPress={onClose} />
      <View style={[s.sheet, { backgroundColor: theme.colors.surface, borderTopLeftRadius: theme.radii.lg, borderTopRightRadius: theme.radii.lg }]}>
        <View style={{ flexDirection: "row", justifyContent: "space-between", alignItems: "center", gap: 8 }}>
          <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "700", flex: 1 }} numberOfLines={1}>{t("admin.syncPreview.title", { title })}</Text>
          <Pressable onPress={onClose}><Text style={{ color: theme.colors.accent, fontWeight: "600" }}>{t("common.close")}</Text></Pressable>
        </View>

        <ScrollView style={{ flexShrink: 1 }} contentContainerStyle={{ gap: 12, paddingVertical: 8 }} keyboardShouldPersistTaps="handled">
          <Text style={s.hint}>{t("admin.syncPreview.intro", "Watch a few seconds and apply what your ear finds.")}</Text>

          <View style={s.videoWrap}>
            <VideoView player={player} style={{ flex: 1 }} contentFit="contain" nativeControls />
            {rendering ? (
              <View style={s.overlay}><ActivityIndicator color="#fff" /><Text style={{ color: "#fff", fontSize: 12, marginTop: 6 }}>{t("admin.syncPreview.rendering", "Rendering…")}</Text></View>
            ) : !shown ? (
              <View style={s.overlay}><Text style={{ color: "#bbb", fontSize: 12, textAlign: "center", paddingHorizontal: 16 }}>{t("admin.syncPreview.idle", "Set a moment and offset, then Render.")}</Text></View>
            ) : null}
          </View>

          {error ? <Text style={{ color: theme.colors.danger, fontSize: 13 }}>{error}</Text> : null}

          {shown ? (
            <View style={{ flexDirection: "row", flexWrap: "wrap", alignItems: "center", gap: 8 }}>
              <Text style={s.label}>{t("admin.syncPreview.speed", "Speed")}</Text>
              {SPEEDS.map((sp) => chip(`${sp}x`, speed === sp, () => setSpeed(sp)))}
              <Text style={s.label}>{t("admin.syncPreview.frameStep", "Frame")}</Text>
              {nudge("-1", () => stepFrame(-1))}
              {nudge("+1", () => stepFrame(1))}
            </View>
          ) : null}

          {/* Moment */}
          <View style={{ gap: 6 }}>
            <View style={{ flexDirection: "row", justifyContent: "space-between" }}>
              <Text style={s.label}>{t("admin.syncPreview.at", "Moment")}</Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12, fontVariant: ["tabular-nums"] }}>{hhmmss(atSec)}{durationSec > 0 ? ` / ${hhmmss(durationSec)}` : ""}</Text>
            </View>
            <View style={s.nudgeRow}>{AT_NUDGES.map((n) => nudge(signed(n), () => setAtSec((v) => clamp(v + n, 0, durationSec > 0 ? durationSec : v + n))))}</View>
          </View>

          {/* Candidate offset */}
          <View style={{ gap: 6 }}>
            <View style={{ flexDirection: "row", justifyContent: "space-between" }}>
              <Text style={s.label}>{t("admin.syncPreview.shift", "Offset")}</Text>
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", fontVariant: ["tabular-nums"] }}>{signed(shift)} s</Text>
            </View>
            <View style={s.nudgeRow}>
              {SHIFT_NUDGES.map((n) => nudge(signed(n), () => setShift((v) => clamp(Math.round((v + n) * 100) / 100, -SHIFT_MAX_SEC, SHIFT_MAX_SEC))))}
              {nudge(t("admin.syncPreview.shiftReset", "0"), () => setShift(0))}
            </View>
            {suggestions.length > 0 ? (
              <View style={[s.nudgeRow, { alignItems: "center" }]}>
                <Text style={s.hint}>{t("admin.syncPreview.measured", "Measured:")}</Text>
                {suggestions.map((sg) => nudge(`${signed(sg)} s`, () => setShift(sg)))}
              </View>
            ) : null}
          </View>

          {/* Track + clip length */}
          <View style={{ gap: 6 }}>
            <Text style={s.label}>{t("admin.syncPreview.track", "Track")}</Text>
            <View style={s.nudgeRow}>{Array.from({ length: trackCount }, (_, n) => chip(trackLabel(n), track === n, () => setTrack(n)))}</View>
            <Text style={s.label}>{t("admin.syncPreview.clipLength", "Clip length")}</Text>
            <View style={s.nudgeRow}>{CLIP_LENGTHS_SEC.map((n) => chip(t("admin.syncPreview.seconds", { n }), lenSec === n, () => setLenSec(n)))}</View>
          </View>

          <Pressable onPress={() => void render()} disabled={rendering} style={[s.primaryBtn, { backgroundColor: theme.colors.accent, opacity: rendering ? 0.5 : 1 }]}>
            {rendering ? <ActivityIndicator size="small" color={theme.colors.onAccent} /> : <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 15 }}>{t("admin.syncPreview.render", "Render clip")}</Text>}
          </Pressable>
          {stale && !rendering ? <Text style={{ color: theme.colors.warning, fontSize: 12 }}>{t("admin.syncPreview.stale", { shift: signed(shown!.shift), at: hhmmss(shown!.at) })}</Text> : null}

          {/* Apply */}
          <View style={{ borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.border, paddingTop: 10, gap: 6 }}>
            <Pressable
              onPress={() => applyShift.mutate({ mediaId, data: { track: String(track + 1), shiftSec: shift } })}
              disabled={shift === 0 || applyShift.isPending}
              style={[s.primaryBtn, { backgroundColor: theme.colors.accent, opacity: shift === 0 || applyShift.isPending ? 0.5 : 1 }]}
            >
              {applyShift.isPending ? <ActivityIndicator size="small" color={theme.colors.onAccent} /> : <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 15 }}>{t("admin.syncPreview.apply", { shift: signed(shift) })}</Text>}
            </Pressable>
            {applyShift.isError ? <Text style={{ color: theme.colors.danger, fontSize: 13 }}>{apiErrorMessage(applyShift.error as Error, t, t("admin.syncPreview.applyFailed", "Could not apply the shift"))}</Text> : null}
            {applyShift.isSuccess ? <Text style={{ color: theme.colors.success, fontSize: 13 }}>{t("admin.syncPreview.applyDone", "Shift applied.")}</Text> : null}
            <Text style={s.hint}>{t("admin.syncPreview.applyHint", "Your ear is the evidence - the journal records it as such.")}</Text>
          </View>
        </ScrollView>
      </View>
    </Modal>
  );
}

const styles = (theme: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    sheet: { position: "absolute", left: 0, right: 0, bottom: 0, padding: 16, paddingBottom: 24, maxHeight: "92%" },
    hint: { color: theme.colors.textMuted, fontSize: 12 },
    label: { color: theme.colors.text, fontSize: 13, fontWeight: "600" },
    videoWrap: { width: "100%", aspectRatio: 16 / 9, borderRadius: theme.radii.md, backgroundColor: "#000", overflow: "hidden" },
    overlay: { ...StyleSheet.absoluteFillObject, alignItems: "center", justifyContent: "center", backgroundColor: "rgba(0,0,0,0.55)" },
    nudge: { borderWidth: 1, borderRadius: 6, paddingHorizontal: 10, paddingVertical: 6 },
    nudgeRow: { flexDirection: "row", flexWrap: "wrap", gap: 6 },
    primaryBtn: { borderRadius: theme.radii.md, paddingVertical: 12, alignItems: "center" },
  });
