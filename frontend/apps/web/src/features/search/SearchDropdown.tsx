import { memo, useEffect, useRef, useCallback } from "react";
import { Clock, X } from "lucide-react";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceSearchMedia } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { typeLabel } from "@/utils/typeLabel";

interface SearchDropdownProps {
  query: string;
  visible: boolean;
  onClose: () => void;
  onNavigateToSearch: (q: string) => void;
  inputRef: React.RefObject<HTMLInputElement | null>;
  recents?: string[];
  onRemoveRecent?: (q: string) => void;
  onClearRecents?: () => void;
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
}: SearchDropdownProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const accessToken = useAuthStore((s) => s.accessToken) ?? "";
  const dropdownRef = useRef<HTMLDivElement>(null);
  const activeIndex = useRef(-1);

  const trimmed = query.trim();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["/api/v1/media/search/quick", trimmed],
    queryFn: ({ signal }) =>
      libraryServiceSearchMedia({ query: trimmed, limit: 6 }, signal),
    enabled: visible && trimmed.length > 1,
    staleTime: 30_000,
  });

  const items = data?.items ?? [];
  const totalHits = data?.totalHits ?? 0;

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
            <li key={r} className="flex items-center gap-1 px-2">
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
                  aria-label={`Remove ${r}`}
                >
                  <X size={12} />
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
      {isLoading && (
        <div className="px-4 py-3 flex items-center gap-2">
          <div
            className="w-4 h-4 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin"
          />
          <span className="text-sm text-[var(--text-muted)]">
            {t("media.search.searching")}
          </span>
        </div>
      )}

      {!isLoading && isError && (
        <div className="px-4 py-3" data-testid="search-dropdown-error">
          <span className="text-sm text-[var(--color-danger)]">
            {t("media.search.searchFailed")}
          </span>
        </div>
      )}

      {!isLoading && !isError && items.length === 0 && (
        <div className="px-4 py-3" data-testid="search-dropdown-empty">
          <span className="text-sm text-[var(--text-muted)]">
            {t("media.search.noResults", { query: trimmed })}
          </span>
        </div>
      )}

      {!isLoading && items.length > 0 && (
        <>
          <ul className="py-1">
            {items.map((item) => (
              <li key={item.id}>
                <button
                  data-search-item
                  onClick={() => goToItem(item.id ?? "")}
                  className="w-full flex items-center gap-3 px-3 py-2 text-left hover:bg-[var(--surface)] focus:bg-[var(--surface)] focus:outline-none transition-colors-fast cursor-pointer"
                >
                  <div
                    className="w-9 h-13 rounded overflow-hidden shrink-0 flex items-center justify-center aspect-[2/3] bg-[var(--surface)]"
                  >
                    {item.id ? (
                      <img
                        src={`/api/v1/media/${item.id}/artwork/poster?token=${encodeURIComponent(accessToken)}`}
                        alt={item.title || "Media poster"}
                        className="w-full h-full object-cover"
                        onError={(e) => {
                          e.currentTarget.style.display = "none";
                        }}
                      />
                    ) : null}
                    <span
                      className="text-xs font-bold text-[var(--text-muted)]"
                    >
                      {item.title?.[0]?.toUpperCase() ?? "?"}
                    </span>
                  </div>
                  <div className="flex-1 min-w-0">
                    <p
                      className="text-sm font-medium truncate text-[var(--text)]"
                    >
                      {item.title}
                    </p>
                    <div
                      className="flex items-center gap-2 text-xs text-[var(--text-muted)]"
                    >
                      {item.year && <span>{item.year}</span>}
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
    </div>
  );
});
