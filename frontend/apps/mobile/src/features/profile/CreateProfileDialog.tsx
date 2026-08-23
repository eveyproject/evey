import { useState } from "react";
import { View, Text, TextInput, Pressable, Switch, Modal, ScrollView, ActivityIndicator, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceCreateProfile,
  getUserServiceListProfilesQueryKey,
  apiErrorMessage,
} from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../providers/ThemeProvider";
import { FONT_SIZE } from "@evey/theme";

interface Props {
  open: boolean;
  onClose: () => void;
  userId: string;
  hasKidsProfile: boolean;
}

type CreateProfileData = {
  name?: string;
  isKids?: boolean;
  pin?: string;
  maxAge?: number;
};

const MAX_AGE_OPTIONS = ["", "0", "6", "7", "10", "12", "13", "14", "15", "16", "17", "18"];

export function CreateProfileDialog({ open, onClose, userId, hasKidsProfile }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [isKids, setIsKids] = useState(false);
  const [pin, setPin] = useState("");
  const [maxAge, setMaxAge] = useState("");
  const [error, setError] = useState<string | null>(null);

  const pinRequired = hasKidsProfile && !isKids;

  const createProfile = useUserServiceCreateProfile({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
        toast.success(t("auth.profile.created", "Profile created"));
        reset();
        onClose();
      },
      onError: (err: Error) =>
        toast.error(apiErrorMessage(err, t, t("auth.profile.failedToUpdate", "Failed to update profile"))),
    },
  });

  function reset() {
    setName("");
    setIsKids(false);
    setPin("");
    setMaxAge("");
    setError(null);
  }

  function handleClose() {
    reset();
    onClose();
  }

  function handleSubmit() {
    setError(null);
    if (!name.trim()) {
      setError(t("auth.profile.fields.nameRequired", "Name is required"));
      return;
    }
    if (pinRequired && pin.length !== 4) {
      setError(t("auth.profile.fields.pinRequired", "A PIN is required because a kids profile exists."));
      return;
    }
    const data: CreateProfileData = { name: name.trim() };
    if (isKids) {
      data.isKids = true;
      if (maxAge !== "") {
        const n = parseInt(maxAge, 10);
        if (!Number.isNaN(n)) data.maxAge = n;
      }
    }
    if (pin.length === 4) data.pin = pin;
    createProfile.mutate({ userId, data });
  }

  const s = styles(theme);

  return (
    <Modal visible={open} transparent animationType="fade" onRequestClose={handleClose}>
      <View style={[s.backdrop, { backgroundColor: theme.colors.overlay }]}>
        <View style={[s.dialog, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={[s.title, { color: theme.colors.text }]}>
            {t("auth.profile.manage.createProfile", "Create Profile")}
          </Text>

          <ScrollView contentContainerStyle={{ gap: theme.spacing.md }} keyboardShouldPersistTaps="handled">
            {/* Name */}
            <View>
              <Text style={[s.label, { color: theme.colors.textMuted }]}>
                {t("auth.profile.fields.name", "Name")}
              </Text>
              <TextInput
                value={name}
                onChangeText={setName}
                placeholder={t("auth.profile.fields.name", "Name")}
                placeholderTextColor={theme.colors.textMuted}
                accessibilityLabel={t("auth.profile.fields.name", "Name")}
                style={[s.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}
                autoCapitalize="words"
              />
            </View>

            {/* isKids */}
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

            {/* MaxAge - kids only */}
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

            {/* PIN - required when there is already a kids profile and we're creating non-kids */}
            {pinRequired && (
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
                  {t("auth.profile.fields.pinRequired", "A PIN is required because a kids profile exists.")}
                </Text>
              </View>
            )}

            {error && (
              <Text style={[s.error, { color: theme.colors.error }]}>{error}</Text>
            )}
          </ScrollView>

          <View style={s.buttons}>
            <Pressable
              onPress={handleClose}
              style={[s.btn, { backgroundColor: theme.colors.bg, borderRadius: theme.radii.md }]}
              accessibilityRole="button"
              accessibilityLabel={t("common.cancel", "Cancel")}
            >
              <Text style={[s.btnText, { color: theme.colors.text }]}>{t("common.cancel", "Cancel")}</Text>
            </Pressable>
            <Pressable
              onPress={handleSubmit}
              disabled={createProfile.isPending}
              style={[s.btn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: createProfile.isPending ? 0.6 : 1 }]}
              accessibilityRole="button"
              accessibilityLabel={t("common.save", "Save")}
            >
              {createProfile.isPending ? (
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
    rowLabel: {
      fontSize: FONT_SIZE.sm,
      fontWeight: "500",
    },
    rowHelp: {
      fontSize: FONT_SIZE.xs,
      marginTop: 2,
    },
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
    error: {
      fontSize: FONT_SIZE.xs,
    },
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
    btnText: {
      fontWeight: "600",
      fontSize: FONT_SIZE.sm,
    },
  });
