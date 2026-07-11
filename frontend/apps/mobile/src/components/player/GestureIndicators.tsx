import { View, Text, StyleSheet } from "react-native";
import { useTheme } from "../../providers/ThemeProvider";

interface GestureIndicatorsProps {
  seekFlash: "left" | "right" | null;
  volumeIndicator: number | null;
  brightnessIndicator: number | null;
  skipFwd: number;
  skipBwd: number;
}

export function GestureIndicators({
  seekFlash,
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
    fontSize: 24,
    fontWeight: "700",
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
    fontSize: 16,
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
    fontSize: 11,
    marginTop: 4,
    fontWeight: "600",
  },
});
