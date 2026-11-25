import { StyleSheet, Text, View } from "react-native";
import type { V1MediaStream } from "@evey/api";
import { hdrLabel, resolutionLabel, videoStream } from "@evey/client";
import { useTheme } from "../providers/ThemeProvider";

// The resolution and HDR of a poster, as one welded chip read off the title's
// video stream: 4K light, any other resolution dark, HDR amber (HLG says its
// own name), the web card's language. Set for a poster rather than for the
// page, since it sits on any picture. One component for the library grid and
// the home rows, so the two cannot drift apart.
const CHIP = {
  uhd: { bg: "#f5f5f4", fg: "#0c0a09" },
  res: { bg: "rgba(12,12,14,0.88)", fg: "#fafafa" },
  hdr: { bg: "#f59e0b", fg: "#000000" },
};

export function QualityBadges({ streams, bottom = 6 }: { streams?: V1MediaStream[]; bottom?: number }) {
  const theme = useTheme();
  const video = videoStream(streams);
  const resLabel = resolutionLabel(video);
  const flavour = hdrLabel(video);
  const hdr = flavour === "HDR10" ? "HDR" : flavour;
  if (!resLabel && !hdr) return null;
  const res = resLabel === "4K" ? CHIP.uhd : CHIP.res;
  // Each segment rounds its own outer corners: a rounded, clipping wrapper is
  // a path clip the television's GPU pays for on every frame.
  const r = theme.radii.sm;
  const left = { borderTopLeftRadius: r, borderBottomLeftRadius: r };
  const right = { borderTopRightRadius: r, borderBottomRightRadius: r };
  return (
    <View style={[styles.row, { bottom, left: 6 }]} pointerEvents="none" testID="quality-badges">
      {resLabel && (
        <View style={[styles.segment, { backgroundColor: res.bg }, resLabel !== "4K" && styles.outlined, left, !hdr && right]}>
          <Text style={[styles.text, { color: res.fg }]} numberOfLines={1}>{resLabel}</Text>
        </View>
      )}
      {hdr && (
        <View style={[styles.segment, { backgroundColor: CHIP.hdr.bg }, right, !resLabel && left]}>
          <Text style={[styles.text, { color: CHIP.hdr.fg }]} numberOfLines={1}>{hdr}</Text>
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { position: "absolute", flexDirection: "row" },
  segment: { paddingHorizontal: 6, paddingVertical: 3, justifyContent: "center" },
  outlined: { borderWidth: StyleSheet.hairlineWidth, borderColor: "rgba(255,255,255,0.3)" },
  text: { fontSize: 11, fontWeight: "800", letterSpacing: 0.4 },
});
