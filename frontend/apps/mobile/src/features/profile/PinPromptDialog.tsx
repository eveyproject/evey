import { useEffect, useRef, useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Modal, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { parentalServiceVerifyPin } from "@evey/api";
import type { V1Profile } from "@evey/api";
import { useTheme } from "../../providers/ThemeProvider";

interface Props {
  target: V1Profile;
  onCancel: () => void;
  onVerified: (target: V1Profile) => void | Promise<void>;
}

export function PinPromptDialog({ target, onCancel, onVerified }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [pin, setPin] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const inputRef = useRef<TextInput>(null);

  useEffect(() => {
    setPin("");
    setError(null);
    setTimeout(() => inputRef.current?.focus(), 100);
  }, []);

  async function handleSubmit() {
    if (!target.id) return;
    if (!/^\d{4}$/.test(pin)) {
      setError(t("auth.profile.select.pinInvalid", "PIN must be 4 digits"));
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const resp = await parentalServiceVerifyPin(target.id, { pin });
      if (!resp.valid) {
        setError(t("auth.profile.select.pinWrong", "Incorrect PIN"));
        setPin("");
        inputRef.current?.focus();
        return;
      }
      await onVerified(target);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("auth.profile.select.pinWrong", "Incorrect PIN"));
      setPin("");
      inputRef.current?.focus();
    } finally {
      setSubmitting(false);
    }
  }

  const s = styles(theme);

  return (
    <Modal visible transparent animationType="fade" onRequestClose={onCancel}>
      <View style={[s.backdrop, { backgroundColor: theme.colors.overlay }]}>
        <View style={[s.dialog, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={[s.title, { color: theme.colors.text }]}>
            {t("auth.profile.select.pinPromptTitle", "Enter PIN")}
          </Text>
          <Text style={[s.desc, { color: theme.colors.textMuted }]}>
            {t("auth.profile.select.pinPromptBody", "Enter the PIN for \"{{name}}\" to continue.", { name: target.name ?? "" })}
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
              s.pinInput,
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
            accessibilityLabel={t("auth.profile.fields.pin", "PIN (4 digits)")}
          />

          {error && <Text style={[s.error, { color: theme.colors.error }]}>{error}</Text>}

          <View style={s.buttons}>
            <Pressable
              onPress={onCancel}
              style={[s.btn, { backgroundColor: theme.colors.bg, borderRadius: theme.radii.md }]}
              accessibilityRole="button"
              accessibilityLabel={t("common.cancel", "Cancel")}
            >
              <Text style={[s.btnText, { color: theme.colors.text }]}>
                {t("common.cancel", "Cancel")}
              </Text>
            </Pressable>
            <Pressable
              onPress={() => void handleSubmit()}
              disabled={pin.length < 4 || submitting}
              style={[
                s.btn,
                {
                  backgroundColor: pin.length < 4 ? theme.colors.border : theme.colors.accent,
                  borderRadius: theme.radii.md,
                  opacity: submitting ? 0.6 : 1,
                },
              ]}
              accessibilityRole="button"
              accessibilityLabel={t("common.confirm", "Confirm")}
            >
              {submitting ? (
                <ActivityIndicator color={theme.colors.onAccent} size="small" />
              ) : (
                <Text style={[s.btnText, { color: theme.colors.onAccent }]}>
                  {t("common.confirm", "Confirm")}
                </Text>
              )}
            </Pressable>
          </View>
        </View>
      </View>
    </Modal>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    backdrop: {
      flex: 1,
      justifyContent: "center",
      alignItems: "center",
      padding: t.spacing.xl,
    },
    dialog: {
      width: "100%",
      maxWidth: 360,
      padding: t.spacing.lg,
    },
    title: {
      fontSize: 18,
      fontWeight: "700",
      marginBottom: t.spacing.sm,
      textAlign: "center",
    },
    desc: {
      fontSize: 14,
      marginBottom: t.spacing.lg,
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
      marginBottom: t.spacing.sm,
    },
    error: {
      fontSize: 13,
      textAlign: "center",
      marginBottom: t.spacing.sm,
    },
    buttons: {
      flexDirection: "row",
      gap: t.spacing.sm,
      marginTop: t.spacing.sm,
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
