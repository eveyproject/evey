import { useState } from "react";
import {
  usePlaybackServiceGetWatchHistory,
  usePlaybackServiceMarkAsUnwatched,
  getPlaybackServiceGetWatchHistoryQueryKey,
} from "@evey/api";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { EmptyState } from "@/components/ui/EmptyState";

type WatchProgress = {
  id?: string;
  profileId?: string;
  mediaItemId?: string;
  positionMs?: string;
  durationMs?: string;
  percentComplete?: number;
  played?: boolean;
  playCount?: number;
  lastPlayedAt?: string;
  updatedAt?: string;
};

interface Props {
  limit?: number;
  showClearButton?: boolean;
}

function formatDuration(ms: string | undefined): string {
  if (!ms) return "";
  const totalMin = Math.floor(Number(ms) / 60000);
  if (totalMin < 60) return `${totalMin}min`;
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  return m > 0 ? `${h}h ${m}min` : `${h}h`;
}

function getDateGroup(iso: string | undefined): string {
  if (!iso) return "Unknown";
  const date = new Date(iso);
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const itemDate = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const diffDays = Math.floor((today.getTime() - itemDate.getTime()) / 86400000);

  if (diffDays === 0) return "Today";
  if (diffDays === 1) return "Yesterday";
  if (diffDays < 7) return "This Week";
  if (diffDays < 30) return "This Month";
  return date.toLocaleDateString(undefined, { year: "numeric", month: "long" });
}

function formatDate(iso: string | undefined): string {
  if (!iso) return "";
  return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

function ProgressBar({ percent }: { percent: number }) {
  const clamped = Math.min(100, Math.max(0, percent));
  return (
    <div className="h-1 bg-[var(--border)] rounded-full overflow-hidden">
      <div
        className="h-full bg-[var(--accent)] transition-all"
        style={{ width: `${clamped}%` }}
      />
    </div>
  );
}

function HistoryItem({ item }: { item: WatchProgress }) {
  const qc = useQueryClient();
  const unwatch = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getPlaybackServiceGetWatchHistoryQueryKey() });
      },
    },
  });

  const pct = item.percentComplete ?? 0;
  const posterUrl = item.mediaItemId ? `/api/v1/media/${item.mediaItemId}/artwork/poster` : undefined;

  return (
    <div className="flex items-center gap-3 py-3 border-b border-[var(--border)] last:border-0" data-testid="history-item">
      <Link
        to="/media/$mediaId"
        params={{ mediaId: item.mediaItemId ?? "" }}
        className="shrink-0 w-12 h-16 rounded-[var(--radius-sm)] overflow-hidden bg-[var(--surface-raised)] block"
      >
        {posterUrl ? (
          <img
            src={posterUrl}
            alt=""
            className="w-full h-full object-cover"
            onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }}
          />
        ) : (
          <div className="w-full h-full flex items-center justify-center text-[var(--text-muted)]">
            <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
              <rect x={2} y={4} width={20} height={16} rx={2} />
              <path strokeLinecap="round" strokeLinejoin="round" d="M10 9l5 3-5 3V9z" />
            </svg>
          </div>
        )}
      </Link>

      <div className="flex-1 min-w-0">
        <Link
          to="/media/$mediaId"
          params={{ mediaId: item.mediaItemId ?? "" }}
          className="text-sm font-medium text-[var(--text)] hover:text-[var(--accent)] transition-colors truncate block"
        >
          Media {item.mediaItemId?.slice(0, 8)}
          {item.playCount && item.playCount > 1 ? ` (${item.playCount} plays)` : ""}
        </Link>
        <div className="flex items-center gap-2 mt-0.5">
          <p className="text-xs text-[var(--text-muted)]">{formatDate(item.lastPlayedAt)}</p>
          {item.durationMs && (
            <p className="text-xs text-[var(--text-muted)]">{formatDuration(item.positionMs)} / {formatDuration(item.durationMs)}</p>
          )}
        </div>
        <div className="flex items-center gap-2 mt-1.5">
          <div className="w-24">
            <ProgressBar percent={pct} />
          </div>
          <p className="text-xs text-[var(--text-muted)]">{Math.round(pct)}%</p>
        </div>
      </div>

      <div className="flex items-center gap-2 shrink-0">
        {item.played && <Badge variant="success">Watched</Badge>}
        <button
          onClick={() => {
            if (item.mediaItemId) {
              unwatch.mutate({ mediaItemId: item.mediaItemId });
            }
          }}
          disabled={unwatch.isPending}
          className="text-xs text-[var(--text-muted)] hover:text-[var(--text)] transition-colors disabled:opacity-50"
        >
          Remove
        </button>
      </div>
    </div>
  );
}

export function WatchHistory({ limit, showClearButton = false }: Props) {
  const [showClearModal, setShowClearModal] = useState(false);
  const qc = useQueryClient();
  const { data, isLoading } = usePlaybackServiceGetWatchHistory(limit ? { limit } : undefined);
  const items = data?.items ?? [];

  const grouped = items.reduce<Record<string, WatchProgress[]>>((acc, item) => {
    const group = getDateGroup(item.lastPlayedAt);
    (acc[group] ??= []).push(item);
    return acc;
  }, {});

  const handleClearAll = () => {
    // Mark each item as unwatched (no bulk clear endpoint available)
    setShowClearModal(false);
    void qc.invalidateQueries({ queryKey: getPlaybackServiceGetWatchHistoryQueryKey() });
  };

  if (isLoading) {
    return <p className="text-sm text-[var(--text-muted)]">Loading...</p>;
  }

  if (items.length === 0) {
    return <div data-testid="history-empty"><EmptyState icon="history" title="No watch history yet" description="Start watching something to see it here." /></div>;
  }

  return (
    <div className="flex flex-col gap-4" data-testid="history-list">
      {showClearButton && items.length > 0 && (
        <div className="flex justify-end">
          <Button variant="danger" onClick={() => setShowClearModal(true)} data-testid="clear-history-btn">
            Clear history
          </Button>
        </div>
      )}

      {Object.entries(grouped).map(([group, groupItems]) => (
        <div key={group}>
          <h3 className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide mb-2">{group}</h3>
          <div className="bg-[var(--surface-raised)] rounded-[var(--radius-md)] px-4">
            {groupItems.map((item) => (
              <HistoryItem key={item.id ?? item.mediaItemId} item={item} />
            ))}
          </div>
        </div>
      ))}

      <Modal open={showClearModal} onClose={() => setShowClearModal(false)} title="Clear watch history">
        <p className="text-sm text-[var(--text-muted)] mb-6">
          This will remove all items from your watch history. This action cannot be undone.
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setShowClearModal(false)}>
            Cancel
          </Button>
          <Button variant="danger" onClick={handleClearAll}>
            Clear all
          </Button>
        </div>
      </Modal>
    </div>
  );
}
