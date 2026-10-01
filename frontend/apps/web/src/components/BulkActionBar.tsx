import { X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { useDemoLock } from "@/hooks/useDemoMode";

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
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  // AddCollectionItem is RoleAdmin server-side. The decision lives here rather
  // than at each call site so the library and the favorites bar can never
  // disagree about it.
  const isAdmin = useAuthStore((s) => s.isAdmin);
  return (
    <div
      className={`fixed bottom-0 left-0 right-0 z-overlay flex items-center gap-2 px-4 py-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] flex-wrap shadow-2xl bg-[var(--surface)] border-t border-[var(--accent)] transition-transform duration-250 ease-out ${count > 0 ? "translate-y-0" : "translate-y-full"}`}
      aria-live="polite"
    >
      <span
        className="text-sm font-semibold px-2 py-0.5 rounded-full bg-[var(--accent)] text-[var(--filter-active-text)]"
      >
        {count}
      </span>
      <span className="text-sm mr-1 text-[var(--text-muted)]">{t("media.library.selected", "selected")}</span>

      <Button {...lock} variant="secondary" onClick={onMarkWatched} loading={markWatchedPending} disabled={!!lock.disabled || (count === 0)}>
        {t("media.library.markWatched", "Mark watched")}
      </Button>
      <Button {...lock} variant="secondary" onClick={onMarkUnwatched} loading={markUnwatchedPending} disabled={!!lock.disabled || (count === 0)}>
        {t("media.library.markUnwatched", "Mark unwatched")}
      </Button>
      <Button {...lock} variant="secondary" onClick={onAddToPlaylist} disabled={!!lock.disabled || (count === 0)}>
        {t("media.library.addToPlaylist", "Add to playlist")}
      </Button>
      {isAdmin && (
        <Button {...lock} variant="secondary" onClick={onAddToCollection} disabled={!!lock.disabled || (count === 0)} data-testid="bulk-add-to-collection">
          {t("media.library.addToCollection", "Add to collection")}
        </Button>
      )}

      <button
        type="button"
        onClick={onClear}
        className="ml-auto flex items-center justify-center w-7 h-7 rounded-full transition-colors-fast hover:bg-[var(--surface-raised)] text-[var(--text-muted)]"
        aria-label={t("common.clearSelection")}
      >
        <X size={16} />
      </button>
    </div>
  );
}
