import { useState, useCallback } from "react";
import { View, Text, TextInput, Pressable, ScrollView, Modal, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";

type RuleField = "genre" | "year" | "type" | "library_id";
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
};

const ALL_FIELDS: RuleField[] = ["genre", "year", "type", "library_id"];

const YEAR_OPERATORS: { value: Operator; label: string }[] = [
  { value: "gte", label: ">=" },
  { value: "lte", label: "<=" },
  { value: "eq", label: "=" },
];

const MEDIA_TYPES = [
  { value: "MEDIA_ITEM_TYPE_MOVIE", label: "Movie" },
  { value: "MEDIA_ITEM_TYPE_SERIES", label: "Series" },
  { value: "MEDIA_ITEM_TYPE_EPISODE", label: "Episode" },
];

const COMMON_GENRES = [
  "Action", "Adventure", "Animation", "Comedy", "Crime", "Documentary",
  "Drama", "Fantasy", "Horror", "Mystery", "Romance", "Sci-Fi", "Thriller",
];

function defaultRule(field: RuleField): SmartRule {
  if (field === "year") return { field: "year", operator: "gte", value: "" };
  if (field === "type") return { field: "type", operator: "eq", value: "MEDIA_ITEM_TYPE_MOVIE" };
  if (field === "library_id") return { field: "library_id", operator: "eq", value: "" };
  return { field: "genre", operator: "eq", value: "" };
}

interface Props {
  visible: boolean;
  onClose: () => void;
  rules: SmartRule[];
  onSave: (rules: SmartRule[]) => void;
}

export function SmartRulesEditor({ visible, onClose, rules, onSave }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [local, setLocal] = useState<SmartRule[]>(rules);

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

  // Reset local when opening
  useState(() => { setLocal(rules); });

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose} />
      <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
        <Text style={[styles.title, { color: theme.colors.text }]}>
          {t("media.playlist.smartRules") || "Smart Rules"}
        </Text>

        <ScrollView style={{ maxHeight: 400 }}>
          {local.map((rule, idx) => (
            <View key={idx} style={[styles.ruleRow, { borderBottomColor: theme.colors.border }]}>
              {/* Field picker */}
              <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ marginBottom: 6 }}>
                {ALL_FIELDS.map((f) => (
                  <Pressable
                    key={f}
                    onPress={() => updateRule(idx, defaultRule(f))}
                    style={[styles.chip, { backgroundColor: rule.field === f ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
                  >
                    <Text style={{ color: rule.field === f ? "#fff" : theme.colors.text, fontSize: 12 }}>{FIELD_LABELS[f]}</Text>
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
                      >
                        <Text style={{ color: rule.operator === op.value ? "#fff" : theme.colors.text, fontSize: 12 }}>{op.label}</Text>
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
                    style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.sm, flex: 1 }]}
                  />
                ) : rule.field === "type" ? (
                  <ScrollView horizontal showsHorizontalScrollIndicator={false}>
                    {MEDIA_TYPES.map((mt) => (
                      <Pressable
                        key={mt.value}
                        onPress={() => updateRule(idx, { ...rule, value: mt.value })}
                        style={[styles.chip, { backgroundColor: rule.value === mt.value ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
                      >
                        <Text style={{ color: rule.value === mt.value ? "#fff" : theme.colors.text, fontSize: 12 }}>{mt.label}</Text>
                      </Pressable>
                    ))}
                  </ScrollView>
                ) : rule.field === "genre" ? (
                  <ScrollView horizontal showsHorizontalScrollIndicator={false}>
                    {COMMON_GENRES.map((g) => (
                      <Pressable
                        key={g}
                        onPress={() => updateRule(idx, { ...rule, value: g })}
                        style={[styles.chip, { backgroundColor: rule.value === g ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
                      >
                        <Text style={{ color: rule.value === g ? "#fff" : theme.colors.text, fontSize: 12 }}>{g}</Text>
                      </Pressable>
                    ))}
                  </ScrollView>
                ) : (
                  <TextInput
                    value={rule.value}
                    onChangeText={(v) => updateRule(idx, { ...rule, value: v })}
                    placeholder="Value"
                    placeholderTextColor={theme.colors.textMuted}
                    style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.sm, flex: 1 }]}
                  />
                )}

                <Pressable onPress={() => removeRule(idx)} hitSlop={8}>
                  <Ionicons name="close-circle" size={20} color="#ef4444" />
                </Pressable>
              </View>
            </View>
          ))}

          <Pressable onPress={addRule} style={{ flexDirection: "row", alignItems: "center", gap: 6, paddingVertical: 12 }}>
            <Ionicons name="add-circle-outline" size={20} color={theme.colors.accent} />
            <Text style={{ color: theme.colors.accent, fontSize: 14, fontWeight: "500" }}>Add Rule</Text>
          </Pressable>
        </ScrollView>

        <View style={{ flexDirection: "row", gap: 8, marginTop: 16 }}>
          <Pressable onPress={handleSave} style={[styles.saveBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, flex: 1 }]}>
            <Text style={{ color: "#fff", fontWeight: "600", fontSize: 15 }}>Save</Text>
          </Pressable>
          <Pressable onPress={onClose} style={[styles.cancelBtn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md, flex: 1 }]}>
            <Text style={{ color: theme.colors.text, fontWeight: "500", fontSize: 15 }}>Cancel</Text>
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { ...StyleSheet.absoluteFillObject, backgroundColor: "rgba(0,0,0,0.5)" },
  sheet: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32 },
  title: { fontSize: 18, fontWeight: "700", marginBottom: 16 },
  ruleRow: { paddingVertical: 10, borderBottomWidth: StyleSheet.hairlineWidth },
  chip: { paddingHorizontal: 10, paddingVertical: 5, marginRight: 6 },
  input: { paddingHorizontal: 10, paddingVertical: 8, fontSize: 14 },
  saveBtn: { paddingVertical: 12, alignItems: "center" },
  cancelBtn: { paddingVertical: 12, alignItems: "center" },
});
