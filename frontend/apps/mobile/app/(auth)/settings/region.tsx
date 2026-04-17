import { useMemo, useState } from "react";
import { View, Text, TextInput, FlatList, Pressable, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
  useUserServiceUpdateProfile,
  getUserServiceListProfilesQueryKey,
} from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";

function regionOptions(locale: string): { code: string; label: string }[] {
  // Intl.supportedValuesOf exists in modern JSC/Hermes (Expo SDK 51+).
  const sv = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf;
  const codes: string[] = typeof sv === "function" ? sv("region") : [];
  const names = new Intl.DisplayNames([locale], { type: "region" });
  return codes
    .filter((c) => /^[A-Z]{2}$/.test(c))
    .map((c) => ({ code: c, label: names.of(c) ?? c }))
    .sort((a, b) => a.label.localeCompare(b.label, locale));
}

export default function RegionScreen() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } } as never);
  const defaultProfile = profilesResp?.profiles?.find((p) => p.isDefault) ?? profilesResp?.profiles?.[0];
  const updateProfile = useUserServiceUpdateProfile();

  const [query, setQuery] = useState("");
  const options = useMemo(() => regionOptions(i18n.language), [i18n.language]);
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return options;
    return options.filter((o) => o.label.toLowerCase().includes(q) || o.code.toLowerCase().includes(q));
  }, [options, query]);

  const current = defaultProfile?.region ?? "";

  async function pick(code: string) {
    if (!defaultProfile?.id) return;
    try {
      await updateProfile.mutateAsync({ id: defaultProfile.id, data: { region: code } });
      if (user?.id) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) });
    } catch (err) {
      console.warn("region update failed", err);
    }
  }

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{ title: t("common.region.title") }} />
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
            style={({ pressed }) => [styles.row, { backgroundColor: pressed ? theme.colors.surfaceRaised : "transparent" }]}
          >
            <Text style={{ color: theme.colors.text, fontSize: 16 }}>{item.label}</Text>
            {current === item.code && (
              <Text style={{ color: theme.colors.accent, fontWeight: "700" }}>{"\u2713"}</Text>
            )}
          </Pressable>
        )}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", paddingHorizontal: 16, paddingVertical: 12, borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: "rgba(255,255,255,0.08)" },
});
