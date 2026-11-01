import { memo } from "react";
import { useTranslation } from "react-i18next";
import { View, Text, StyleSheet, Image, useWindowDimensions } from "react-native";
import type { SubtitleCue } from "@evey/api";
import { useSubtitleCue } from "@evey/client";

import { subtitleBgRgba, subtitleCueFontSizePx, type SubtitleStyle } from "../../lib/subtitleStyle";

interface SubtitleOverlayProps {
  mediaId: string;
  // The subtitle-relative ordinal of the track to render client-side, or
  // null when none is selected. Both kinds land here now: a TEXT track arrives
  // as WebVTT, a BITMAP (PGS) track as the image blob the server decodes once
  // and caches - which is what lets a bitmap subtitle be shown without the
  // server burning it into the video, and so without denying the copy rung.
  subtitleIndex: number | null;
  currentTimeSec: number;
  offsetMs?: number;
  style: SubtitleStyle;
  // Cues from a client-picked external file (upload button in
  // PlayerSettingsPanel): when present, rendered instead of - and with
  // priority over - any server subtitleIndex track, with no network fetch.
  externalCues?: SubtitleCue[] | null;
}

// The fetch, the offset and the active-cue lookup (text or bitmap) live in
// @evey/client's useSubtitleCue, shared with the two televisions; only the
// painting below is this platform's.
function SubtitleOverlayImpl({ mediaId, subtitleIndex, currentTimeSec, offsetMs = 0, style, externalCues = null }: SubtitleOverlayProps) {
  const { height: windowHeight, width: windowWidth } = useWindowDimensions();
  const { preparing, text, pgs } = useSubtitleCue(mediaId, subtitleIndex, { currentTimeSec, offsetMs, externalCues });
  const { t } = useTranslation();

  // The server is still producing the track (202 + Retry-After): said in the
  // subtitle's place rather than a menu entry that shows nothing for minutes.
  if (preparing) {
    return (
      <View pointerEvents="none" style={[styles.container, style.position === "top" ? styles.top : styles.bottom]}>
        <Text style={[styles.text, { color: style.color, backgroundColor: subtitleBgRgba(style.bgColor, style.bgOpacity) }]}>
          {t("player.subtitlesPreparing")}
        </Text>
      </View>
    );
  }

  if (pgs) {
    // Scaled from the SUBTITLE plane, not the video resolution (a 2160p disc
    // carries a 1920x1080 plane): the ratio comes from the track. Letterboxing
    // is not modelled - the overlay fills the surface like the text one.
    const sx = windowWidth / pgs.screenW;
    const sy = windowHeight / pgs.screenH;
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
  const fontSize = subtitleCueFontSizePx(windowHeight, style.fontSize);

  return (
    <View
      pointerEvents="none"
      style={[styles.container, style.position === "top" ? styles.top : styles.bottom]}
    >
      <Text
        style={[
          styles.text,
          {
            color: style.color,
            backgroundColor: subtitleBgRgba(style.bgColor, style.bgOpacity),
            ...(fontSize > 0 ? { fontSize } : null),
          },
          style.shadow ? styles.shadow : null,
        ]}
      >
        {text}
      </Text>
    </View>
  );
}

// Memoized: with the cue selection behind usePlayheadSelector in useSubtitleCue,
// the lookup no longer forces a render on every tick.
export const SubtitleOverlay = memo(SubtitleOverlayImpl);

const styles = StyleSheet.create({
  container: {
    position: "absolute",
    left: 0,
    right: 0,
    alignItems: "center",
    paddingHorizontal: 24,
  },
  bottom: {
    bottom: "10%",
  },
  top: {
    top: "8%",
  },
  text: {
    textAlign: "center",
    paddingHorizontal: 8,
    paddingVertical: 4,
    borderRadius: 4,
    overflow: "hidden",
  },
  shadow: {
    textShadowColor: "rgba(0,0,0,0.8)",
    textShadowOffset: { width: 0, height: 1 },
    textShadowRadius: 2,
  },
});
