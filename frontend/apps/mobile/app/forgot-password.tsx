import { useState } from "react";
import { View, Text, TextInput, Pressable, Alert } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../src/providers/ThemeProvider";

export default function ForgotPassword() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [email, setEmail] = useState("");

  function handleSubmit() {
    if (!email.trim()) return;
    // TODO: call password reset API
    Alert.alert(t("auth.forgotPassword.checkEmail"), t("auth.forgotPassword.emailSent"), [
      { text: "OK", onPress: () => router.back() },
    ]);
  }

  return (
    <View style={{ flex: 1, justifyContent: "center", padding: theme.spacing.xl, backgroundColor: theme.colors.bg }}>
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
        style={{
          backgroundColor: theme.colors.accent,
          borderRadius: theme.radii.md,
          padding: theme.spacing.md,
          alignItems: "center",
        }}
      >
        <Text style={{ color: "#fff", fontWeight: "600", fontSize: 16 }}>{t("auth.forgotPassword.submit")}</Text>
      </Pressable>
    </View>
  );
}
