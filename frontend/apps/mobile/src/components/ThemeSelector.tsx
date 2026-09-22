import { memo, useCallback, useEffect } from "react";
import { ScrollView, View, Pressable, Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, useUserServiceUpdateProfile } from "@evey/api";
import { useThemeStore, useAuthStore, themePreferences } from "@evey/store";
import { themes, FONT_SIZE } from "@evey/theme";
import { pluginThemeKey, usePluginThemes } from "@evey/client";
import { useTheme } from "../providers/ThemeProvider";

const builtInLabels: Record<string, string> = {
  default: "Dark",
  amoled: "AMOLED",
  "dark-red": "Dark Red",
  "light-default": "Light",
  "light-blue": "Light Blue",
};

const builtInI18nKeys: Record<string, string> = {
  default: "mobile.settings.themeDark",
  amoled: "mobile.settings.themeAmoled",
  "dark-red": "mobile.settings.themeDarkRed",
  "light-default": "mobile.settings.themeLight",
  "light-blue": "mobile.settings.themeLightBlue",
};

interface Pill {
  key: string;
  label: string;
  swatch: string;
  pluginId?: string;
}

function ThemeSelectorImpl() {
  const { t } = useTranslation();
  const tk = useTheme();
  const themeName = useThemeStore((s) => s.theme);
  const setTheme = useThemeStore((s) => s.setTheme);
  const applyFromPreferences = useThemeStore((s) => s.applyFromPreferences);

  // The theme belongs to the profile, not to this handset: pick it here and
  // the web and the television follow, and vice versa.
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const activeProfileId = useAuthStore((s) => s.profileId);
  const activeProfile =
    profilesResp?.profiles?.find((p: { id?: string }) => p.id === activeProfileId) ??
    profilesResp?.profiles?.find((p: { isDefault?: boolean }) => p.isDefault) ??
    profilesResp?.profiles?.[0];
  const updateProfile = useUserServiceUpdateProfile();

  useEffect(() => {
    applyFromPreferences(activeProfile?.preferences);
  }, [activeProfile?.preferences, applyFromPreferences]);

  // MERGED, because UpdateProfile replaces the whole preferences map server
  // side - sending just the theme would wipe the audio/subtitle languages.
  const pickTheme = useCallback(
    (key: string, pluginId?: string) => {
      setTheme(key, pluginId);
      if (!activeProfile?.id) return;
      updateProfile.mutate({
        id: activeProfile.id,
        data: { preferences: themePreferences(activeProfile.preferences, key, pluginId) },
      });
    },
    [setTheme, updateProfile, activeProfile],
  );

  const { data: pluginThemes } = usePluginThemes();

  const pills: Pill[] = (Object.keys(themes) as Array<keyof typeof themes>).map(
    (name) => ({
      key: name,
      label: t(builtInI18nKeys[name], builtInLabels[name]),
      swatch: themes[name].colors.accent,
    }),
  );
  for (const pt of pluginThemes ?? []) {
    pills.push({
      key: pluginThemeKey(pt.name),
      label: pt.name,
      swatch: tk.colors.accent,
      pluginId: pt.id,
    });
  }

  const styles = makeStyles(tk);

  return (
    <ScrollView
      horizontal
      showsHorizontalScrollIndicator={false}
      contentContainerStyle={styles.row}
    >
      {pills.map((p) => {
        const active = themeName === p.key;
        return (
          <Pressable
            key={p.key}
            onPress={() => pickTheme(p.key, p.pluginId)}
            accessible={true}
            accessibilityRole="radio"
            accessibilityState={{ selected: active }}
            accessibilityLabel={p.label}
          >
            <View style={[styles.pill, active && styles.pillActive]}>
              <View style={[styles.swatch, { backgroundColor: p.swatch }]} />
              <Text style={[styles.pillText, active && styles.pillTextActive]}>
                {p.label}
              </Text>
            </View>
          </Pressable>
        );
      })}
    </ScrollView>
  );
}

// memo: no props; ThemeStore + plugin themes query gate re-renders.
// Parent re-renders (settings screen state changes) become free.
export const ThemeSelector = memo(ThemeSelectorImpl);

function makeStyles(tk: ReturnType<typeof useTheme>) {
  return StyleSheet.create({
    row: {
      paddingHorizontal: tk.spacing.md,
      paddingVertical: tk.spacing.sm,
      gap: tk.spacing.sm,
    },
    pill: {
      flexDirection: "row",
      alignItems: "center",
      gap: tk.spacing.xs + 2,
      paddingHorizontal: tk.spacing.md,
      paddingVertical: tk.spacing.sm,
      borderRadius: tk.radii.md,
      backgroundColor: tk.colors.surfaceRaised,
      borderWidth: 1,
      borderColor: tk.colors.border,
    },
    pillActive: {
      backgroundColor: tk.colors.accent,
      borderColor: tk.colors.accent,
    },
    swatch: {
      width: 14,
      height: 14,
      borderRadius: 7,
      borderWidth: StyleSheet.hairlineWidth,
      borderColor: "rgba(255,255,255,0.25)",
    },
    pillText: { color: tk.colors.text, fontSize: FONT_SIZE.sm, fontWeight: "500" },
    pillTextActive: { color: tk.colors.onAccent, fontWeight: "600" },
  });
}
