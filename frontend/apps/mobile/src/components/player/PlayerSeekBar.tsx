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
  // The fraction comes from pageX against the bar's own window position, never
  // from locationX: on Android locationX is relative to the view the finger
  // LANDED on, not the responder, and a second drag naturally lands on the
  // thumb the first one left behind - a 14 dp view whose locationX reads 0-14,
  // and which then moves under the finger with every preview, so the
  // reference runs away with the gesture (measured on the emulator: a drag to
  // 63 % landed at 81 %, a tap at 50 % at 25 %; on a phone it ends at 0:00 or
  // the far end). The bar is flex:1 between the time labels, so its width and
  // origin are measured, not derived from the screen.
  const barRef = useRef<View>(null);
  const seekBarWidthRef = useRef(0);
  const seekBarPageXRef = useRef(0);
  const seekPctRef = useRef(0);
  const measureBar = useCallback(() => {
    barRef.current?.measureInWindow((x, _y, width) => {
      seekBarPageXRef.current = x;
      if (width > 0) seekBarWidthRef.current = width;
    });
  }, []);
  const pctFromPageX = useCallback((pageX: number) => {
    const w = seekBarWidthRef.current;
    if (w <= 0) return 0;
    return Math.max(0, Math.min(1, (pageX - seekBarPageXRef.current) / w));
  }, []);

  return (
    <View
      ref={barRef}
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
      onLayout={(e) => { seekBarWidthRef.current = e.nativeEvent.layout.width; measureBar(); }}
      onStartShouldSetResponder={() => true}
      onMoveShouldSetResponder={() => true}
      onResponderGrant={(e) => {
        // Re-measure on every grant: the controls overlay slides in and the
        // layout callback can predate its final position.
        measureBar();
        seekPctRef.current = pctFromPageX(e.nativeEvent.pageX);
        setSeekTouchX(seekPctRef.current * seekBarWidthRef.current);
        onPreview(seekPctRef.current);
      }}
      onResponderMove={(e) => {
        seekPctRef.current = pctFromPageX(e.nativeEvent.pageX);
        setSeekTouchX(seekPctRef.current * seekBarWidthRef.current);
        onPreview(seekPctRef.current);
      }}
      onResponderRelease={(e) => {
        // The finger's window position at lift-off is the chosen spot: the
        // last move the JS thread got to can lag well behind it while the
        // preview and its thumbnail are rendering (measured on the emulator:
        // a drag to 63 % committed 47 %). No pageX (a synthetic release) falls
        // back to the last previewed fraction.
        const pageX = e.nativeEvent?.pageX;
        onCommit(typeof pageX === "number" ? pctFromPageX(pageX) : seekPctRef.current);
        setSeekTouchX(null);
      }}
      // A termination REQUEST comes from another view wanting the gesture: the
      // player stage's pan responder claims every horizontal move past 16 dp
      // as a relative scrub (usePlayerGestures). Handing over turned each drag
      // on this bar into "current position + swipe distance" - measured on the
      // emulator, a drag to 63 % landed at 0:18 + 25 s - so the bar keeps it.
      onResponderTerminationRequest={() => false}
      // The OS, on the other hand, TAKES the responder without asking - the
      // Android back-edge gesture, the notification shade pulled down, a
      // system dialog mid-drag - and no release ever comes. seekTouchX would
      // stay non-null forever, the player would treat it as "still scrubbing"
      // and stop applying timeUpdate: clock, duration and thumb frozen while
      // the film plays on. Ending the scrub here is the whole fix; no commit,
      // because an interrupted drag is not a chosen position.
      onResponderTerminate={() => setSeekTouchX(null)}
    >
      {seekTouchX != null && (
        <View pointerEvents="none" style={{ position: "absolute", bottom: "50%", left: seekTouchX, transform: [{ translateX: -80 }] }}>
          <TrickplayPreview mediaId={mediaId} currentTime={currentTime} visible={effectiveDuration > 0} />
        </View>
      )}
      <View pointerEvents="none" style={styles.seekBarTrack}>
        <View style={[styles.seekBarFill, { flex: progress, backgroundColor: theme.colors.accent }]} />
        <View style={{ flex: 1 - progress }} />
      </View>
      <View pointerEvents="none" style={[styles.seekThumb, { left: `${progress * 100}%`, backgroundColor: theme.colors.accent }]} />
      {effectiveDuration > 0 && chapters.map((ch, i) => {
        const pct = (ch.startTimeMs / 1000) / effectiveDuration;
        if (pct <= 0 || pct >= 1) return null;
        return (
          <View
            key={i}
            pointerEvents="none"
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
