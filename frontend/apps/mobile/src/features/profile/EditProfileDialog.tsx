import { useEffect, useState } from "react";
import { View, Text, TextInput, Pressable, Switch, Modal, ScrollView, ActivityIndicator, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceUpdateProfile,
  getUserServiceListProfilesQueryKey,
} from "@evey/api";
import type { V1Profile } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../providers/ThemeProvider";

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
  // -1 clears any persisted MaxAge, 0+ sets it. Only honored when isKids=true server-side.
  maxAge?: number;
};

const MAX_AGE_OPTIONS = ["", "0", "6", "7", "10", "12", "13", "14", "15", "16", "17", "18"];

export function EditProfileDialog({ open, onClose, profile, userId, hasKidsProfile }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [name, setName] = useState(profile.name ?? "");
  const [isKids, setIsKids] = useState(!!profile.isKids);
  const [pin, setPin] = useState("");
  const [maxAge, setMaxAge] = useState(profile.maxAge != null ? String(profile.maxAge) : "");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setName(profile.name ?? "");
      setIsKids(!!profile.isKids);
      setPin("");
      setMaxAge(profile.maxAge != null ? String(profile.maxAge) : "");
      setError(null);
    }
  }, [open, profile]);

  const otherProfileHasKids = hasKidsProfile && !profile.isKids;
  const pinRequiredBecauseOfKids = otherProfileHasKids && !isKids;

  const updateProfile = useUserServiceUpdateProfile({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
        toast.success(t("auth.profile.updated", "Profile updated"));
        onClose();
      },
      onError: (err: Error) =>
        toast.error(err.message || t("auth.profile.failedToUpdate", "Failed to update profile")),
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
    if (!profile.id) return;
    const data: UpdateProfileData = {};
    if (name.trim() !== (profile.name ?? "")) data.name = name.trim();
    if (isKids !== !!profile.isKids) data.isKids = isKids;
    if (pin.length === 4) data.pin = pin;
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
                        <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 13, fontWeight: "500" }}>
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
                style={[s.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}
              />
              <Text style={[s.rowHelp, { color: theme.colors.textMuted, marginTop: 4 }]}>
                {pinRequiredBecauseOfKids
                  ? t("auth.profile.fields.pinRequired", "A PIN is required because a kids profile exists.")
                  : t("auth.profile.fields.pinKeep", "Leave blank to keep current PIN.")}
              </Text>
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
      fontSize: 18,
      fontWeight: "700",
      marginBottom: t.spacing.md,
    },
    label: {
      fontSize: 12,
      fontWeight: "600",
      textTransform: "uppercase",
      marginBottom: 6,
    },
    input: {
      borderWidth: 1,
      paddingHorizontal: 12,
      paddingVertical: 10,
      fontSize: 15,
    },
    row: {
      flexDirection: "row",
      alignItems: "center",
      gap: t.spacing.md,
    },
    rowLabel: { fontSize: 15, fontWeight: "500" },
    rowHelp: { fontSize: 12, marginTop: 2 },
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
    error: { fontSize: 13 },
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
    btnText: { fontWeight: "600", fontSize: 15 },
  });
