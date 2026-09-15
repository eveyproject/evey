import { useEffect, useRef, useState } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { useVideoPlayer, VideoView } from "expo-video";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";

// SegmentPlayer is the mobile counterpart of the web segment editor's "Section
// A" mini player + "Section B" timeline: a real native player (ExoPlayer via
// expo-video decodes the MKV/HEVC the browser can't) over the raw stream, with
// mark-start/mark-end from the live playhead and a visual timeline of the
// segments with a moving playhead. The parent owns the segment data and applies
// the marks; this component only reads the playhead and reports taps.

interface Seg { type: string; startMs: number; endMs: number }

export function SegmentPlayer({
  streamUrl,
  durationMsFallback,
  segments,
  selectedType,
  onSelectType,
  onMark,
}: {
  streamUrl: string;
  durationMsFallback: number;
  segments: Seg[];
  selectedType: string | null;
  onSelectType: (type: string) => void;
  onMark: (edge: "start" | "end", ms: number) => void;
}) {
  const theme = useTheme();
  const { t } = useTranslation();
  const s = styles(theme);
  const [currentMs, setCurrentMs] = useState(0);
  const [durationMs, setDurationMs] = useState(durationMsFallback);
  const playerRef = useRef<ReturnType<typeof useVideoPlayer> | null>(null);

  const player = useVideoPlayer(streamUrl, (p) => {
    p.pause(); // don't auto-play the whole title in an admin editor
  });
  playerRef.current = player;

  useEffect(() => {
    const sub = player.addListener("timeUpdate", (ev) => {
      setCurrentMs((ev.currentTime ?? 0) * 1000);
      try { if (player.duration > 0) setDurationMs(player.duration * 1000); } catch { /* released */ }
    });
    return () => sub.remove();
  }, [player]);

  const total = durationMs > 0 ? durationMs : durationMsFallback;
  const pct = (ms: number) => (total > 0 ? Math.min(100, Math.max(0, (ms / total) * 100)) : 0);

  const seekTo = (ms: number) => { try { player.currentTime = ms / 1000; } catch { /* released */ } };
  const markNow = (edge: "start" | "end") => { try { onMark(edge, Math.round(player.currentTime * 1000)); } catch { /* released */ } };

  return (
    <View style={{ gap: 10 }}>
      <VideoView player={player} style={s.video} contentFit="contain" nativeControls />

      <View style={{ flexDirection: "row", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <Pressable onPress={() => markNow("start")} disabled={!selectedType} style={[s.markBtn, { borderColor: theme.colors.border, opacity: selectedType ? 1 : 0.4 }]}>
          <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>{t("admin.segments.markStart", "Mark start")}</Text>
        </Pressable>
        <Pressable onPress={() => markNow("end")} disabled={!selectedType} style={[s.markBtn, { borderColor: theme.colors.border, opacity: selectedType ? 1 : 0.4 }]}>
          <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>{t("admin.segments.markEnd", "Mark end")}</Text>
        </Pressable>
        {!selectedType ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("admin.segments.selectHint", "Select a segment to mark")}</Text>
        ) : null}
      </View>

      {/* Visual timeline: one bar per segment over the duration + a live playhead. */}
      <View style={[s.track, { backgroundColor: theme.colors.surfaceRaised }]}>
        {segments.map((seg) => {
          const active = seg.type === selectedType;
          const left = pct(seg.startMs);
          const width = Math.max(1, pct(seg.endMs) - left);
          return (
            <Pressable
              key={seg.type}
              onPress={() => { onSelectType(seg.type); seekTo(seg.startMs); }}
              style={{ position: "absolute", left: `${left}%`, width: `${width}%`, top: 0, bottom: 0, backgroundColor: active ? theme.colors.accent : theme.colors.accent + "55", borderRadius: 3 }}
            />
          );
        })}
        <View style={{ position: "absolute", left: `${pct(currentMs)}%`, top: -3, bottom: -3, width: 2, backgroundColor: theme.colors.text }} />
      </View>
      <View style={{ flexDirection: "row", justifyContent: "space-between" }}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>{fmt(currentMs)}</Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>{fmt(total)}</Text>
      </View>
    </View>
  );
}

function fmt(ms: number): string {
  const s = Math.floor(ms / 1000);
  return `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
}

const styles = (theme: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    video: { width: "100%", aspectRatio: 16 / 9, borderRadius: theme.radii.md, backgroundColor: "#000" },
    markBtn: { borderWidth: 1, borderRadius: theme.radii.md, paddingHorizontal: 14, paddingVertical: 8 },
    track: { height: 10, borderRadius: 3, marginTop: 4 },
  });
