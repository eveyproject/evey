import { memo, useMemo } from "react";
import { View, Text, StyleSheet, Image, useWindowDimensions } from "react-native";
import {
  findActivePGSCue,
  findActiveCueIndex,
  shiftCues,
  type PGSTrack,
  type SubtitleCue,
} from "@evey/api";
import { useSubtitleTrack } from "@evey/client";

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
  // Parsing (parseVttCues) happens once at pick time in the player screen;
  // this component only applies the offset/style and does the active-cue
  // lookup, same as it does for a server-fetched track.
  externalCues?: SubtitleCue[] | null;
}

function SubtitleOverlayImpl({ mediaId, subtitleIndex, currentTimeSec, offsetMs = 0, style, externalCues = null }: SubtitleOverlayProps) {
  const { height: windowHeight, width: windowWidth } = useWindowDimensions();
  // Shared with both televisions: the packager's segments around the playhead
  // (in the film's own time, before the resync offset), /subtitle otherwise.
  // An external (client-picked) file skips the fetch entirely.
  const sourceTimeSec = Math.max(0, currentTimeSec - offsetMs / 1000);
  const { cues: fetchedCues, pgs } = useSubtitleTrack(mediaId, externalCues ? null : subtitleIndex, sourceTimeSec);

  // A client-picked external file always wins, as before - it is an explicit
  // choice made after the track was selected.
  const pgsTrack = externalCues ? null : pgs;
  const cues = externalCues ?? fetchedCues;
  const shifted = useMemo(() => shiftCues(cues, offsetMs), [cues, offsetMs]);
  const activeIdx = useMemo(() => findActiveCueIndex(shifted, currentTimeSec), [shifted, currentTimeSec]);

  if (pgsTrack) {
    return (
      <PGSCueView
        track={pgsTrack.track}
        uris={pgsTrack.uris}
        currentTimeSec={currentTimeSec}
        offsetMs={offsetMs}
        windowWidth={windowWidth}
        windowHeight={windowHeight}
      />
    );
  }

  if (activeIdx === -1) return null;
  const cue = shifted[activeIdx];
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
        {cue.text}
      </Text>
    </View>
  );
}

// Draws one decoded bitmap cue, scaled from the SUBTITLE plane onto the video
// surface. The plane is not the video resolution - every 2160p disc measured
// carries a 1920x1080 one - so the ratio comes from the track and never from
// the media's dimensions.
//
// Letterboxing is deliberately not modelled: the overlay fills the player
// surface, exactly as the text overlay does, so the two stay consistent and a
// cue never lands somewhere the text never would.
function PGSCueView({
  track,
  uris,
  currentTimeSec,
  offsetMs,
  windowWidth,
  windowHeight,
}: {
  track: PGSTrack;
  uris: string[];
  currentTimeSec: number;
  offsetMs: number;
  windowWidth: number;
  windowHeight: number;
}) {
  const timeMs = currentTimeSec * 1000 - offsetMs;
  const idx = findActivePGSCue(track.cues, timeMs);
  if (idx === -1 || track.screenW <= 0 || track.screenH <= 0) return null;
  const cue = track.cues[idx];
  const sx = windowWidth / track.screenW;
  const sy = windowHeight / track.screenH;
  return (
    <View pointerEvents="none" style={StyleSheet.absoluteFill}>
      <Image
        source={{ uri: uris[idx] }}
        resizeMode="stretch"
        style={{
          position: "absolute",
          left: cue.x * sx,
          top: cue.y * sy,
          width: cue.w * sx,
          height: cue.h * sy,
        }}
      />
    </View>
  );
}

// Isolated + memoized: this is the one piece of the player screen that must
// re-render on every playhead tick (~2/s, see the player's
// timeUpdateEventInterval), so it's kept out of the main tree instead of
// inlining the lookup where currentTime already lives.
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
