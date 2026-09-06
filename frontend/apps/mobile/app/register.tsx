import { useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert, ScrollView } from "react-native";
import { router, useLocalSearchParams } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceRegisterWithInvite, eveyFetch } from "@evey/api";
import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";
import { PasswordInput } from "../src/components/PasswordInput";

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
  // Set when a submit is refused because the link is no longer usable - the
  // screen may have been open since before it was disabled.
  const [inviteRejected, setInviteRejected] = useState(false);

  // Validate a link that came from the URL BEFORE showing the form. Without
  // this, a disabled, expired or already-used invite let someone fill in five
  // fields and only then get a flat "Registration failed" with no reason. The
  // web has answered this properly since its own register screen was written.
  const { data: validation, isLoading: validating } = useQuery<{ valid?: boolean }>({
    queryKey: ["invite-validate", invite],
    queryFn: () =>
      eveyFetch<{ valid?: boolean }>({
        url: `/api/v1/invites/${encodeURIComponent(invite ?? "")}/validate`,
        method: "GET",
      }),
    enabled: !!invite,
    retry: false,
    staleTime: 30_000,
  });
  const inviteUnusable = !!invite && (inviteRejected || (!validating && validation?.valid === false));

  const { mutate: register, isPending } = useUserServiceRegisterWithInvite({
    mutation: {
      onSuccess: (data) => {
        if (data.accessToken) {
          setTokens(data.accessToken);
          router.replace("/profile-select");
        }
      },
      onError: (err) => {
        // The server refuses an unusable invite at submit time too: show the
        // dedicated "link no longer valid" screen rather than a raw error.
        const status = (err as { httpStatus?: number } | undefined)?.httpStatus;
        if (invite && (status === 403 || status === 404 || status === 410)) {
          setInviteRejected(true);
          return;
        }
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

  // Checking the link from the URL - hold the form until we know it is usable.
  if (invite && validating && !inviteRejected) {
    return (
      <View
        testID="invite-checking"
        style={{ flex: 1, backgroundColor: theme.colors.bg, justifyContent: "center", alignItems: "center", gap: theme.spacing.md }}
      >
        <ActivityIndicator color={theme.colors.accent} />
        <Text style={{ color: theme.colors.textMuted }}>{t("auth.register.inviteChecking")}</Text>
      </View>
    );
  }

  // Disabled, expired or already used: no form at all, just the reason.
  if (inviteUnusable) {
    return (
      <View
        testID="invite-invalid"
        style={{ flex: 1, backgroundColor: theme.colors.bg, justifyContent: "center", alignItems: "center", padding: theme.spacing.xl, gap: theme.spacing.md }}
      >
        <Text style={{ fontSize: 20, fontWeight: "bold", color: theme.colors.text, textAlign: "center" }}>
          {t("auth.register.inviteInvalidTitle")}
        </Text>
        <Text style={{ color: theme.colors.textMuted, textAlign: "center" }}>
          {t("auth.register.inviteInvalidMessage")}
        </Text>
        <Pressable
          onPress={() => router.replace("/login")}
          accessible={true}
          accessibilityRole="button"
          accessibilityLabel={t("auth.register.backToLogin")}
        >
          <Text style={{ color: theme.colors.accent, fontSize: 16 }}>{t("auth.register.backToLogin")}</Text>
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
        accessibilityLabel={t("auth.register.inviteCode")}
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
        accessibilityLabel={t("auth.register.username")}
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
        accessibilityLabel={t("auth.register.email")}
      />
      <PasswordInput
        placeholder={t("auth.register.password")}
        placeholderTextColor={theme.colors.textMuted}
        value={password}
        onChangeText={setPassword}
        style={inputStyle}
        accessible={true}
        accessibilityLabel={t("auth.register.password")}
      />
      <PasswordInput
        placeholder={t("auth.register.confirmPassword")}
        placeholderTextColor={theme.colors.textMuted}
        value={confirm}
        onChangeText={setConfirm}
        style={inputStyle}
        accessible={true}
        accessibilityLabel={t("auth.register.confirmPassword")}
      />

      {!!validationError && (
        <Text
          style={{ color: theme.colors.error, textAlign: "center", marginBottom: theme.spacing.md }}
          accessibilityRole="alert"
          accessibilityLiveRegion="polite"
        >
          {validationError}
        </Text>
      )}

      <Pressable
        onPress={handleRegister}
        disabled={isPending || !canSubmit}
        accessible={true}
        accessibilityLabel={t("auth.register.createAccountBtn")}
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
          <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 16 }}>
            {t("auth.register.createAccountBtn")}
          </Text>
        )}
      </Pressable>

      <Pressable
        onPress={() => router.back()}
        style={{ marginTop: theme.spacing.md }}
        accessible={true}
        accessibilityLabel={`${t("auth.register.alreadyHaveAccount")} ${t("auth.register.signIn")}`}
        accessibilityRole="button"
      >
        <Text style={{ color: theme.colors.accent, textAlign: "center" }}>
          {t("auth.register.alreadyHaveAccount")} {t("auth.register.signIn")}
        </Text>
      </Pressable>
    </ScrollView>
  );
}
