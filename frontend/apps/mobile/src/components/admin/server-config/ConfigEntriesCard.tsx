import { useState } from "react";
import { View, Text, TextInput, TouchableOpacity, Pressable, Alert, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../providers/ThemeProvider";
import type { ConfigEntry, SaveEntry } from "./types";

interface ConfigEntriesCardProps {
  entries: ConfigEntry[] | undefined;
  saveEntry: SaveEntry;
  deleteEntry: (key: string) => void;
  saving: boolean;
}

// Raw key/value CRUD over server_config, the mobile counterpart of the web's
// ConfigEntriesTable (ConfigRow + AddEntryRow). The web's operator-flag
// datalist/hints are web-only (no datalist idiom on mobile); the exact key
// can still be typed by hand, same as the web before the catalog existed.
export function ConfigEntriesCard({ entries, saveEntry, deleteEntry, saving }: ConfigEntriesCardProps) {
  const theme = useTheme();
  const { t } = useTranslation();

  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [editValue, setEditValue] = useState("");
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");

  function startEdit(entry: ConfigEntry) {
    setEditingKey(entry.key);
    setEditValue(entry.value ?? "");
  }

  function handleSaveEdit() {
    if (editingKey === null) return;
    saveEntry({ key: editingKey, value: editValue });
    setEditingKey(null);
  }

  function handleDelete(key: string) {
    Alert.alert(
      t("common.delete", "Delete"),
      t("admin.serverConfig.deleteEntryConfirm", "Delete the entry {{key}}?", { key }),
      [
        { text: t("common.cancel", "Cancel"), style: "cancel" },
        { text: t("common.delete", "Delete"), style: "destructive", onPress: () => deleteEntry(key) },
      ],
    );
  }

  function handleAdd() {
    if (!newKey.trim()) return;
    saveEntry(
      { key: newKey.trim(), value: newValue },
      {
        onSuccess: () => {
          setNewKey("");
          setNewValue("");
        },
      },
    );
  }

  const inputStyle = [
    styles.input,
    { backgroundColor: theme.colors.surfaceRaised, color: theme.colors.text, borderColor: theme.colors.border },
  ];

  return (
    <View style={{ gap: 10 }}>
      {(entries ?? []).map((entry) => (
        <View
          key={entry.key}
          style={{
            backgroundColor: theme.colors.surface,
            borderRadius: theme.radii.sm,
            padding: 10,
            gap: 6,
          }}
        >
          <Text style={{ color: theme.colors.text, fontSize: 13, fontFamily: "monospace" }}>{entry.key}</Text>
          {editingKey === entry.key ? (
            <View style={{ gap: 8 }}>
              <TextInput
                testID={`entry-edit-${entry.key}`}
                style={inputStyle}
                value={editValue}
                onChangeText={setEditValue}
                autoCapitalize="none"
                autoCorrect={false}
                autoFocus
              />
              <View style={{ flexDirection: "row", gap: 12 }}>
                <TouchableOpacity testID={`entry-save-${entry.key}`} onPress={handleSaveEdit} disabled={saving}>
                  <Text style={{ color: theme.colors.accent, fontSize: 13, fontWeight: "600" }}>{t("common.save", "Save")}</Text>
                </TouchableOpacity>
                <TouchableOpacity onPress={() => setEditingKey(null)}>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("common.cancel", "Cancel")}</Text>
                </TouchableOpacity>
              </View>
            </View>
          ) : (
            <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12, flex: 1, fontFamily: "monospace" }} numberOfLines={2}>
                {entry.value}
              </Text>
              <Pressable
                testID={`entry-editbtn-${entry.key}`}
                onPress={() => startEdit(entry)}
                hitSlop={8}
                accessible={true}
                accessibilityLabel={`${t("common.edit", "Edit")} ${entry.key}`}
                accessibilityRole="button"
              >
                <Ionicons name="pencil-outline" size={18} color={theme.colors.accent} />
              </Pressable>
              <Pressable
                testID={`entry-delete-${entry.key}`}
                onPress={() => handleDelete(entry.key)}
                hitSlop={8}
                accessible={true}
                accessibilityLabel={`${t("common.delete", "Delete")} ${entry.key}`}
                accessibilityRole="button"
              >
                <Ionicons name="trash-outline" size={18} color={theme.colors.error} />
              </Pressable>
            </View>
          )}
        </View>
      ))}

      <View style={{ gap: 8, paddingTop: 6 }}>
        <TextInput
          testID="entry-new-key"
          style={inputStyle}
          value={newKey}
          onChangeText={setNewKey}
          placeholder={t("admin.serverConfig.tableHeaders.key", "Key")}
          placeholderTextColor={theme.colors.textMuted}
          autoCapitalize="none"
          autoCorrect={false}
        />
        <TextInput
          testID="entry-new-value"
          style={inputStyle}
          value={newValue}
          onChangeText={setNewValue}
          placeholder={t("admin.serverConfig.tableHeaders.value", "Value")}
          placeholderTextColor={theme.colors.textMuted}
          autoCapitalize="none"
          autoCorrect={false}
        />
        <TouchableOpacity
          testID="entry-add"
          onPress={handleAdd}
          disabled={!newKey.trim() || saving}
          style={{
            backgroundColor: theme.colors.accent,
            borderRadius: theme.radii.sm,
            paddingVertical: 10,
            alignItems: "center",
            opacity: !newKey.trim() || saving ? 0.5 : 1,
          }}
        >
          <Text style={{ color: theme.colors.onAccent, fontSize: 14, fontWeight: "600" }}>{t("common.add", "Add")}</Text>
        </TouchableOpacity>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  input: { borderWidth: 1, borderRadius: 8, paddingHorizontal: 12, paddingVertical: 9, fontSize: 13 },
});
