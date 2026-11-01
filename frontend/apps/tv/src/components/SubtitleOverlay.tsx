import { memo } from "react";
import { useSubtitleCue } from "@evey/client";
import { useTranslation } from "react-i18next";
import { View, Text, StyleSheet, Image, useWindowDimensions } from "react-native";
import {
  DEFAULT_SUBTITLE_STYLE,
  subtitleBgRgba,
  subtitleCueFontSizePx,
  type SubtitleStyle,
} from "../lib/subtitleStyle";


/**
 * Drawing subtitles on the TV, which until now drew none at all.
 *
 * The TV had no client-side subtitle rendering: a selected track was passed to
 * the server as `?subtitleTrack=` and that was the whole story. For a TEXT track
 * the server answers a sidecar nobody fetched, so it simply never appeared. For
 * a BITMAP track the server burnt it into the video, which denies the copy rung
 * and turns a direct-playing remux into a full transcode.
 *
 * The fetch and the cue selection (text or bitmap) live in @evey/client's
 * useSubtitleCue, shared with the phone and the browser television and written
 * so the overlay re-renders only when the active cue changes, not on every
 * playhead tick. Only the painting below is this platform's.
 */

interface Props {
  mediaId: string;
  /** Subtitle-relative ordinal to render, or null for none. */
  subtitleIndex: number | null;
  /** Given by a caller that drives the time itself; the playhead store otherwise. */
  currentTimeSec?: number;
  /** Appearance the viewer chose (colour, size, background, position). */
  style?: SubtitleStyle;
}

function SubtitleOverlayImpl({ mediaId, subtitleIndex, currentTimeSec, style }: Props) {
  const { width: screenWidth, height: screenHeight } = useWindowDimensions();
  const { preparing, text, pgs } = useSubtitleCue(
    mediaId,
    subtitleIndex,
    currentTimeSec !== undefined ? { currentTimeSec } : {},
  );
  const { t } = useTranslation();

  // The server is still producing the track (202 + Retry-After): said in the
  // subtitle's place rather than a menu entry that shows nothing for minutes.
  if (preparing) {
    return (
      <View pointerEvents="none" style={styles.textContainer}>
        <Text style={styles.text}>{t("player.subtitlesPreparing")}</Text>
      </View>
    );
  }

  if (pgs) {
    // Scaled from the SUBTITLE plane, which is not the video resolution: every
    // 2160p disc measured carries a 1920x1080 plane, so using the media's
    // dimensions would offset and mis-size every cue by a factor of two.
    const sx = screenWidth / pgs.screenW;
    const sy = screenHeight / pgs.screenH;
    return (
      <View pointerEvents="none" style={StyleSheet.absoluteFill}>
        <Image
          source={{ uri: pgs.uri }}
          resizeMode="stretch"
          style={{ position: "absolute", left: pgs.cue.x * sx, top: pgs.cue.y * sy, width: pgs.cue.w * sx, height: pgs.cue.h * sy }}
        />
      </View>
    );
  }

  if (text === null) return null;
  const cueStyle = style ?? DEFAULT_SUBTITLE_STYLE;
  const fontSize = subtitleCueFontSizePx(screenHeight, cueStyle.fontSize);
  return (
    <View
      pointerEvents="none"
      style={[
        styles.textContainer,
        cueStyle.position === "top" ? { top: "10%", bottom: undefined } : null,
      ]}
    >
      <Text
        style={[
          styles.text,
          {
            color: cueStyle.color,
            backgroundColor: subtitleBgRgba(cueStyle.bgColor, cueStyle.bgOpacity),
            ...(fontSize > 0 ? { fontSize } : null),
            ...(cueStyle.shadow
              ? { textShadowColor: "rgba(0,0,0,0.9)", textShadowOffset: { width: 0, height: 2 }, textShadowRadius: 4 }
              : null),
          },
        ]}
      >
        {text}
      </Text>
    </View>
  );
}

// Memoized: with the cue selection behind usePlayheadSelector, this re-renders
// only when the active cue changes, not on every playhead tick.
export const SubtitleOverlay = memo(SubtitleOverlayImpl);

const styles = StyleSheet.create({
  textContainer: {
    position: "absolute",
    left: 0,
    right: 0,
    bottom: "10%",
    alignItems: "center",
    paddingHorizontal: 48,
  },
  text: {
    color: "#fff",
    fontSize: 27,
    textAlign: "center",
    backgroundColor: "rgba(0,0,0,0.6)",
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: 6,
    overflow: "hidden",
  },
});
