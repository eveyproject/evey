import { useEffect } from "react";
import { View, Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useAdminSystemServiceGetSetupStatus } from "@evey/api";
import { useDemoStore } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";
import { FONT_SIZE } from "@evey/theme";

// Strip shown on the public demo instance, next to the impersonation banner:
// says the changes are reset and the administration is read-only, so a
// refused action reads as the demo's boundary and not as a bug. The server
// says which instance this is on its public setup-status answer; off the demo
// this renders nothing.
export function DemoBanner() {
  const theme = useTheme();
  const { t } = useTranslation();
  const setupStatus = useAdminSystemServiceGetSetupStatus({ query: { staleTime: 60_000 } });
  const demo = setupStatus.data?.demoMode === true;
  // The shared stores (locale, theme) read the flag: on the demo they keep
  // this device's choice instead of following the shared profile.
  useEffect(() => {
    if (setupStatus.data) useDemoStore.getState().setDemoMode(demo);
  }, [demo, setupStatus.data]);
  if (!demo) return null;
  return (
    <View style={[styles.banner, { backgroundColor: theme.colors.warning + "33" }]} accessibilityRole="text" testID="demo-banner">
      <Text style={[styles.label, { color: theme.colors.text }]} numberOfLines={2}>
        {t("common.demo.banner")}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  banner: {
    alignSelf: "stretch",
    paddingHorizontal: 16,
    paddingVertical: 8,
  },
  label: {
    fontSize: FONT_SIZE.xs,
    fontWeight: "600",
    textAlign: "center",
  },
});
