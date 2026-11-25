import { StyleSheet, Text, View } from "react-native";
import type { V1MediaStream } from "@evey/api";
import { isHdrStream, resolutionLabel, videoStream } from "@evey/client";
import { useTheme } from "../providers/ThemeProvider";

// The resolution and HDR badges of a poster, read off the title's video
// stream. One component for the library grid and the home rows, so the two
// cannot drift apart.
export function QualityBadges({ streams, bottom = 6 }: { streams?: V1MediaStream[]; bottom?: number }) {
  const theme = useTheme();
  const video = videoStream(streams);
  const resLabel = resolutionLabel(video);
  const hdr = isHdrStream(video);
  if (!resLabel && !hdr) return null;
  return (
    <View style={[styles.row, { bottom, left: 6 }]} pointerEvents="none" testID="quality-badges">
      {resLabel && (
        <View style={[styles.badge, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}>
          <Text style={[styles.text, { color: theme.colors.text }]} numberOfLines={1}>{resLabel}</Text>
        </View>
      )}
      {hdr && (
        <View style={[styles.badge, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.sm }]}>
          <Text style={[styles.text, { color: theme.colors.onAccent }]} numberOfLines={1}>HDR</Text>
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { position: "absolute", flexDirection: "row", gap: 4 },
  badge: { flexDirection: "row", alignItems: "center", gap: 3, paddingHorizontal: 5, paddingVertical: 2 },
  text: { fontSize: 11, fontWeight: "700", flexShrink: 1 },
});
