import { useEffect, useState } from "react";
import { Modal, View, Text, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import type { CastDevice } from "../hooks/useCast";

interface Props {
  visible: boolean;
  onClose: () => void;
  onSelect: (device: CastDevice) => void;
  discover: () => Promise<CastDevice[]>;
}

export function CastPicker({ visible, onClose, onSelect, discover }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [devices, setDevices] = useState<CastDevice[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!visible) return;
    setLoading(true);
    discover().then((r) => { setDevices(r); setLoading(false); });
  }, [visible, discover]);

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("player.controls.cast")}
          </Text>
          {loading ? (
            <ActivityIndicator color={theme.colors.accent} style={{ marginVertical: 24 }} />
          ) : devices.length === 0 ? (
            <Text style={{ color: theme.colors.textMuted, textAlign: "center", marginVertical: 24, fontSize: 14 }}>
              {t("player.controls.noDevices", "No devices found")}
            </Text>
          ) : (
            devices.map((d) => (
              <Pressable
                key={d.id}
                style={[styles.row, { borderBottomColor: theme.colors.border }]}
                onPress={() => { onSelect(d); onClose(); }}
              >
                <Ionicons
                  name={d.type === "chromecast" ? "tv-outline" : "desktop-outline"}
                  size={20}
                  color={theme.colors.text}
                />
                <View style={{ flex: 1 }}>
                  <Text style={[styles.name, { color: theme.colors.text }]}>{d.name}</Text>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                    {d.type === "chromecast" ? "Chromecast" : "DLNA"}
                  </Text>
                </View>
              </Pressable>
            ))
          )}
          <Pressable onPress={onClose} style={[styles.closeBtn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}>
            <Text style={{ color: theme.colors.text, fontWeight: "500" }}>{t("common.close", "Close")}</Text>
          </Pressable>
        </View>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: "rgba(0,0,0,0.6)" },
  sheet: { width: "80%", maxWidth: 340, padding: 20 },
  title: { fontSize: 17, fontWeight: "700", marginBottom: 12 },
  row: { flexDirection: "row", alignItems: "center", gap: 12, paddingVertical: 14, borderBottomWidth: StyleSheet.hairlineWidth },
  name: { fontSize: 15, fontWeight: "500" },
  closeBtn: { marginTop: 16, paddingVertical: 10, alignItems: "center" },
});
