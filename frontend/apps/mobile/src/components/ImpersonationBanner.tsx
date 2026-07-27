import { View, Text, TouchableOpacity, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";

// Admin "connect as" has no exit path of its own: without this banner the only
// way back to your own account is a full logout. It lives in the authenticated
// layout rather than on one screen, because an impersonating admin can navigate
// anywhere and needs the way out to follow them.
export function ImpersonationBanner() {
  const theme = useTheme();
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const impersonatedBy = useAuthStore((s) => s.impersonatedBy);
  const impersonatedUsername = useAuthStore((s) => s.impersonatedUsername);
  const exitImpersonation = useAuthStore((s) => s.exitImpersonation);

  if (!impersonatedBy) return null;

  function handleExit() {
    exitImpersonation();
    // Every cached response was fetched as the impersonated user; keeping any
    // of it would show their data under the admin's restored identity.
    queryClient.clear();
  }

  return (
    <View style={[styles.banner, { backgroundColor: theme.colors.warning + "33" }]}>
      <Text style={[styles.label, { color: theme.colors.text }]} numberOfLines={1}>
        {impersonatedUsername
          ? t("admin.users.impersonationBanner", { name: impersonatedUsername })
          : t("admin.users.impersonationBannerGeneric")}
      </Text>
      <TouchableOpacity onPress={handleExit} accessibilityRole="button">
        <Text style={[styles.action, { color: theme.colors.warning }]}>
          {t("admin.users.exitImpersonation")}
        </Text>
      </TouchableOpacity>
    </View>
  );
}

const styles = StyleSheet.create({
  banner: {
    flexDirection: "row",
    alignItems: "center",
    alignSelf: "stretch",
    gap: 12,
    paddingHorizontal: 16,
    paddingVertical: 8,
  },
  label: { fontSize: 12, flex: 1 },
  action: { fontSize: 12, fontWeight: "700" },
});
