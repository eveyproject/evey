import { memo, useEffect, useRef, useCallback } from "react";
import { Clock, Music, X } from "lucide-react";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceSearchMedia, musicServiceSearchMusic, displayTitle } from "@evey/api";
import { typeLabel } from "@/utils/typeLabel";
import { PosterPlaceholder } from "@/features/library/PosterPlaceholder";
import { coverArtSrc } from "../../lib/coverArtSrc";

type SearchKind = "media" | "music";

interface SearchDropdownProps {
  query: string;
  visible: boolean;
  onClose: () => void;
  onNavigateToSearch: (q: string) => void;
  inputRef: React.RefObject<HTMLInputElement | null>;
  recents?: string[];
  onRemoveRecent?: (q: string) => void;
  onClearRecents?: () => void;
  kind?: SearchKind;
  onKindChange?: (k: SearchKind) => void;
}

export const SearchDropdown = memo(function SearchDropdown({
  query,
  visible,
  onClose,
  onNavigateToSearch,
  inputRef,
  recents = [],
  onRemoveRecent,
  onClearRecents,
  kind = "media",
  onKindChange,
}: SearchDropdownProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const dropdownRef = useRef<HTMLDivElement>(null);
  const activeIndex = useRef(-1);

  const trimmed = query.trim();

  // Only the active scope queries - the user picks video vs music up front.
  const { data, isLoading, isError } = useQuery({
    queryKey: ["/api/v1/media/search/quick", trimmed],
    queryFn: ({ signal }) =>
      libraryServiceSearchMedia({ query: trimmed, limit: 6 }, signal),
    enabled: visible && trimmed.length > 1 && kind === "media",
    staleTime: 30_000,
  });

  const items = data?.items ?? [];
  const totalHits = data?.totalHits ?? 0;

  // Music lives in a separate index; searched cross-library (libraryId omitted
  // → all libraries the user may access).
  const { data: musicData, isLoading: musicLoading, isError: musicError } = useQuery({
    queryKey: ["/api/v1/music/search/quick", trimmed],
    queryFn: ({ signal }) =>
      musicServiceSearchMusic({ query: trimmed, limit: 6 }, signal),
    enabled: visible && trimmed.length > 1 && kind === "music",
    staleTime: 30_000,
  });
  const musicItems = musicData?.tracks ?? [];

  const activeLoading = kind === "media" ? isLoading : musicLoading;
  const activeCount = kind === "media" ? items.length : musicItems.length;

  useEffect(() => {
    if (!visible) return;
    const handler = (e: MouseEvent) => {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node) &&
        inputRef.current &&
        !inputRef.current.contains(e.target as Node)
      ) {
        onClose();
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [visible, onClose, inputRef]);

  const goToItem = useCallback(
    (id: string) => {
      onClose();
      void navigate({ to: "/media/$mediaId", params: { mediaId: id } });
    },
    [navigate, onClose],
  );

  const goToTrack = useCallback(
    (albumId: string, libraryId: string) => {
      onClose();
      void navigate({
        to: "/music",
        search: { tab: "tracks", album: albumId || undefined, libraryId: libraryId || undefined },
      });
    },
    [navigate, onClose],
  );

  useEffect(() => {
    if (!visible) {
      activeIndex.current = -1;
      return;
    }
    const handler = (e: KeyboardEvent) => {
      const list = dropdownRef.current?.querySelectorAll("[data-search-item]");
      if (!list?.length) return;
      const count = list.length;

      if (e.key === "ArrowDown") {
        e.preventDefault();
        activeIndex.current = Math.min(activeIndex.current + 1, count - 1);
        (list[activeIndex.current] as HTMLElement).focus();
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        activeIndex.current = Math.max(activeIndex.current - 1, 0);
        (list[activeIndex.current] as HTMLElement).focus();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [visible]);

  const showRecents = visible && trimmed.length === 0 && recents.length > 0;

  if (!visible || (trimmed.length <= 1 && !showRecents)) return null;

  if (showRecents) {
    return (
      <div
        ref={dropdownRef}
        id="search-results"
        role="listbox"
        data-testid="search-dropdown-recents"
        className="absolute left-0 right-0 top-full mt-1 rounded-[var(--radius-md)] border border-[var(--border)] shadow-xl z-overlay overflow-hidden animate-fade-in bg-[var(--surface-raised)]"
      >
        <div className="flex items-center justify-between px-3 pt-2.5 pb-1">
          <div className="flex items-center gap-1.5 text-xs font-semibold text-[var(--text-muted)]">
            <Clock size={14} />
            {t("media.search.recentSearches")}
          </div>
          {onClearRecents && (
            <button
              onClick={onClearRecents}
              className="text-xs hover:underline cursor-pointer text-[var(--text-muted)]"
            >
              {t("media.search.clearAll")}
            </button>
          )}
        </div>
        <ul className="py-1">
          {recents.map((r) => (
            <li key={r} role="option" className="flex items-center gap-1 px-2">
              <button
                data-search-item
                onClick={() => { onNavigateToSearch(r); onClose(); }}
                className="flex-1 flex items-center gap-2 px-2 py-2 text-left hover:bg-[var(--surface)] focus:bg-[var(--surface)] focus:outline-none transition-colors-fast cursor-pointer rounded text-[var(--text)]"
              >
                <Clock size={14} className="shrink-0 text-[var(--text-muted)]" />
                <span className="text-sm truncate">{r}</span>
              </button>
              {onRemoveRecent && (
                <button
                  onClick={() => onRemoveRecent(r)}
                  className="p-1 rounded hover:bg-[var(--surface)] transition-colors-fast cursor-pointer text-[var(--text-muted)]"
                  aria-label={t("common.removeItem", { item: r })}
                >
                  <X size={12}  aria-hidden="true" />
                </button>
              )}
            </li>
          ))}
        </ul>
      </div>
    );
  }

  return (
    <div
      ref={dropdownRef}
      id="search-results"
      role="listbox"
      data-testid="search-dropdown"
      className="absolute left-0 right-0 top-full mt-1 rounded-[var(--radius-md)] border border-[var(--border)] shadow-xl z-overlay overflow-hidden animate-fade-in bg-[var(--surface-raised)]"
    >
      <div role="status" aria-live="polite" className="sr-only">
        {!isLoading && !isError && items.length > 0
          ? t("media.search.resultCount", { count: items.length })
          : !isLoading && !isError && items.length === 0 && trimmed.length > 1
            ? t("media.search.noResults")
            : null}
      </div>

      {/* Scope selector: video catalog (default) or music. */}
      <div className="flex gap-1 px-2 pt-2 pb-1 border-b border-[var(--border)]" role="tablist" aria-label={t("common.search")}>
        {(["media", "music"] as const).map((k) => (
          <button
            key={k}
            role="tab"
            aria-selected={kind === k}
            onClick={() => onKindChange?.(k)}
            className={`px-2.5 py-1 rounded-[var(--radius-sm)] text-xs font-medium cursor-pointer transition-colors-fast ${
              kind === k
                ? "bg-[var(--accent)] text-[var(--filter-active-text)]"
                : "bg-transparent text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            {k === "media" ? t("media.search.sectionMedia") : t("media.search.sectionMusic")}
          </button>
        ))}
      </div>

      {activeLoading && (
        <div className="px-4 py-3 flex items-center gap-2">
          <div
            className="w-4 h-4 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin"
          />
          <span className="text-sm text-[var(--text-muted)]">
            {t("media.search.searching")}
          </span>
        </div>
      )}

      {kind === "media" && !isLoading && isError && (
        <div className="px-4 py-3" data-testid="search-dropdown-error">
          <span className="text-sm text-[var(--color-danger)]">
            {t("media.search.searchFailed")}
          </span>
        </div>
      )}

      {kind === "music" && !musicLoading && musicError && (
        <div className="px-4 py-3" data-testid="search-dropdown-music-error">
          <span className="text-sm text-[var(--color-danger)]">
            {t("common.error")}
          </span>
        </div>
      )}

      {!activeLoading && !(kind === "media" && isError) && !(kind === "music" && musicError) && activeCount === 0 && (
        <div className="px-4 py-3" data-testid="search-dropdown-empty">
          <span className="text-sm text-[var(--text-muted)]">
            {t("media.search.noResults", { query: trimmed })}
          </span>
        </div>
      )}

      {kind === "media" && items.length > 0 && (
        <>
          <ul className="py-1">
            {items.map((item) => (
              <li key={item.id} role="option">
                <button
                  data-search-item
                  onClick={() => goToItem(item.id ?? "")}
                  className="w-full flex items-center gap-3 px-3 py-2 text-left hover:bg-[var(--surface)] focus:bg-[var(--surface)] focus:outline-none transition-colors-fast cursor-pointer"
                >
                  <div
                    className="relative w-9 h-13 rounded overflow-hidden shrink-0 aspect-[2/3]"
                  >
                    <PosterPlaceholder
                      title={item.title}
                      type={item.type}
                      iconSize={16}
                      showTitle={false}
                      className="absolute inset-0"
                    />
                    {item.id ? (
                      <img
                        src={`/api/v1/media/${item.id}/artwork/poster`}
                        srcSet={`/api/v1/media/${item.id}/artwork/poster?w=50 50w, /api/v1/media/${item.id}/artwork/poster?w=100 100w`}
                        sizes="36px"
                        alt={displayTitle(item, t) || "Media poster"}
                        className="absolute inset-0 w-full h-full object-cover"
                        onError={(e) => {
                          e.currentTarget.style.display = "none";
                        }}
                      />
                    ) : null}
                  </div>
                  <div className="flex-1 min-w-0">
                    <p
                      className="text-sm font-medium truncate text-[var(--text)]"
                    >
                      {displayTitle(item, t)}
                    </p>
                    <div
                      className="flex items-center gap-2 text-xs text-[var(--text-muted)]"
                    >
                      {!!item.year && <span>{item.year}</span>}
                      {typeLabel(item.type) && (
                        <span
                          className="px-1.5 py-0.5 rounded bg-[var(--surface)] text-xs"
                        >
                          {typeLabel(item.type)}
                        </span>
                      )}
                      {item.genres && item.genres.length > 0 && (
                        <span className="truncate">
                          {item.genres.slice(0, 2).join(", ")}
                        </span>
                      )}
                    </div>
                  </div>
                </button>
              </li>
            ))}
          </ul>

          {totalHits > items.length && (
            <button
              data-search-item
              onClick={() => {
                onClose();
                onNavigateToSearch(trimmed);
              }}
              className="w-full px-3 py-2.5 text-sm font-medium border-t border-[var(--border)] hover:bg-[var(--surface)] focus:bg-[var(--surface)] focus:outline-none transition-colors-fast cursor-pointer text-center text-[var(--accent)]"
            >
              {t("media.search.viewAllResults", { count: totalHits })}
            </button>
          )}
        </>
      )}

      {kind === "music" && musicItems.length > 0 && (
        <div data-testid="search-dropdown-music">
          <ul className="py-1">
            {musicItems.map((track) => (
              <li key={track.id} role="option">
                <button
                  data-search-item
                  onClick={() => goToTrack(track.albumId ?? "", track.libraryId ?? "")}
                  className="w-full flex items-center gap-3 px-3 py-2 text-left hover:bg-[var(--surface)] focus:bg-[var(--surface)] focus:outline-none transition-colors-fast cursor-pointer"
                >
                  <div className="w-9 h-9 rounded overflow-hidden shrink-0 flex items-center justify-center bg-[var(--surface)]">
                    {track.coverArtUrl ? (
                      <img
                        src={coverArtSrc(track.coverArtUrl, "", 72)}
                        alt=""
                        aria-hidden="true"
                        className="w-full h-full object-cover"
                        onError={(e) => { e.currentTarget.style.display = "none"; }}
                      />
                    ) : (
                      <Music size={16} className="text-[var(--text-muted)]" aria-hidden="true" />
                    )}
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium truncate text-[var(--text)]">{track.title}</p>
                    <div className="flex items-center gap-2 text-xs text-[var(--text-muted)] truncate">
                      {track.artist && <span className="truncate">{track.artist}</span>}
                      {track.album && <span className="truncate opacity-70">· {track.album}</span>}
                    </div>
                  </div>
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
});
