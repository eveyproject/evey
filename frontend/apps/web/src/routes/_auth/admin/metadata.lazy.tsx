import { useState, useEffect, useCallback } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQueryClient, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceListMediaItems,
  libraryServiceGetMediaItem,
  eveyFetch,
} from "@evey/api";
import { toast } from "@evey/store";
import type { V1MediaItem } from "@evey/api/src/generated/library.schemas";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
import { Textarea } from "@/components/ui/Textarea";
import { EmptyState } from "@/components/ui/EmptyState";

export const Route = createLazyFileRoute("/_auth/admin/metadata")({
  component: MetadataPage,
});

interface TmdbResult {
  tmdbId: string;
  title: string;
  year: number;
  overview: string;
  posterUrl: string;
}

function mediaTypeForTmdb(item: V1MediaItem): string {
  if (item.type === "MEDIA_ITEM_TYPE_SERIES") return "tv";
  return "movie";
}

function posterSrc(id: string) {
  return `/api/v1/media/${id}/artwork/poster`;
}

function TmdbSearchPanel({
  item,
  onApplied,
}: {
  item: V1MediaItem;
  onApplied: () => void;
}) {
  const { t } = useTranslation("admin");
  const [results, setResults] = useState<TmdbResult[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [applying, setApplying] = useState<string | null>(null);
  const queryClient = useQueryClient();

  const search = useCallback(async () => {
    if (!item.id) return;
    setLoading(true);
    setError(null);
    try {
      const data = await eveyFetch<TmdbResult[]>({
        url: `/api/v1/admin/media/${item.id}/search-metadata`,
        method: "GET",
        params: { query: item.title ?? "", type: mediaTypeForTmdb(item) },
      });
      setResults(data ?? []);
    } catch (e) {
      setError((e as Error).message ?? "Search failed");
      setResults([]);
    } finally {
      setLoading(false);
    }
  }, [item]);

  async function apply(result: TmdbResult) {
    if (!item.id) return;
    setApplying(result.tmdbId);
    try {
      await eveyFetch({
        url: `/api/v1/admin/media/${item.id}/refresh-metadata`,
        method: "POST",
        headers: { "Content-Type": "application/json" },
        data: { tmdb_id: result.tmdbId, media_type: mediaTypeForTmdb(item) },
      });
      await queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${item.id}`] });
      onApplied();
    } catch (e) {
      setError((e as Error).message ?? "Apply failed");
    } finally {
      setApplying(null);
    }
  }

  return (
    <div className="flex flex-col gap-3 mt-4">
      <Button variant="secondary" onClick={search} loading={loading}>
        {t("metadata.searchTmdb")}
      </Button>
      {error && <p className="text-sm" style={{ color: "var(--color-danger)" }}>{error}</p>}
      {results !== null && results.length === 0 && !loading && (
        <p className="text-sm" style={{ color: "var(--text-muted)" }}>
          {t("metadata.noTmdbResults")}
        </p>
      )}
      {results && results.length > 0 && (
        <div className="flex flex-col gap-2 max-h-64 overflow-y-auto">
          {results.map((r) => (
            <div
              key={r.tmdbId}
              className="flex items-start gap-3 p-3 rounded-[var(--radius-md)]"
              style={{ backgroundColor: "var(--surface-raised)", border: "1px solid var(--border)" }}
            >
              {r.posterUrl && (
                <img
                  src={r.posterUrl}
                  alt={r.title}
                  className="w-12 h-16 object-cover rounded flex-shrink-0"
                  onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }}
                />
              )}
              <div className="flex-1 min-w-0">
                <p className="font-medium text-sm" style={{ color: "var(--text)" }}>
                  {r.title} {r.year ? `(${r.year})` : ""}
                </p>
                <p className="text-xs mt-1 line-clamp-2" style={{ color: "var(--text-muted)" }}>
                  {r.overview}
                </p>
              </div>
              <Button
                variant="primary"
                loading={applying === r.tmdbId}
                onClick={() => apply(r)}
                className="flex-shrink-0"
              >
                {t("common:apply")}
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function GenrePills({
  genres,
  onChange,
}: {
  genres: string[];
  onChange: (genres: string[]) => void;
}) {
  const { t } = useTranslation("admin");
  const [input, setInput] = useState("");

  function add() {
    const trimmed = input.trim();
    if (trimmed && !genres.includes(trimmed)) {
      onChange([...genres, trimmed]);
    }
    setInput("");
  }

  function remove(g: string) {
    onChange(genres.filter((x) => x !== g));
  }

  return (
    <div className="flex flex-col gap-2">
      <label className="text-sm font-medium" style={{ color: "var(--text-muted)" }}>
        {t("metadata.genres")}
      </label>
      <div className="flex flex-wrap gap-1 min-h-8">
        {genres.map((g) => (
          <span
            key={g}
            className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs"
            style={{ backgroundColor: "var(--surface-raised)", color: "var(--text)" }}
          >
            {g}
            <button
              onClick={() => remove(g)}
              className="transition-colors hover:opacity-80"
              style={{ color: "var(--color-danger)" }}
              aria-label={`Remove ${g}`}
            >
              \u00D7
            </button>
          </span>
        ))}
      </div>
      <div className="flex gap-2">
        <input
          className="flex-1 px-3 py-1.5 rounded-[var(--radius-md)] text-sm bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
          placeholder={t("metadata.addGenre")}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); add(); } }}
        />
        <Button variant="secondary" onClick={add} disabled={!input.trim()}>
          {t("common:add")}
        </Button>
      </div>
    </div>
  );
}

function MetadataEditorModal({
  itemId,
  open,
  onClose,
}: {
  itemId: string;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation("admin");
  const { data: item, isLoading } = useQuery({
    queryKey: [`/api/v1/media/${itemId}`],
    queryFn: ({ signal }) => libraryServiceGetMediaItem(itemId, signal),
    enabled: open && !!itemId,
  });

  const [title, setTitle] = useState("");
  const [overview, setOverview] = useState("");
  const [year, setYear] = useState<string>("");
  const [genres, setGenres] = useState<string[]>([]);
  const [showTmdb, setShowTmdb] = useState(false);

  useEffect(() => {
    if (item) {
      setTitle(item.title ?? "");
      setOverview(item.overview ?? "");
      setYear(item.year != null ? String(item.year) : "");
      setGenres(item.genres ?? []);
      setShowTmdb(false);
    }
  }, [item]);

  if (!open) return null;

  const poster = `/api/v1/media/${itemId}/artwork/poster`;
  const backdrop = `/api/v1/media/${itemId}/artwork/backdrop`;

  return (
    <Modal open={open} onClose={onClose} title={item?.title ?? t("metadata.editMetadata")}>
      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner />
        </div>
      ) : item ? (
        <div className="flex flex-col gap-4 max-h-[80vh] overflow-y-auto pr-1">
          <div className="flex gap-3">
            <img
              src={poster}
              alt="Poster"
              className="w-20 h-28 object-cover rounded-[var(--radius-md)] flex-shrink-0"
              onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }}
            />
            <img
              src={backdrop}
              alt="Backdrop"
              className="flex-1 h-28 object-cover rounded-[var(--radius-md)]"
              onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }}
            />
          </div>

          <Input
            label={t("metadata.titleLabel")}
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />

          <Textarea
            label={t("metadata.overview")}
            value={overview}
            onChange={(e) => setOverview(e.target.value)}
            placeholder={t("metadata.overviewPlaceholder")}
          />

          <Input
            label={t("metadata.year")}
            type="number"
            value={year}
            onChange={(e) => setYear(e.target.value)}
          />

          <GenrePills genres={genres} onChange={setGenres} />

          <div>
            <Button
              variant="secondary"
              onClick={() => setShowTmdb((v) => !v)}
            >
              {showTmdb ? t("metadata.hideTmdbSearch") : t("metadata.searchTmdb")}
            </Button>
            {showTmdb && (
              <TmdbSearchPanel
                item={{ ...item, title }}
                onApplied={() => {
                  setShowTmdb(false);
                  onClose();
                }}
              />
            )}
          </div>
        </div>
      ) : (
        <p style={{ color: "var(--text-muted)" }}>{t("metadata.failedLoad")}</p>
      )}
    </Modal>
  );
}

function MediaThumb({ item, onClick }: { item: V1MediaItem; onClick: () => void }) {
  return (
    <button onClick={onClick} className="flex flex-col gap-2 text-left group">
      <div
        className="w-full aspect-[2/3] rounded-[var(--radius-md)] overflow-hidden"
        style={{ backgroundColor: "var(--surface-raised)" }}
      >
        {item.id && (
          <img
            src={posterSrc(item.id)}
            alt={item.title}
            className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-200"
            onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }}
          />
        )}
      </div>
      <div>
        <p className="text-sm font-medium truncate" style={{ color: "var(--text)" }}>
          {item.title ?? "Untitled"}
        </p>
        <p className="text-xs" style={{ color: "var(--text-muted)" }}>
          {item.type?.replace("MEDIA_ITEM_TYPE_", "") ?? ""}
          {item.year ? ` \u00B7 ${item.year}` : ""}
        </p>
      </div>
    </button>
  );
}

function useRefreshAll() {
  const { t } = useTranslation("admin");
  const [loading, setLoading] = useState(false);
  async function refreshAll() {
    setLoading(true);
    try {
      await eveyFetch({ url: "/api/v1/admin/metadata/refresh-all", method: "POST" });
      toast.success(t("metadata.refreshStarted"));
    } catch {
      toast.error(t("metadata.failedRefresh"));
    } finally {
      setLoading(false);
    }
  }
  return { refreshAll, loading };
}

function MetadataPage() {
  const { t } = useTranslation("admin");
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const { refreshAll, loading: refreshing } = useRefreshAll();

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedQuery(query), 400);
    return () => clearTimeout(timer);
  }, [query]);

  const { data, isLoading } = useLibraryServiceListMediaItems({
    "pagination.pageSize": 96,
  });

  const items = data?.items ?? [];

  const filtered = debouncedQuery
    ? items.filter((i) => i.title?.toLowerCase().includes(debouncedQuery.toLowerCase()))
    : items;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>{t("metadata.title")}</h1>
        <Button variant="secondary" loading={refreshing} onClick={refreshAll}>
          {t("metadata.refreshAll")}
        </Button>
      </div>

      <input
        className="w-full px-4 py-2.5 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
        placeholder={t("metadata.searchPlaceholder")}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={debouncedQuery ? "search" : "media"}
          title={debouncedQuery ? t("metadata.noResults") : t("metadata.noMedia")}
          description={debouncedQuery ? t("metadata.tryDifferent") : undefined}
        />
      ) : (
        <div className="grid grid-cols-2 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-8 gap-4">
          {filtered.map((item) => (
            <MediaThumb
              key={item.id}
              item={item}
              onClick={() => setSelectedId(item.id ?? null)}
            />
          ))}
        </div>
      )}

      {selectedId && (
        <MetadataEditorModal
          itemId={selectedId}
          open
          onClose={() => setSelectedId(null)}
        />
      )}
    </div>
  );
}
