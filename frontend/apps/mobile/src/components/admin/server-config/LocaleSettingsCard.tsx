import { useMemo, useState } from "react";
import { View, Text, Pressable, Modal, TextInput, FlatList, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { configDefaultsKey, defaultRegionForLocale, getConfigDefaults } from "@evey/api";
import { timeZoneOptions, useServerClock } from "@evey/client";
import { useTheme } from "../../../providers/ThemeProvider";
import { regionOptions } from "../../../lib/regionOptions";
import { supportedLanguages } from "../../../i18n";
import { SettingRow } from "../settings/SettingRow";
import { ChipSelect } from "./ChipSelect";
import type { SaveEntry } from "./types";

const SERVER_LANG_KEY = "server.language";
const SERVER_REGION_KEY = "server.region";
const METADATA_LANG_KEY = "metadata.language";
// Kids profiles' opening hours are read in this zone (server-side).
const SERVER_TZ_KEY = "server.timezone";

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
  const [tzPickerOpen, setTzPickerOpen] = useState(false);
  const [tzQuery, setTzQuery] = useState("");

  const serverLang = getEntryValue(SERVER_LANG_KEY);
  const serverRegion = getEntryValue(SERVER_REGION_KEY);
  const metaLang = getEntryValue(METADATA_LANG_KEY);
  const serverTz = getEntryValue(SERVER_TZ_KEY);
  const zones = useMemo(() => timeZoneOptions(serverTz), [serverTz]);
  const filteredZones = useMemo(() => {
    const q = tzQuery.trim().toLowerCase().replace(/\s+/g, "_");
    return q ? zones.filter((z) => z.toLowerCase().includes(q)) : zones;
  }, [zones, tzQuery]);

  const regions = useMemo(() => regionOptions(i18n.language || "en"), [i18n.language]);
  const filteredRegions = useMemo(() => {
    const q = regionQuery.trim().toLowerCase();
    if (!q) return regions;
    return regions.filter((r) => r.label.toLowerCase().includes(q) || r.code.toLowerCase().includes(q));
  }, [regions, regionQuery]);

  const envDefaultLabel = t("admin.serverConfig.envDefault", "- env default -");
  // "Default: French", what an empty setting really resolves to; the region
  // and the metadata language follow the server language shown here.
  const { data: defaults } = useQuery({ queryKey: configDefaultsKey, queryFn: ({ signal }) => getConfigDefaults(signal) });
  const langName = (code?: string) => LANGUAGE_OPTIONS.find((o) => o.value === code)?.label ?? code;
  const effectiveLang = serverLang || defaults?.["server.language"] || "";
  const defaultLabel = (value: string | undefined) =>
    value ? t("admin.serverConfig.defaultValue", "Default: {{value}}", { value }) : envDefaultLabel;
  const regionDefault = effectiveLang ? (regions.find((r) => r.code === defaultRegionForLocale(effectiveLang))?.label ?? defaultRegionForLocale(effectiveLang)) : undefined;
  // The time it is now in the zone in force, by the server's clock.
  const serverTime = useServerClock(defaults?.now, serverTz || defaults?.["server.timezone"], i18n.language || "en");
  const tzDefaultLabel = defaultLabel(defaults?.["server.timezone"]?.replace(/_/g, " "));
  // Mirrors the web card: the "auto" region option spells out the region it
  // actually resolves to (server.language -> defaultRegionForLocale), so the
  // admin isn't left guessing what "auto" means.
  const regionEnvDefaultLabel = defaultLabel(regionDefault);
  const regionLabel = serverRegion
    ? (regions.find((r) => r.code === serverRegion)?.label ?? serverRegion)
    : regionEnvDefaultLabel;

  function pickZone(zone: string) {
    saveEntry({ key: SERVER_TZ_KEY, value: zone });
    setTzPickerOpen(false);
    setTzQuery("");
  }

  function pickRegion(code: string) {
    saveEntry({ key: SERVER_REGION_KEY, value: code });
    setRegionPickerOpen(false);
    setRegionQuery("");
  }

  const s = styles(theme);

  const langLabel = t("admin.serverConfig.serverLanguage", "Server language");
  const regionRowLabel = t("admin.serverConfig.serverRegion", "Server region");
  const metaLabel = t("admin.serverConfig.metadataLanguage", "Metadata language");
  const tzLabel = t("admin.serverConfig.timezone", "Server time zone");

  return (
    <View>
      <SettingRow label={langLabel} info={t("admin.settingsHelp.serverLanguage")} kind="wide" testID="setting-row-server-language">
        <ChipSelect
          label={langLabel}
          value={serverLang}
          onChange={(v) => saveEntry({ key: SERVER_LANG_KEY, value: v })}
          disabled={saving}
          options={[{ value: "", label: defaultLabel(langName(defaults?.["server.language"])) }, ...LANGUAGE_OPTIONS]}
        />
      </SettingRow>

      <SettingRow label={regionRowLabel} info={t("admin.settingsHelp.serverRegion")} testID="setting-row-server-region">
        <Pressable onPress={() => setRegionPickerOpen(true)} style={s.pickerBtn} accessibilityRole="button" accessibilityLabel={regionRowLabel}>
          <Text style={{ color: theme.colors.text, fontSize: 15 }}>{regionLabel}</Text>
        </Pressable>
      </SettingRow>

      <SettingRow label={metaLabel} info={t("admin.settingsHelp.metadataLanguage")} kind="wide" testID="setting-row-metadata-language">
        <ChipSelect
          label={metaLabel}
          value={metaLang}
          onChange={(v) => saveEntry({ key: METADATA_LANG_KEY, value: v })}
          disabled={saving}
          options={[{ value: "", label: defaultLabel(langName(effectiveLang || undefined)) }, ...LANGUAGE_OPTIONS]}
        />
      </SettingRow>

      <SettingRow
        label={tzLabel}
        info={t("admin.serverConfig.timezoneHint", "Kids profiles' opening hours are read in this zone, whatever the device's clock says.")}
        hint={
          serverTime ? (
            <Text style={{ color: theme.colors.text, fontWeight: "600" }} testID="server-time">
              {t("admin.serverConfig.timezoneNow", "Server time: {{time}}", { time: serverTime })}
            </Text>
          ) : undefined
        }
        testID="setting-row-server-timezone"
      >
        <Pressable onPress={() => setTzPickerOpen(true)} style={s.pickerBtn} accessibilityRole="button" accessibilityLabel={tzLabel}>
          <Text style={{ color: theme.colors.text, fontSize: 15 }}>{serverTz ? serverTz.replace(/_/g, " ") : tzDefaultLabel}</Text>
        </Pressable>
      </SettingRow>

      <Modal visible={tzPickerOpen} animationType="slide" onRequestClose={() => setTzPickerOpen(false)}>
        <View style={{ flex: 1, backgroundColor: theme.colors.bg, paddingTop: 48 }}>
          <TextInput
            value={tzQuery}
            onChangeText={setTzQuery}
            placeholder={t("common.search", "Search")}
            placeholderTextColor={theme.colors.textMuted}
            style={s.searchInput}
            autoCapitalize="none"
          />
          <FlatList
            data={["", ...filteredZones]}
            keyExtractor={(z: string) => z || "auto"}
            renderItem={({ item }: { item: string }) => (
              <Pressable onPress={() => pickZone(item)} style={s.regionRow}>
                <Text style={{ color: theme.colors.text, fontSize: 16 }}>{item ? item.replace(/_/g, " ") : tzDefaultLabel}</Text>
                {serverTz === item && <Text style={{ color: theme.colors.accent }}>{"✓"}</Text>}
              </Pressable>
            )}
          />
          <Pressable onPress={() => setTzPickerOpen(false)} style={s.closeBtn}>
            <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("common.close", "Close")}</Text>
          </Pressable>
        </View>
      </Modal>

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
    pickerBtn: { backgroundColor: t.colors.surfaceRaised, borderWidth: StyleSheet.hairlineWidth, borderColor: t.colors.border, borderRadius: t.radii.md, paddingHorizontal: 14, paddingVertical: 12 },
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
