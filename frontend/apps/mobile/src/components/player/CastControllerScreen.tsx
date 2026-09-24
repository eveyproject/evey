import { useRef, useState } from "react";
import { View, Text, Pressable, StatusBar, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { CastButtonNative } from "../../hooks/useChromecast";
import { formatTime } from "@evey/api";
import { useBarGesture } from "../../hooks/useBarGesture";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

interface CastControllerScreenProps {
  title: string;
  deviceName?: string;
  currentTime: number;
  duration: number;
  isPlaying: boolean;
  skipFwd: number;
  skipBwd: number;
  // The receiver's own stream volume (0-1) and mute state, plus setters. When
  // the setters are omitted (device SDK without volume control) the volume row
  // is hidden. Mirrors the web cast controls (PlayerControls onCastSetVolume/
  // onCastSetMuted).
  volume?: number;
  muted?: boolean;
  onSeekToPosition: (sec: number) => void;
  onPlayPause: () => void;
  onSetVolume?: (v: number) => void;
  onSetMuted?: (m: boolean) => void;
  onBack: () => void;
}

// A responder-driven horizontal bar (same pattern as PlayerSeekBar) - avoids a
// slider dependency. Reports a 0-1 fraction on drag and on release.
function DragBar({
  progress,
  accentColor,
  onPreview,
  onCommit,
  accessibilityLabel,
}: {
  progress: number;
  accentColor: string;
  onPreview?: (pct: number) => void;
  onCommit: (pct: number) => void;
  accessibilityLabel: string;
}) {
  const pctRef = useRef(progress);
  const { barProps, fractionAt } = useBarGesture((pct) => { pctRef.current = pct; onPreview?.(pct); });
  return (
    <View
      {...barProps}
      style={styles.dragBar}
      accessible
      accessibilityRole="adjustable"
      accessibilityLabel={accessibilityLabel}
      onResponderRelease={(e) => onCommit(fractionAt(e) ?? pctRef.current)}
    >
      <View style={styles.dragTrack}>
        <View style={{ flex: progress, backgroundColor: accentColor }} />
        <View style={{ flex: 1 - progress }} />
      </View>
      <View style={[styles.dragThumb, { left: `${progress * 100}%`, backgroundColor: accentColor }]} />
    </View>
  );
}

export function CastControllerScreen({
  title,
  deviceName,
  currentTime,
  duration,
  isPlaying,
  skipFwd,
  skipBwd,
  volume,
  muted,
  onSeekToPosition,
  onPlayPause,
  onSetVolume,
  onSetMuted,
  onBack,
}: CastControllerScreenProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  // While dragging the timeline, show the previewed position instead of the
  // receiver's live (pre-seek) one, so the thumb tracks the finger.
  const [seekPreview, setSeekPreview] = useState<number | null>(null);
  const castProgress = seekPreview ?? (duration > 0 ? currentTime / duration : 0);
  const displayTime = seekPreview != null ? seekPreview * duration : currentTime;
  const showVolume = !!onSetVolume;
  const volPct = muted ? 0 : Math.max(0, Math.min(1, volume ?? 1));

  return (
    <View style={[styles.container, { backgroundColor: "#000" }]}>
      <StatusBar hidden />
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", padding: 40 }}>
        <Ionicons name="tv" size={64} color={theme.colors.accent} />
        <Text style={{ color: "#fff", fontSize: FONT_SIZE.xl, fontWeight: "700", marginTop: 20 }}>
          {t("player.controls.castingTo", "Casting to")}
        </Text>
        <Text style={{ color: theme.colors.accent, fontSize: FONT_SIZE.base, marginTop: 4 }} numberOfLines={1}>
          {deviceName || title}
        </Text>
        {deviceName ? (
          <Text style={{ color: "rgba(255,255,255,0.7)", fontSize: FONT_SIZE.sm, marginTop: 2 }} numberOfLines={1}>
            {title}
          </Text>
        ) : null}

        {/* Timeline - draggable (parity with web's cast seek bar) */}
        <View style={{ width: "100%", marginTop: 32, paddingHorizontal: 8 }}>
          <DragBar
            progress={Math.min(1, castProgress)}
            accentColor={theme.colors.accent}
            accessibilityLabel={t("player.controls.seek", "Seek")}
            onPreview={(pct) => setSeekPreview(pct)}
            onCommit={(pct) => { setSeekPreview(null); if (duration > 0) onSeekToPosition(pct * duration); }}
          />
          <View style={{ flexDirection: "row", justifyContent: "space-between", marginTop: 6 }}>
            <Text style={{ color: "rgba(255,255,255,0.7)", fontSize: FONT_SIZE.xs }}>{formatTime(displayTime)}</Text>
            <Text style={{ color: "rgba(255,255,255,0.7)", fontSize: FONT_SIZE.xs }}>{formatTime(duration)}</Text>
          </View>
        </View>

        {/* Controls */}
        <View style={{ flexDirection: "row", alignItems: "center", gap: 32, marginTop: 24 }}>
          <Pressable onPress={() => onSeekToPosition(Math.max(0, currentTime - skipBwd))} hitSlop={12} accessible accessibilityRole="button" accessibilityLabel={t("player.controls.rewind", { seconds: skipBwd })}>
            <Ionicons name="play-back" size={ICON_SIZE["2xl"]} color="#fff" />
          </Pressable>
          <Pressable onPress={onPlayPause} hitSlop={12} accessible accessibilityRole="button" accessibilityLabel={isPlaying ? t("player.controls.pause") : t("player.controls.play")}>
            <Ionicons name={isPlaying ? "pause" : "play"} size={ICON_SIZE["4xl"]} color="#fff" />
          </Pressable>
          <Pressable onPress={() => onSeekToPosition(currentTime + skipFwd)} hitSlop={12} accessible accessibilityRole="button" accessibilityLabel={t("player.controls.forward", { seconds: skipFwd })}>
            <Ionicons name="play-forward" size={ICON_SIZE["2xl"]} color="#fff" />
          </Pressable>
        </View>

        {/* Volume + mute (parity with web's cast volume control) */}
        {showVolume && (
          <View style={{ flexDirection: "row", alignItems: "center", gap: 12, marginTop: 24, width: "100%", paddingHorizontal: 8 }}>
            <Pressable
              onPress={() => onSetMuted?.(!muted)}
              hitSlop={12}
              accessible
              accessibilityRole="button"
              accessibilityLabel={muted ? t("player.controls.unmute", "Unmute") : t("player.controls.mute", "Mute")}
            >
              <Ionicons name={muted || volPct === 0 ? "volume-mute" : "volume-high"} size={ICON_SIZE.lg} color="#fff" />
            </Pressable>
            <View style={{ flex: 1 }}>
              <DragBar
                progress={volPct}
                accentColor={theme.colors.accent}
                accessibilityLabel={t("player.controls.volume", "Volume")}
                onCommit={(pct) => { if (muted && pct > 0) onSetMuted?.(false); onSetVolume?.(pct); }}
              />
            </View>
          </View>
        )}

        {CastButtonNative && (
          <View style={{ marginTop: 32 }}>
            <CastButtonNative style={{ width: 48, height: 48, tintColor: theme.colors.accent }} />
          </View>
        )}
        <Pressable onPress={onBack} style={{ marginTop: 24 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.sm }}>{t("player.goBack")}</Text>
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  dragBar: { height: 28, justifyContent: "center", overflow: "visible" },
  dragTrack: { height: 4, borderRadius: 2, backgroundColor: "rgba(255,255,255,0.2)", flexDirection: "row", overflow: "hidden" },
  dragThumb: { position: "absolute", top: 7, width: 14, height: 14, borderRadius: 7, marginLeft: -7 },
});
