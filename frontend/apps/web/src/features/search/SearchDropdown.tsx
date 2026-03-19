import { useEffect, useRef, useCallback } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceSearchMedia } from "@evey/api";
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

export function SearchDropdown({
  query,
  visible,
  onClose,
  onNavigateToSearch,
  inputRef,
  recents = [],
  onRemoveRecent,
  onClearRecents,
}: SearchDropdownProps) {
  const { t } = useTranslation("media");
  const navigate = useNavigate();
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
        className="absolute left-0 right-0 top-full mt-1 rounded-[var(--radius-md)] border border-[var(--border)] shadow-xl z-50 overflow-hidden animate-fade-in"
        style={{ backgroundColor: "var(--surface-raised)" }}
      >
        <div className="flex items-center justify-between px-3 pt-2.5 pb-1">
          <div className="flex items-center gap-1.5 text-xs font-semibold" style={{ color: "var(--text-muted)" }}>
            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
              <circle cx="12" cy="12" r="10" />
              <path strokeLinecap="round" d="M12 6v6l4 2" />
            </svg>
            {t("search.recentSearches")}
          </div>
          {onClearRecents && (
            <button
              onClick={onClearRecents}
              className="text-xs hover:underline cursor-pointer"
              style={{ color: "var(--text-muted)" }}
            >
              {t("search.clearAll")}
            </button>
          )}
        </div>
        <ul className="py-1">
          {recents.map((r) => (
            <li key={r} className="flex items-center gap-1 px-2">
              <button
                data-search-item
                onClick={() => { onNavigateToSearch(r); onClose(); }}
                className="flex-1 flex items-center gap-2 px-2 py-2 text-left hover:bg-[var(--surface)] focus:bg-[var(--surface)] focus:outline-none transition-colors cursor-pointer rounded"
                style={{ color: "var(--text)" }}
              >
                <svg className="w-3.5 h-3.5 shrink-0" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24" style={{ color: "var(--text-muted)" }}>
                  <circle cx="12" cy="12" r="10" />
                  <path strokeLinecap="round" d="M12 6v6l4 2" />
                </svg>
                <span className="text-sm truncate">{r}</span>
              </button>
              {onRemoveRecent && (
                <button
                  onClick={() => onRemoveRecent(r)}
                  className="p-1 rounded hover:bg-[var(--surface)] transition-colors cursor-pointer"
                  style={{ color: "var(--text-muted)" }}
                  aria-label={`Remove ${r}`}
                >
                  <svg className="w-3 h-3" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                    <path strokeLinecap="round" d="M6 18L18 6M6 6l12 12" />
                  </svg>
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
      className="absolute left-0 right-0 top-full mt-1 rounded-[var(--radius-md)] border border-[var(--border)] shadow-xl z-50 overflow-hidden animate-fade-in"
      style={{ backgroundColor: "var(--surface-raised)" }}
    >
      {isLoading && (
        <div className="px-4 py-3 flex items-center gap-2">
          <div
            className="w-4 h-4 border-2 border-t-transparent rounded-full animate-spin"
            style={{ borderColor: "var(--accent)", borderTopColor: "transparent" }}
          />
          <span className="text-sm" style={{ color: "var(--text-muted)" }}>
            {t("search.searching")}
          </span>
        </div>
      )}

      {!isLoading && isError && (
        <div className="px-4 py-3" data-testid="search-dropdown-error">
          <span className="text-sm" style={{ color: "var(--color-danger)" }}>
            {t("search.searchFailed")}
          </span>
        </div>
      )}

      {!isLoading && !isError && items.length === 0 && (
        <div className="px-4 py-3" data-testid="search-dropdown-empty">
          <span className="text-sm" style={{ color: "var(--text-muted)" }}>
            {t("search.noResults", { query: trimmed })}
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
                  className="w-full flex items-center gap-3 px-3 py-2 text-left hover:bg-[var(--surface)] focus:bg-[var(--surface)] focus:outline-none transition-colors cursor-pointer"
                >
                  <div
                    className="w-9 h-13 rounded overflow-hidden shrink-0 flex items-center justify-center"
                    style={{
                      aspectRatio: "2/3",
                      backgroundColor: "var(--surface)",
                    }}
                  >
                    {item.id ? (
                      <img
                        src={`/api/v1/media/${item.id}/artwork/poster`}
                        alt=""
                        className="w-full h-full object-cover"
                        onError={(e) => {
                          e.currentTarget.style.display = "none";
                        }}
                      />
                    ) : null}
                    <span
                      className="text-xs font-bold"
                      style={{ color: "var(--text-muted)" }}
                    >
                      {item.title?.[0]?.toUpperCase() ?? "?"}
                    </span>
                  </div>
                  <div className="flex-1 min-w-0">
                    <p
                      className="text-sm font-medium truncate"
                      style={{ color: "var(--text)" }}
                    >
                      {item.title}
                    </p>
                    <div
                      className="flex items-center gap-2 text-xs"
                      style={{ color: "var(--text-muted)" }}
                    >
                      {item.year && <span>{item.year}</span>}
                      {typeLabel(item.type) && (
                        <span
                          className="px-1.5 py-0.5 rounded"
                          style={{
                            backgroundColor: "var(--surface)",
                            fontSize: "10px",
                          }}
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
              className="w-full px-3 py-2.5 text-sm font-medium border-t border-[var(--border)] hover:bg-[var(--surface)] focus:bg-[var(--surface)] focus:outline-none transition-colors cursor-pointer text-center"
              style={{ color: "var(--accent)" }}
            >
              {t("search.viewAllResults", { count: totalHits })}
            </button>
          )}
        </>
      )}
    </div>
  );
}
