import { memo } from "react";
import { ScrollView, View, Pressable, Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { useThemeStore } from "@evey/store";
import { themes } from "@evey/theme";
import { useTheme } from "../providers/ThemeProvider";

const PLUGIN_PREFIX = "plugin:";

interface PluginThemeInfo {
  id: string;
  name: string;
}

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

  const { data: pluginThemes } = useQuery<PluginThemeInfo[]>({
    queryKey: ["plugin-themes"],
    queryFn: async () => {
      const data = await eveyFetch<{ themes: PluginThemeInfo[] }>({
        url: "/api/v1/plugins/themes",
        method: "GET",
      });
      return data.themes ?? [];
    },
    staleTime: 60_000,
  });

  const pills: Pill[] = (Object.keys(themes) as Array<keyof typeof themes>).map(
    (name) => ({
      key: name,
      label: t(builtInI18nKeys[name], builtInLabels[name]),
      swatch: themes[name].colors.accent,
    }),
  );
  for (const pt of pluginThemes ?? []) {
    pills.push({
      key: PLUGIN_PREFIX + pt.name,
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
            onPress={() => setTheme(p.key, p.pluginId)}
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
    pillText: { color: tk.colors.text, fontSize: 14, fontWeight: "500" },
    pillTextActive: { color: tk.colors.onAccent, fontWeight: "600" },
  });
}
