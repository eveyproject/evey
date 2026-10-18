import { useState, useCallback, useEffect, useRef } from "react";
import { View, Text, TextInput, Pressable, ScrollView, Modal, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";
import { useAllGenres } from "../hooks/useAllGenres";

type RuleField = "genre" | "year" | "type" | "library_id" | "rating_gte";
type Operator = "eq" | "gte" | "lte" | "gt" | "lt";

export interface SmartRule {
  field: RuleField;
  operator: Operator;
  value: string;
}

const FIELD_LABELS: Record<RuleField, string> = {
  genre: "Genre",
  year: "Year",
  type: "Type",
  library_id: "Library",
  rating_gte: "Rating ≥",
};

// The i18n key of each field; FIELD_LABELS above is only the fallback copy.
const FIELD_I18N_KEYS: Record<RuleField, string> = {
  genre: "media.playlist.fieldGenre",
  year: "media.playlist.fieldYear",
  type: "media.playlist.fieldType",
  library_id: "media.playlist.fieldLibrary",
  rating_gte: "media.playlist.fieldRating",
};

// Playlists only support these four fields; collections add rating_gte (web
// parity: SmartCollectionDialog). Callers pass availableFields to opt in.
const PLAYLIST_FIELDS: RuleField[] = ["genre", "year", "type", "library_id"];
export const COLLECTION_FIELDS: RuleField[] = ["genre", "year", "type", "library_id", "rating_gte"];

const YEAR_OPERATORS: { value: Operator; label: string; a11yKey: string; a11yFallback: string }[] = [
  { value: "gte", label: ">=", a11yKey: "media.playlist.opGte", a11yFallback: "Greater than or equal to" },
  { value: "lte", label: "<=", a11yKey: "media.playlist.opLte", a11yFallback: "Less than or equal to" },
  { value: "eq", label: "=", a11yKey: "media.playlist.opEq", a11yFallback: "Equal to" },
];

const MEDIA_TYPES = [
  { value: "MEDIA_ITEM_TYPE_MOVIE", label: "Movie", i18nKey: "media.playlist.typeMovie" },
  { value: "MEDIA_ITEM_TYPE_SERIES", label: "Series", i18nKey: "media.playlist.typeSeries" },
  { value: "MEDIA_ITEM_TYPE_EPISODE", label: "Episode", i18nKey: "media.playlist.typeEpisode" },
];

function defaultRule(field: RuleField): SmartRule {
  if (field === "year") return { field: "year", operator: "gte", value: "" };
  if (field === "type") return { field: "type", operator: "eq", value: "MEDIA_ITEM_TYPE_MOVIE" };
  if (field === "library_id") return { field: "library_id", operator: "eq", value: "" };
  if (field === "rating_gte") return { field: "rating_gte", operator: "gte", value: "" };
  return { field: "genre", operator: "eq", value: "" };
}

interface Props {
  visible: boolean;
  onClose: () => void;
  rules: SmartRule[];
  onSave: (rules: SmartRule[]) => void;
  // Which rule fields the add-field picker offers. Defaults to the playlist set;
  // collections pass COLLECTION_FIELDS to also offer rating_gte.
  availableFields?: RuleField[];
}

export function SmartRulesEditor({ visible, onClose, rules, onSave, availableFields = PLAYLIST_FIELDS }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [local, setLocal] = useState<SmartRule[]>(rules);
  // Real genres from the user's libraries. A hardcoded English list produced
  // rules that could never match anything on a library whose metadata is in
  // another language. Only fetched while the editor is open.
  const { data: availableGenres = [] } = useAllGenres(visible);

  const addRule = useCallback(() => {
    setLocal((prev) => [...prev, defaultRule("genre")]);
  }, []);

  const removeRule = useCallback((idx: number) => {
    setLocal((prev) => prev.filter((_, i) => i !== idx));
  }, []);

  const updateRule = useCallback((idx: number, updated: SmartRule) => {
    setLocal((prev) => prev.map((r, i) => (i === idx ? updated : r)));
  }, []);

  const handleSave = useCallback(() => {
    onSave(local.filter((r) => r.value.trim()));
    onClose();
  }, [local, onSave, onClose]);

  // Reload the parent's rules each time the editor OPENS (another playlist,
  // rules saved elsewhere), never while it is open: a parent re-render hands
  // a fresh array and used to wipe what was being typed.
  const wasVisible = useRef(false);
  useEffect(() => {
    if (visible && !wasVisible.current) setLocal(rules);
    wasVisible.current = visible;
  }, [visible, rules]);

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <Pressable
        style={[styles.backdrop, { backgroundColor: theme.colors.overlay }]}
        onPress={onClose}
        accessibilityElementsHidden={true}
        importantForAccessibility="no-hide-descendants"
      />
      <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
        <Text style={[styles.title, { color: theme.colors.text }]}>
          {t("media.playlist.smartRules") || "Smart Rules"}
        </Text>

        <ScrollView style={{ maxHeight: 400 }}>
          {local.map((rule, idx) => (
            <View key={idx} style={[styles.ruleRow, { borderBottomColor: theme.colors.border }]}>
              {/* Field picker */}
              <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ marginBottom: 6 }}>
                {availableFields.map((f) => (
                  <Pressable
                    key={f}
                    onPress={() => updateRule(idx, defaultRule(f))}
                    style={[styles.chip, { backgroundColor: rule.field === f ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
                    accessible={true}
                    accessibilityLabel={t(FIELD_I18N_KEYS[f], FIELD_LABELS[f])}
                    accessibilityRole="radio"
                    accessibilityState={{ selected: rule.field === f }}
                  >
                    <Text style={{ color: rule.field === f ? theme.colors.onAccent : theme.colors.text, fontSize: FONT_SIZE.xs }}>{t(FIELD_I18N_KEYS[f], FIELD_LABELS[f])}</Text>
                  </Pressable>
                ))}
              </ScrollView>

              {/* Value input based on field */}
              <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
                {rule.field === "year" && (
                  <ScrollView horizontal showsHorizontalScrollIndicator={false}>
                    {YEAR_OPERATORS.map((op) => (
                      <Pressable
                        key={op.value}
                        onPress={() => updateRule(idx, { ...rule, operator: op.value })}
                        style={[styles.chip, { backgroundColor: rule.operator === op.value ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
                        accessible={true}
                        accessibilityLabel={t(op.a11yKey, op.a11yFallback)}
                        accessibilityRole="radio"
                        accessibilityState={{ selected: rule.operator === op.value }}
                      >
                        <Text style={{ color: rule.operator === op.value ? theme.colors.onAccent : theme.colors.text, fontSize: FONT_SIZE.xs }}>{op.label}</Text>
                      </Pressable>
                    ))}
                  </ScrollView>
                )}

                {rule.field === "year" ? (
                  <TextInput
                    value={rule.value}
                    onChangeText={(v) => updateRule(idx, { ...rule, value: v.replace(/\D/g, "") })}
                    placeholder="2020"
                    placeholderTextColor={theme.colors.textMuted}
                    keyboardType="number-pad"
                    maxLength={4}
                    accessibilityLabel={t("media.playlist.fields.year", "Year")}
                    style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.sm, flex: 1 }]}
                  />
                ) : rule.field === "rating_gte" ? (
                  <TextInput
                    value={rule.value}
                    // A single decimal on a 0-10 scale: keep digits and at most one dot.
                    onChangeText={(v) => updateRule(idx, { ...rule, value: v.replace(/[^0-9.]/g, "").replace(/(\..*)\./g, "$1") })}
                    placeholder="7.5"
                    placeholderTextColor={theme.colors.textMuted}
                    keyboardType="decimal-pad"
                    maxLength={4}
                    accessibilityLabel={t("media.playlist.fieldRating", "Rating ≥")}
                    style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.sm, flex: 1 }]}
                  />
                ) : rule.field === "type" ? (
                  <ScrollView horizontal showsHorizontalScrollIndicator={false}>
                    {MEDIA_TYPES.map((mt) => (
                      <Pressable
                        key={mt.value}
                        onPress={() => updateRule(idx, { ...rule, value: mt.value })}
                        style={[styles.chip, { backgroundColor: rule.value === mt.value ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
                        accessible={true}
                        accessibilityLabel={t(mt.i18nKey, mt.label)}
                        accessibilityRole="radio"
                        accessibilityState={{ selected: rule.value === mt.value }}
                      >
                        <Text style={{ color: rule.value === mt.value ? theme.colors.onAccent : theme.colors.text, fontSize: FONT_SIZE.xs }}>{t(mt.i18nKey, mt.label)}</Text>
                      </Pressable>
                    ))}
                  </ScrollView>
                ) : rule.field === "genre" ? (
                  <ScrollView horizontal showsHorizontalScrollIndicator={false}>
                    {availableGenres.map((g) => (
                      <Pressable
                        key={g}
                        onPress={() => updateRule(idx, { ...rule, value: g })}
                        style={[styles.chip, { backgroundColor: rule.value === g ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
                        accessible={true}
                        accessibilityLabel={g}
                        accessibilityRole="radio"
                        accessibilityState={{ selected: rule.value === g }}
                      >
                        <Text style={{ color: rule.value === g ? theme.colors.onAccent : theme.colors.text, fontSize: FONT_SIZE.xs }}>{g}</Text>
                      </Pressable>
                    ))}
                  </ScrollView>
                ) : (
                  <TextInput
                    value={rule.value}
                    onChangeText={(v) => updateRule(idx, { ...rule, value: v })}
                    placeholder={t("media.playlist.value", "Value")}
                    placeholderTextColor={theme.colors.textMuted}
                    accessibilityLabel={t("media.playlist.fields.ruleValue", "Rule value")}
                    style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.sm, flex: 1 }]}
                  />
                )}

                <Pressable
                  onPress={() => removeRule(idx)}
                  hitSlop={8}
                  accessibilityRole="button"
                  accessibilityLabel={t("common.remove", "Remove")}
                >
                  <Ionicons name="close-circle" size={ICON_SIZE.lg} color={theme.colors.error} />
                </Pressable>
              </View>
            </View>
          ))}

          <Pressable
            onPress={addRule}
            style={{ flexDirection: "row", alignItems: "center", gap: 6, paddingVertical: 12 }}
            accessible={true}
            accessibilityLabel={t("media.playlist.addRule", "Add Rule")}
            accessibilityRole="button"
          >
            <Ionicons name="add-circle-outline" size={ICON_SIZE.lg} color={theme.colors.accent} />
            <Text style={{ color: theme.colors.accent, fontSize: FONT_SIZE.sm, fontWeight: "500" }}>{t("media.playlist.addRule", "Add Rule")}</Text>
          </Pressable>
        </ScrollView>

        <View style={{ flexDirection: "row", gap: 8, marginTop: 16 }}>
          <Pressable
            onPress={handleSave}
            style={[styles.saveBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, flex: 1 }]}
            accessible={true}
            accessibilityLabel={t("common.save")}
            accessibilityRole="button"
          >
            <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: FONT_SIZE.sm }}>{t("common.save")}</Text>
          </Pressable>
          <Pressable
            onPress={onClose}
            style={[styles.cancelBtn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md, flex: 1 }]}
            accessible={true}
            accessibilityLabel={t("common.cancel")}
            accessibilityRole="button"
          >
            <Text style={{ color: theme.colors.text, fontWeight: "500", fontSize: FONT_SIZE.sm }}>{t("common.cancel")}</Text>
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { ...StyleSheet.absoluteFillObject },
  sheet: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32 },
  title: { fontSize: FONT_SIZE.lg, fontWeight: "700", marginBottom: 16 },
  ruleRow: { paddingVertical: 10, borderBottomWidth: StyleSheet.hairlineWidth },
  chip: { paddingHorizontal: 10, paddingVertical: 5, marginRight: 6 },
  input: { paddingHorizontal: 10, paddingVertical: 8, fontSize: FONT_SIZE.sm },
  saveBtn: { paddingVertical: 12, alignItems: "center" },
  cancelBtn: { paddingVertical: 12, alignItems: "center" },
});
