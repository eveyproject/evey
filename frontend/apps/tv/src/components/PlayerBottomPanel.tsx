import { memo, useCallback } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { formatTime } from "@evey/api";
import { usePlayhead } from "@evey/client";
import { FONT_SIZE } from "@evey/theme";
import type { Chapter } from "../hooks/useChapters";
import { useTheme } from "../providers/ThemeProvider";
import { PlayerControlButton } from "./PlayerControlButton";
import { TrickplaySeekPreview } from "./TrickplaySeekPreview";
import { PlayerScrim } from "./PlayerScrim";

// Panel geometry, shared with the trickplay anchor math.
const PANEL_PADDING_H = 40;
const PANEL_GAP = 16;
const TIME_LABEL_WIDTH = 70;
const TRICKPLAY_HALF_WIDTH = 160;

export type PlayerFocusTags = {
  rewind?: number;
  play?: number;
  forward?: number;
  settings?: number;
  speed?: number;
  timeline?: number;
};

export type PlayerBarRefs = {
  rewind: React.RefObject<View | null>;
  play: React.RefObject<View | null>;
  forward: React.RefObject<View | null>;
  speed: React.RefObject<View | null>;
  settings: React.RefObject<View | null>;
  timeline: React.RefObject<View | null>;
};

// One panel at the bottom holds everything the remote can do: a row of
// buttons, the timeline under it, and the clock. The three seek controls used
// to float in the middle of the picture while the options hid bottom right -
// two places to look, and a mask over the film (owner, 2026-09-22: "something
// obvious, something clean"). Focus lands on the play button on the first DOWN
// press, walks the row left and right, and drops onto the timeline with
// another DOWN.
export const PlayerBottomPanel = memo(function PlayerBottomPanel({
  mediaId,
  screenWidth,
  duration,
  seekPreviewSec,
  pendingSeekDelta,
  isPlaying,
  playbackSpeed,
  skipFwd,
  skipBwd,
  chapters,
  barFocused,
  focusTags,
  refs,
  timelineFocused,
  onTimelineFocusChange,
  onSeekBy,
  onTogglePlayPause,
  onCycleSpeed,
  onOpenSettings,
  onCommitSeek,
  onActivity,
  style,
}: {
  mediaId: string;
  screenWidth: number;
  // The preview position while a seek is pending, else the playhead.
  duration: number;
  seekPreviewSec: number | null;
  pendingSeekDelta: number;
  isPlaying: boolean;
  playbackSpeed: number;
  skipFwd: number;
  skipBwd: number;
  chapters: Chapter[];
  // False while the surface owns the D-pad: left/right then seek instead of
  // walking this row.
  barFocused: boolean;
  focusTags: PlayerFocusTags;
  refs: PlayerBarRefs;
  timelineFocused: boolean;
  onTimelineFocusChange: (focused: boolean) => void;
  onSeekBy: (deltaSec: number) => void;
  onTogglePlayPause: () => void;
  onCycleSpeed: () => void;
  onOpenSettings: () => void;
  // Lands the pending jump; false when there was none.
  onCommitSeek: () => boolean;
  onActivity: () => void;
  style?: React.ComponentProps<typeof View>["style"];
}) {
  const { t } = useTranslation();
  const theme = useTheme();

  // The playhead store moves twice a second; only this panel follows it.
  const playhead = usePlayhead();
  const displayTime = seekPreviewSec ?? playhead;
  const seekProgress = duration > 0 ? displayTime / duration : 0;
  // The thumbnail rides above the timeline at the playhead, kept inside the
  // panel's width so a seek near either edge never pushes it off screen.
  const trickplayLaneWidth = Math.max(0, screenWidth - 2 * PANEL_PADDING_H - 2 * TIME_LABEL_WIDTH - 2 * PANEL_GAP);
  const trickplayLeft = PANEL_PADDING_H + TIME_LABEL_WIDTH + PANEL_GAP
    + Math.max(TRICKPLAY_HALF_WIDTH, Math.min(trickplayLaneWidth - TRICKPLAY_HALF_WIDTH, seekProgress * trickplayLaneWidth));
  // The wall clock at the end of the film at this speed, rounded to the
  // minute: the question a viewer asks at eleven at night.
  const endsAtLabel = duration > 0
    ? new Date(Date.now() + ((duration - displayTime) * 1000) / (playbackSpeed || 1)).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
    : null;

  // Stable, so the memoised buttons skip the panel's re-renders.
  const seekBack = useCallback(() => onSeekBy(-skipBwd), [onSeekBy, skipBwd]);
  const seekForward = useCallback(() => onSeekBy(skipFwd), [onSeekBy, skipFwd]);
  return (
    <View style={[styles.bottomPanel, style]}>
      <PlayerScrim edge="bottom" />
      {/* Trickplay thumbnail, above the panel at the playhead: the D-pad seek
          has no live picture to look at, so the tile sits where the cursor
          is, not in the middle of the screen. Renders nothing on its own
          without a manifest or sprites. */}
      {seekPreviewSec !== null && (
        <View style={styles.trickplayLane} pointerEvents="none">
          <View style={[styles.trickplayAnchor, { left: trickplayLeft }]}>
            <TrickplaySeekPreview mediaId={mediaId} previewTimeSec={seekPreviewSec} warm />
          </View>
        </View>
      )}
      {seekPreviewSec === null && (
        <View style={styles.trickplayWarm} pointerEvents="none">
          <TrickplaySeekPreview mediaId={mediaId} previewTimeSec={null} warm />
        </View>
      )}

      <View style={styles.controlsRow}>
        <View style={styles.controlsGroup}>
          <PlayerControlButton
            icon="play-back"
            caption={`${skipBwd}s`}
            focusable={barFocused}
            buttonRef={refs.rewind}
            nextFocusLeft={focusTags.rewind}
            nextFocusRight={focusTags.play}
            nextFocusUp={focusTags.rewind}
            nextFocusDown={focusTags.timeline ?? focusTags.rewind}
            onPress={seekBack}
            accessibilityLabel={t("player.controls.rewind", { seconds: skipBwd })}
          />
          <PlayerControlButton
            icon={isPlaying ? "pause" : "play"}
            primary
            focusable={barFocused}
            buttonRef={refs.play}
            nextFocusLeft={focusTags.rewind}
            nextFocusRight={focusTags.forward}
            nextFocusUp={focusTags.play}
            nextFocusDown={focusTags.timeline ?? focusTags.play}
            onPress={onTogglePlayPause}
            accessibilityLabel={isPlaying ? t("player.controls.pause") : t("player.controls.play")}
          />
          <PlayerControlButton
            icon="play-forward"
            caption={`${skipFwd}s`}
            focusable={barFocused}
            buttonRef={refs.forward}
            nextFocusLeft={focusTags.play}
            nextFocusRight={focusTags.speed ?? focusTags.settings}
            nextFocusUp={focusTags.forward}
            nextFocusDown={focusTags.timeline ?? focusTags.forward}
            onPress={seekForward}
            accessibilityLabel={t("player.controls.forward", { seconds: skipFwd })}
          />
        </View>
        <View style={styles.controlsGroup}>
          {/* The row is out of the D-pad's reach until DOWN: said on screen,
              or the buttons read as reachable and are not. */}
          {!barFocused && (
            <Text style={styles.barHintText}>{String(t("player.controls.barHint", "▼ for options"))}</Text>
          )}
          {endsAtLabel !== null && (
            <Text style={styles.endsAtText}>{String(t("player.controls.endsAt", `Ends at ${endsAtLabel}`, { time: endsAtLabel }))}</Text>
          )}
          <PlayerControlButton
            text={`${playbackSpeed}x`}
            small
            focusable={barFocused}
            buttonRef={refs.speed}
            nextFocusLeft={focusTags.forward}
            nextFocusRight={focusTags.settings}
            nextFocusUp={focusTags.speed}
            nextFocusDown={focusTags.timeline ?? focusTags.speed}
            onPress={onCycleSpeed}
            accessibilityLabel={t("player.settings.speed", "Speed")}
          />
          <PlayerControlButton
            icon="settings-sharp"
            small
            focusable={barFocused}
            buttonRef={refs.settings}
            nextFocusLeft={focusTags.speed ?? focusTags.forward}
            nextFocusRight={focusTags.settings}
            nextFocusUp={focusTags.settings}
            nextFocusDown={focusTags.timeline ?? focusTags.settings}
            onPress={onOpenSettings}
            accessibilityLabel={t("player.controls.more", "More")}
          />
        </View>
      </View>

      {/* The timeline is a control: reached with DOWN from any button, it
          keeps the native focus on itself (left and right point back at it)
          so those keys move the playhead, and OK lands the pending jump.
          Focused, it thickens and shows a knob at the playhead - a thin bar
          alone never said it was the one lit. */}
      <View style={styles.timelineRow}>
        <Text style={styles.timeText}>{formatTime(displayTime)}</Text>
        {/* How far the pending jump goes. Pressing the seek button four times
            is one jump of four steps, and this is what says so before it
            commits. */}
        {pendingSeekDelta !== 0 && (
          <Text style={[styles.seekDeltaText, { color: theme.colors.accent }]}>
            {pendingSeekDelta > 0 ? "+" : "-"}{formatTime(Math.abs(pendingSeekDelta))}
          </Text>
        )}
        <Pressable
          ref={refs.timeline as never}
          focusable={barFocused}
          nextFocusLeft={focusTags.timeline}
          nextFocusRight={focusTags.timeline}
          nextFocusUp={focusTags.play}
          nextFocusDown={focusTags.timeline}
          onFocus={() => { onTimelineFocusChange(true); onActivity(); }}
          onBlur={() => onTimelineFocusChange(false)}
          onPress={() => { if (!onCommitSeek()) onTogglePlayPause(); }}
          accessible={true}
          accessibilityRole="adjustable"
          accessibilityLabel={t("player.a11y.progress", "Playback progress")}
          accessibilityValue={{
            min: 0,
            max: Math.max(0, Math.round(duration)),
            now: Math.max(0, Math.round(displayTime)),
            text: `${formatTime(displayTime)} / ${formatTime(duration)}`,
          }}
          style={styles.timelineHit}
        >
          <View style={[styles.seekBarTrack, timelineFocused && styles.seekBarTrackFocused]}>
            <View style={[styles.seekBarFill, { backgroundColor: theme.colors.accent, flex: seekProgress }]} />
            <View style={{ flex: 1 - seekProgress }} />
            {duration > 0 &&
              chapters.map((ch, i) => {
                const pct = (ch.startTimeMs / 1000 / duration) * 100;
                if (pct <= 0 || pct >= 100) return null;
                return <View key={i} style={[styles.chapterNotch, { left: `${pct}%` }]} />;
              })}
          </View>
          {timelineFocused && (
            <View
              pointerEvents="none"
              style={[styles.seekKnob, { left: `${Math.max(0, Math.min(100, seekProgress * 100))}%`, borderColor: theme.colors.accent }]}
            />
          )}
        </Pressable>
        <Text style={styles.timeText}>{formatTime(duration)}</Text>
      </View>
    </View>
  );
});

const styles = StyleSheet.create({
  bottomPanel: {
    paddingTop: 72,
    paddingBottom: 32,
    paddingHorizontal: PANEL_PADDING_H,
    gap: 14,
  },
  controlsRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  controlsGroup: {
    flexDirection: "row",
    alignItems: "center",
    gap: 16,
  },
  endsAtText: {
    color: "rgba(255,255,255,0.75)",
    fontSize: FONT_SIZE.base,
    fontVariant: ["tabular-nums"],
    marginRight: 12,
  },
  barHintText: {
    color: "rgba(255,255,255,0.75)",
    fontSize: FONT_SIZE.base,
    marginRight: 12,
  },
  seekDeltaText: {
    fontSize: FONT_SIZE.lg,
    fontWeight: "700",
    fontVariant: ["tabular-nums"],
    marginLeft: 8,
  },
  // A lane the tile floats in, sitting on top of the panel; the anchor is
  // positioned by the playhead and centred on it.
  trickplayLane: {
    position: "absolute",
    left: 0,
    right: 0,
    top: -(180 + 16),
    height: 180,
  },
  trickplayAnchor: {
    position: "absolute",
    top: 0,
    marginLeft: -TRICKPLAY_HALF_WIDTH,
  },
  // Off screen while nothing is previewed: keeps the manifest and the first
  // sheet warm without drawing anything.
  trickplayWarm: {
    position: "absolute",
    width: 0,
    height: 0,
    overflow: "hidden",
  },
  timelineRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: PANEL_GAP,
  },
  timeText: {
    color: "#fff",
    fontSize: FONT_SIZE.lg,
    fontWeight: "600",
    fontVariant: ["tabular-nums"],
    minWidth: TIME_LABEL_WIDTH,
    textAlign: "center",
  },
  // A fixed height, not flex: in the column-wise hit area `flex: 1` stretched
  // the track to the area's whole height - a thick grey slab, not a line.
  seekBarTrack: {
    alignSelf: "stretch",
    height: 6,
    borderRadius: 3,
    backgroundColor: "rgba(255,255,255,0.25)",
    flexDirection: "row",
    overflow: "hidden",
  },
  seekBarTrackFocused: {
    height: 10,
    borderRadius: 5,
  },
  seekBarFill: {},
  // Tall enough to be a target for the focus engine and the pointer, the
  // track itself sits in its middle.
  timelineHit: {
    flex: 1,
    height: 32,
    justifyContent: "center",
  },
  seekKnob: {
    position: "absolute",
    top: 4,
    marginLeft: -12,
    width: 24,
    height: 24,
    borderRadius: 12,
    borderWidth: 4,
    backgroundColor: "#fff",
  },
  chapterNotch: {
    position: "absolute",
    top: 0,
    bottom: 0,
    width: 3,
    backgroundColor: "rgba(0,0,0,0.55)",
  },
});
