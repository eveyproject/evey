import { useState, useEffect } from "react";
import { View, Text, ScrollView, Pressable, TextInput, Alert, ActivityIndicator, Switch, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  useParentalServiceGetParentalSettings,
  parentalServiceUpdateParentalSettings,
  getParentalServiceGetParentalSettingsQueryKey,
  apiErrorMessage,
} from "@evey/api";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useActiveProfile } from "../../../src/hooks/useActiveProfile";

const AGE_OPTIONS = ["", "0", "6", "7", "10", "12", "13", "14", "15", "16", "17", "18"];

export default function ParentalSettingsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  // useParentalGate enforces the PIN and the age cap of the ACTIVE profile
  // (useParentalGate.ts reads useAuthStore's profileId). Reading and writing
  // the DEFAULT profile's settings here meant a household member on their own
  // profile was shown, and edited, restrictions that were never the ones
  // applied to them.
  const { profile: editedProfile } = useActiveProfile();
  const profileId = editedProfile?.id ?? "";

  const { data: parental } = useParentalServiceGetParentalSettings(profileId, { query: { enabled: !!profileId } } as never);

  const enabled = parental?.enabled ?? false;
  const hasPin = parental?.hasPin ?? false;
  const [maxAge, setMaxAge] = useState<string>("");
  const [pin, setPin] = useState("");
  const [confirmPin, setConfirmPin] = useState("");
  const [currentPin, setCurrentPin] = useState("");
  const [password, setPassword] = useState("");

  useEffect(() => {
    setMaxAge(parental?.maxAge != null ? String(parental.maxAge) : "");
  }, [parental?.maxAge]);

  const updateParental = useMutation({
    mutationFn: (body: {
      enabled?: boolean;
      maxAge?: number;
      unsetMaxAge?: boolean;
      newPin?: string;
      currentPin?: string;
      currentPassword?: string;
    }) => parentalServiceUpdateParentalSettings(profileId, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: getParentalServiceGetParentalSettingsQueryKey(profileId) });
    },
    onError: (err: Error) => Alert.alert(t("common.error", "Error"), apiErrorMessage(err, t, t("common.error", "Error"))),
  });

  function handleToggle() {
    if (!profileId) return;
    updateParental.mutate({ enabled: !enabled });
  }

  function handleSaveAge(age: string) {
    if (!profileId) return;
    // The checkmark moves only once the SERVER has taken the new cap. Moving it
    // optimistically meant a failed save left the tick sitting on an age
    // rating that was never applied - a parent reads that as a restriction in
    // force and it is not. This is the one screen where showing an unconfirmed
    // value is worse than showing a slow one. Same shape as settings/playback.
    const body = age === "" ? { unsetMaxAge: true } : { maxAge: parseInt(age, 10) };
    updateParental.mutate(body, { onSuccess: () => setMaxAge(age) });
  }

  function handleSavePin() {
    if (pin.length !== 4) {
      Alert.alert(t("auth.settings.parental.pin.mustBe4", "Le PIN doit contenir 4 chiffres"));
      return;
    }
    if (pin !== confirmPin) {
      Alert.alert(t("auth.settings.parental.pin.mismatch", "Les PIN ne correspondent pas"));
      return;
    }
    // Rotation requires the current PIN; the first-time set requires the
    // account password instead - otherwise a stolen JWT could silently
    // lock the owner out of (or seize) their profile's parental gate.
    if (hasPin && currentPin.length !== 4) {
      Alert.alert(t("auth.settings.parental.pin.currentPinRequired", "Saisir le code PIN actuel"));
      return;
    }
    if (!hasPin && password === "") {
      Alert.alert(t("auth.settings.parental.pin.passwordRequired", "Mot de passe requis pour définir un PIN la première fois"));
      return;
    }
    if (!profileId) return;
    const body: { newPin: string; currentPin?: string; currentPassword?: string } = { newPin: pin };
    if (hasPin) {
      body.currentPin = currentPin;
    } else {
      body.currentPassword = password;
    }
    updateParental.mutate(body, {
      onSuccess: () => {
        setPin("");
        setConfirmPin("");
        setCurrentPin("");
        setPassword("");
        Alert.alert(t("auth.settings.parental.pin.saved", "PIN enregistré"));
      },
    });
  }

  const s = styles(theme);

  return (
    <ScrollView keyboardShouldPersistTaps="handled" style={s.container} contentContainerStyle={{ paddingBottom: 40 }}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("auth.settings.tabs.parental", "Parental Controls"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />

      {/* Enable toggle */}
      <Text style={s.sectionTitle}>{t("auth.settings.parental.enable", "Enable")}</Text>
      <View style={s.section}>
        <View style={s.row}>
          <View style={{ flex: 1 }}>
            <Text style={s.label}>{t("auth.settings.parental.enable", "Parental Controls")}</Text>
            <Text style={s.hint}>{t("auth.settings.parental.enableDesc", "Restrict content based on rating")}</Text>
          </View>
          <Switch
            value={enabled}
            onValueChange={handleToggle}
            trackColor={{ true: theme.colors.accent, false: theme.colors.border }}
          />
        </View>
      </View>

      {/* Max age */}
      <Text style={s.sectionTitle}>{t("auth.settings.parental.maxAge", "Max Age")}</Text>
      <View style={s.section}>
        {AGE_OPTIONS.map((age) => (
          <Pressable
            key={age || "none"}
            style={s.row}
            onPress={() => handleSaveAge(age)}
            disabled={!enabled}
          >
            <Text style={[s.label, !enabled && { opacity: 0.4 }]}>
              {age === "" ? t("auth.settings.parental.noRestriction", "No restriction") : `${age}+`}
            </Text>
            {maxAge === age && (
              <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>
                {"\u2713"}
              </Text>
            )}
          </Pressable>
        ))}
      </View>

      {/* PIN */}
      <Text style={s.sectionTitle}>
        {hasPin
          ? t("auth.settings.parental.pin.changeTitle", "Changer le code PIN")
          : t("auth.settings.parental.pin.title", "Définir le PIN")}
      </Text>
      <View style={s.section}>
        <View style={{ padding: theme.spacing.md }}>
          <Text style={s.hint}>
            {t("auth.settings.parental.pin.description", "PIN à 4 chiffres pour déverrouiller le contenu restreint.")}
          </Text>
          {hasPin && (
            <TextInput
              secureTextEntry
              keyboardType="number-pad"
              maxLength={4}
              value={currentPin}
              onChangeText={(v) => setCurrentPin(v.replace(/\D/g, "").slice(0, 4))}
              placeholder={t("auth.settings.parental.pin.currentPin", "Code PIN actuel")}
              placeholderTextColor={theme.colors.textMuted}
              accessibilityLabel={t("auth.settings.parental.pin.currentPin", "Code PIN actuel")}
              editable={enabled}
              style={[
                s.input,
                {
                  backgroundColor: theme.colors.bg,
                  color: theme.colors.text,
                  borderColor: theme.colors.border,
                },
                !enabled && { opacity: 0.4 },
              ]}
            />
          )}
          <TextInput
            secureTextEntry
            keyboardType="number-pad"
            maxLength={4}
            value={pin}
            onChangeText={(v) => setPin(v.replace(/\D/g, "").slice(0, 4))}
            placeholder={t("auth.settings.parental.pin.newPin", "Nouveau PIN")}
            placeholderTextColor={theme.colors.textMuted}
            accessibilityLabel={t("auth.settings.parental.pin.newPin", "Nouveau PIN")}
            editable={enabled}
            style={[
              s.input,
              {
                backgroundColor: theme.colors.bg,
                color: theme.colors.text,
                borderColor: theme.colors.border,
              },
              !enabled && { opacity: 0.4 },
            ]}
          />
          <TextInput
            secureTextEntry
            keyboardType="number-pad"
            maxLength={4}
            value={confirmPin}
            onChangeText={(v) => setConfirmPin(v.replace(/\D/g, "").slice(0, 4))}
            placeholder={t("auth.settings.parental.pin.confirmPin", "Confirmer le PIN")}
            placeholderTextColor={theme.colors.textMuted}
            accessibilityLabel={t("auth.settings.parental.pin.confirmPin", "Confirmer le PIN")}
            editable={enabled}
            style={[
              s.input,
              {
                backgroundColor: theme.colors.bg,
                color: theme.colors.text,
                borderColor: theme.colors.border,
              },
              !enabled && { opacity: 0.4 },
            ]}
          />
          {!hasPin && (
            <TextInput
              secureTextEntry
              autoComplete="current-password"
              value={password}
              onChangeText={setPassword}
              placeholder={t("auth.settings.parental.pin.passwordReauth", "Mot de passe du compte")}
              placeholderTextColor={theme.colors.textMuted}
              accessibilityLabel={t("auth.settings.parental.pin.passwordReauth", "Mot de passe du compte")}
              editable={enabled}
              style={[
                s.input,
                {
                  fontFamily: undefined,
                  letterSpacing: 0,
                  textAlign: "left",
                  backgroundColor: theme.colors.bg,
                  color: theme.colors.text,
                  borderColor: theme.colors.border,
                },
                !enabled && { opacity: 0.4 },
              ]}
            />
          )}
          <Pressable
            onPress={handleSavePin}
            disabled={
              !enabled ||
              pin.length < 4 ||
              (hasPin && currentPin.length < 4) ||
              (!hasPin && password === "") ||
              updateParental.isPending
            }
            style={[
              s.saveBtn,
              {
                backgroundColor:
                  !enabled ||
                  pin.length < 4 ||
                  (hasPin && currentPin.length < 4) ||
                  (!hasPin && password === "")
                    ? theme.colors.border
                    : theme.colors.accent,
                borderRadius: theme.radii.md,
              },
            ]}
          >
            {updateParental.isPending ? (
              <ActivityIndicator color={theme.colors.onAccent} size="small" />
            ) : (
              <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>
                {hasPin
                  ? t("auth.settings.parental.pin.updatePin", "Mettre à jour le code PIN")
                  : t("auth.settings.parental.pin.savePin", "Enregistrer le PIN")}
              </Text>
            )}
          </Pressable>
        </View>
      </View>
    </ScrollView>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    container: { flex: 1, backgroundColor: t.colors.bg },
    sectionTitle: {
      color: t.colors.textMuted,
      fontSize: 13,
      fontWeight: "600",
      textTransform: "uppercase",
      marginTop: t.spacing.lg,
      marginBottom: t.spacing.xs,
      marginHorizontal: t.spacing.md,
    },
    section: {
      backgroundColor: t.colors.surface,
      borderRadius: t.radii.md,
      marginHorizontal: t.spacing.md,
      overflow: "hidden",
    },
    row: {
      flexDirection: "row",
      alignItems: "center",
      justifyContent: "space-between",
      paddingVertical: 14,
      paddingHorizontal: t.spacing.md,
      borderBottomWidth: StyleSheet.hairlineWidth,
      borderBottomColor: t.colors.border,
    },
    label: { color: t.colors.text, fontSize: 16 },
    hint: { color: t.colors.textMuted, fontSize: 13, marginTop: 2 },
    input: {
      borderWidth: 1,
      borderRadius: t.radii.md,
      padding: t.spacing.sm,
      fontSize: 18,
      fontFamily: "monospace",
      letterSpacing: 8,
      textAlign: "center",
      marginTop: t.spacing.sm,
    },
    saveBtn: {
      paddingVertical: 12,
      alignItems: "center",
      marginTop: t.spacing.md,
    },
  });
