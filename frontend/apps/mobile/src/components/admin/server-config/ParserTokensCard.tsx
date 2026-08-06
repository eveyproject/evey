import { useEffect, useMemo, useState } from "react";
import { View, Text, TextInput, TouchableOpacity, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../providers/ThemeProvider";
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

  return (
    <View style={{ gap: 14 }}>
      <View style={{ gap: 4 }}>
        <Text style={{ color: theme.colors.text, fontSize: 14 }}>
          {t("admin.serverConfig.parserTokensQualities", "Extra quality tokens")}
        </Text>
        <TextInput
          testID="parser-tokens-qualities"
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
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
          {t(
            "admin.serverConfig.parserTokensQualitiesHint",
            "Comma-separated, single-word tokens (lowercased). The parser splits filenames on '.', '_', '-' and space.",
          )}
        </Text>
      </View>
      <View style={{ gap: 4 }}>
        <Text style={{ color: theme.colors.text, fontSize: 14 }}>
          {t("admin.serverConfig.parserTokensEditions", "Extra edition suffixes")}
        </Text>
        <TextInput
          testID="parser-tokens-editions"
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
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
          {t(
            "admin.serverConfig.parserTokensEditionsHint",
            "Comma-separated, can span multiple words (regex pre-tokenisation). Stripped from the title before TMDB lookup.",
          )}
        </Text>
      </View>
      <TouchableOpacity
        testID="parser-tokens-save-btn"
        onPress={handleSave}
        disabled={!dirty || saving}
        style={{
          backgroundColor: theme.colors.accent,
          borderRadius: theme.radii.sm,
          paddingVertical: 10,
          alignItems: "center",
          opacity: !dirty || saving ? 0.5 : 1,
        }}
      >
        <Text style={{ color: theme.colors.onAccent, fontSize: 14, fontWeight: "600" }}>{t("common.save", "Save")}</Text>
      </TouchableOpacity>
    </View>
  );
}

const styles = StyleSheet.create({
  input: { borderWidth: 1, borderRadius: 8, paddingHorizontal: 12, paddingVertical: 9, fontSize: 13, minHeight: 44 },
});
