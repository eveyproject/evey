import { View, Pressable, StyleSheet, type ViewStyle } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { CastButtonNative } from "../../hooks/useChromecast";
import { ICON_SIZE } from "@evey/theme";

interface PlayerTopRightClusterProps {
  syncConnected: boolean;
  castConnected: boolean;
  castAvailable: boolean;
  onCast: () => void;
  onSyncPress: () => void;
  // "Play on" (send this title to another connected Evey client) - web exposes
  // it in the player's RemotePlaybackMenu alongside cast, so it belongs here too.
  onPlayOnPress?: () => void;
  style?: ViewStyle;
}

// Mirrors web's HLSPlayer "Top-right cluster" (Watch together, then cast) -
// moved out of the bottom transport row per the 2026-07-25 feedback that cast
// and watch-together belonged top-right, not buried among the other buttons.
// Fades with the rest of the controls (parent only renders this while
// showControls is true) instead of staying pinned on screen during playback.
export function PlayerTopRightCluster({
  syncConnected,
  castConnected,
  castAvailable,
  onCast,
  onSyncPress,
  onPlayOnPress,
  style,
}: PlayerTopRightClusterProps) {
  const theme = useTheme();
  const { t } = useTranslation();

  return (
    <View style={[styles.cluster, style]}>
      <Pressable
        onPress={onSyncPress}
        hitSlop={8}
        style={styles.button}
        accessible={true}
        accessibilityLabel={syncConnected ? t("media.syncplay.leave", "Leave") : t("media.syncplay.title", "Watch Together")}
        accessibilityRole="button"
      >
        <Ionicons name="people" size={ICON_SIZE.lg} color={syncConnected ? theme.colors.accent : "#fff"} />
      </Pressable>
      {onPlayOnPress && (
        <Pressable
          onPress={onPlayOnPress}
          hitSlop={8}
          style={styles.button}
          accessible={true}
          accessibilityLabel={t("player.playOn.button")}
          accessibilityRole="button"
        >
          <Ionicons name="phone-portrait-outline" size={ICON_SIZE.lg} color="#fff" />
        </Pressable>
      )}
      {castAvailable && (
        <>
          {/* Invisible native cast button, mounted purely so the platform SDK can
              discover receivers - keep it rendered even though onCast (below)
              is the real tap target, exactly as it was in the bottom row before. */}
          {CastButtonNative && <CastButtonNative style={{ width: 0, height: 0, opacity: 0 }} />}
          <Pressable
            onPress={onCast}
            hitSlop={8}
            style={styles.button}
            accessible={true}
            accessibilityLabel={castConnected ? t("player.controls.stopCast") : t("player.controls.cast")}
            accessibilityRole="button"
          >
            <Ionicons name="tv-outline" size={ICON_SIZE.lg} color={castConnected ? theme.colors.accent : "#fff"} />
          </Pressable>
        </>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  cluster: {
    position: "absolute",
    top: 0,
    right: 0,
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    zIndex: 3,
  },
  button: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: "rgba(255,255,255,0.15)",
    justifyContent: "center",
    alignItems: "center",
  },
});
