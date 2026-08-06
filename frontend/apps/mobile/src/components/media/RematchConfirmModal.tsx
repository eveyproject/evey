import { View, Text, Pressable, ActivityIndicator, Modal, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { FONT_SIZE } from "@evey/theme";

// Mirrors apps/web/src/features/library/MediaDetailEdit.tsx's rematch <Modal>:
// a destructive action (drops overrides + provider IDs, re-searches from the
// original filename) gets an explicit confirm step rather than firing on tap.
export function RematchConfirmModal({
  visible,
  onClose,
  onConfirm,
  pending,
}: {
  visible: boolean;
  onClose: () => void;
  onConfirm: () => void;
  pending: boolean;
}) {
  const { t } = useTranslation();
  const theme = useTheme();
  const s = styles;

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={() => !pending && onClose()}>
      <View style={[s.backdrop, { backgroundColor: theme.colors.overlay }]}>
        <Pressable
          style={StyleSheet.absoluteFillObject}
          onPress={() => !pending && onClose()}
          accessible={true}
          accessibilityLabel={t("common.close", "Close")}
          accessibilityRole="button"
        />
        <View style={[s.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={[s.title, { color: theme.colors.text }]}>{t("media.detail.rematchMetadata")}</Text>
          <Text style={[s.body, { color: theme.colors.textMuted }]}>{t("media.detail.rematchConfirm")}</Text>
          <View style={s.buttons}>
            <Pressable onPress={onClose} disabled={pending} style={s.btn} accessibilityRole="button" accessibilityLabel={t("common.cancel")}>
              <Text style={[s.btnText, { color: theme.colors.accent }]}>{t("common.cancel")}</Text>
            </Pressable>
            <Pressable
              onPress={onConfirm}
              disabled={pending}
              style={[s.btn, { backgroundColor: theme.colors.danger, opacity: pending ? 0.6 : 1 }]}
              accessibilityRole="button"
              accessibilityLabel={t("media.detail.rematchMetadata")}
            >
              {pending ? (
                <ActivityIndicator color={theme.colors.onDanger} size="small" />
              ) : (
                <Text style={[s.btnText, { color: theme.colors.onDanger }]}>{t("media.detail.rematchMetadata")}</Text>
              )}
            </Pressable>
          </View>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: "flex-end" },
  sheet: { padding: 20, paddingBottom: 32, gap: 12 },
  title: { fontSize: FONT_SIZE.lg, fontWeight: "700" },
  body: { fontSize: FONT_SIZE.sm },
  buttons: { flexDirection: "row", justifyContent: "flex-end", gap: 8, marginTop: 8 },
  btn: { paddingHorizontal: 16, paddingVertical: 10, borderRadius: 8, alignItems: "center", justifyContent: "center" },
  btnText: { fontSize: FONT_SIZE.sm, fontWeight: "600" },
});
