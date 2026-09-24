import { View, Text, Pressable, StyleSheet, useWindowDimensions } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";
import { useBarGesture } from "../../hooks/useBarGesture";

interface PlayerVolumeControlProps {
  /** Continuous level 0-2: 0-1 tracks native volume, 1-2 is the amplification. */
  volume: number;
  muted: boolean;
  onToggleMute: () => void;
  onVolumeChange: (value: number) => void;
  /** Top of the bar: 2 where the boost exists, 1 where nothing can amplify. */
  maxLevel?: number;
}

// Copy of the web PlayerVolumeControl: mute icon + a 0-2 bar whose top half
// (beyond 100%) fills with the warning color to signal amplification,
// plus a percentage on the right.
export function PlayerVolumeControl({ volume, muted, onToggleMute, onVolumeChange, maxLevel = 2 }: PlayerVolumeControlProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  // Like the web under the sm breakpoint: in narrow width (portrait), only
  // the icon remains - the bar and percentage only appear in landscape.
  const { width: windowW } = useWindowDimensions();
  const compact = windowW < 600;

  const level = muted ? 0 : Math.min(volume, maxLevel);
  const boosted = level > 1;
  const pct = Math.round(level * 100);
  const fillPct = (level / maxLevel) * 100; // with the boost, 100% lands in the middle
  const unitPct = 100 / maxLevel;

  const { barProps } = useBarGesture((f) => onVolumeChange(f * maxLevel));

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
        {...barProps}
        style={styles.sliderTouch}
        accessible={true}
        accessibilityRole="adjustable"
        accessibilityLabel={t("player.controls.volume")}
        accessibilityValue={{ min: 0, max: maxLevel * 100, now: pct, text: `${pct}%` }}
        onAccessibilityAction={(ev) => {
          if (ev.nativeEvent.actionName === "increment") onVolumeChange(Math.min(maxLevel, level + 0.1));
          else onVolumeChange(Math.max(0, level - 0.1));
        }}
        accessibilityActions={[{ name: "increment" }, { name: "decrement" }]}
      >
        <View style={styles.track}>
          <View style={{ width: `${Math.min(fillPct, unitPct)}%`, backgroundColor: theme.colors.accent }} />
          {boosted && <View style={{ width: `${fillPct - unitPct}%`, backgroundColor: theme.colors.warning }} />}
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
