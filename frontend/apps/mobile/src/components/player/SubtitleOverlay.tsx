import { memo, useEffect, useMemo, useState } from "react";
import { View, Text, StyleSheet, useWindowDimensions } from "react-native";
import { eveyFetchRaw } from "@evey/api";
import { findActiveCueIndex, parseVttCues, shiftCues, type SubtitleCue } from "../../lib/subtitleCues";
import { subtitleBgRgba, subtitleCueFontSizePx, type SubtitleStyle } from "../../lib/subtitleStyle";

interface SubtitleOverlayProps {
  mediaId: string;
  // The absolute media-stream index of the TEXT subtitle to render client-side,
  // or null when no client-rendered subtitle is selected (off, or the current
  // selection is a burn-in image track the server already draws into the video).
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

// Cached per (mediaId, subtitleIndex): switching back to an already-loaded
// track, or a next-episode remount that reuses a sibling's subtitle stream,
// doesn't re-fetch. Mirrors TrickplayPreview's module-level memCache for the
// same shape of problem (a VTT asset keyed by media/track).
const cueCache = new Map<string, SubtitleCue[]>();

function SubtitleOverlayImpl({ mediaId, subtitleIndex, currentTimeSec, offsetMs = 0, style, externalCues = null }: SubtitleOverlayProps) {
  const { height: windowHeight } = useWindowDimensions();
  const [fetchedCues, setFetchedCues] = useState<SubtitleCue[]>([]);

  useEffect(() => {
    // An external (client-picked) file skips the fetch entirely - its cues
    // are already parsed and passed in directly.
    if (externalCues) return;
    if (!mediaId || subtitleIndex === null) {
      setFetchedCues([]);
      return;
    }
    const key = `${mediaId}:${subtitleIndex}`;
    const cached = cueCache.get(key);
    if (cached) {
      setFetchedCues(cached);
      return;
    }
    setFetchedCues([]);
    let cancelled = false;
    (async () => {
      try {
        const res = await eveyFetchRaw({
          url: `/api/v1/media/${encodeURIComponent(mediaId)}/subtitle/${subtitleIndex}`,
          method: "GET",
        });
        const text = await res.text();
        if (cancelled) return;
        const parsed = parseVttCues(text);
        cueCache.set(key, parsed);
        setFetchedCues(parsed);
      } catch (err) {
        console.warn("[subtitles] failed to load track for client-side rendering:", err);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [mediaId, subtitleIndex, externalCues]);

  const cues = externalCues ?? fetchedCues;
  const shifted = useMemo(() => shiftCues(cues, offsetMs), [cues, offsetMs]);
  const activeIdx = useMemo(() => findActiveCueIndex(shifted, currentTimeSec), [shifted, currentTimeSec]);

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
