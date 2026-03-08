import {
  usePlaybackServiceGetWatchHistory,
  usePlaybackServiceMarkAsUnwatched,
  getPlaybackServiceGetWatchHistoryQueryKey,
} from "@evey/api";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { Badge } from "@/components/ui/Badge";

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
}

function formatDate(iso: string | undefined): string {
  if (!iso) return "";
  return new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
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

  return (
    <div className="flex items-center justify-between gap-4 py-3 border-b border-[var(--border)] last:border-0">
      <div className="flex-1 min-w-0">
        <Link
          to="/media/$mediaId"
          params={{ mediaId: item.mediaItemId ?? "" }}
          className="text-sm font-medium text-[var(--text)] hover:text-[var(--accent)] transition-colors truncate block"
        >
          Media item{item.playCount && item.playCount > 1 ? ` (${item.playCount} plays)` : ""}
        </Link>
        <p className="text-xs text-[var(--text-muted)] mt-0.5">{formatDate(item.lastPlayedAt)}</p>
        <div className="mt-1.5 w-32">
          <ProgressBar percent={pct} />
        </div>
        <p className="text-xs text-[var(--text-muted)] mt-0.5">{Math.round(pct)}%</p>
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
          Mark unwatched
        </button>
      </div>
    </div>
  );
}

export function WatchHistory({ limit }: Props) {
  const { data, isLoading } = usePlaybackServiceGetWatchHistory(limit ? { limit } : undefined);
  const items = data?.items ?? [];

  if (isLoading) {
    return <p className="text-sm text-[var(--text-muted)]">Loading...</p>;
  }

  if (items.length === 0) {
    return <p className="text-sm text-[var(--text-muted)]">No watch history yet.</p>;
  }

  return (
    <div className="bg-[var(--surface-raised)] rounded-[var(--radius-md)] px-4">
      {items.map((item) => (
        <HistoryItem key={item.id ?? item.mediaItemId} item={item} />
      ))}
    </div>
  );
}
