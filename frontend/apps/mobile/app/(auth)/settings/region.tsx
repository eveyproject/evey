import { useMemo, useState } from "react";
import { View, Text, TextInput, FlatList, Pressable, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceGetCurrentUser,
  useUserServiceUpdateProfile,
  getUserServiceListProfilesQueryKey,
} from "@evey/api";
import { toast, isDemoMode } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { regionOptions } from "../../../src/lib/regionOptions";
import { useActiveProfile } from "../../../src/hooks/useActiveProfile";

export default function RegionScreen() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();
  // The profile in front of the user, not the household's default one: the
  // region set here is what the parental age check reads on the ACTIVE profile
  // (offlineAccess.ts, useParentalGate), so writing it onto the default one
  // left the setting visibly saved and functionally dead.
  const { profile: editedProfile } = useActiveProfile();
  const updateProfile = useUserServiceUpdateProfile({
    mutation: {
      onError: () => toast.error(t("common.region.updateFailed", "Failed to update region")),
    },
  });

  const [query, setQuery] = useState("");
  const options = useMemo(() => regionOptions(i18n.language), [i18n.language]);
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return options;
    return options.filter((o) => o.label.toLowerCase().includes(q) || o.code.toLowerCase().includes(q));
  }, [options, query]);

  const current = editedProfile?.region ?? "";

  async function pick(code: string) {
    if (!editedProfile?.id) return;
    // Demo: the region lives on the shared profile only. Shown, not changed.
    if (isDemoMode()) {
      toast.info(t("common.demo.locked"));
      return;
    }
    try {
      await updateProfile.mutateAsync({ id: editedProfile.id, data: { region: code } });
      if (user?.id) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) });
    } catch {
      // mutation onError already toasts
    }
  }

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("common.region.title"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />
      <TextInput
        placeholder={t("common.search") || "Search"}
        placeholderTextColor={theme.colors.textMuted}
        value={query}
        onChangeText={setQuery}
        style={{
          margin: theme.spacing.md,
          padding: theme.spacing.sm,
          borderRadius: theme.radii.md,
          backgroundColor: theme.colors.surface,
          color: theme.colors.text,
        }}
      />
      <FlatList
        data={[{ code: "", label: t("common.region.auto") }, ...filtered]}
        keyExtractor={(it) => it.code || "auto"}
        renderItem={({ item }) => (
          <Pressable
            onPress={() => pick(item.code)}
            style={({ pressed }) => [styles.row, { borderBottomColor: theme.colors.border, backgroundColor: pressed ? theme.colors.surfaceRaised : "transparent" }]}
            accessibilityRole="radio"
            accessibilityLabel={item.label}
            accessibilityState={{ selected: current === item.code }}
          >
            <Text style={[styles.label, { color: theme.colors.text }]} numberOfLines={2}>{item.label}</Text>
            {current === item.code && (
              <Text style={[styles.check, { color: theme.colors.accent }]}>{"\u2713"}</Text>
            )}
          </Pressable>
        )}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", paddingHorizontal: 16, paddingVertical: 12, borderBottomWidth: StyleSheet.hairlineWidth },
  label: { fontSize: 16, flex: 1 },
  check: { fontWeight: "700", flexShrink: 0, marginLeft: 8 },
});
