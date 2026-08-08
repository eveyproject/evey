import { useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, ScrollView } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceRequestPasswordReset } from "@evey/api";
import { useTheme } from "../src/providers/ThemeProvider";

export default function ForgotPassword() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);

  const requestReset = useUserServiceRequestPasswordReset({
    mutation: {
      onSuccess: () => setSent(true),
      onError: () => setSent(true),
    },
  });

  function handleSubmit() {
    if (!email.trim()) return;
    requestReset.mutate({ data: { email } });
  }

  if (sent) {
    return (
      <View style={{ flex: 1, justifyContent: "center", padding: theme.spacing.xl, backgroundColor: theme.colors.bg }}>
        <Text style={{ fontSize: 20, fontWeight: "bold", color: theme.colors.text, marginBottom: theme.spacing.md, textAlign: "center" }}>
          {t("auth.forgotPassword.checkEmail")}
        </Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 14, textAlign: "center", marginBottom: theme.spacing.lg }}>
          {t("auth.forgotPassword.emailSent")}
        </Text>
        <Pressable
          onPress={() => router.back()}
          accessible={true}
          accessibilityLabel={t("auth.forgotPassword.backToLogin")}
          accessibilityRole="button"
        >
          <Text style={{ color: theme.colors.accent, textAlign: "center", fontSize: 14 }}>
            {t("auth.forgotPassword.backToLogin")}
          </Text>
        </Pressable>
      </View>
    );
  }

  return (
    <ScrollView
      keyboardShouldPersistTaps="handled"
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ flexGrow: 1, justifyContent: "center", padding: theme.spacing.xl }}
    >
      <Text style={{ fontSize: 24, fontWeight: "bold", color: theme.colors.text, marginBottom: theme.spacing.lg }}>
        {t("auth.forgotPassword.title")}
      </Text>
      <TextInput
        placeholder={t("auth.forgotPassword.email")}
        placeholderTextColor={theme.colors.textMuted}
        value={email}
        onChangeText={setEmail}
        autoCapitalize="none"
        keyboardType="email-address"
        accessible={true}
        accessibilityLabel={t("auth.forgotPassword.email")}
        style={{
          backgroundColor: theme.colors.surface,
          color: theme.colors.text,
          borderRadius: theme.radii.md,
          padding: theme.spacing.md,
          borderWidth: 1,
          borderColor: theme.colors.border,
          marginBottom: theme.spacing.md,
        }}
      />
      <Pressable
        onPress={handleSubmit}
        disabled={requestReset.isPending}
        accessible={true}
        accessibilityLabel={t("auth.forgotPassword.submit")}
        accessibilityRole="button"
        style={{
          backgroundColor: theme.colors.accent,
          borderRadius: theme.radii.md,
          padding: theme.spacing.md,
          alignItems: "center",
          opacity: requestReset.isPending ? 0.7 : 1,
        }}
      >
        {requestReset.isPending ? (
          <ActivityIndicator color={theme.colors.onAccent} />
        ) : (
          <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 16 }}>{t("auth.forgotPassword.submit")}</Text>
        )}
      </Pressable>
    </ScrollView>
  );
}
