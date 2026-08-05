import { useCallback, useEffect, useRef, useState } from "react";
import { View, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { TrickplayPreview } from "../TrickplayPreview";
import type { Chapter } from "../../hooks/useChapters";
import { formatTime } from "@evey/api";

interface PlayerSeekBarProps {
  mediaId: string;
  progress: number;
  currentTime: number;
  effectiveDuration: number;
  chapters: Chapter[];
  onPreview: (pct: number) => void;
  onCommit: (pct: number) => void;
  onScrubbingChange: (scrubbing: boolean) => void;
}

export function PlayerSeekBar({
  mediaId,
  progress,
  currentTime,
  effectiveDuration,
  chapters,
  onPreview,
  onCommit,
  onScrubbingChange,
}: PlayerSeekBarProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [seekTouchX, setSeekTouchX] = useState<number | null>(null);
  useEffect(() => {
    onScrubbingChange(seekTouchX != null);
  }, [seekTouchX, onScrubbingChange]);
  // Measured seek-bar width (the bar is flex:1 between the time labels, so it is
  // NOT screenW-minus-insets) and the last previewed fraction. onResponderRelease's
  // locationX is unreliable on Android (it can report a coordinate relative to a
  // nested absolute child like the trickplay preview → a near-zero x → the seek
  // jumps to ~0:00). So we commit the fraction tracked during the reliable move
  // events, divided by the real measured width.
  const seekBarWidthRef = useRef(0);
  const seekPctRef = useRef(0);
  const pctFromX = useCallback((x: number) => {
    const w = seekBarWidthRef.current;
    if (w <= 0) return 0;
    return Math.max(0, Math.min(1, x / w));
  }, []);

  return (
    <View
      style={styles.seekBarContainer}
      accessible={true}
      accessibilityRole="adjustable"
      accessibilityLabel={t("player.controls.seek")}
      // An "adjustable" role without a value and without an action handler
      // announces a control the screen reader cannot read or move: TalkBack
      // offers swipe up/down and nothing happens. The value is expressed as a
      // percentage because that is what the gesture path already works in, and
      // the text form reads as a position rather than a bare number.
      accessibilityValue={{
        min: 0,
        max: 100,
        now: Math.round(progress * 100),
        text: t("player.controls.seekPosition", {
          current: formatTime(currentTime),
          total: formatTime(effectiveDuration),
        }),
      }}
      onAccessibilityAction={(e) => {
        // One step = 10s, the same granularity as the skip buttons.
        const stepPct = effectiveDuration > 0 ? 10 / effectiveDuration : 0;
        if (stepPct <= 0) return;
        const next =
          e.nativeEvent.actionName === "increment"
            ? Math.min(1, progress + stepPct)
            : Math.max(0, progress - stepPct);
        onCommit(next);
      }}
      accessibilityActions={[{ name: "increment" }, { name: "decrement" }]}
      onLayout={(e) => { seekBarWidthRef.current = e.nativeEvent.layout.width; }}
      onStartShouldSetResponder={() => true}
      onMoveShouldSetResponder={() => true}
      onResponderGrant={(e) => {
        const x = e.nativeEvent.locationX;
        setSeekTouchX(x);
        seekPctRef.current = pctFromX(x);
        onPreview(seekPctRef.current);
      }}
      onResponderMove={(e) => {
        const x = e.nativeEvent.locationX;
        setSeekTouchX(x);
        seekPctRef.current = pctFromX(x);
        onPreview(seekPctRef.current);
      }}
      onResponderRelease={() => {
        // Commit the fraction tracked during move - NOT the release
        // event's locationX, which is unreliable on Android.
        onCommit(seekPctRef.current);
        setSeekTouchX(null);
      }}
    >
      {seekTouchX != null && (
        <View style={{ position: "absolute", bottom: "50%", left: seekTouchX, transform: [{ translateX: -80 }] }}>
          <TrickplayPreview mediaId={mediaId} currentTime={currentTime} visible={effectiveDuration > 0} />
        </View>
      )}
      <View style={styles.seekBarTrack}>
        <View style={[styles.seekBarFill, { flex: progress, backgroundColor: theme.colors.accent }]} />
        <View style={{ flex: 1 - progress }} />
      </View>
      <View style={[styles.seekThumb, { left: `${progress * 100}%`, backgroundColor: theme.colors.accent }]} />
      {effectiveDuration > 0 && chapters.map((ch, i) => {
        const pct = (ch.startTimeMs / 1000) / effectiveDuration;
        if (pct <= 0 || pct >= 1) return null;
        return (
          <View
            key={i}
            style={[styles.chapterMarker, { left: `${pct * 100}%` }]}
          />
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  seekBarContainer: {
    flex: 1,
    height: 32,
    justifyContent: "center",
    overflow: "visible",
  },
  seekBarTrack: {
    height: 4,
    borderRadius: 2,
    backgroundColor: "rgba(255,255,255,0.3)",
    flexDirection: "row",
    overflow: "hidden",
  },
  seekBarFill: {
  },
  seekThumb: {
    position: "absolute",
    top: 10,
    width: 14,
    height: 14,
    borderRadius: 7,
    marginLeft: -7,
  },
  chapterMarker: {
    position: "absolute",
    top: 12,
    width: 3,
    height: 8,
    borderRadius: 1,
    backgroundColor: "rgba(255,255,255,0.7)",
    marginLeft: -1,
  },
});
