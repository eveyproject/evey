import { useEffect, useState } from "react";
import { View, Text, Modal, ActivityIndicator, StyleSheet, AccessibilityInfo } from "react-native";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { markUnlocked } from "../hooks/useParentalGate";
import { FocusableCard } from "./FocusableCard";
import { FONT_SIZE } from "@evey/theme";

interface ParentalPINModalProps {
  visible: boolean;
  onClose: () => void;
  // pinToken is the short-lived server proof minted by verify-pin; callers
  // that only need to unlock content viewing on the CURRENT profile can
  // ignore it, but a profile switch must forward it to RefreshToken to
  // authorise leaving a restricted profile server-side.
  onVerified: (pinToken?: string) => void;
  profileId: string;
  // Second profile whose PIN also authorises the action (e.g. the currently
  // active kids profile when the switch TARGET carries no PIN of its own).
  // The client cannot know which profile actually holds the PIN, so the
  // same digits are tried against both - mirrors web/mobile PinPromptDialog.
  fallbackProfileId?: string;
}

interface VerifyPinResponse {
  valid: boolean;
  pinToken?: string;
}

// Exported for unit testing: a failed check against the primary profile is
// only worth retrying against a second profile when one is actually
// configured and distinct from the one that just failed.
export function shouldTryFallback(primaryProfileId: string, fallbackProfileId: string | undefined): boolean {
  return !!fallbackProfileId && fallbackProfileId !== primaryProfileId;
}

const PIN_LENGTH = 4;
// A 10-foot numeric keypad: D-pad navigable digits plus clear/verify actions,
// laid out like a phone dialpad since a TextInput can't be typed with a
// remote control.
const KEYPAD_ROWS: (string | null)[][] = [
  ["1", "2", "3"],
  ["4", "5", "6"],
  ["7", "8", "9"],
  [null, "0", "del"],
];

export function ParentalPINModal({ visible, onClose, onVerified, profileId, fallbackProfileId }: ParentalPINModalProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const [pin, setPin] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (visible) {
      setPin("");
      setError(null);
    }
  }, [visible]);

  // Progress only, never the digit itself - the PIN is a secret. Deliberately
  // keyed on pin.length alone: t's identity is not guaranteed stable across
  // renders, and this must announce once per digit, not once per render.
  useEffect(() => {
    if (pin.length === 0) return;
    AccessibilityInfo.announceForAccessibility(
      t("media.parental.pinDigitsEntered", { count: pin.length, total: PIN_LENGTH }),
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pin.length]);

  useEffect(() => {
    if (error) AccessibilityInfo.announceForAccessibility(error);
  }, [error]);

  async function verifyAgainst(targetProfileId: string, candidate: string): Promise<VerifyPinResponse | null> {
    try {
      return await eveyFetch<VerifyPinResponse>({
        url: `/api/v1/profiles/${targetProfileId}/parental/verify-pin`,
        method: "POST",
        data: { pin: candidate },
      });
    } catch {
      return null;
    }
  }

  async function handleSubmit(candidate: string) {
    if (candidate.length < PIN_LENGTH || loading) return;
    setLoading(true);
    setError(null);
    try {
      let result = await verifyAgainst(profileId, candidate);
      if (!result?.valid && shouldTryFallback(profileId, fallbackProfileId)) {
        result = await verifyAgainst(fallbackProfileId as string, candidate);
      }
      if (result?.valid) {
        markUnlocked(profileId);
        onVerified(result.pinToken);
      } else {
        setError(t("common.parental.pinIncorrect", "Incorrect PIN"));
        setPin("");
      }
    } finally {
      setLoading(false);
    }
  }

  function handleKeyPress(key: string) {
    if (loading) return;
    if (key === "del") {
      setPin((prev) => prev.slice(0, -1));
      if (error) setError(null);
      return;
    }
    setPin((prev) => {
      if (prev.length >= PIN_LENGTH) return prev;
      const next = prev + key;
      if (next.length === PIN_LENGTH) void handleSubmit(next);
      return next;
    });
    if (error) setError(null);
  }

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onClose}>
      <View style={[styles.backdrop, { backgroundColor: theme.colors.overlay }]}>
        <View style={[styles.dialog, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("common.parental.restrictedContent", "Restricted Content")}
          </Text>
          <Text style={[styles.desc, { color: theme.colors.textMuted }]}>
            {t("common.parental.pinRequired", "Enter your parental PIN to continue")}
          </Text>

          <View
            style={styles.dots}
            accessible
            accessibilityLabel={t("media.parental.pinDigitsEntered", { count: pin.length, total: PIN_LENGTH })}
          >
            {Array.from({ length: PIN_LENGTH }).map((_, i) => (
              <View
                key={i}
                style={[
                  styles.dot,
                  {
                    borderColor: error ? theme.colors.error : theme.colors.border,
                    backgroundColor: i < pin.length ? theme.colors.accent : "transparent",
                  },
                ]}
              />
            ))}
          </View>

          {loading ? (
            <ActivityIndicator color={theme.colors.accent} size="small" style={styles.loading} />
          ) : error ? (
            <Text style={[styles.error, { color: theme.colors.error }]}>{error}</Text>
          ) : (
            <View style={styles.error} />
          )}

          <View style={styles.keypad}>
            {KEYPAD_ROWS.map((row, ri) => (
              <View key={ri} style={styles.keypadRow}>
                {row.map((key, ki) =>
                  key === null ? (
                    <View key={ki} style={styles.key} />
                  ) : (
                    <FocusableCard
                      key={ki}
                      hasTVPreferredFocus={ri === 0 && ki === 0}
                      onPress={() => handleKeyPress(key)}
                      accessibilityLabel={key === "del" ? t("common.parental.delete", "Delete") : key}
                      style={[styles.key, { backgroundColor: theme.colors.bg }]}
                    >
                      <Text style={[styles.keyText, { color: theme.colors.text }]}>
                        {key === "del" ? "⌫" : key}
                      </Text>
                    </FocusableCard>
                  ),
                )}
              </View>
            ))}
          </View>

          <FocusableCard
            onPress={onClose}
            accessibilityLabel={t("common.cancel", "Cancel")}
            style={[styles.cancelBtn, { backgroundColor: theme.colors.bg, borderRadius: theme.radii.md }]}
          >
            <Text style={[styles.cancelText, { color: theme.colors.text }]}>{t("common.cancel", "Cancel")}</Text>
          </FocusableCard>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    padding: 32,
  },
  dialog: {
    width: "100%",
    maxWidth: 420,
    padding: 32,
    alignItems: "center",
  },
  title: {
    fontSize: FONT_SIZE.xl,
    fontWeight: "700",
    marginBottom: 8,
    textAlign: "center",
  },
  desc: {
    fontSize: FONT_SIZE.base,
    marginBottom: 20,
    textAlign: "center",
  },
  dots: {
    flexDirection: "row",
    gap: 12,
    marginBottom: 8,
  },
  dot: {
    width: 20,
    height: 20,
    borderRadius: 10,
    borderWidth: 2,
  },
  loading: {
    height: 20,
    marginBottom: 8,
  },
  error: {
    fontSize: FONT_SIZE.sm,
    textAlign: "center",
    marginBottom: 8,
    height: 20,
  },
  keypad: {
    gap: 10,
    marginTop: 12,
  },
  keypadRow: {
    flexDirection: "row",
    gap: 10,
  },
  key: {
    width: 72,
    height: 56,
    borderRadius: 8,
    alignItems: "center",
    justifyContent: "center",
  },
  keyText: {
    fontSize: FONT_SIZE.xl,
    fontWeight: "600",
  },
  cancelBtn: {
    marginTop: 20,
    paddingVertical: 9,
    paddingHorizontal: 32,
    alignItems: "center",
  },
  cancelText: {
    fontWeight: "600",
    fontSize: FONT_SIZE.base,
  },
});
