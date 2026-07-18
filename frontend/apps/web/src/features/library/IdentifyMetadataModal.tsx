import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { eveyFetch, getMetadataServiceListMetadataOverridesQueryKey } from "@evey/api";
import { toast } from "@evey/store";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";

interface IdentifyResult {
  tmdb_id?: string;
  imdb_id?: string;
  /** "tmdb" | "imdb" | "" - populated by the admin handler for clarity */
  provider?: string;
  title: string;
  year?: number;
  overview: string;
  poster_url?: string;
}

interface Props {
  open: boolean;
  onClose: () => void;
  mediaItemId: string;
  /** Pre-fills the search input. Usually the scan-parsed title. */
  initialQuery: string;
  /** "movie" or "tv" - passed to the search endpoint to narrow results. */
  mediaType: "movie" | "tv";
  /** Fires after a successful pick so the parent can refresh its data. */
  onApplied?: () => void;
}

/**
 * Classic "Identify on TMDB" workflow for orphans the parser can't
 * clean (typos in filenames: "Greyhoud", "Snowpiercier", "Oprhan", …).
 * The admin types the real title, picks the right TMDB result from a
 * preview list, and the backend re-enriches the item with that tmdb_id.
 *
 * Wraps two pre-existing admin endpoints:
 *   GET  /api/v1/admin/media/{id}/search-metadata?query=…&type=…
 *   POST /api/v1/admin/media/{id}/refresh-metadata  body {tmdb_id, media_type}
 */
export function IdentifyMetadataModal({ open, onClose, mediaItemId, initialQuery, mediaType, onApplied }: Props) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [query, setQuery] = useState(initialQuery);
  const [results, setResults] = useState<IdentifyResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [applyingId, setApplyingId] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);

  async function handleSearch() {
    const q = query.trim();
    if (q === "" || !mediaItemId) return;
    setSearching(true);
    try {
      const data = await eveyFetch<IdentifyResult[]>({
        url: `/api/v1/admin/media/${mediaItemId}/search-metadata`,
        method: "GET",
        params: { query: q, type: mediaType },
      });
      setResults(data ?? []);
      setSearched(true);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`${t("media.detail.identify.searchFailed", "Search failed")}: ${msg}`);
    } finally {
      setSearching(false);
    }
  }

  async function handleApply(result: IdentifyResult) {
    // Backend accepts either tmdb_id or imdb_id (RefreshItemMetadataByIMDB
    // is the fallback path). Prefer tmdb when both are present - it gives
    // a richer enrichment chain (translations, artwork, etc.).
    const applyId = result.tmdb_id || result.imdb_id;
    if (!applyId || !mediaItemId) return;
    setApplyingId(applyId);
    try {
      const payload: Record<string, string> = { media_type: mediaType };
      if (result.tmdb_id) payload.tmdb_id = result.tmdb_id;
      else if (result.imdb_id) payload.imdb_id = result.imdb_id;
      await eveyFetch<void>({
        url: `/api/v1/admin/media/${mediaItemId}/refresh-metadata`,
        method: "POST",
        data: payload,
      });
      // Invalidate so MediaDetailEdit + parent list refetch the item with
      // its new provider IDs / TMDB-sourced title / overview.
      queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${mediaItemId}`] });
      queryClient.invalidateQueries({ queryKey: getMetadataServiceListMetadataOverridesQueryKey(mediaItemId) });
      toast.success(t("media.detail.identify.appliedTo", "Identified as {{title}}", { title: result.title }));
      onApplied?.();
      onClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`${t("media.detail.identify.applyFailed", "Identify failed")}: ${msg}`);
    } finally {
      setApplyingId(null);
    }
  }

  function handleClose() {
    if (applyingId !== null) return; // don't close mid-apply
    setResults([]);
    setSearched(false);
    setQuery(initialQuery);
    onClose();
  }

  return (
    <Modal open={open} onClose={handleClose} title={t("media.detail.identify.title", "Identify on TMDB")} size="md">
      {/*
       * No <form> here: React replays form-submit events across portals,
       * so a submit inside this nested modal would bubble up to the parent
       * MediaDetailEdit's <form onSubmit={handleSave}> and silently save +
       * close the edit modal. Use a plain div + onClick on the Search
       * button instead, plus Enter-key handling on the Input.
       */}
      <div className="flex flex-col gap-3">
        <div className="flex gap-2 items-end">
          <div className="flex-1">
            <Input
              label={t("media.detail.identify.queryLabel", "Search title")}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  e.stopPropagation();
                  void handleSearch();
                }
              }}
              placeholder={t("media.detail.identify.queryPlaceholder", "e.g. Greyhound")}
              data-testid="identify-query-input"
            />
          </div>
          <Button type="button" loading={searching} disabled={!query.trim()} onClick={() => void handleSearch()} data-testid="identify-search-btn">
            {t("media.detail.identify.search", "Search")}
          </Button>
        </div>

        {searched && results.length === 0 && !searching && (
          <p className="text-sm text-[var(--text-muted)]">
            {t("media.detail.identify.noResults", "No TMDB matches. Try a shorter or differently spelled query.")}
          </p>
        )}

        {results.length > 0 && (
          <ul className="flex flex-col gap-2 max-h-[60vh] overflow-y-auto" data-testid="identify-results">
            {results.map((r, idx) => (
              <li
                key={`${r.tmdb_id ?? "noid"}-${idx}`}
                className="flex gap-3 p-2 rounded border"
                style={{ borderColor: "var(--border)", backgroundColor: "var(--surface-raised)" }}
              >
                {r.poster_url ? (
                  <img
                    src={r.poster_url}
                    alt=""
                    className="w-16 h-24 object-cover rounded flex-shrink-0"
                    style={{ backgroundColor: "var(--surface)" }}
                  />
                ) : (
                  <div
                    className="w-16 h-24 rounded flex-shrink-0 flex items-center justify-center text-xs text-[var(--text-muted)]"
                    style={{ backgroundColor: "var(--surface)" }}
                  >
                    -
                  </div>
                )}
                <div className="flex-1 min-w-0">
                  <div className="flex items-baseline gap-2 flex-wrap">
                    <span className="font-medium">{r.title}</span>
                    {r.year != null && r.year > 0 && (
                      <span className="text-xs text-[var(--text-muted)]">({r.year})</span>
                    )}
                    {r.tmdb_id && (
                      <a
                        href={`https://www.themoviedb.org/${mediaType}/${r.tmdb_id}`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-xs underline text-[var(--accent)]"
                      >
                        TMDB
                      </a>
                    )}
                    {r.imdb_id && (
                      <a
                        href={`https://www.imdb.com/title/${r.imdb_id}/`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-xs underline text-[var(--accent)]"
                      >
                        IMDB
                      </a>
                    )}
                    {r.provider && !r.tmdb_id && !r.imdb_id && (
                      <span className="text-xs italic text-[var(--text-muted)]">{r.provider}</span>
                    )}
                  </div>
                  {r.overview && (
                    <p className="text-xs text-[var(--text-muted)] mt-1 line-clamp-3">{r.overview}</p>
                  )}
                  <div className="mt-2">
                    <Button
                      type="button"
                      onClick={() => handleApply(r)}
                      loading={applyingId === (r.tmdb_id || r.imdb_id)}
                      disabled={(!r.tmdb_id && !r.imdb_id) || applyingId !== null}
                      data-testid={`identify-apply-${r.tmdb_id ?? r.imdb_id ?? "noid"}`}
                    >
                      {t("media.detail.identify.useThis", "Use this match")}
                    </Button>
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}

        <div className="flex justify-end pt-2">
          <Button type="button" variant="secondary" onClick={handleClose} disabled={applyingId !== null}>
            {t("common.close", "Close")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
