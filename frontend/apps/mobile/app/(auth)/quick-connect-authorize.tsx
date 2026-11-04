import { useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert, StyleSheet } from "react-native";
import { Stack, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceAuthorizeQuickConnect } from "@evey/api";
import { useTheme } from "../../src/providers/ThemeProvider";

// Approving another device from this phone: the code that device shows can be
// typed or scanned. The profile menu used to open the sign-in screen instead,
// the one that SHOWS a code for this phone - a signed-in viewer had no field
// to type the television's code into without digging through Settings.
export default function QuickConnectAuthorizeScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const [code, setCode] = useState("");
  const [done, setDone] = useState(false);

  const authorize = useUserServiceAuthorizeQuickConnect({
    mutation: {
      onSuccess: () => {
        setCode("");
        setDone(true);
      },
      onError: () => Alert.alert(t("auth.settings.quickConnect.invalidCode")),
    },
  });

  const trimmed = code.trim().toUpperCase();
  const canSubmit = trimmed.length >= 4 && !authorize.isPending;
  const submit = () => {
    if (!canSubmit) return;
    setDone(false);
    authorize.mutate({ data: { code: trimmed } });
  };

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg, padding: theme.spacing.xl }]}>
      <Stack.Screen options={{ title: t("auth.settings.quickConnect.title"), headerShown: true, headerStyle: { backgroundColor: theme.colors.surface }, headerTintColor: theme.colors.text }} />
      <Text style={{ color: theme.colors.textMuted, fontSize: 14, textAlign: "center", marginBottom: theme.spacing.lg }}>
        {t("auth.settings.quickConnect.description")}
      </Text>

      <TextInput
        placeholder="A1B2C3"
        placeholderTextColor={theme.colors.textMuted}
        value={code}
        onChangeText={(v) => setCode(v.toUpperCase())}
        onSubmitEditing={submit}
        autoCapitalize="characters"
        autoCorrect={false}
        autoFocus
        maxLength={6}
        returnKeyType="done"
        accessibilityLabel={t("auth.settings.quickConnect.title")}
        style={[styles.input, { backgroundColor: theme.colors.surface, color: theme.colors.text, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}
      />

      <Pressable
        onPress={submit}
        disabled={!canSubmit}
        style={[styles.button, { backgroundColor: canSubmit ? theme.colors.accent : theme.colors.border, borderRadius: theme.radii.md }]}
        accessibilityRole="button"
        accessibilityLabel={t("auth.settings.quickConnect.authorize")}
      >
        {authorize.isPending
          ? <ActivityIndicator color={theme.colors.onAccent} size="small" />
          : <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 16 }}>{t("auth.settings.quickConnect.authorize")}</Text>}
      </Pressable>

      {done && (
        <Text style={{ color: theme.colors.accent, fontSize: 14, textAlign: "center", marginTop: theme.spacing.md }}>
          {t("auth.settings.quickConnect.authorized")}
        </Text>
      )}

      <Pressable
        onPress={() => router.push("/(auth)/qr-scan")}
        style={[styles.button, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginTop: theme.spacing.lg }]}
        accessibilityRole="button"
        accessibilityLabel={t("auth.settings.quickConnect.scanQr")}
      >
        <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: 16 }}>{t("auth.settings.quickConnect.scanQr")}</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, alignItems: "stretch", justifyContent: "flex-start" },
  input: { borderWidth: 1, padding: 12, fontFamily: "monospace", letterSpacing: 6, fontSize: 24, textAlign: "center" },
  button: { marginTop: 12, paddingVertical: 12, alignItems: "center" },
});
