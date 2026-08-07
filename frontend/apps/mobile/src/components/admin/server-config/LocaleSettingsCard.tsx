import { useMemo, useState } from "react";
import { View, Text, Pressable, Modal, TextInput, FlatList, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { defaultRegionForLocale } from "@evey/api";
import { useTheme } from "../../../providers/ThemeProvider";
import { regionOptions } from "../../../lib/regionOptions";
import { supportedLanguages } from "../../../i18n";
import { ChipSelect } from "./ChipSelect";
import type { SaveEntry } from "./types";

const SERVER_LANG_KEY = "server.language";
const SERVER_REGION_KEY = "server.region";
const METADATA_LANG_KEY = "metadata.language";

const LANGUAGE_OPTIONS = supportedLanguages.map((l) => ({ value: l.code, label: l.label }));

interface LocaleSettingsCardProps {
  getEntryValue: (key: string) => string;
  saveEntry: SaveEntry;
  saving: boolean;
}

// Unlike the web card, this does NOT call useLocaleStore().setLocale after
// saving server.language. On mobile, language precedence is
// profile > user choice > device > server default; flipping this admin's
// own UI language here would fight that chain instead of just persisting
// the server-wide default.
export function LocaleSettingsCard({ getEntryValue, saveEntry, saving }: LocaleSettingsCardProps) {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const [regionPickerOpen, setRegionPickerOpen] = useState(false);
  const [regionQuery, setRegionQuery] = useState("");

  const serverLang = getEntryValue(SERVER_LANG_KEY);
  const serverRegion = getEntryValue(SERVER_REGION_KEY);
  const metaLang = getEntryValue(METADATA_LANG_KEY);

  const regions = useMemo(() => regionOptions(i18n.language || "en"), [i18n.language]);
  const filteredRegions = useMemo(() => {
    const q = regionQuery.trim().toLowerCase();
    if (!q) return regions;
    return regions.filter((r) => r.label.toLowerCase().includes(q) || r.code.toLowerCase().includes(q));
  }, [regions, regionQuery]);

  const envDefaultLabel = t("admin.serverConfig.envDefault", "- env default -");
  // Mirrors the web card: the "auto" region option spells out the region it
  // actually resolves to (server.language -> defaultRegionForLocale), so the
  // admin isn't left guessing what "auto" means.
  const regionEnvDefaultLabel = `${envDefaultLabel} (${defaultRegionForLocale(serverLang)})`;
  const regionLabel = serverRegion
    ? (regions.find((r) => r.code === serverRegion)?.label ?? serverRegion)
    : regionEnvDefaultLabel;

  function pickRegion(code: string) {
    saveEntry({ key: SERVER_REGION_KEY, value: code });
    setRegionPickerOpen(false);
    setRegionQuery("");
  }

  const s = styles(theme);

  return (
    <View>
      <Text style={s.label}>{t("admin.serverConfig.serverLanguage", "Server language")}</Text>
      <ChipSelect
        value={serverLang}
        onChange={(v) => saveEntry({ key: SERVER_LANG_KEY, value: v })}
        disabled={saving}
        options={[{ value: "", label: envDefaultLabel }, ...LANGUAGE_OPTIONS]}
      />

      <Text style={[s.label, { marginTop: 16 }]}>{t("admin.serverConfig.serverRegion", "Server region")}</Text>
      <Pressable
        onPress={() => setRegionPickerOpen(true)}
        style={s.pickerBtn}
        accessibilityRole="button"
        accessibilityLabel={t("admin.serverConfig.serverRegion", "Server region")}
      >
        <Text style={{ color: theme.colors.text, fontSize: 15 }}>{regionLabel}</Text>
      </Pressable>

      <Text style={[s.label, { marginTop: 16 }]}>{t("admin.serverConfig.metadataLanguage", "Metadata language")}</Text>
      <ChipSelect
        value={metaLang}
        onChange={(v) => saveEntry({ key: METADATA_LANG_KEY, value: v })}
        disabled={saving}
        options={[{ value: "", label: envDefaultLabel }, ...LANGUAGE_OPTIONS]}
      />

      <Modal visible={regionPickerOpen} animationType="slide" onRequestClose={() => setRegionPickerOpen(false)}>
        <View style={{ flex: 1, backgroundColor: theme.colors.bg, paddingTop: 48 }}>
          <TextInput
            value={regionQuery}
            onChangeText={setRegionQuery}
            placeholder={t("common.search", "Search")}
            placeholderTextColor={theme.colors.textMuted}
            style={s.searchInput}
            autoCapitalize="none"
          />
          <FlatList
            data={[{ code: "", label: regionEnvDefaultLabel }, ...filteredRegions]}
            keyExtractor={(it: { code: string }) => it.code || "auto"}
            renderItem={({ item }: { item: { code: string; label: string } }) => (
              <Pressable onPress={() => pickRegion(item.code)} style={s.regionRow}>
                <Text style={{ color: theme.colors.text, fontSize: 16 }}>{item.label}</Text>
                {serverRegion === item.code && <Text style={{ color: theme.colors.accent }}>{"✓"}</Text>}
              </Pressable>
            )}
          />
          <Pressable onPress={() => setRegionPickerOpen(false)} style={s.closeBtn}>
            <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("common.close", "Close")}</Text>
          </Pressable>
        </View>
      </Modal>
    </View>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    label: { fontSize: 12, fontWeight: "600", textTransform: "uppercase", marginBottom: 6, color: t.colors.textMuted },
    pickerBtn: { backgroundColor: t.colors.surface, borderRadius: t.radii.md, paddingHorizontal: 14, paddingVertical: 12 },
    searchInput: { margin: 16, padding: 12, borderRadius: t.radii.md, backgroundColor: t.colors.surface, color: t.colors.text },
    regionRow: {
      flexDirection: "row",
      justifyContent: "space-between",
      alignItems: "center",
      paddingHorizontal: 16,
      paddingVertical: 12,
      borderBottomWidth: StyleSheet.hairlineWidth,
      borderBottomColor: t.colors.border,
    },
    closeBtn: { margin: 16, paddingVertical: 14, borderRadius: 8, alignItems: "center", backgroundColor: t.colors.accent },
  });
