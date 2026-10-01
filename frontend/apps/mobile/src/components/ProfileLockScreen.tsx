import { useEffect, useState } from "react";
import { View, Text, TextInput, Pressable, Modal, ActivityIndicator, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { formatOpening, useProfileLock } from "@evey/client";
import { useMusicPlayerStore } from "@evey/store";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../providers/ThemeProvider";

// Full-screen lock while the active kids profile is outside its opening
// hours. The server refuses everything already; this says why and when it
// opens, and offers a parent's PIN or another profile.
export function ProfileLockScreen() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const lock = useProfileLock(() => void qc.invalidateQueries());
  const [pin, setPin] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Music stops with the lock; the video player pauses itself on the same signal.
  useEffect(() => {
    if (!lock.closed) return;
    useMusicPlayerStore.getState().pauseExternal();
    setPin("");
    setError(null);
  }, [lock.closed]);

  async function submit() {
    if (pin.length !== 4) return;
    setBusy(true);
    const res = await lock.unlock(pin);
    setBusy(false);
    setPin("");
    if (res === null) return;
    setError(
      res === "wrong_pin"
        ? t("auth.profile.locked.wrongPin", "Wrong parent PIN")
        : res === "no_parent_pin"
          ? t("auth.profile.locked.noParentPin", "No parent PIN is set on this account.")
          : res === "rate_limited"
            ? t("auth.profile.locked.rateLimited", "Too many attempts, try again in a minute.")
            : t("auth.profile.locked.failed", "Could not unlock the profile."),
    );
  }

  return (
    <Modal visible={lock.closed} animationType="fade" transparent onRequestClose={() => {}}>
      <View style={[styles.backdrop, { backgroundColor: theme.colors.overlay }]}>
        <View accessibilityRole="alert" style={[styles.panel, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Ionicons name="lock-closed" size={32} color={theme.colors.accent} />
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("auth.profile.locked.title", "This profile is locked right now")}
          </Text>
          <Text style={[styles.body, { color: theme.colors.textMuted }]}>
            {lock.nextOpen
              ? t("auth.profile.locked.body", "It opens again {{when}}.", { when: formatOpening(lock.nextOpen, i18n.language) })
              : t("auth.profile.locked.bodyNoNext", "No opening is planned in the coming days.")}
          </Text>
          <TextInput
            value={pin}
            onChangeText={(v) => setPin(v.replace(/\D/g, "").slice(0, 4))}
            onSubmitEditing={() => void submit()}
            keyboardType="number-pad"
            secureTextEntry
            maxLength={4}
            placeholder="••••"
            placeholderTextColor={theme.colors.textMuted}
            accessibilityLabel={t("auth.profile.locked.pinLabel", "Parent PIN")}
            style={[styles.pin, { color: theme.colors.text, borderColor: error ? theme.colors.error : theme.colors.border, borderRadius: theme.radii.md }]}
          />
          {error ? (
            <Text accessibilityRole="alert" style={{ color: theme.colors.error, fontSize: FONT_SIZE.sm }}>{error}</Text>
          ) : (
            <Text style={[styles.hint, { color: theme.colors.textMuted }]}>
              {t("auth.profile.locked.pinHint", "A parent's PIN opens the profile until its next opening.")}
            </Text>
          )}
          <Pressable
            onPress={() => void submit()}
            disabled={pin.length !== 4 || busy}
            accessibilityRole="button"
            style={[styles.btn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: pin.length !== 4 ? 0.5 : 1 }]}
          >
            {busy ? (
              <ActivityIndicator color={theme.colors.onAccent} />
            ) : (
              <Text style={{ color: theme.colors.onAccent, fontWeight: "700" }}>{t("auth.profile.locked.unlock", "Unlock")}</Text>
            )}
          </Pressable>
          <Pressable onPress={() => router.replace("/profile-select")} accessibilityRole="button" style={styles.link}>
            <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("auth.profile.locked.switchProfile", "Switch profile")}</Text>
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, alignItems: "center", justifyContent: "center", padding: 24 },
  panel: { width: "100%", maxWidth: 380, padding: 24, alignItems: "center", gap: 12 },
  title: { fontSize: FONT_SIZE.lg, fontWeight: "700", textAlign: "center" },
  body: { fontSize: FONT_SIZE.sm, textAlign: "center" },
  hint: { fontSize: FONT_SIZE.xs, textAlign: "center" },
  pin: { width: 160, borderWidth: 1, paddingVertical: 10, textAlign: "center", fontSize: FONT_SIZE.xl, letterSpacing: 8 },
  btn: { alignSelf: "stretch", paddingVertical: 12, alignItems: "center" },
  link: { paddingVertical: 8 },
});
