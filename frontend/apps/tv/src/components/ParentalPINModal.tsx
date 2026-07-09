import { useEffect, useState } from "react";
import { View, Text, Modal, ActivityIndicator, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { markUnlocked } from "../hooks/useParentalGate";
import { FocusableCard } from "./FocusableCard";

interface ParentalPINModalProps {
  visible: boolean;
  onClose: () => void;
  onVerified: () => void;
  profileId: string;
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

export function ParentalPINModal({ visible, onClose, onVerified, profileId }: ParentalPINModalProps) {
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

  async function handleSubmit(candidate: string) {
    if (candidate.length < PIN_LENGTH || loading) return;
    setLoading(true);
    setError(null);
    try {
      const result = await eveyFetch<{ valid: boolean }>({
        url: `/api/v1/profiles/${profileId}/parental/verify-pin`,
        method: "POST",
        data: { pin: candidate },
      });
      if (result.valid) {
        markUnlocked(profileId);
        onVerified();
      } else {
        setError(t("common.parental.pinIncorrect", "Incorrect PIN"));
        setPin("");
      }
    } catch {
      setError(t("common.parental.pinIncorrect", "Incorrect PIN"));
      setPin("");
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

          <View style={styles.dots}>
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
    fontSize: 22,
    fontWeight: "700",
    marginBottom: 8,
    textAlign: "center",
  },
  desc: {
    fontSize: 16,
    marginBottom: 20,
    textAlign: "center",
  },
  dots: {
    flexDirection: "row",
    gap: 16,
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
    fontSize: 14,
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
    fontSize: 22,
    fontWeight: "600",
  },
  cancelBtn: {
    marginTop: 20,
    paddingVertical: 12,
    paddingHorizontal: 32,
    alignItems: "center",
  },
  cancelText: {
    fontWeight: "600",
    fontSize: 16,
  },
});
