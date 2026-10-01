import { useEffect, useState } from "react";
import { View, Text, Modal, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { formatOpening, useProfileLock } from "@evey/client";
import { useAuthStore } from "@evey/store";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../providers/ThemeProvider";
import { useMusicQueueStore } from "../stores/musicQueue";
import { FocusableCard } from "./FocusableCard";
import { ParentalPINModal } from "./ParentalPINModal";

// Full-screen lock while the active kids profile is outside its opening hours,
// driven by the remote: a parent's PIN on the keypad, or another profile.
export function ProfileLockScreen() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const profileId = useAuthStore((s) => s.profileId) ?? "";
  const lock = useProfileLock(() => void qc.invalidateQueries());
  const [pinOpen, setPinOpen] = useState(false);

  // Music stops with the lock; the video player pauses itself on the same signal.
  useEffect(() => {
    if (!lock.closed) {
      setPinOpen(false);
      return;
    }
    useMusicQueueStore.setState({ isPlaying: false });
  }, [lock.closed]);

  async function submitPin(pin: string): Promise<string | null> {
    const res = await lock.unlock(pin);
    if (res === null) return null;
    return res === "wrong_pin"
      ? t("auth.profile.locked.wrongPin", "Wrong parent PIN")
      : res === "no_parent_pin"
        ? t("auth.profile.locked.noParentPin", "No parent PIN is set on this account.")
        : res === "rate_limited"
          ? t("auth.profile.locked.rateLimited", "Too many attempts, try again in a minute.")
          : t("auth.profile.locked.failed", "Could not unlock the profile.");
  }

  return (
    <Modal visible={lock.closed} animationType="fade" transparent onRequestClose={() => {}}>
      <View style={[styles.backdrop, { backgroundColor: theme.colors.bg }]}>
        <View style={[styles.panel, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]} accessibilityRole="alert">
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("auth.profile.locked.title", "This profile is locked right now")}
          </Text>
          <Text style={[styles.body, { color: theme.colors.textMuted }]}>
            {lock.nextOpen
              ? t("auth.profile.locked.body", "It opens again {{when}}.", { when: formatOpening(lock.nextOpen, i18n.language) })
              : t("auth.profile.locked.bodyNoNext", "No opening is planned in the coming days.")}
          </Text>
          <View style={styles.actions}>
            <FocusableCard
              hasTVPreferredFocus
              onPress={() => setPinOpen(true)}
              accessibilityLabel={t("auth.profile.locked.pinLabel", "Parent PIN")}
              style={[styles.btn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md }]}
            >
              <Text style={[styles.btnText, { color: theme.colors.onAccent }]}>{t("auth.profile.locked.pinLabel", "Parent PIN")}</Text>
            </FocusableCard>
            <FocusableCard
              onPress={() => router.replace("/profile-select")}
              accessibilityLabel={t("auth.profile.locked.switchProfile", "Switch profile")}
              style={[styles.btn, { backgroundColor: theme.colors.bg, borderRadius: theme.radii.md }]}
            >
              <Text style={[styles.btnText, { color: theme.colors.text }]}>{t("auth.profile.locked.switchProfile", "Switch profile")}</Text>
            </FocusableCard>
          </View>
          <Text style={[styles.hint, { color: theme.colors.textMuted }]}>
            {t("auth.profile.locked.pinHint", "A parent's PIN opens the profile until its next opening.")}
          </Text>
        </View>
      </View>
      <ParentalPINModal
        visible={pinOpen}
        profileId={profileId}
        title={t("auth.profile.locked.title", "This profile is locked right now")}
        description={t("auth.profile.locked.pinHint", "A parent's PIN opens the profile until its next opening.")}
        submitPin={submitPin}
        onVerified={() => setPinOpen(false)}
        onClose={() => setPinOpen(false)}
      />
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, alignItems: "center", justifyContent: "center", padding: 48 },
  panel: { width: "100%", maxWidth: 640, padding: 40, alignItems: "center", gap: 16 },
  title: { fontSize: FONT_SIZE["2xl"], fontWeight: "700", textAlign: "center" },
  body: { fontSize: FONT_SIZE.lg, textAlign: "center" },
  hint: { fontSize: FONT_SIZE.sm, textAlign: "center" },
  actions: { flexDirection: "row", gap: 16, marginTop: 8 },
  btn: { paddingHorizontal: 28, paddingVertical: 14, alignItems: "center", justifyContent: "center" },
  btnText: { fontSize: FONT_SIZE.lg, fontWeight: "700" },
});
