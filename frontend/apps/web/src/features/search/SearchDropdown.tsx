import { useEffect, useRef, useCallback } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceSearchMedia } from "@evey/api";

const typeLabel = (t?: string) => {
  if (t === "MEDIA_ITEM_TYPE_MOVIE") return "Film";
  if (t === "MEDIA_ITEM_TYPE_SERIES") return "Serie";
  return "";
};

interface SearchDropdownProps {
  query: string;
  visible: boolean;
  onClose: () => void;
  onNavigateToSearch: (q: string) => void;
  inputRef: React.RefObject<HTMLInputElement | null>;
}

export function SearchDropdown({
  query,
  visible,
  onClose,
  onNavigateToSearch,
  inputRef,
}: SearchDropdownProps) {
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

  // Close on outside click
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

  // Keyboard nav inside dropdown
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

  if (!visible || trimmed.length <= 1) return null;

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
            Searching...
          </span>
        </div>
      )}

      {!isLoading && isError && (
        <div className="px-4 py-3" data-testid="search-dropdown-error">
          <span className="text-sm" style={{ color: "#ef4444" }}>
            Search failed
          </span>
        </div>
      )}

      {!isLoading && !isError && items.length === 0 && (
        <div className="px-4 py-3" data-testid="search-dropdown-empty">
          <span className="text-sm" style={{ color: "var(--text-muted)" }}>
            No results for &ldquo;{trimmed}&rdquo;
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
              View all {totalHits} results
            </button>
          )}
        </>
      )}
    </div>
  );
}
