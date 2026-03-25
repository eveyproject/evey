import { X } from "lucide-react";
import { Button } from "@/components/ui/Button";

export interface BulkActionBarProps {
  count: number;
  onClear: () => void;
  onMarkWatched: () => void;
  onMarkUnwatched: () => void;
  onAddToPlaylist: () => void;
  onAddToCollection: () => void;
  markWatchedPending?: boolean;
  markUnwatchedPending?: boolean;
}

export function BulkActionBar({
  count,
  onClear,
  onMarkWatched,
  onMarkUnwatched,
  onAddToPlaylist,
  onAddToCollection,
  markWatchedPending,
  markUnwatchedPending,
}: BulkActionBarProps) {
  return (
    <div
      className="fixed bottom-0 left-0 right-0 z-overlay flex items-center gap-2 px-4 py-3 flex-wrap shadow-2xl"
      style={{
        backgroundColor: "var(--surface)",
        borderTop: "1px solid var(--accent)",
        transform: count > 0 ? "translateY(0)" : "translateY(100%)",
        transition: "transform 0.25s ease",
      }}
      aria-live="polite"
    >
      <span
        className="text-sm font-semibold px-2 py-0.5 rounded-full"
        style={{ backgroundColor: "var(--accent)", color: "var(--filter-active-text)" }}
      >
        {count}
      </span>
      <span className="text-sm mr-1" style={{ color: "var(--text-muted)" }}>selected</span>

      <Button variant="secondary" onClick={onMarkWatched} loading={markWatchedPending} disabled={count === 0}>
        Mark watched
      </Button>
      <Button variant="secondary" onClick={onMarkUnwatched} loading={markUnwatchedPending} disabled={count === 0}>
        Mark unwatched
      </Button>
      <Button variant="secondary" onClick={onAddToPlaylist} disabled={count === 0}>
        Add to playlist
      </Button>
      <Button variant="secondary" onClick={onAddToCollection} disabled={count === 0}>
        Add to collection
      </Button>

      <button
        type="button"
        onClick={onClear}
        className="ml-auto flex items-center justify-center w-7 h-7 rounded-full transition-colors hover:bg-[var(--surface-raised)]"
        style={{ color: "var(--text-muted)" }}
        aria-label="Clear selection"
      >
        <X size={16} />
      </button>
    </div>
  );
}
