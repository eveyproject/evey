import { useState } from "react";
import { Linking, Pressable, StyleSheet, Text, TextInput, View } from "react-native";
import { useTranslation } from "react-i18next";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../../../providers/ThemeProvider";
import { SettingRow } from "../settings/SettingRow";
import type { SaveEntry } from "./types";

// The metadata provider keys, same list, copy and links as the web's
// ApiKeysCard.
export const API_KEY_FIELDS = [
  { dbKey: "api.tmdb_key", labelKey: "admin.serverConfig.apiKeys.tmdb", infoKey: "admin.settingsHelp.apiKeyTmdb", linkKey: "admin.serverConfig.apiKeys.tmdbLink", linkUrl: "https://www.themoviedb.org/settings/api", hasDefault: true },
  { dbKey: "api.omdb_key", labelKey: "admin.serverConfig.apiKeys.omdb", infoKey: "admin.settingsHelp.apiKeyOmdb", linkKey: "admin.serverConfig.apiKeys.omdbLink", linkUrl: "https://www.omdbapi.com/apikey.aspx", hasDefault: false },
  { dbKey: "api.opensubtitles_key", labelKey: "admin.serverConfig.apiKeys.opensubtitles", infoKey: "admin.settingsHelp.apiKeyOpenSubtitles", linkKey: "admin.serverConfig.apiKeys.opensubtitlesLink", linkUrl: "https://www.opensubtitles.com/en/consumers", hasDefault: false },
  { dbKey: "api.tidb_key", labelKey: "admin.serverConfig.apiKeys.tidb", infoKey: "admin.settingsHelp.apiKeyTidb", linkKey: "admin.serverConfig.apiKeys.tidbLink", linkUrl: "https://theintrodb.org/account", hasDefault: false },
] as const;

// Mirrors appconfig.SecretMask (internal/app/config/secrets.go): a configured
// key reads back as this mask, never as its value.
const SECRET_MASK = "••••••••";

interface ApiKeysCardProps {
  getEntryValue: (key: string) => string;
  saveEntry: SaveEntry;
  deleteEntry: (key: string) => void;
  saving: boolean;
}

export function ApiKeysCard({ getEntryValue, saveEntry, deleteEntry, saving }: ApiKeysCardProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [shown, setShown] = useState<Record<string, boolean>>({});

  // An untouched field on a configured key holds the mask, and an emptied
  // one would wipe a credential nobody can read back: neither is a write, so
  // Save stays greyed until a new key is typed. Going back to the shared
  // TMDB key is what Reset is for (the web's rule: only TMDB has a default).
  function savable(draft: string, current: string) {
    return draft !== "" && draft !== SECRET_MASK && draft !== current;
  }

  return (
    <>
      {API_KEY_FIELDS.map((field) => {
        const current = getEntryValue(field.dbKey);
        const draft = drafts[field.dbKey] ?? current;
        const isShown = shown[field.dbKey] ?? false;
        const label = t(field.labelKey);
        const canSave = savable(draft, current);
        return (
          <SettingRow
            key={field.dbKey}
            label={label}
            info={t(field.infoKey)}
            hint={
              <Text style={{ color: theme.colors.accent }} accessibilityRole="link" onPress={() => void Linking.openURL(field.linkUrl)}>
                {t(field.linkKey)}
              </Text>
            }
            kind="wide"
            testID={`api-key-row-${field.dbKey}`}
          >
            <View style={styles.badges}>
              {field.hasDefault && !current ? (
                <View style={[styles.badge, { backgroundColor: theme.colors.surfaceRaised }]}>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{t("admin.serverConfig.apiKeys.usingDefault")}</Text>
                </View>
              ) : null}
              {current ? (
                <View style={[styles.badge, { backgroundColor: theme.colors.surfaceRaised }]}>
                  <Text style={{ color: theme.colors.accent, fontSize: 11 }}>{t("admin.serverConfig.apiKeys.usingCustom")}</Text>
                </View>
              ) : null}
            </View>
            <View style={styles.inputLine}>
              <TextInput
                value={draft}
                onChangeText={(v) => setDrafts((p) => ({ ...p, [field.dbKey]: v }))}
                style={[styles.input, { backgroundColor: theme.colors.surfaceRaised, color: theme.colors.text, borderRadius: theme.radii.md, borderColor: theme.colors.border }]}
                placeholderTextColor={theme.colors.textMuted}
                placeholder={current ? t("admin.serverConfig.apiKeys.keepCurrent") : label}
                accessibilityLabel={label}
                autoComplete="off"
                autoCapitalize="none"
                autoCorrect={false}
                secureTextEntry={!isShown}
                testID={`api-key-input-${field.dbKey}`}
              />
              <Pressable
                onPress={() => setShown((p) => ({ ...p, [field.dbKey]: !p[field.dbKey] }))}
                style={[styles.smallBtn, { borderColor: theme.colors.border, backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
                hitSlop={8}
                accessibilityRole="button"
                accessibilityLabel={isShown ? t("admin.serverConfig.hide") : t("admin.serverConfig.show")}
              >
                <Text style={{ color: theme.colors.textMuted, fontSize: 12, fontWeight: "600" }}>
                  {isShown ? t("admin.serverConfig.hide") : t("admin.serverConfig.show")}
                </Text>
              </Pressable>
            </View>
            <View style={styles.actions}>
              <Pressable
                onPress={() => saveEntry({ key: field.dbKey, value: draft })}
                disabled={saving || !canSave}
                accessibilityRole="button"
                accessibilityLabel={`${t("common.save")} ${label}`}
                accessibilityState={{ disabled: saving || !canSave }}
                testID={`api-key-save-${field.dbKey}`}
                style={[styles.action, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: saving || !canSave ? 0.5 : 1 }]}
              >
                <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: FONT_SIZE.xs }}>{t("common.save")}</Text>
              </Pressable>
              {field.hasDefault && current ? (
                <Pressable
                  onPress={() => {
                    deleteEntry(field.dbKey);
                    setDrafts((p) => ({ ...p, [field.dbKey]: "" }));
                  }}
                  disabled={saving}
                  accessibilityRole="button"
                  accessibilityLabel={`${t("admin.serverConfig.apiKeys.resetToDefault")} ${label}`}
                  testID={`api-key-reset-${field.dbKey}`}
                  style={[styles.action, { backgroundColor: theme.colors.border, borderRadius: theme.radii.md, opacity: saving ? 0.5 : 1 }]}
                >
                  <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: FONT_SIZE.xs }}>{t("admin.serverConfig.apiKeys.resetToDefault")}</Text>
                </Pressable>
              ) : null}
            </View>
          </SettingRow>
        );
      })}
    </>
  );
}

const styles = StyleSheet.create({
  badges: { flexDirection: "row", gap: 6, marginBottom: 6 },
  badge: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: 4 },
  inputLine: { flexDirection: "row", alignItems: "center", gap: 8 },
  input: { flex: 1, borderWidth: StyleSheet.hairlineWidth, paddingHorizontal: 14, paddingVertical: 12, fontSize: FONT_SIZE.sm },
  smallBtn: { paddingHorizontal: 10, paddingVertical: 12, borderWidth: StyleSheet.hairlineWidth },
  actions: { flexDirection: "row", gap: 8, marginTop: 8 },
  action: { flex: 1, minHeight: 44, alignItems: "center", justifyContent: "center" },
});
