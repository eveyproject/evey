import { View, Text, StyleSheet } from "react-native";
import { useTheme } from "../../providers/ThemeProvider";
import { FONT_SIZE } from "@evey/theme";
import type { ScrubPreview } from "../../hooks/usePlayerGestures";

// hh:mm:ss, dropping the hours on anything under an hour so an episode reads
// as 12:04 rather than 00:12:04.
function formatClock(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const hh = Math.floor(s / 3600);
  const mm = Math.floor((s % 3600) / 60);
  const ss = s % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return hh > 0 ? `${hh}:${pad(mm)}:${pad(ss)}` : `${mm}:${pad(ss)}`;
}

function formatDelta(deltaSeconds: number): string {
  const sign = deltaSeconds < 0 ? "-" : "+";
  return `${sign}${formatClock(Math.abs(deltaSeconds))}`;
}

interface GestureIndicatorsProps {
  seekFlash: "left" | "right" | null;
  scrubPreview?: ScrubPreview | null;
  volumeIndicator: number | null;
  brightnessIndicator: number | null;
  skipFwd: number;
  skipBwd: number;
}

export function GestureIndicators({
  seekFlash,
  scrubPreview,
  volumeIndicator,
  brightnessIndicator,
  skipFwd,
  skipBwd,
}: GestureIndicatorsProps) {
  const theme = useTheme();
  return (
    <>
      {/* Seek flash */}
      {seekFlash && (
        <View
          style={[
            styles.seekFlash,
            seekFlash === "left" ? styles.seekFlashLeft : styles.seekFlashRight,
          ]}
        >
          <Text style={styles.seekFlashText}>
            {seekFlash === "left" ? `« ${skipBwd}` : `${skipFwd} »`}
          </Text>
        </View>
      )}

      {/* Horizontal scrub preview: where the release lands, and by how much.
          Centred because the gesture is screen-wide, unlike the edge-anchored
          volume/brightness bars. */}
      {scrubPreview && (
        <View style={styles.scrubPreview}>
          <Text style={styles.scrubDelta}>{formatDelta(scrubPreview.deltaSec)}</Text>
          <Text style={styles.scrubTarget}>
            {formatClock(scrubPreview.targetSec)}
            <Text style={styles.scrubDuration}>{` / ${formatClock(scrubPreview.durationSec)}`}</Text>
          </Text>
          <View style={styles.scrubTrack}>
            <View
              style={[
                styles.scrubTrackFill,
                {
                  width: `${scrubPreview.durationSec > 0
                    ? Math.max(0, Math.min(100, (scrubPreview.targetSec / scrubPreview.durationSec) * 100))
                    : 0}%`,
                  backgroundColor: theme.colors.accent,
                },
              ]}
            />
          </View>
        </View>
      )}

      {/* Volume indicator */}
      {volumeIndicator !== null && (
        <View style={styles.volumeIndicator}>
          <Text style={styles.volumeIcon}>{"♪"}</Text>
          <View style={styles.volumeBarBg}>
            {/* Bar is full at 100%; the >100% boost is shown by the amber fill + the % text. */}
            <View style={[
              styles.volumeBarFill,
              { height: `${Math.min(100, Math.round(volumeIndicator * 100))}%` },
              volumeIndicator > 1 && { backgroundColor: theme.colors.warning },
            ]} />
          </View>
          <Text style={[styles.volumeText, volumeIndicator > 1 && { color: theme.colors.warning }]}>{Math.round(volumeIndicator * 100)}</Text>
        </View>
      )}

      {/* Brightness indicator */}
      {brightnessIndicator !== null && (
        <View style={styles.brightnessIndicator}>
          <Text style={styles.volumeIcon}>{"☀"}</Text>
          <View style={styles.volumeBarBg}>
            <View style={[styles.volumeBarFill, { height: `${Math.round(brightnessIndicator * 100)}%` }]} />
          </View>
          <Text style={styles.volumeText}>{Math.round(brightnessIndicator * 100)}</Text>
        </View>
      )}
    </>
  );
}

const styles = StyleSheet.create({
  seekFlash: {
    position: "absolute",
    top: 0,
    bottom: 0,
    width: "35%",
    justifyContent: "center",
    alignItems: "center",
    backgroundColor: "rgba(255,255,255,0.08)",
  },
  seekFlashLeft: {
    left: 0,
    borderTopRightRadius: 80,
    borderBottomRightRadius: 80,
  },
  seekFlashRight: {
    right: 0,
    borderTopLeftRadius: 80,
    borderBottomLeftRadius: 80,
  },
  seekFlashText: {
    color: "#fff",
    fontSize: FONT_SIZE["2xl"],
    fontWeight: "700",
  },
  scrubPreview: {
    position: "absolute",
    alignSelf: "center",
    top: "38%",
    minWidth: 200,
    paddingHorizontal: 20,
    paddingVertical: 12,
    alignItems: "center",
    backgroundColor: "rgba(0,0,0,0.72)",
    borderRadius: 12,
  },
  scrubDelta: {
    color: "#fff",
    fontSize: FONT_SIZE.xl,
    fontWeight: "700",
    fontVariant: ["tabular-nums"],
  },
  scrubTarget: {
    color: "#fff",
    fontSize: FONT_SIZE.sm,
    marginTop: 2,
    fontVariant: ["tabular-nums"],
  },
  scrubDuration: {
    color: "rgba(255,255,255,0.6)",
  },
  scrubTrack: {
    marginTop: 8,
    height: 3,
    width: 180,
    borderRadius: 2,
    backgroundColor: "rgba(255,255,255,0.3)",
    overflow: "hidden",
  },
  scrubTrackFill: {
    height: "100%",
    borderRadius: 2,
  },
  brightnessIndicator: {
    position: "absolute",
    left: 40,
    top: "25%",
    height: "50%",
    width: 36,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: "rgba(0,0,0,0.6)",
    borderRadius: 18,
    paddingVertical: 8,
  },
  volumeIndicator: {
    position: "absolute",
    right: 40,
    top: "25%",
    height: "50%",
    width: 36,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: "rgba(0,0,0,0.6)",
    borderRadius: 18,
    paddingVertical: 8,
  },
  volumeIcon: {
    color: "#fff",
    fontSize: FONT_SIZE.base,
    marginBottom: 6,
  },
  volumeBarBg: {
    flex: 1,
    width: 4,
    backgroundColor: "rgba(255,255,255,0.3)",
    borderRadius: 2,
    overflow: "hidden",
    justifyContent: "flex-end",
  },
  volumeBarFill: {
    width: "100%",
    backgroundColor: "#fff",
    borderRadius: 2,
  },
  volumeText: {
    color: "#fff",
    fontSize: FONT_SIZE.xs,
    marginTop: 4,
    fontWeight: "600",
  },
});
