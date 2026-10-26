import { memo } from "react";
import { usePlayhead } from "@evey/client";
import { useTranslation } from "react-i18next";
import { View, Text, StyleSheet, Image, useWindowDimensions } from "react-native";
import {
  findActivePGSCue,
  findActiveCueIndex,
} from "@evey/api";
import { useSubtitleTrack } from "@evey/client";
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
 * One component covers both, exactly as on mobile: the same route answers WebVTT
 * for a text track and a decoded-image blob for a bitmap one, and the first
 * bytes say which.
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

function SubtitleOverlayImpl({ mediaId, subtitleIndex, currentTimeSec: givenTime, style }: Props) {
  const playhead = usePlayhead();
  const currentTimeSec = givenTime ?? playhead;
  const { width: screenWidth, height: screenHeight } = useWindowDimensions();
  // The fetch and the text/bitmap sniff live in @evey/client, shared with the
  // browser television; only the painting below is this platform's.
  const { cues, pgs, preparing } = useSubtitleTrack(mediaId, subtitleIndex, currentTimeSec);
  const { t } = useTranslation();

  // The server is still producing the track (202 + Retry-After): said in the
  // subtitle's place rather than a menu entry that shows nothing for minutes.
  if (preparing && !pgs && cues.length === 0) {
    return (
      <View pointerEvents="none" style={styles.textContainer}>
        <Text style={styles.text}>{t("player.subtitlesPreparing")}</Text>
      </View>
    );
  }

  if (pgs) {
    const { track, uris } = pgs;
    const idx = findActivePGSCue(track.cues, currentTimeSec * 1000);
    if (idx === -1 || track.screenW <= 0 || track.screenH <= 0) return null;
    const cue = track.cues[idx];
    // Scaled from the SUBTITLE plane, which is not the video resolution: every
    // 2160p disc measured carries a 1920x1080 plane, so using the media's
    // dimensions would offset and mis-size every cue by a factor of two.
    const sx = screenWidth / track.screenW;
    const sy = screenHeight / track.screenH;
    return (
      <View pointerEvents="none" style={StyleSheet.absoluteFill}>
        <Image
          source={{ uri: uris[idx] }}
          resizeMode="stretch"
          style={{ position: "absolute", left: cue.x * sx, top: cue.y * sy, width: cue.w * sx, height: cue.h * sy }}
        />
      </View>
    );
  }

  const active = findActiveCueIndex(cues, currentTimeSec);
  if (active === -1) return null;
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
        {cues[active].text}
      </Text>
    </View>
  );
}

// Memoized: this is the one piece of the TV player that re-renders on every
// playhead tick, so it stays out of the screen's own render.
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
