import { useRef, useState } from "react";
import { AccessibilityInfo, Modal, Pressable, ScrollView, StyleSheet, Text, View, findNodeHandle } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../../../providers/ThemeProvider";

interface InfoButtonProps {
  /** The setting (or group) explained: names the button and titles the sheet. */
  label: string;
  /** The plain-language explanation. */
  info: string;
  testID?: string;
}

// InfoButton is the "i" next to a setting: a tap opens a sheet with the
// explanation in plain words, closed by its button, the backdrop or the
// system back gesture. On close the screen reader focus goes back to the "i"
// it came from, so a TalkBack / VoiceOver user does not land at the top of
// the screen after each explanation.
export function InfoButton({ label, info, testID }: InfoButtonProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const anchor = useRef<View>(null);
  const name = t("admin.settingsLayout.moreInfo", { label });

  function close() {
    setOpen(false);
    // The node exists once the sheet is gone; a test double has neither API.
    const node = anchor.current ? findNodeHandle?.(anchor.current) : null;
    if (node) AccessibilityInfo?.setAccessibilityFocus?.(node);
  }

  return (
    <>
      <Pressable
        ref={anchor}
        onPress={() => setOpen(true)}
        hitSlop={12}
        accessible={true}
        accessibilityRole="button"
        accessibilityLabel={name}
        testID={testID ?? "setting-info-button"}
        style={styles.button}
      >
        <Ionicons name="information-circle-outline" size={18} color={theme.colors.textMuted} />
      </Pressable>
      <Modal visible={open} transparent animationType="fade" onRequestClose={close}>
        <View style={[styles.backdrop, { backgroundColor: theme.colors.overlay }]}>
          <Pressable
            style={StyleSheet.absoluteFillObject}
            onPress={close}
            accessible={true}
            accessibilityRole="button"
            accessibilityLabel={t("common.close")}
          />
          <View
            style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}
            accessibilityViewIsModal={true}
            testID="setting-info-sheet"
          >
            <Text style={[styles.title, { color: theme.colors.text }]} accessibilityRole="header">
              {label}
            </Text>
            <ScrollView style={styles.body}>
              <Text style={[styles.text, { color: theme.colors.textMuted }]}>{info}</Text>
            </ScrollView>
            <Pressable
              onPress={close}
              accessibilityRole="button"
              accessibilityLabel={t("common.close")}
              testID="setting-info-close"
              style={[styles.close, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md }]}
            >
              <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: FONT_SIZE.sm }}>{t("common.close")}</Text>
            </Pressable>
          </View>
        </View>
      </Modal>
    </>
  );
}

const styles = StyleSheet.create({
  // 18 px glyph, 12 px hit slop each side: a 42 px target that does not
  // push the label around.
  button: { paddingHorizontal: 2, justifyContent: "center", alignItems: "center" },
  backdrop: { flex: 1, justifyContent: "flex-end" },
  sheet: { padding: 20, paddingBottom: 32, gap: 12, maxHeight: "80%" },
  title: { fontSize: FONT_SIZE.lg, fontWeight: "700" },
  body: { flexGrow: 0 },
  text: { fontSize: FONT_SIZE.sm, lineHeight: 22 },
  close: { paddingVertical: 12, alignItems: "center", marginTop: 4 },
});
