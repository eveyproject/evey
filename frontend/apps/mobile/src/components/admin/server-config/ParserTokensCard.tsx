import { useEffect, useMemo, useState } from "react";
import { View, TextInput, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../providers/ThemeProvider";
import { SettingRow } from "../settings/SettingRow";
import { SaveButton } from "../settings/SaveButton";
import type { ConfigEntry, SaveEntry } from "./types";

const PARSER_CONFIG_KEY = "parser.config";

interface ParserConfigShape {
  extra_qualities?: string[];
  extra_edition_suffixes?: string[];
}

// Extends the filename parser's quality / edition token lists without a
// redeploy - one JSON blob under parser.config, re-applied in-process by the
// backend subscriber on every save. Mirrors the web ParserTokensCard.
export function ParserTokensCard({ entries, saveEntry, saving }: { entries: ConfigEntry[] | undefined; saveEntry: SaveEntry; saving: boolean }) {
  const theme = useTheme();
  const { t } = useTranslation();

  const raw = entries?.find((e) => e.key === PARSER_CONFIG_KEY)?.value ?? "";
  const initial: ParserConfigShape = useMemo(() => {
    if (!raw) return {};
    try {
      return JSON.parse(raw) as ParserConfigShape;
    } catch {
      return {};
    }
  }, [raw]);

  const initialQualities = (initial.extra_qualities ?? []).join(", ");
  const initialEditions = (initial.extra_edition_suffixes ?? []).join(", ");

  const [qualities, setQualities] = useState(initialQualities);
  const [editions, setEditions] = useState(initialEditions);
  const [dirty, setDirty] = useState(false);

  // Re-sync when the persisted blob changes externally.
  useEffect(() => {
    setQualities(initialQualities);
    setEditions(initialEditions);
    setDirty(false);
  }, [initialQualities, initialEditions]);

  function handleSave() {
    const parse = (s: string) =>
      s
        .split(",")
        .map((x) => x.trim())
        .filter((x) => x.length > 0);
    const payload: ParserConfigShape = {
      extra_qualities: parse(qualities),
      extra_edition_suffixes: parse(editions),
    };
    // Only clear the dirty flag once the write LANDS - clearing up front
    // leaves an unsaved draft behind a disabled Save button on failure
    // (same rule as BackgroundJobsCard).
    saveEntry(
      { key: PARSER_CONFIG_KEY, value: JSON.stringify(payload) },
      { onSuccess: () => setDirty(false) },
    );
  }

  const inputStyle = [
    styles.input,
    { backgroundColor: theme.colors.surfaceRaised, color: theme.colors.text, borderColor: theme.colors.border },
  ];

  const qualitiesLabel = t("admin.serverConfig.parserTokensQualities", "Extra quality tokens");
  const editionsLabel = t("admin.serverConfig.parserTokensEditions", "Extra edition suffixes");

  return (
    <View>
      <SettingRow label={qualitiesLabel} info={t("admin.settingsHelp.parserQualities")} hint={t("admin.settingsLayout.commaSeparated")}>
        <TextInput
          testID="parser-tokens-qualities"
          accessibilityLabel={qualitiesLabel}
          style={inputStyle}
          value={qualities}
          onChangeText={(v) => {
            setQualities(v);
            setDirty(true);
          }}
          placeholder="hdrip, hdr, web-dl"
          placeholderTextColor={theme.colors.textMuted}
          autoCapitalize="none"
          autoCorrect={false}
          multiline
        />
      </SettingRow>
      <SettingRow label={editionsLabel} info={t("admin.settingsHelp.parserEditions")} hint={t("admin.settingsLayout.commaSeparated")}>
        <TextInput
          testID="parser-tokens-editions"
          accessibilityLabel={editionsLabel}
          style={inputStyle}
          value={editions}
          onChangeText={(v) => {
            setEditions(v);
            setDirty(true);
          }}
          placeholder="distributor cut, anniversary edition, imax"
          placeholderTextColor={theme.colors.textMuted}
          autoCapitalize="none"
          autoCorrect={false}
          multiline
        />
      </SettingRow>
      <SaveButton testID="parser-tokens-save-btn" onPress={handleSave} disabled={!dirty || saving} />
    </View>
  );
}

const styles = StyleSheet.create({
  input: { borderWidth: 1, borderRadius: 8, paddingHorizontal: 12, paddingVertical: 9, fontSize: 13, minHeight: 44 },
});
