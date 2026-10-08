import { useEffect, useMemo, useState } from "react";
import { View, Text, TextInput, Pressable, Switch, Modal, ScrollView, ActivityIndicator, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceUpdateProfile,
  getUserServiceListProfilesQueryKey,
  apiErrorMessage,
} from "@evey/api";
import type { V1Profile } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../providers/ThemeProvider";
import { FONT_SIZE } from "@evey/theme";
import { BottomSheet } from "../../components/BottomSheet";
import { supportedLanguages, applyEditedProfileLocale } from "../../i18n";
import { regionOptions } from "../../lib/regionOptions";

interface Props {
  open: boolean;
  onClose: () => void;
  profile: V1Profile;
  userId: string;
  hasKidsProfile: boolean;
}

type UpdateProfileData = {
  name?: string;
  isKids?: boolean;
  pin?: string;
  // Proof the server demands with a new PIN: the current PIN when one is set,
  // the account password for a first one (same rule as the parental route).
  currentPin?: string;
  currentPassword?: string;
  locale?: string;
  region?: string;
  // -1 clears any persisted MaxAge, 0+ sets it. Only honored when isKids=true server-side.
  maxAge?: number;
};

const MAX_AGE_OPTIONS = ["", "0", "6", "7", "10", "12", "13", "14", "15", "16", "17", "18"];

export function EditProfileDialog({ open, onClose, profile, userId, hasKidsProfile }: Props) {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const [name, setName] = useState(profile.name ?? "");
  const [isKids, setIsKids] = useState(!!profile.isKids);
  const [pin, setPin] = useState("");
  const [currentPin, setCurrentPin] = useState("");
  const [password, setPassword] = useState("");
  const [locale, setLocale] = useState(profile.locale ?? "");
  const [region, setRegion] = useState(profile.region ?? "");
  const [maxAge, setMaxAge] = useState(profile.maxAge != null ? String(profile.maxAge) : "");
  const [error, setError] = useState<string | null>(null);
  const [langSheetOpen, setLangSheetOpen] = useState(false);
  const [regionSheetOpen, setRegionSheetOpen] = useState(false);
  const [regionQuery, setRegionQuery] = useState("");

  useEffect(() => {
    if (open) {
      setName(profile.name ?? "");
      setIsKids(!!profile.isKids);
      setPin("");
      setCurrentPin("");
      setPassword("");
      setLocale(profile.locale ?? "");
      setRegion(profile.region ?? "");
      setMaxAge(profile.maxAge != null ? String(profile.maxAge) : "");
      setError(null);
      setRegionQuery("");
    }
  }, [open, profile]);

  const otherProfileHasKids = hasKidsProfile && !profile.isKids;
  const pinRequiredBecauseOfKids = otherProfileHasKids && !isKids;
  // A new PIN is proven like on the parental settings page: by the current
  // PIN when the profile has one, by the account password for a first one.
  const needsCurrentPin = pin.length === 4 && !!profile.hasPin;
  const needsPassword = pin.length === 4 && !profile.hasPin;

  const localeOptions = useMemo(
    () => [
      { code: "", label: t("auth.profile.fields.localeAuto", "Automatic") },
      ...supportedLanguages,
    ],
    [t],
  );
  const currentLocaleLabel = localeOptions.find((o) => o.code === locale)?.label ?? locale;

  const regionOpts = useMemo(() => regionOptions(i18n.language), [i18n.language]);
  const regionOptsWithAuto = useMemo(
    () => [{ code: "", label: t("common.region.auto", "Auto") }, ...regionOpts],
    [regionOpts, t],
  );
  const filteredRegionOpts = useMemo(() => {
    const q = regionQuery.trim().toLowerCase();
    if (!q) return regionOptsWithAuto;
    return regionOptsWithAuto.filter(
      (o) => o.label.toLowerCase().includes(q) || o.code.toLowerCase().includes(q),
    );
  }, [regionOptsWithAuto, regionQuery]);
  const currentRegionLabel = regionOptsWithAuto.find((o) => o.code === region)?.label ?? region;

  const updateProfile = useUserServiceUpdateProfile({
    mutation: {
      onSuccess: (_res, vars) => {
        void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
        // The server accepted the new locale; retarget the RUNNING session too
        // when the edited profile is the active one - otherwise the picker
        // saves silently and the UI language never moves until the next login.
        const submitted = (vars?.data ?? {}) as UpdateProfileData;
        if (submitted.locale !== undefined) {
          void applyEditedProfileLocale(profile, submitted.locale);
        }
        toast.success(t("auth.profile.updated", "Profile updated"));
        onClose();
      },
      onError: (err: Error) =>
        toast.error(apiErrorMessage(err, t, t("auth.profile.failedToUpdate", "Failed to update profile"))),
    },
  });

  function handleSubmit() {
    setError(null);
    if (!name.trim()) {
      setError(t("auth.profile.fields.nameRequired", "Name is required"));
      return;
    }
    if (pin && pin.length !== 4) {
      setError(t("auth.profile.select.pinInvalid", "PIN must be 4 digits"));
      return;
    }
    if (needsCurrentPin && currentPin.length !== 4) {
      setError(t("auth.settings.parental.pin.currentPinRequired", "Enter your current PIN"));
      return;
    }
    if (needsPassword && !password) {
      setError(t("auth.settings.parental.pin.passwordRequired", "Password required to set a PIN for the first time"));
      return;
    }
    if (!profile.id) return;
    const data: UpdateProfileData = {};
    if (name.trim() !== (profile.name ?? "")) data.name = name.trim();
    if (isKids !== !!profile.isKids) data.isKids = isKids;
    if (locale !== (profile.locale ?? "")) data.locale = locale;
    if (region !== (profile.region ?? "")) data.region = region;
    if (pin.length === 4) {
      data.pin = pin;
      if (needsCurrentPin) data.currentPin = currentPin;
      else data.currentPassword = password;
    }
    if (isKids) {
      const currentMaxAge = profile.maxAge != null ? String(profile.maxAge) : "";
      if (maxAge !== currentMaxAge) {
        if (maxAge === "") {
          data.maxAge = -1; // sentinel: clear
        } else {
          const n = parseInt(maxAge, 10);
          if (!Number.isNaN(n)) data.maxAge = n;
        }
      }
    }
    if (Object.keys(data).length === 0) {
      onClose();
      return;
    }
    updateProfile.mutate({ id: profile.id, data });
  }

  const s = styles(theme);

  return (
    <>
      <Modal visible={open} transparent animationType="fade" onRequestClose={onClose}>
        <View style={[s.backdrop, { backgroundColor: theme.colors.overlay }]}>
          <View style={[s.dialog, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
            <Text style={[s.title, { color: theme.colors.text }]}>
              {t("auth.profile.editProfile", "Edit Profile")}
            </Text>

            <ScrollView contentContainerStyle={{ gap: theme.spacing.md }} keyboardShouldPersistTaps="handled">
              <View>
                <Text style={[s.label, { color: theme.colors.textMuted }]}>
                  {t("auth.profile.fields.name", "Name")}
                </Text>
                <TextInput
                  value={name}
                  onChangeText={setName}
                  placeholderTextColor={theme.colors.textMuted}
                  accessibilityLabel={t("auth.profile.fields.name", "Name")}
                  style={[s.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}
                  autoCapitalize="words"
                />
              </View>

              <View style={s.row}>
                <View style={{ flex: 1 }}>
                  <Text style={[s.rowLabel, { color: theme.colors.text }]}>
                    {t("auth.profile.fields.isKids", "Kids profile")}
                  </Text>
                  <Text style={[s.rowHelp, { color: theme.colors.textMuted }]}>
                    {t("auth.profile.fields.isKidsHelp", "Kids profiles only see age-appropriate content.")}
                  </Text>
                </View>
                <Switch
                  value={isKids}
                  onValueChange={setIsKids}
                  trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
                />
              </View>

              {isKids && (
                <View>
                  <Text style={[s.label, { color: theme.colors.textMuted }]}>
                    {t("auth.profile.fields.maxAge", "Max age rating")}
                  </Text>
                  <View style={s.chipsRow}>
                    {MAX_AGE_OPTIONS.map((opt) => {
                      const active = maxAge === opt;
                      const label = opt === ""
                        ? t("auth.settings.parental.defaultMaxAge", "Default (12+)")
                        : `${opt}+`;
                      return (
                        <Pressable
                          key={opt || "default"}
                          onPress={() => setMaxAge(opt)}
                          style={[
                            s.chip,
                            {
                              backgroundColor: active ? theme.colors.accent : theme.colors.bg,
                              borderColor: active ? theme.colors.accent : theme.colors.border,
                              borderRadius: theme.radii.sm,
                            },
                          ]}
                          accessibilityRole="radio"
                          accessibilityState={{ selected: active }}
                          accessibilityLabel={label}
                        >
                          <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: FONT_SIZE.xs, fontWeight: "500" }}>
                            {label}
                          </Text>
                        </Pressable>
                      );
                    })}
                  </View>
                </View>
              )}

              <View>
                <Text style={[s.label, { color: theme.colors.textMuted }]}>
                  {t("auth.profile.fields.pin", "PIN (4 digits)")}
                </Text>
                <TextInput
                  value={pin}
                  onChangeText={(v) => setPin(v.replace(/\D/g, "").slice(0, 4))}
                  placeholder="••••"
                  placeholderTextColor={theme.colors.textMuted}
                  secureTextEntry
                  keyboardType="number-pad"
                  maxLength={4}
                  accessibilityLabel={t("auth.profile.fields.pin", "PIN (4 digits)")}
                  style={[s.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}
                />
                <Text style={[s.rowHelp, { color: theme.colors.textMuted, marginTop: 4 }]}>
                  {pinRequiredBecauseOfKids
                    ? t("auth.profile.fields.pinRequired", "A PIN is required because a kids profile exists.")
                    : t("auth.profile.fields.pinKeep", "Leave blank to keep current PIN.")}
                </Text>
              </View>

              {needsCurrentPin && (
                <View>
                  <Text style={[s.label, { color: theme.colors.textMuted }]}>
                    {t("auth.settings.parental.pin.currentPin", "Current PIN")}
                  </Text>
                  <TextInput
                    value={currentPin}
                    onChangeText={(v) => setCurrentPin(v.replace(/\D/g, "").slice(0, 4))}
                    placeholder="••••"
                    placeholderTextColor={theme.colors.textMuted}
                    secureTextEntry
                    keyboardType="number-pad"
                    maxLength={4}
                    accessibilityLabel={t("auth.settings.parental.pin.currentPin", "Current PIN")}
                    style={[s.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}
                  />
                </View>
              )}

              {needsPassword && (
                <View>
                  <Text style={[s.label, { color: theme.colors.textMuted }]}>
                    {t("auth.settings.parental.pin.passwordReauth", "Account password")}
                  </Text>
                  <TextInput
                    value={password}
                    onChangeText={setPassword}
                    placeholderTextColor={theme.colors.textMuted}
                    secureTextEntry
                    autoCapitalize="none"
                    accessibilityLabel={t("auth.settings.parental.pin.passwordReauth", "Account password")}
                    style={[s.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}
                  />
                  <Text style={[s.rowHelp, { color: theme.colors.textMuted, marginTop: 4 }]}>
                    {t("auth.settings.parental.pin.passwordRequired", "Password required to set a PIN for the first time")}
                  </Text>
                </View>
              )}

              <View>
                <Text style={[s.label, { color: theme.colors.textMuted }]}>
                  {t("auth.profile.fields.locale", "Language")}
                </Text>
                <Pressable
                  onPress={() => setLangSheetOpen(true)}
                  style={[s.selectRow, { backgroundColor: theme.colors.bg, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}
                  accessibilityRole="button"
                  accessibilityLabel={t("auth.profile.fields.locale", "Language")}
                >
                  <Text style={{ color: theme.colors.text, fontSize: FONT_SIZE.sm }}>{currentLocaleLabel}</Text>
                </Pressable>
              </View>

              <View>
                <Text style={[s.label, { color: theme.colors.textMuted }]}>
                  {t("auth.profile.fields.region", "Region")}
                </Text>
                <Pressable
                  onPress={() => setRegionSheetOpen(true)}
                  style={[s.selectRow, { backgroundColor: theme.colors.bg, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}
                  accessibilityRole="button"
                  accessibilityLabel={t("auth.profile.fields.region", "Region")}
                >
                  <Text style={{ color: theme.colors.text, fontSize: FONT_SIZE.sm }}>{currentRegionLabel}</Text>
                </Pressable>
              </View>

              {error && <Text style={[s.error, { color: theme.colors.error }]}>{error}</Text>}
            </ScrollView>

            <View style={s.buttons}>
              <Pressable
                onPress={onClose}
                style={[s.btn, { backgroundColor: theme.colors.bg, borderRadius: theme.radii.md }]}
                accessibilityRole="button"
                accessibilityLabel={t("common.cancel", "Cancel")}
              >
                <Text style={[s.btnText, { color: theme.colors.text }]}>{t("common.cancel", "Cancel")}</Text>
              </Pressable>
              <Pressable
                onPress={handleSubmit}
                disabled={updateProfile.isPending}
                style={[s.btn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: updateProfile.isPending ? 0.6 : 1 }]}
                accessibilityRole="button"
                accessibilityLabel={t("common.save", "Save")}
              >
                {updateProfile.isPending ? (
                  <ActivityIndicator color={theme.colors.onAccent} size="small" />
                ) : (
                  <Text style={[s.btnText, { color: theme.colors.onAccent }]}>{t("common.save", "Save")}</Text>
                )}
              </Pressable>
            </View>
          </View>
        </View>
      </Modal>

      <BottomSheet
        visible={langSheetOpen}
        onClose={() => setLangSheetOpen(false)}
        title={t("auth.profile.fields.locale", "Language")}
      >
        {localeOptions.map((opt) => {
          const active = locale === opt.code;
          return (
            <Pressable
              key={opt.code || "auto"}
              style={[s.sheetRow, { borderBottomColor: theme.colors.border }]}
              onPress={() => { setLocale(opt.code); setLangSheetOpen(false); }}
              accessibilityRole="radio"
              accessibilityState={{ selected: active }}
              accessibilityLabel={opt.label}
            >
              <Text style={[s.sheetLabel, { color: active ? theme.colors.accent : theme.colors.text }]}>
                {opt.label}
              </Text>
              {active && <Text style={[s.sheetCheck, { color: theme.colors.accent }]}>{"✓"}</Text>}
            </Pressable>
          );
        })}
      </BottomSheet>

      <BottomSheet
        visible={regionSheetOpen}
        onClose={() => setRegionSheetOpen(false)}
        title={t("auth.profile.fields.region", "Region")}
      >
        <TextInput
          value={regionQuery}
          onChangeText={setRegionQuery}
          placeholder={t("common.search", "Search") || "Search"}
          placeholderTextColor={theme.colors.textMuted}
          style={[s.searchInput, { backgroundColor: theme.colors.surfaceRaised, color: theme.colors.text, borderRadius: theme.radii.md }]}
        />
        {filteredRegionOpts.map((opt) => {
          const active = region === opt.code;
          return (
            <Pressable
              key={opt.code || "auto"}
              style={[s.sheetRow, { borderBottomColor: theme.colors.border }]}
              onPress={() => { setRegion(opt.code); setRegionSheetOpen(false); }}
              accessibilityRole="radio"
              accessibilityState={{ selected: active }}
              accessibilityLabel={opt.label}
            >
              <Text style={[s.sheetLabel, { color: active ? theme.colors.accent : theme.colors.text }]}>
                {opt.label}
              </Text>
              {active && <Text style={[s.sheetCheck, { color: theme.colors.accent }]}>{"✓"}</Text>}
            </Pressable>
          );
        })}
      </BottomSheet>
    </>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    backdrop: {
      flex: 1,
      justifyContent: "center",
      alignItems: "center",
      padding: t.spacing.lg,
    },
    dialog: {
      width: "100%",
      maxWidth: 420,
      maxHeight: "85%",
      padding: t.spacing.lg,
    },
    title: {
      fontSize: FONT_SIZE.lg,
      fontWeight: "700",
      marginBottom: t.spacing.md,
    },
    label: {
      fontSize: FONT_SIZE.xs,
      fontWeight: "600",
      textTransform: "uppercase",
      marginBottom: 6,
    },
    input: {
      borderWidth: 1,
      paddingHorizontal: 12,
      paddingVertical: 10,
      fontSize: FONT_SIZE.sm,
    },
    row: {
      flexDirection: "row",
      alignItems: "center",
      gap: t.spacing.md,
    },
    rowLabel: { fontSize: FONT_SIZE.sm, fontWeight: "500" },
    rowHelp: { fontSize: FONT_SIZE.xs, marginTop: 2 },
    chipsRow: {
      flexDirection: "row",
      flexWrap: "wrap",
      gap: 6,
    },
    chip: {
      paddingVertical: 6,
      paddingHorizontal: 12,
      borderWidth: 1,
    },
    selectRow: {
      borderWidth: 1,
      paddingHorizontal: 12,
      paddingVertical: 10,
    },
    sheetRow: {
      flexDirection: "row",
      alignItems: "center",
      justifyContent: "space-between",
      paddingVertical: 14,
      borderBottomWidth: StyleSheet.hairlineWidth,
    },
    sheetLabel: { fontSize: FONT_SIZE.sm, flex: 1 },
    sheetCheck: { fontSize: FONT_SIZE.lg, fontWeight: "600", marginLeft: 12 },
    searchInput: {
      paddingHorizontal: 12,
      paddingVertical: 8,
      fontSize: FONT_SIZE.sm,
      marginBottom: 8,
    },
    error: { fontSize: FONT_SIZE.xs },
    buttons: {
      flexDirection: "row",
      gap: t.spacing.sm,
      marginTop: t.spacing.md,
    },
    btn: {
      flex: 1,
      paddingVertical: 12,
      alignItems: "center",
      justifyContent: "center",
    },
    btnText: { fontWeight: "600", fontSize: FONT_SIZE.sm },
  });
