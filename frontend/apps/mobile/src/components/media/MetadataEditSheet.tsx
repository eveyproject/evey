import { useEffect, useMemo, useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Linking, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import type { V1MediaItem } from "@evey/api";
import {
  useMetadataServiceSetMetadataOverride,
  useMetadataServiceRemoveMetadataOverride,
  useMetadataServiceRefreshMetadata,
  useMetadataServiceListMetadataOverrides,
  getMetadataServiceListMetadataOverridesQueryKey,
  getLibraryServiceGetMediaItemQueryKey,
  eveyFetch,
} from "@evey/api";
import { toast } from "@evey/store";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../../providers/ThemeProvider";
import { BottomSheet } from "../BottomSheet";
import { ArtworkPicker } from "./ArtworkPicker";
import { RematchConfirmModal } from "./RematchConfirmModal";
import { IdentifyMetadataSheet } from "./IdentifyMetadataSheet";
import {
  METADATA_FIELD_MAP,
  buildInitialMetadataForm,
  canSaveMetadataForm,
  getDirtyOverrideFields,
  validateMetadataForm,
  hasMetadataFormErrors,
  type MetadataFormValues,
} from "./metadataFormValidation";

// Mirrors apps/web/src/features/library/MediaDetailEdit.tsx's per-field label keys,
// reusing the exact same i18n keys since both clients read from the same server-side
// translation catalog (mobile's i18n backend fetches /api/v1/i18n/{lng} at runtime).
const FIELD_LABEL_KEYS: Record<string, string> = {
  title: "media.detail.title",
  originalTitle: "media.detail.originalTitle",
  year: "media.detail.year",
  overview: "media.detail.overview",
  genres: "media.detail.genresLabel",
  communityRating: "media.detail.rating",
};

interface MetadataEditSheetProps {
  visible: boolean;
  onClose: () => void;
  item: V1MediaItem;
  onSaved?: () => void;
}

export function MetadataEditSheet({ visible, onClose, item, onSaved }: MetadataEditSheetProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const qc = useQueryClient();

  const initialForm = useMemo(() => buildInitialMetadataForm(item), [item]);
  const [form, setForm] = useState<MetadataFormValues>(initialForm);
  const [saving, setSaving] = useState(false);
  const [refreshStatus, setRefreshStatus] = useState<"idle" | "running" | "done" | "error">("idle");
  const [rematchOpen, setRematchOpen] = useState(false);
  const [rematching, setRematching] = useState(false);
  const [identifyOpen, setIdentifyOpen] = useState(false);

  // Re-seed the form every time the sheet opens, same as EditProfileDialog - a
  // stale draft from a previous open (or from another media item) must never leak in.
  useEffect(() => {
    if (visible) setForm(initialForm);
  }, [visible, initialForm]);

  const setOverride = useMetadataServiceSetMetadataOverride();
  const removeOverride = useMetadataServiceRemoveMetadataOverride();
  const refresh = useMetadataServiceRefreshMetadata();

  const { data: overridesData } = useMetadataServiceListMetadataOverrides(item.id ?? "", {
    query: { enabled: visible && !!item.id },
  });
  const overridesByField = useMemo(
    () => Object.fromEntries((overridesData?.overrides ?? []).map((o) => [o.fieldName ?? "", o])),
    [overridesData],
  );

  const mediaType =
    item.type === "MEDIA_ITEM_TYPE_SERIES" ||
    item.type === "MEDIA_ITEM_TYPE_SEASON" ||
    item.type === "MEDIA_ITEM_TYPE_EPISODE"
      ? "tv"
      : "movie";

  const errors = validateMetadataForm(form);
  const disableSave = saving || hasMetadataFormErrors(errors);

  function set(field: keyof MetadataFormValues) {
    return (value: string) => setForm((prev) => ({ ...prev, [field]: value }));
  }

  async function handleSave() {
    if (!item.id || !canSaveMetadataForm(form) || hasMetadataFormErrors(errors)) return;
    setSaving(true);

    const newTmdbId = form.tmdbId.trim();
    const newImdbId = form.imdbId.trim();
    const origTmdbId = initialForm.tmdbId;
    const origImdbId = initialForm.imdbId;
    const tmdbChanged = newTmdbId !== origTmdbId && newTmdbId !== "";
    const imdbChanged = newImdbId !== origImdbId && newImdbId !== "";

    try {
      if (tmdbChanged || imdbChanged) {
        await eveyFetch<void>({
          url: `/api/v1/admin/media/${item.id}/refresh-metadata`,
          method: "POST",
          data: {
            tmdb_id: tmdbChanged ? newTmdbId : "",
            imdb_id: imdbChanged ? newImdbId : "",
            media_type: mediaType,
          },
        });
        void qc.invalidateQueries({ queryKey: getLibraryServiceGetMediaItemQueryKey(item.id) });
        toast.success(t("media.detail.reEnrichedWithNewId"));
      }

      const dirtyFields = getDirtyOverrideFields(form, initialForm);
      if (dirtyFields.length > 0) {
        for (const { overrideKey, value } of dirtyFields) {
          await setOverride.mutateAsync({ data: { mediaItemId: item.id, fieldName: overrideKey, customValue: value } });
        }
        void qc.invalidateQueries({ queryKey: getLibraryServiceGetMediaItemQueryKey(item.id) });
        void qc.invalidateQueries({ queryKey: getMetadataServiceListMetadataOverridesQueryKey(item.id) });
        toast.success(t("media.detail.metadataSaved"));
      }
      onSaved?.();
      onClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`${t("media.detail.failedToSave")}: ${msg}`);
    } finally {
      setSaving(false);
    }
  }

  async function handleResetField(overrideId: string, fieldName: string) {
    if (!item.id) return;
    try {
      await removeOverride.mutateAsync({ id: overrideId });
      void qc.invalidateQueries({ queryKey: getMetadataServiceListMetadataOverridesQueryKey(item.id) });
      void qc.invalidateQueries({ queryKey: getLibraryServiceGetMediaItemQueryKey(item.id) });
      toast.success(t("media.detail.resetSuccess", { field: fieldName }));
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`${t("media.detail.resetFailed")}: ${msg}`);
    }
  }

  async function handleRefresh() {
    if (!item.id) return;
    setRefreshStatus("running");
    try {
      await refresh.mutateAsync({ mediaItemId: item.id });
      setRefreshStatus("done");
      void qc.invalidateQueries({ queryKey: getLibraryServiceGetMediaItemQueryKey(item.id) });
      setTimeout(() => { setRefreshStatus("idle"); onSaved?.(); onClose(); }, 1200);
    } catch (err) {
      setRefreshStatus("error");
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`${t("media.detail.refreshFailed")}: ${msg}`);
    }
  }

  // Re-match: ask the backend to drop overrides + provider IDs and re-search
  // metadata from ScanTitle. Mirrors web's handleRematch (eveyFetch: orval
  // not regenerated for the new RPC).
  async function handleRematch() {
    if (!item.id) return;
    setRematching(true);
    try {
      const resp = await eveyFetch<{ mediaItemId?: string; matched?: boolean; newTitle?: string }>({
        url: `/api/v1/metadata/${item.id}/research`,
        method: "POST",
        data: {},
      });
      void qc.invalidateQueries({ queryKey: getLibraryServiceGetMediaItemQueryKey(item.id) });
      void qc.invalidateQueries({ queryKey: getMetadataServiceListMetadataOverridesQueryKey(item.id) });
      const newTitle = resp?.newTitle ?? "";
      toast.success(t("media.detail.rematchSuccess", { title: newTitle || t("media.detail.rematchSuccessFallback") }));
      setRematchOpen(false);
      onSaved?.();
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`${t("media.detail.rematchFailed")}: ${msg}`);
    } finally {
      setRematching(false);
    }
  }

  // Front-only clear, mirrors web: nothing persists until Save, so this lets the
  // user start from a blank slate and chain a fresh IMDB/TMDB ID or manual fields.
  function handleResetForm() {
    setForm({ title: "", originalTitle: "", year: "", overview: "", genres: "", communityRating: "", imdbId: "", tmdbId: "" });
  }

  const tmdbLink = form.tmdbId.trim() ? `https://www.themoviedb.org/${mediaType}/${form.tmdbId.trim()}` : null;
  const imdbLink = form.imdbId.trim() ? `https://www.imdb.com/title/${form.imdbId.trim()}/` : null;
  const idChangedWarning =
    (form.tmdbId.trim() !== initialForm.tmdbId || form.imdbId.trim() !== initialForm.imdbId) &&
    (form.tmdbId.trim() !== "" || form.imdbId.trim() !== "");

  const s = styles(theme);

  function renderField(formKey: keyof MetadataFormValues, overrideKey: string) {
    const label = t(FIELD_LABEL_KEYS[formKey]);
    const override = overridesByField[overrideKey];
    const hasOverride = !!override?.id;
    const fieldError = errors[formKey];
    return (
      <View key={formKey} style={s.field}>
        <View style={s.fieldHeader}>
          <View style={[s.badge, { backgroundColor: hasOverride ? theme.colors.warning + "33" : theme.colors.surfaceRaised }]}>
            <Text style={{ color: hasOverride ? theme.colors.warning : theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>
              {hasOverride ? t("media.detail.overridden") : t("media.detail.auto")}
            </Text>
          </View>
          {hasOverride && override?.id && (
            <Pressable onPress={() => handleResetField(override.id!, overrideKey)} accessibilityRole="button" accessibilityLabel={t("media.detail.resetToAuto")}>
              <Text style={[s.resetLink, { color: theme.colors.textMuted }]}>{t("media.detail.resetToAuto")}</Text>
            </Pressable>
          )}
        </View>
        <Text style={[s.label, { color: theme.colors.textMuted }]}>{label}</Text>
        <TextInput
          value={form[formKey]}
          onChangeText={set(formKey)}
          multiline={formKey === "overview"}
          numberOfLines={formKey === "overview" ? 4 : 1}
          keyboardType={formKey === "year" || formKey === "communityRating" ? "numeric" : "default"}
          placeholderTextColor={theme.colors.textMuted}
          accessibilityLabel={label}
          style={[
            s.input,
            formKey === "overview" && s.inputMultiline,
            { backgroundColor: theme.colors.bg, color: theme.colors.text, borderColor: fieldError ? theme.colors.error : theme.colors.border, borderRadius: theme.radii.md },
          ]}
        />
        {fieldError && (
          <Text style={[s.fieldErrorText, { color: theme.colors.error }]}>
            {fieldError === "invalidYear"
              ? t("media.detail.invalidYear", "Invalid year")
              : fieldError === "invalidRating"
                ? t("media.detail.invalidRating", "Rating must be between 0 and 10")
                : t("media.detail.tooLong", "Value is too long")}
          </Text>
        )}
      </View>
    );
  }

  return (
    <>
    <BottomSheet visible={visible} onClose={onClose} title={t("media.detail.editMetadata")}>
      <View style={{ gap: theme.spacing.md, paddingBottom: theme.spacing.lg }}>
        {METADATA_FIELD_MAP.map(({ formKey, overrideKey }) => renderField(formKey, overrideKey))}

        <View style={s.field}>
          <Text style={[s.label, { color: theme.colors.textMuted }]}>{t("media.detail.imdbId")}</Text>
          <TextInput
            value={form.imdbId}
            onChangeText={set("imdbId")}
            placeholder="tt1234567"
            placeholderTextColor={theme.colors.textMuted}
            autoCapitalize="none"
            accessibilityLabel={t("media.detail.imdbId")}
            style={[s.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderColor: errors.imdbId ? theme.colors.error : theme.colors.border, borderRadius: theme.radii.md }]}
          />
          {imdbLink && (
            <Pressable onPress={() => Linking.openURL(imdbLink)} accessibilityRole="link" accessibilityLabel={t("media.detail.viewOnImdb")}>
              <Text style={[s.link, { color: theme.colors.accent }]}>{t("media.detail.viewOnImdb")}</Text>
            </Pressable>
          )}
        </View>

        <View style={s.field}>
          <Text style={[s.label, { color: theme.colors.textMuted }]}>{t("media.detail.tmdbId")}</Text>
          <TextInput
            value={form.tmdbId}
            onChangeText={set("tmdbId")}
            placeholder="12345"
            placeholderTextColor={theme.colors.textMuted}
            keyboardType="numeric"
            accessibilityLabel={t("media.detail.tmdbId")}
            style={[s.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderColor: errors.tmdbId ? theme.colors.error : theme.colors.border, borderRadius: theme.radii.md }]}
          />
          {tmdbLink && (
            <Pressable onPress={() => Linking.openURL(tmdbLink)} accessibilityRole="link" accessibilityLabel={t("media.detail.viewOnTmdb")}>
              <Text style={[s.link, { color: theme.colors.accent }]}>{t("media.detail.viewOnTmdb")}</Text>
            </Pressable>
          )}
        </View>

        {idChangedWarning && (
          <Text style={[s.warning, { color: theme.colors.warning }]}>{t("media.detail.idChangedWarning")}</Text>
        )}

        {item.id && <ArtworkPicker mediaItemId={item.id} />}

        <View style={s.buttons}>
          <Pressable
            onPress={handleSave}
            disabled={disableSave}
            style={[s.btn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: disableSave ? 0.5 : 1 }]}
            accessibilityRole="button"
            accessibilityLabel={t("common.save")}
            accessibilityState={{ disabled: disableSave }}
          >
            {saving ? <ActivityIndicator color={theme.colors.onAccent} size="small" /> : <Text style={[s.btnText, { color: theme.colors.onAccent }]}>{t("common.save")}</Text>}
          </Pressable>
          <Pressable
            onPress={handleRefresh}
            disabled={refreshStatus === "running"}
            style={[s.btn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md, opacity: refreshStatus === "running" ? 0.6 : 1 }]}
            accessibilityRole="button"
            accessibilityLabel={t("media.detail.refreshMetadata")}
          >
            {refreshStatus === "running" ? (
              <ActivityIndicator color={theme.colors.text} size="small" />
            ) : (
              <Text style={[s.btnText, { color: theme.colors.text }]}>
                {refreshStatus === "done" ? t("common.done") : refreshStatus === "error" ? t("common.error") : t("media.detail.refreshMetadata")}
              </Text>
            )}
          </Pressable>
        </View>
        <View style={s.buttons}>
          <Pressable
            onPress={handleResetForm}
            style={[s.btn, { backgroundColor: theme.colors.bg, borderRadius: theme.radii.md, borderWidth: 1, borderColor: theme.colors.danger }]}
            accessibilityRole="button"
            accessibilityLabel={t("media.detail.resetMetadata")}
          >
            <Text style={[s.btnText, { color: theme.colors.danger }]}>{t("media.detail.resetMetadata")}</Text>
          </Pressable>
          <Pressable
            onPress={onClose}
            style={[s.btn, { backgroundColor: theme.colors.bg, borderRadius: theme.radii.md }]}
            accessibilityRole="button"
            accessibilityLabel={t("common.cancel")}
          >
            <Text style={[s.btnText, { color: theme.colors.text }]}>{t("common.cancel")}</Text>
          </Pressable>
        </View>
        <View style={s.buttons}>
          <Pressable
            onPress={() => setRematchOpen(true)}
            style={[s.btn, { backgroundColor: theme.colors.bg, borderRadius: theme.radii.md, borderWidth: 1, borderColor: theme.colors.danger }]}
            accessibilityRole="button"
            accessibilityLabel={t("media.detail.rematchMetadata")}
          >
            <Text style={[s.btnText, { color: theme.colors.danger }]}>{t("media.detail.rematchMetadata")}</Text>
          </Pressable>
          {item.id && (
            <Pressable
              onPress={() => setIdentifyOpen(true)}
              style={[s.btn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
              accessibilityRole="button"
              accessibilityLabel={t("media.detail.identify.openButton", "Identify on TMDB")}
            >
              <Text style={[s.btnText, { color: theme.colors.text }]}>{t("media.detail.identify.openButton", "Identify on TMDB")}</Text>
            </Pressable>
          )}
        </View>
      </View>
    </BottomSheet>
    <RematchConfirmModal
      visible={rematchOpen}
      onClose={() => !rematching && setRematchOpen(false)}
      onConfirm={handleRematch}
      pending={rematching}
    />
    {item.id && (
      <IdentifyMetadataSheet
        visible={identifyOpen}
        onClose={() => setIdentifyOpen(false)}
        mediaItemId={item.id}
        initialQuery={(item.title ?? "").replace(/\s*\d{3,4}p?\s*$/i, "").trim()}
        mediaType={mediaType}
        onApplied={() => onSaved?.()}
      />
    )}
    </>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    field: { gap: 4 },
    fieldHeader: { flexDirection: "row", alignItems: "center", gap: t.spacing.sm },
    badge: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: t.radii.sm },
    resetLink: { fontSize: FONT_SIZE.xs, textDecorationLine: "underline" },
    label: { fontSize: FONT_SIZE.xs, fontWeight: "600", textTransform: "uppercase" },
    input: { borderWidth: 1, paddingHorizontal: 12, paddingVertical: 10, fontSize: FONT_SIZE.sm },
    inputMultiline: { minHeight: 90, textAlignVertical: "top" },
    fieldErrorText: { fontSize: FONT_SIZE.xs },
    link: { fontSize: FONT_SIZE.xs, textDecorationLine: "underline" },
    warning: { fontSize: FONT_SIZE.xs },
    buttons: { flexDirection: "row", gap: t.spacing.sm },
    btn: { flex: 1, paddingVertical: 12, alignItems: "center", justifyContent: "center" },
    btnText: { fontWeight: "600", fontSize: FONT_SIZE.sm },
  });
