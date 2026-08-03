import { useCallback, useRef } from "react";
import { View, Text, Pressable, StyleSheet, useWindowDimensions } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

interface PlayerVolumeControlProps {
  /** Continuous level 0-2: 0-1 tracks native volume, 1-2 is the amplification. */
  volume: number;
  muted: boolean;
  onToggleMute: () => void;
  onVolumeChange: (value: number) => void;
}

// Copy of the web PlayerVolumeControl: mute icon + a 0-2 bar whose top half
// (beyond 100%) fills with the warning color to signal amplification,
// plus a percentage on the right.
export function PlayerVolumeControl({ volume, muted, onToggleMute, onVolumeChange }: PlayerVolumeControlProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const widthRef = useRef(0);
  // Like the web under the sm breakpoint: in narrow width (portrait), only
  // the icon remains - the bar and percentage only appear in landscape.
  const { width: windowW } = useWindowDimensions();
  const compact = windowW < 600;

  const level = muted ? 0 : volume;
  const boosted = level > 1;
  const pct = Math.round(level * 100);
  const fillPct = (level / 2) * 100; // the bar covers 0..2, 100% lands in the middle

  const valueFromX = useCallback((x: number) => {
    const w = widthRef.current;
    if (w <= 0) return 0;
    return Math.max(0, Math.min(2, (x / w) * 2));
  }, []);

  const icon = muted || level === 0 ? "volume-mute" : level < 0.5 ? "volume-low" : "volume-high";

  return (
    <View style={styles.container}>
      <Pressable
        onPress={onToggleMute}
        hitSlop={8}
        style={styles.muteBtn}
        accessible={true}
        accessibilityLabel={muted || volume === 0 ? t("player.controls.unmute") : t("player.controls.mute")}
        accessibilityRole="button"
      >
        <Ionicons name={icon} size={ICON_SIZE.lg} color="#fff" />
      </Pressable>
      {!compact && (
      <View
        style={styles.sliderTouch}
        onLayout={(e) => { widthRef.current = e.nativeEvent.layout.width; }}
        onStartShouldSetResponder={() => true}
        onMoveShouldSetResponder={() => true}
        onResponderGrant={(e) => onVolumeChange(valueFromX(e.nativeEvent.locationX))}
        onResponderMove={(e) => onVolumeChange(valueFromX(e.nativeEvent.locationX))}
        accessible={true}
        accessibilityRole="adjustable"
        accessibilityLabel={t("player.controls.volume")}
        accessibilityValue={{ min: 0, max: 200, now: pct, text: `${pct}%` }}
        onAccessibilityAction={(ev) => {
          if (ev.nativeEvent.actionName === "increment") onVolumeChange(Math.min(2, level + 0.1));
          else onVolumeChange(Math.max(0, level - 0.1));
        }}
        accessibilityActions={[{ name: "increment" }, { name: "decrement" }]}
      >
        <View style={styles.track}>
          <View style={{ width: `${Math.min(fillPct, 50)}%`, backgroundColor: theme.colors.accent }} />
          {boosted && <View style={{ width: `${fillPct - 50}%`, backgroundColor: theme.colors.warning }} />}
        </View>
        <View style={[styles.thumb, { left: `${fillPct}%` }]} />
      </View>
      )}
      {!compact && (
        <Text style={[styles.pct, { color: boosted ? theme.colors.warning : "rgba(255,255,255,0.7)" }]}>{pct}%</Text>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flexDirection: "row",
    alignItems: "center",
    gap: 4,
  },
  muteBtn: {
    padding: 4,
  },
  sliderTouch: {
    width: 80,
    height: 32,
    justifyContent: "center",
  },
  track: {
    height: 6,
    borderRadius: 3,
    backgroundColor: "rgba(255,255,255,0.3)",
    flexDirection: "row",
    overflow: "hidden",
  },
  thumb: {
    position: "absolute",
    top: 10,
    width: 12,
    height: 12,
    borderRadius: 6,
    marginLeft: -6,
    backgroundColor: "#fff",
  },
  pct: {
    fontSize: FONT_SIZE.xs,
    fontVariant: ["tabular-nums"],
    minWidth: 38,
    textAlign: "right",
  },
});
