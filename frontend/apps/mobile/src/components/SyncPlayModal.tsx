import { useState } from "react";
import { View, Text, Pressable, TextInput, Modal, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";

interface Props {
  visible: boolean;
  onClose: () => void;
  onCreateRoom: () => void;
  onJoinRoom: (code: string) => void;
  creating?: boolean;
  joining?: boolean;
}

export function SyncPlayModal({ visible, onClose, onCreateRoom, onJoinRoom, creating, joining }: Props) {
  const { t } = useTranslation();
  const theme = useTheme();
  const [code, setCode] = useState("");

  function handleJoin() {
    const trimmed = code.trim();
    if (trimmed.length >= 5) onJoinRoom(trimmed);
  }

  return (
    <Modal transparent visible={visible} animationType="fade" onRequestClose={onClose}>
      <Pressable style={[styles.backdrop, { backgroundColor: theme.colors.overlay }]} onPress={onClose} />
      <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
        <Text style={[styles.title, { color: theme.colors.text }]}>
          {t("media.syncplay.title", "Watch Together")}
        </Text>

        <Pressable
          onPress={onCreateRoom}
          disabled={creating}
          style={[styles.btn, { backgroundColor: theme.colors.accent, opacity: creating ? 0.5 : 1 }]}
        >
          <Ionicons name="add-circle-outline" size={18} color={theme.colors.onAccent} />
          <Text style={[styles.btnText, { color: theme.colors.onAccent }]}>{t("media.syncplay.createRoom", "Create room")}</Text>
        </Pressable>

        <View style={[styles.divider, { backgroundColor: theme.colors.border }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11, paddingHorizontal: 8 }}>
            {t("common.or", "or")}
          </Text>
        </View>

        <Text style={[styles.label, { color: theme.colors.textMuted }]}>
          {t("media.syncplay.joinByCode", "Join with code")}
        </Text>
        <View style={{ flexDirection: "row", gap: 8 }}>
          <TextInput
            value={code}
            onChangeText={setCode}
            placeholder="12345"
            placeholderTextColor={theme.colors.textMuted}
            keyboardType="number-pad"
            maxLength={5}
            style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderColor: theme.colors.border, flex: 1 }]}
          />
          <Pressable
            onPress={handleJoin}
            disabled={joining || code.trim().length < 5}
            style={[styles.btn, { backgroundColor: theme.colors.accent, opacity: joining || code.trim().length < 5 ? 0.5 : 1, flex: 0, paddingHorizontal: 16 }]}
          >
            <Text style={[styles.btnText, { color: theme.colors.onAccent }]}>{t("media.syncplay.join", "Join")}</Text>
          </Pressable>
        </View>

        <Pressable onPress={onClose} style={{ alignSelf: "center", marginTop: 12 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("common.cancel")}</Text>
        </Pressable>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { ...StyleSheet.absoluteFillObject },
  sheet: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32 },
  title: { fontSize: 18, fontWeight: "700", marginBottom: 16 },
  btn: { flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 8, paddingVertical: 12, borderRadius: 8 },
  btnText: { fontWeight: "600", fontSize: 14 },
  divider: { height: 1, marginVertical: 16, position: "relative", alignItems: "center", justifyContent: "center" },
  label: { fontSize: 12, fontWeight: "600", textTransform: "uppercase", marginBottom: 8 },
  input: { paddingHorizontal: 14, paddingVertical: 10, borderRadius: 8, borderWidth: 1, fontSize: 18, fontWeight: "700", letterSpacing: 4, textAlign: "center" },
});
