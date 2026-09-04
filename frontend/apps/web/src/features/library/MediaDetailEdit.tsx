import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import type { V1MediaItem } from "@evey/api";
import {
  useMetadataServiceSetMetadataOverride,
  useMetadataServiceRemoveMetadataOverride,
  useMetadataServiceRefreshMetadata,
  useMetadataServiceGetArtwork,
  useMetadataServiceListMetadataOverrides,
  getMetadataServiceListMetadataOverridesQueryKey,
  eveyFetch,
  niceNameFromPath,
} from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Textarea } from "@/components/ui/Textarea";
import { IdentifyMetadataModal } from "./IdentifyMetadataModal";

interface EditForm {
  title: string;
  originalTitle: string;
  year: string;
  overview: string;
  genres: string;
  communityRating: string;
  imdbId: string;
  tmdbId: string;
}

const FIELD_MAP: Array<{ formKey: keyof EditForm; overrideKey: string; labelKey: string }> = [
  { formKey: "title", overrideKey: "title", labelKey: "media.detail.title" },
  { formKey: "originalTitle", overrideKey: "original_title", labelKey: "media.detail.originalTitle" },
  { formKey: "year", overrideKey: "year", labelKey: "media.detail.year" },
  { formKey: "overview", overrideKey: "overview", labelKey: "media.detail.overview" },
  { formKey: "genres", overrideKey: "genres", labelKey: "media.detail.genresLabel" },
  { formKey: "communityRating", overrideKey: "community_rating", labelKey: "media.detail.rating" },
];

function FieldBadge({ isOverridden }: { isOverridden: boolean }) {
  const { t } = useTranslation();
  return isOverridden ? (
    <span className="text-xs px-1.5 py-0.5 rounded bg-[var(--badge-warning-bg,#f59e0b33)] text-[var(--badge-warning-text,#f59e0b)]">
      {t("media.detail.overridden")}
    </span>
  ) : (
    <span className="text-xs px-1.5 py-0.5 rounded bg-[var(--surface-raised)] text-[var(--text-muted)]">
      {t("media.detail.auto")}
    </span>
  );
}

function ArtworkPicker({ mediaItemId }: { mediaItemId: string }) {
  const { t } = useTranslation();
  const { data } = useMetadataServiceGetArtwork(mediaItemId, undefined);
  const queryClient = useQueryClient();
  const [selecting, setSelecting] = useState<string | null>(null);

  // Both the picker and the upload buttons need the same two invalidations:
  // the artwork list, and the item itself (whose poster URL is embedded).
  const refreshArtwork = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: [`/api/v1/metadata/${mediaItemId}/artwork`] });
    queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${mediaItemId}`] });
  }, [queryClient, mediaItemId]);

  const allArtwork = data?.artwork ?? [];
  const posters = allArtwork.filter((a) => a.type === "ARTWORK_TYPE_PRIMARY");
  const backdrops = allArtwork.filter((a) => a.type === "ARTWORK_TYPE_BACKDROP");

  async function selectArtwork(artworkId: string) {
    setSelecting(artworkId);
    try {
      await eveyFetch<void>({ url: `/api/v1/media/${mediaItemId}/artwork/${artworkId}/select`, method: "POST" });
      refreshArtwork();
      toast.success(t("media.detail.artworkUpdated"));
    } catch {
      toast.error(t("media.detail.artworkFailed"));
    } finally {
      setSelecting(null);
    }
  }

  // Previously returned null when there was nothing to choose between. It now
  // also hosts the upload buttons, and an item with no artwork at all is
  // exactly the case where an admin most needs them.

  return (
    <div className="flex flex-col gap-3">
      {posters.length > 1 && (
        <div>
          <p className="text-xs font-medium mb-2 text-[var(--text-muted)]">{t("media.detail.poster")}</p>
          <div className="flex gap-2 overflow-x-auto pb-1 scrollbar-thin">
            {posters.map((a, i) => (
              <button
                key={a.id}
                onClick={() => a.id && selectArtwork(a.id)}
                disabled={selecting === a.id}
                aria-label={t("media.detail.poster") + ` ${i + 1}`}
                className="flex-shrink-0 rounded overflow-hidden transition-all"
                style={{
                  width: 150,
                  aspectRatio: "2/3",
                  border: i === 0 ? "2px solid var(--accent)" : "2px solid transparent",
                  opacity: selecting === a.id ? 0.5 : 1,
                }}
              >
                <img src={a.url} alt="" className="w-full h-full object-cover" loading="lazy" />
              </button>
            ))}
          </div>
        </div>
      )}
      {backdrops.length > 1 && (
        <div>
          <p className="text-xs font-medium mb-2 text-[var(--text-muted)]">{t("media.detail.backdrop")}</p>
          <div className="flex gap-2 overflow-x-auto pb-1 scrollbar-thin">
            {backdrops.map((a, i) => (
              <button
                key={a.id}
                onClick={() => a.id && selectArtwork(a.id)}
                disabled={selecting === a.id}
                aria-label={t("media.detail.backdrop") + ` ${i + 1}`}
                className="flex-shrink-0 rounded overflow-hidden transition-all"
                style={{
                  width: 288,
                  aspectRatio: "16/9",
                  border: i === 0 ? "2px solid var(--accent)" : "2px solid transparent",
                  opacity: selecting === a.id ? 0.5 : 1,
                }}
              >
                <img src={a.url} alt="" className="w-full h-full object-cover" loading="lazy" />
              </button>
            ))}
          </div>
        </div>
      )}
      <div className="flex gap-2">
        <UploadArtworkButton mediaItemId={mediaItemId} type="poster" label={t("media.detail.uploadPoster")} onDone={refreshArtwork} />
        <UploadArtworkButton mediaItemId={mediaItemId} type="backdrop" label={t("media.detail.uploadBackdrop")} onDone={refreshArtwork} />
      </div>
    </div>
  );
}

// A file picker that posts one image and makes it the item's current poster or
// backdrop. Rendered as a label wrapping a hidden input: a styled <button> that
// programmatically clicks an input is the usual approach and the usual source of
// keyboard/screen-reader bugs, whereas a label IS the input's accessible name.
function UploadArtworkButton({
  mediaItemId,
  type,
  label,
  onDone,
}: {
  mediaItemId: string;
  type: "poster" | "backdrop";
  label: string;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);

  async function upload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    // Reset immediately so picking the SAME file again still fires onChange.
    e.target.value = "";
    if (!file) return;
    setBusy(true);
    try {
      const body = new FormData();
      body.append("artwork", file);
      body.append("type", type);
      await eveyFetch<{ id: string }>({
        url: `/api/v1/admin/media/${mediaItemId}/artwork`,
        method: "POST",
        data: body,
      });
      toast.success(t("media.detail.artworkUpdated"));
      onDone();
    } catch {
      toast.error(t("media.detail.artworkFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <label
      className="text-xs px-3 py-2 rounded-[var(--radius-md)] border border-[var(--border)] cursor-pointer hover:bg-[var(--surface-raised)] transition-colors-fast"
      style={{ opacity: busy ? 0.5 : 1 }}
    >
      {busy ? t("common.loading") : label}
      <input
        type="file"
        accept="image/jpeg,image/png,image/webp"
        className="sr-only"
        disabled={busy}
        onChange={upload}
        data-testid={`upload-${type}`}
      />
    </label>
  );
}

export function MetadataEditPanel({
  item,
  onSave,
  onCancel,
}: {
  item: V1MediaItem;
  onSave: () => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const origImdbId = item.providerIds?.imdb ?? "";
  // The readable title of the file on disk, e.g. "Rocket 2 - 1080P.mkv" -> "Rocket 2".
  // Empty when the path yields nothing usable, which is why every use of it
  // falls back rather than showing a blank.
  const fileNiceName = niceNameFromPath(item.filePath);

  const origTmdbId = item.providerIds?.tmdb ?? "";
  const initialForm: EditForm = {
    title: item.title ?? "",
    originalTitle: item.originalTitle ?? "",
    year: item.year ? String(item.year) : "",
    overview: item.overview ?? "",
    genres: item.genres?.join(", ") ?? "",
    communityRating: item.communityRating ? String(Math.round(item.communityRating * 100) / 100) : "",
    imdbId: origImdbId,
    tmdbId: origTmdbId,
  };
  const [form, setForm] = useState<EditForm>(initialForm);
  const [saving, setSaving] = useState(false);
  const [refreshStatus, setRefreshStatus] = useState<"idle" | "running" | "done" | "error">("idle");
  const [rematchOpen, setRematchOpen] = useState(false);
  const [rematching, setRematching] = useState(false);
  const [identifyOpen, setIdentifyOpen] = useState(false);
  const canSave =
    form.title.trim() !== "" ||
    form.originalTitle.trim() !== "" ||
    form.imdbId.trim() !== "" ||
    form.tmdbId.trim() !== "" ||
    form.overview.trim() !== "" ||
    form.genres.trim() !== "" ||
    form.year.trim() !== "" ||
    form.communityRating.trim() !== "";
  const setOverride = useMetadataServiceSetMetadataOverride();
  const removeOverride = useMetadataServiceRemoveMetadataOverride();
  const refresh = useMetadataServiceRefreshMetadata();
  const queryClient = useQueryClient();

  const { data: overridesData } = useMetadataServiceListMetadataOverrides(item.id ?? "", {
    query: { enabled: !!item.id },
  });
  const overridesByField = Object.fromEntries(
    (overridesData?.overrides ?? []).map((o) => [o.fieldName ?? "", o]),
  );

  const mediaType = item.type === "MEDIA_ITEM_TYPE_SERIES" || item.type === "MEDIA_ITEM_TYPE_SEASON" || item.type === "MEDIA_ITEM_TYPE_EPISODE" ? "tv" : "movie";

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    if (!item.id) return;
    setSaving(true);

    const newTmdbId = form.tmdbId.trim();
    const newImdbId = form.imdbId.trim();
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
        queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${item.id}`] });
        toast.success(t("media.detail.reEnrichedWithNewId"));
      }

      // Only save overrides for fields the user explicitly edited in the form.
      // Pre-filled fields equal to initialForm are NOT user intent; saving them
      // would shadow a fresh enrichment (e.g. after IMDB/TMDB ID change).
      const dirtyFields = FIELD_MAP
        .map(({ overrideKey, formKey }) => ({ overrideKey, formKey, value: form[formKey].trim() }))
        .filter(({ formKey, value }) => value !== "" && value !== initialForm[formKey].trim());
      if (dirtyFields.length > 0) {
        for (const { overrideKey, value } of dirtyFields) {
          await setOverride.mutateAsync({ data: { mediaItemId: item.id, fieldName: overrideKey, customValue: value } });
        }
        queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${item.id}`] });
        queryClient.invalidateQueries({ queryKey: getMetadataServiceListMetadataOverridesQueryKey(item.id) });
        toast.success(t("media.detail.metadataSaved"));
      }
      onSave();
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
      queryClient.invalidateQueries({ queryKey: getMetadataServiceListMetadataOverridesQueryKey(item.id) });
      queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${item.id}`] });
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
      setTimeout(() => { setRefreshStatus("idle"); onSave(); }, 1500);
    } catch (err) {
      setRefreshStatus("error");
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`${t("media.detail.refreshFailed")}: ${msg}`);
    }
  }

  // Re-match: ask the backend to drop overrides + provider IDs and re-search
  // metadata from ScanTitle. Used to recover from a wrong TMDB match in one
  // click. Wrapped with eveyFetch (orval not regenerated for the new RPC).
  async function handleRematch() {
    if (!item.id) return;
    setRematching(true);
    try {
      const resp = await eveyFetch<{ mediaItemId?: string; matched?: boolean; newTitle?: string }>({
        url: `/api/v1/metadata/${item.id}/research`,
        method: "POST",
        data: {},
      });
      queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${item.id}`] });
      queryClient.invalidateQueries({ queryKey: getMetadataServiceListMetadataOverridesQueryKey(item.id) });
      const newTitle = resp?.newTitle ?? "";
      toast.success(t("media.detail.rematchSuccess", { title: newTitle || t("media.detail.rematchSuccessFallback") }));
      setRematchOpen(false);
      onSave();
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`${t("media.detail.rematchFailed")}: ${msg}`);
    } finally {
      setRematching(false);
    }
  }

  // Front-only: nothing persists until user hits Save. Lets the user start
  // from a blank slate and chain (reset → IMDB/TMDB ID → Save) or (reset →
  // manual fields → Save) without closing the modal.
  function handleResetMetadata() {
    setForm({
      title: "",
      originalTitle: "",
      year: "",
      overview: "",
      genres: "",
      communityRating: "",
      imdbId: "",
      tmdbId: "",
    });
  }

  function set(field: keyof EditForm) {
    return (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      setForm((prev) => ({ ...prev, [field]: e.target.value }));
  }

  const tmdbLink = form.tmdbId.trim()
    ? `https://www.themoviedb.org/${mediaType}/${form.tmdbId.trim()}`
    : null;
  const imdbLink = form.imdbId.trim()
    ? `https://www.imdb.com/title/${form.imdbId.trim()}/`
    : null;

  function renderField({ formKey, overrideKey, labelKey }: typeof FIELD_MAP[number]) {
    const label = t(labelKey);
    const hasOverride = !!overridesByField[overrideKey]?.id;
    const overrideId = overridesByField[overrideKey]?.id;
    return (
      <div key={formKey} className="flex flex-col gap-1">
        <div className="flex items-center gap-2">
          <FieldBadge isOverridden={hasOverride} />
          {hasOverride && overrideId && (
            <button
              type="button"
              onClick={() => handleResetField(overrideId, overrideKey)}
              className="text-xs underline text-[var(--text-muted)]"
            >
              {t("media.detail.resetToAuto")}
            </button>
          )}
        </div>
        {formKey === "overview" ? (
          <Textarea label={label} value={form[formKey]} onChange={set(formKey)} rows={4} />
        ) : formKey === "year" ? (
          <Input label={label} type="number" value={form[formKey]} onChange={set(formKey)} placeholder="2024" />
        ) : formKey === "communityRating" ? (
          <Input label={label} type="number" step="0.01" min="0" max="10" value={form[formKey]} onChange={set(formKey)} placeholder="7.5" />
        ) : formKey === "genres" ? (
          <Input label={label} value={form[formKey]} onChange={set(formKey)} placeholder="Action, Drama" />
        ) : formKey === "title" ? (
          <Input id="media-edit-title" data-testid="media-edit-title-input" label={label} value={form[formKey]} onChange={set(formKey)} />
        ) : (
          <Input label={label} value={form[formKey]} onChange={set(formKey)} placeholder={label} />
        )}
      </div>
    );
  }

  return (
    <form onSubmit={handleSave} className="flex flex-col gap-4">
      {/* What is actually on disk, read-only. When a match is wrong every other
          field here already holds the WRONG film's data - the filename is the
          only evidence that has not been overwritten, so it belongs at the top
          where it can be compared at a glance. It also seeds the TMDB identify
          box below. */}
      {fileNiceName && (
        <Input
          label={t("media.detail.fileName", "Nom du fichier")}
          value={fileNiceName}
          readOnly
          disabled
          data-testid="media-edit-filename"
          title={item.filePath ?? ""}
        />
      )}
      {FIELD_MAP.map(renderField)}
      <div className="grid grid-cols-2 gap-3">
        <div className="flex flex-col gap-1">
          <Input label={t("media.detail.imdbId")} value={form.imdbId} onChange={set("imdbId")} placeholder="tt1234567" />
          {imdbLink && (
            <a href={imdbLink} target="_blank" rel="noopener noreferrer"
              className="text-xs underline text-[var(--accent)]">
              {t("media.detail.viewOnImdb")}
            </a>
          )}
        </div>
        <div className="flex flex-col gap-1">
          <Input label={t("media.detail.tmdbId")} value={form.tmdbId} onChange={set("tmdbId")} placeholder="12345" />
          {tmdbLink && (
            <a href={tmdbLink} target="_blank" rel="noopener noreferrer"
              className="text-xs underline text-[var(--accent)]">
              {t("media.detail.viewOnTmdb")}
            </a>
          )}
        </div>
      </div>
      {(form.tmdbId.trim() !== origTmdbId || form.imdbId.trim() !== origImdbId) &&
        (form.tmdbId.trim() !== "" || form.imdbId.trim() !== "") && (
        <p className="text-xs text-[var(--color-warning)]">
          {t("media.detail.idChangedWarning")}
        </p>
      )}
      {item.id && <ArtworkPicker mediaItemId={item.id} />}
      <div className="flex gap-2 flex-wrap pt-1">
        <Button type="submit" loading={saving} disabled={!canSave} data-testid="media-edit-save-btn">{t("common.save")}</Button>
        <Button type="button" variant="secondary" loading={refreshStatus === "running"} onClick={handleRefresh} data-testid="media-edit-refresh-btn">
          {refreshStatus === "done" ? t("common.done") : refreshStatus === "error" ? t("common.error") : t("media.detail.refreshMetadata")}
        </Button>
        <Button type="button" variant="danger" onClick={handleResetMetadata} data-testid="media-edit-reset-btn">
          {t("media.detail.resetMetadata")}
        </Button>
        <Button type="button" variant="danger" onClick={() => setRematchOpen(true)} data-testid="media-edit-rematch-btn">
          {t("media.detail.rematchMetadata")}
        </Button>
        {item.id && (
          <Button type="button" variant="secondary" onClick={() => setIdentifyOpen(true)} data-testid="media-edit-identify-btn">
            {t("media.detail.identify.openButton", "Identify on TMDB")}
          </Button>
        )}
        <Button type="button" variant="secondary" onClick={onCancel} data-testid="media-edit-cancel-btn">{t("common.cancel")}</Button>
      </div>
      {setOverride.isError && <p className="text-sm text-[var(--color-danger)]">{t("media.detail.failedToSaveDot")}</p>}
      {item.id && (
        <IdentifyMetadataModal
          open={identifyOpen}
          onClose={() => setIdentifyOpen(false)}
          mediaItemId={item.id}
          initialQuery={fileNiceName || (item.title ?? "").trim()}
          mediaType={mediaType}
          onApplied={() => onSave()}
        />
      )}
      <Modal open={rematchOpen} onClose={() => !rematching && setRematchOpen(false)} title={t("media.detail.rematchMetadata")} size="sm">
        <div className="flex flex-col gap-4">
          <p className="text-sm text-[var(--text-muted)]">{t("media.detail.rematchConfirm")}</p>
          <div className="flex gap-2 justify-end">
            <Button type="button" variant="secondary" onClick={() => setRematchOpen(false)} disabled={rematching}>
              {t("common.cancel")}
            </Button>
            <Button type="button" variant="danger" loading={rematching} onClick={handleRematch} data-testid="media-edit-rematch-confirm-btn">
              {t("media.detail.rematchMetadata")}
            </Button>
          </div>
        </div>
      </Modal>
    </form>
  );
}
