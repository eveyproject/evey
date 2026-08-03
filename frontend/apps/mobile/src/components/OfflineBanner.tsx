import { memo, useEffect, useRef, useState } from "react";
import { View, Text, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { usePathname } from "expo-router";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useTheme } from "../providers/ThemeProvider";
import { useOnlineStatus } from "../hooks/useOnlineStatus";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

// Long enough to be read, short enough that browsing offline (a supported
// mode, not an error state) isn't spent under a permanent warning strip.
const AUTO_HIDE_MS = 5000;

/**
 * Banner that announces the device just lost internet connectivity, then
 * gets out of the way. It shows on the online->offline TRANSITION and
 * auto-hides a few seconds later: offline is a supported mode (downloads
 * play fine), so a permanent strip reads as a nag, not information - the
 * screens' own offline states carry the rest.
 *
 * Never shown over the player: it floats above every route of the (auth)
 * layout, and the player is a fullscreen surface whose own chrome auto-hides
 * - a strip that stays visible there defeats both.
 */
function OfflineBannerImpl() {
  const online = useOnlineStatus();
  const theme = useTheme();
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();
  const pathname = usePathname();

  const [visible, setVisible] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (timer.current) {
      clearTimeout(timer.current);
      timer.current = null;
    }
    if (!online) {
      setVisible(true);
      timer.current = setTimeout(() => setVisible(false), AUTO_HIDE_MS);
    } else {
      setVisible(false);
    }
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [online]);

  if (online || !visible) return null;
  if (pathname?.startsWith("/player")) return null;

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
      <Ionicons name="cloud-offline-outline" size={ICON_SIZE.sm} color={theme.colors.onWarning} />
      <Text style={[styles.text, { color: theme.colors.onWarning }]}>{t("common.offline.title", "Offline")}</Text>
    </View>
  );
}

// memo: no props; re-renders only when its own hooks (online status, theme,
// i18n, route) change. Parent re-renders (every navigation, every state
// mutation in the root layout) become free for this banner.
export const OfflineBanner = memo(OfflineBannerImpl);

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
    fontSize: FONT_SIZE.xs,
    fontWeight: "600",
  },
});
