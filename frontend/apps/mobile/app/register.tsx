import { useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert, ScrollView } from "react-native";
import { router, useLocalSearchParams } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceRegisterWithInvite } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";

export default function Register() {
  const theme = useTheme();
  const { t } = useTranslation();
  const { invite } = useLocalSearchParams<{ invite?: string }>();
  const setTokens = useAuthStore((s) => s.setTokens);

  const [inviteCode, setInviteCode] = useState(invite ?? "");
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [validationError, setValidationError] = useState("");

  const { mutate: register, isPending } = useUserServiceRegisterWithInvite({
    mutation: {
      onSuccess: (data) => {
        if (data.accessToken) {
          setTokens(data.accessToken);
          router.replace("/profile-select");
        }
      },
      onError: (err) => {
        Alert.alert(
          t("auth.register.registrationFailed"),
          err instanceof Error ? err.message : t("auth.register.registrationFailed"),
        );
      },
    },
  });

  function handleRegister() {
    setValidationError("");
    if (password !== confirm) {
      setValidationError(t("auth.register.passwordMismatch"));
      return;
    }
    if (password.length < 8) {
      setValidationError(t("auth.register.passwordMin"));
      return;
    }
    register({ data: { inviteCode, username, email, password } });
  }

  const inputStyle = {
    backgroundColor: theme.colors.surface,
    color: theme.colors.text,
    borderRadius: theme.radii.md,
    padding: theme.spacing.md,
    borderWidth: 1,
    borderColor: theme.colors.border,
    marginBottom: theme.spacing.md,
  };

  const canSubmit = !!username.trim() && !!password && !!confirm && !!inviteCode.trim();

  return (
    <ScrollView
      keyboardShouldPersistTaps="handled"
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ flexGrow: 1, justifyContent: "center", padding: theme.spacing.xl }}
    >
      <Text style={{ fontSize: 24, fontWeight: "bold", color: theme.colors.text, marginBottom: theme.spacing.sm }}>
        {t("auth.register.createAccount")}
      </Text>
      <Text style={{ color: theme.colors.textMuted, marginBottom: theme.spacing.lg }}>
        {t("auth.register.signUpWithInvite")}
      </Text>

      <TextInput
        placeholder={t("auth.register.inviteCode")}
        placeholderTextColor={theme.colors.textMuted}
        value={inviteCode}
        onChangeText={setInviteCode}
        autoCapitalize="none"
        autoCorrect={false}
        style={inputStyle}
        accessible={true}
        accessibilityLabel="Invite code"
      />
      <TextInput
        placeholder={t("auth.register.username")}
        placeholderTextColor={theme.colors.textMuted}
        value={username}
        onChangeText={setUsername}
        autoCapitalize="none"
        autoCorrect={false}
        style={inputStyle}
        accessible={true}
        accessibilityLabel="Username"
      />
      <TextInput
        placeholder={t("auth.register.email")}
        placeholderTextColor={theme.colors.textMuted}
        value={email}
        onChangeText={setEmail}
        autoCapitalize="none"
        autoCorrect={false}
        keyboardType="email-address"
        style={inputStyle}
        accessible={true}
        accessibilityLabel="Email"
      />
      <TextInput
        placeholder={t("auth.register.password")}
        placeholderTextColor={theme.colors.textMuted}
        value={password}
        onChangeText={setPassword}
        secureTextEntry
        style={inputStyle}
        accessible={true}
        accessibilityLabel="Password"
      />
      <TextInput
        placeholder={t("auth.register.confirmPassword")}
        placeholderTextColor={theme.colors.textMuted}
        value={confirm}
        onChangeText={setConfirm}
        secureTextEntry
        style={inputStyle}
        accessible={true}
        accessibilityLabel="Confirm password"
      />

      {!!validationError && (
        <Text style={{ color: theme.colors.error, textAlign: "center", marginBottom: theme.spacing.md }}>
          {validationError}
        </Text>
      )}

      <Pressable
        onPress={handleRegister}
        disabled={isPending || !canSubmit}
        accessible={true}
        accessibilityLabel="Create account"
        accessibilityRole="button"
        style={{
          backgroundColor: canSubmit ? theme.colors.accent : theme.colors.border,
          borderRadius: theme.radii.md,
          padding: theme.spacing.md,
          alignItems: "center",
        }}
      >
        {isPending ? (
          <ActivityIndicator color={theme.colors.text} />
        ) : (
          <Text style={{ color: "#fff", fontWeight: "600", fontSize: 16 }}>
            {t("auth.register.createAccountBtn")}
          </Text>
        )}
      </Pressable>

      <Pressable onPress={() => router.back()} style={{ marginTop: theme.spacing.md }}>
        <Text style={{ color: theme.colors.accent, textAlign: "center" }}>
          {t("auth.register.alreadyHaveAccount")} {t("auth.register.signIn")}
        </Text>
      </Pressable>
    </ScrollView>
  );
}
