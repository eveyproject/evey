import { useCallback, useRef } from "react";
import { View, Text, Pressable, StyleSheet, useWindowDimensions } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

interface PlayerVolumeControlProps {
  /** Niveau continu 0-2 : 0-1 suit le volume natif, 1-2 est l'amplification. */
  volume: number;
  muted: boolean;
  onToggleMute: () => void;
  onVolumeChange: (value: number) => void;
}

// Copie du PlayerVolumeControl web : icône mute + barre 0-2 dont la moitié
// haute (au-delà de 100 %) se remplit en couleur warning pour signaler
// l'amplification, + pourcentage à droite.
export function PlayerVolumeControl({ volume, muted, onToggleMute, onVolumeChange }: PlayerVolumeControlProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const widthRef = useRef(0);
  // Comme le web sous le breakpoint sm : en largeur étroite (portrait), seule
  // l'icône reste - la barre et le pourcentage n'apparaissent qu'en paysage.
  const { width: windowW } = useWindowDimensions();
  const compact = windowW < 600;

  const level = muted ? 0 : volume;
  const boosted = level > 1;
  const pct = Math.round(level * 100);
  const fillPct = (level / 2) * 100; // la barre couvre 0..2, 100 % tombe au milieu

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
