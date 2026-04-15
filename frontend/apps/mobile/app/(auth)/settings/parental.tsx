import { useState, useEffect } from "react";
import { View, Text, ScrollView, Pressable, TextInput, Alert, ActivityIndicator, Switch, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, useUserServiceUpdateProfile, eveyFetch } from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import { useTheme } from "../../../src/providers/ThemeProvider";

const RATING_OPTIONS = [
  { value: "", label: "No restriction" },
  { value: "G", label: "G" },
  { value: "PG", label: "PG" },
  { value: "PG-13", label: "PG-13" },
  { value: "R", label: "R" },
  { value: "NC-17", label: "NC-17" },
];

export default function ParentalSettingsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", {
    query: { enabled: !!user?.id },
  });

  const profiles = profilesResp?.profiles;
  const defaultProfile = profiles?.find((p: any) => p.isDefault) ?? profiles?.[0];

  const enabled = (defaultProfile as any)?.preferences?.parental_enabled === "true";
  const [maxRating, setMaxRating] = useState((defaultProfile as any)?.maxRating ?? "");
  const [pin, setPin] = useState("");
  const [confirmPin, setConfirmPin] = useState("");
  const [saving] = useState(false);

  useEffect(() => {
    setMaxRating((defaultProfile as any)?.maxRating ?? "");
  }, [defaultProfile]);

  const updateProfile = useUserServiceUpdateProfile({
    mutation: {
      onSuccess: () => {
        if (user?.id) {
          void qc.invalidateQueries({ queryKey: [`/api/v1/users/${user.id}/profiles`] });
        }
      },
      onError: (err: Error) => Alert.alert(t("common.error", "Error"), err.message),
    },
  });

  function handleToggle() {
    if (!defaultProfile?.id) return;
    updateProfile.mutate({
      id: defaultProfile.id,
      data: {
        preferences: {
          ...(defaultProfile as any).preferences,
          parental_enabled: enabled ? "false" : "true",
        },
        maxRating: enabled ? undefined : (maxRating || undefined),
      },
    });
  }

  function handleSaveRating(rating: string) {
    if (!defaultProfile?.id) return;
    setMaxRating(rating);
    updateProfile.mutate({
      id: defaultProfile.id,
      data: { maxRating: rating || undefined },
    });
  }

  function handleSavePin() {
    if (pin.length !== 4) {
      Alert.alert(t("auth.settings.parental.pin.mustBe4", "PIN must be 4 digits"));
      return;
    }
    if (pin !== confirmPin) {
      Alert.alert(t("auth.settings.parental.pin.mismatch", "PINs do not match"));
      return;
    }
    if (!defaultProfile?.id) return;
    updateProfile.mutate({
      id: defaultProfile.id,
      data: {
        preferences: {
          ...(defaultProfile as any).preferences,
          parental_pin: pin,
        },
      },
    });
    setPin("");
    setConfirmPin("");
    Alert.alert(t("auth.settings.parental.pin.saved", "PIN saved"));
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

      {/* Max rating */}
      <Text style={s.sectionTitle}>{t("auth.settings.parental.maxRating", "Max Rating")}</Text>
      <View style={s.section}>
        {RATING_OPTIONS.map((opt) => (
          <Pressable
            key={opt.value}
            style={s.row}
            onPress={() => handleSaveRating(opt.value)}
            disabled={!enabled}
          >
            <Text style={[s.label, !enabled && { opacity: 0.4 }]}>{opt.label}</Text>
            {maxRating === opt.value && (
              <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>
                {"\u2713"}
              </Text>
            )}
          </Pressable>
        ))}
      </View>

      {/* PIN */}
      <Text style={s.sectionTitle}>{t("auth.settings.parental.pin.title", "PIN")}</Text>
      <View style={s.section}>
        <View style={{ padding: theme.spacing.md }}>
          <Text style={s.hint}>
            {t("auth.settings.parental.pin.description", "Set a 4-digit PIN to unlock restricted content")}
          </Text>
          <TextInput
            secureTextEntry
            keyboardType="number-pad"
            maxLength={4}
            value={pin}
            onChangeText={(v) => setPin(v.replace(/\D/g, "").slice(0, 4))}
            placeholder={t("auth.settings.parental.pin.newPin", "New PIN")}
            placeholderTextColor={theme.colors.textMuted}
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
            placeholder={t("auth.settings.parental.pin.confirmPin", "Confirm PIN")}
            placeholderTextColor={theme.colors.textMuted}
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
          <Pressable
            onPress={handleSavePin}
            disabled={!enabled || pin.length < 4 || updateProfile.isPending}
            style={[
              s.saveBtn,
              {
                backgroundColor: !enabled || pin.length < 4 ? theme.colors.border : theme.colors.accent,
                borderRadius: theme.radii.md,
              },
            ]}
          >
            {updateProfile.isPending ? (
              <ActivityIndicator color="#fff" size="small" />
            ) : (
              <Text style={{ color: "#fff", fontWeight: "600" }}>
                {t("auth.settings.parental.pin.savePin", "Save PIN")}
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
