import { useEffect } from "react";
import { View, Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useAdminSystemServiceGetSetupStatus } from "@evey/api";
import { useDemoStore } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";

// One line at the top of the authenticated shell on the public demo instance:
// the changes are reset and the administration is read-only. The server says
// which instance this is on its public setup-status answer; off the demo this
// renders nothing, so the shell paints no extra layer (overdraw matters on a
// television, see (auth)/_layout.tsx).
export function DemoBanner() {
  const theme = useTheme();
  const { t } = useTranslation();
  const setupStatus = useAdminSystemServiceGetSetupStatus({ query: { staleTime: 60_000 } });
  const demo = setupStatus.data?.demoMode === true;
  // The shared stores (locale, theme) read the flag: on the demo they keep
  // this television's choice instead of following the shared profile.
  useEffect(() => {
    if (setupStatus.data) {
      useDemoStore.getState().setDemoMode(demo);
      useDemoStore.getState().setExtendedDemo(setupStatus.data.demoExtended === true);
    }
  }, [demo, setupStatus.data]);
  if (!demo) return null;
  return (
    <View style={[styles.banner, { backgroundColor: theme.colors.warning + "33" }]} testID="demo-banner">
      <Text style={[styles.label, { color: theme.colors.text }]} numberOfLines={1}>
        {t("common.demo.banner")}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  banner: {
    alignSelf: "stretch",
    paddingHorizontal: 48,
    paddingVertical: 6,
  },
  label: {
    fontSize: 16,
    fontWeight: "600",
    textAlign: "center",
  },
});
