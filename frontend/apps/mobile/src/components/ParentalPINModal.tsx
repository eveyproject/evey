import { useState, useEffect, useRef } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Modal, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { markUnlocked } from "../hooks/useParentalGate";

interface ParentalPINModalProps {
  visible: boolean;
  onClose: () => void;
  onVerified: () => void;
  profileId: string;
}

export function ParentalPINModal({ visible, onClose, onVerified, profileId }: ParentalPINModalProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const [pin, setPin] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const inputRef = useRef<TextInput>(null);

  useEffect(() => {
    if (visible) {
      setPin("");
      setError(null);
      setTimeout(() => inputRef.current?.focus(), 100);
    }
  }, [visible]);

  async function handleSubmit() {
    if (pin.length < 4) return;
    setLoading(true);
    setError(null);
    try {
      const result = await eveyFetch<{ valid: boolean }>({
        url: `/api/v1/profiles/${profileId}/parental/verify-pin`,
        method: "POST",
        data: { pin },
      });
      if (result.valid) {
        markUnlocked(profileId);
        onVerified();
      } else {
        setError(t("common:parental.pinIncorrect", "Incorrect PIN"));
        setPin("");
        inputRef.current?.focus();
      }
    } catch {
      setError(t("common:parental.pinIncorrect", "Incorrect PIN"));
      setPin("");
      inputRef.current?.focus();
    } finally {
      setLoading(false);
    }
  }

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onClose}>
      <View style={styles.backdrop}>
        <View style={[styles.dialog, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("common:parental.restrictedContent", "Restricted Content")}
          </Text>
          <Text style={[styles.desc, { color: theme.colors.textMuted }]}>
            {t("common:parental.pinRequired", "Enter your parental PIN to continue")}
          </Text>

          <TextInput
            ref={inputRef}
            secureTextEntry
            keyboardType="number-pad"
            maxLength={4}
            value={pin}
            onChangeText={(v) => {
              const clean = v.replace(/\D/g, "").slice(0, 4);
              setPin(clean);
              if (error) setError(null);
            }}
            style={[
              styles.pinInput,
              {
                backgroundColor: theme.colors.bg,
                color: theme.colors.text,
                borderColor: error ? theme.colors.error : theme.colors.border,
                borderRadius: theme.radii.md,
              },
            ]}
            placeholder="----"
            placeholderTextColor={theme.colors.textMuted}
            autoComplete="off"
          />

          {error && <Text style={[styles.error, { color: theme.colors.error }]}>{error}</Text>}

          <View style={styles.buttons}>
            <Pressable
              onPress={() => void handleSubmit()}
              disabled={pin.length < 4 || loading}
              style={[
                styles.btn,
                {
                  backgroundColor: pin.length < 4 ? theme.colors.border : theme.colors.accent,
                  borderRadius: theme.radii.md,
                },
              ]}
            >
              {loading ? (
                <ActivityIndicator color={theme.colors.text} size="small" />
              ) : (
                <Text style={[styles.btnText, { color: theme.colors.text }]}>
                  {t("common:parental.validate", "Verify")}
                </Text>
              )}
            </Pressable>
            <Pressable
              onPress={onClose}
              style={[styles.btn, { backgroundColor: theme.colors.bg, borderRadius: theme.radii.md }]}
            >
              <Text style={[styles.btnText, { color: theme.colors.text }]}>
                {t("common.cancel", "Cancel")}
              </Text>
            </Pressable>
          </View>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.6)",
    justifyContent: "center",
    alignItems: "center",
    padding: 32,
  },
  dialog: {
    width: "100%",
    maxWidth: 340,
    padding: 24,
  },
  title: {
    fontSize: 18,
    fontWeight: "700",
    marginBottom: 8,
    textAlign: "center",
  },
  desc: {
    fontSize: 14,
    marginBottom: 20,
    textAlign: "center",
  },
  pinInput: {
    textAlign: "center",
    fontSize: 28,
    fontFamily: "monospace",
    letterSpacing: 12,
    paddingVertical: 12,
    paddingHorizontal: 16,
    borderWidth: 1,
    marginBottom: 8,
  },
  error: {
    fontSize: 13,
    textAlign: "center",
    marginBottom: 8,
  },
  buttons: {
    flexDirection: "row",
    gap: 12,
    marginTop: 12,
  },
  btn: {
    flex: 1,
    paddingVertical: 12,
    alignItems: "center",
    justifyContent: "center",
  },
  btnText: {
    fontWeight: "600",
    fontSize: 15,
  },
});
