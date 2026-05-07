import { View, Text, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useTheme } from "../providers/ThemeProvider";
import { useOnlineStatus } from "../hooks/useOnlineStatus";

/**
 * Persistent banner that shows when the device loses internet
 * connectivity. Mirrors the pattern Spotify/Netflix use: a thin coloured
 * strip at the top edge with an icon and a one-liner so the user
 * understands why content stops updating instead of staring at empty
 * skeletons.
 */
export function OfflineBanner() {
  const online = useOnlineStatus();
  const theme = useTheme();
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();

  if (online) return null;

  return (
    <View
      style={[
        styles.banner,
        {
          paddingTop: Math.max(8, insets.top + 4),
          backgroundColor: theme.colors.warning,
        },
      ]}
      accessible
      accessibilityRole="alert"
      accessibilityLiveRegion="polite"
      accessibilityLabel={t("common.offline.title", "Offline")}
    >
      <Ionicons name="cloud-offline-outline" size={14} color="#fff" />
      <Text style={styles.text}>{t("common.offline.title", "Offline")}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  banner: {
    position: "absolute",
    top: 0,
    left: 0,
    right: 0,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 6,
    paddingBottom: 6,
    zIndex: 1000,
    elevation: 1000,
  },
  text: {
    color: "#fff",
    fontSize: 12,
    fontWeight: "600",
  },
});
