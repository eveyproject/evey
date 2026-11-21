import { useState, useMemo } from "react";
import {
  formatDurationMs,
  formatWatchTotalMs,
  isPositive,
  playbackServiceGetWatchHistory,
  getPlaybackServiceGetWatchHistoryQueryKey,
  usePlaybackServiceMarkAsUnwatched,
  usePlaybackServiceClearWatchHistory,
  watchHistoryLabel,
  type V1WatchProgress,
} from "@evey/api";
import { Check, Clock, Film, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { invalidateWatchedState } from "@evey/client";
import { Link } from "@tanstack/react-router";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { PageHeader } from "@/components/ui/PageHeader";
import { Spinner } from "@/components/ui/Spinner";
import { MS_PER_DAY } from "@/lib/time";
import { useDemoLock } from "@/hooks/useDemoMode";
import { useInfiniteSentinel } from "@/hooks/useInfiniteSentinel";

interface Props {
  showClearButton?: boolean;
}

const HISTORY_PAGE_SIZE = 50;

// Whole days between an entry and today, on the viewer's calendar.
function daysAgo(iso: string | undefined): number | null {
  if (!iso) return null;
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return null;
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const day = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  return Math.floor((today.getTime() - day.getTime()) / MS_PER_DAY);
}

// Today and Yesterday already say the day: the row keeps the time alone.
function formatWhen(iso: string | undefined): string {
  const age = daysAgo(iso);
  if (age === null || !iso) return "";
  const date = new Date(iso);
  if (age <= 1) return date.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  return date.toLocaleDateString(undefined, { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
}

function HistoryItem({ item }: { item: V1WatchProgress }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const unwatch = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => {
        // Unwatching restores the item to the watched-filtered home feeds.
        invalidateWatchedState(qc, item.mediaItemId);
      },
    },
  });

  const pct = Math.min(100, Math.max(0, item.percentComplete ?? 0));
  const posterUrl = item.mediaItemId
    ? `/api/v1/media/${item.mediaItemId}/artwork/poster`
    : undefined;

  // The formatting itself lives in @evey/api so web, mobile and TV cannot drift
  // apart again; only the localized fallback stays here.
  const label = watchHistoryLabel(item) ?? t("auth.watchHistory.unknownMedia", "Unknown media");
  const removeLabel = t("auth.watchHistory.removeFromHistory");
  const when = formatWhen(item.lastPlayedAt);

  return (
    <li
      className="group flex items-center gap-4 px-3 sm:px-4 py-3 border-b border-[var(--border)] last:border-0 hover:bg-[var(--surface-hover)] transition-colors-fast"
      data-testid="history-item"
    >
      <Link
        to="/media/$mediaId"
        params={{ mediaId: item.mediaItemId ?? "" }}
        aria-label={t("auth.watchHistory.openMedia", "Open media details")}
        className="relative shrink-0 w-12 h-[72px] rounded-[var(--radius-sm)] overflow-hidden bg-[var(--surface-raised)] flex items-center justify-center text-[var(--text-muted)]"
      >
        <Film size={20} strokeWidth={1.5} aria-hidden="true" />
        {posterUrl && (
          <img
            src={posterUrl}
            alt=""
            aria-hidden="true"
            loading="lazy"
            className="absolute inset-0 w-full h-full object-cover"
            onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }}
          />
        )}
      </Link>

      <div className="flex-1 min-w-0 flex flex-col gap-1.5">
        <Link
          to="/media/$mediaId"
          params={{ mediaId: item.mediaItemId ?? "" }}
          className="text-sm font-medium text-[var(--text)] hover:text-[var(--accent)] transition-colors-fast truncate"
        >
          {label}
          {item.playCount && item.playCount > 1 ? ` (${t("auth.watchHistory.plays", { count: item.playCount })})` : ""}
        </Link>
        <div className="flex items-center gap-3">
          {item.played ? (
            <span className="inline-flex items-center gap-1 text-xs font-medium text-[var(--badge-success-text,#6ee7b7)]">
              <Check size={14} strokeWidth={2.5} aria-hidden="true" />
              {t("auth.watchHistory.watched")}
            </span>
          ) : (
            <div
              className="shrink-0 w-24 sm:w-32 h-1 bg-[color-mix(in_srgb,var(--text-muted)_30%,transparent)] rounded-full overflow-hidden"
              role="progressbar"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={Math.round(pct)}
            >
              <div className="h-full bg-[var(--accent)]" style={{ width: `${pct}%` }} />
            </div>
          )}
          {isPositive(item.durationMs) && (
            <p className="text-xs text-[var(--text-muted)] truncate">
              {item.played
                ? formatDurationMs(item.durationMs)
                : `${formatWatchTotalMs(item.positionMs)} / ${formatDurationMs(item.durationMs)}`}
            </p>
          )}
        </div>
        <time dateTime={item.lastPlayedAt} className="sm:hidden text-xs text-[var(--text-muted)] tabular-nums">
          {when}
        </time>
      </div>

      <time dateTime={item.lastPlayedAt} className="hidden sm:block shrink-0 text-xs text-[var(--text-muted)] tabular-nums">
        {when}
      </time>
      <button
        type="button"
        onClick={() => {
          if (item.mediaItemId) {
            unwatch.mutate({ mediaItemId: item.mediaItemId });
          }
        }}
        disabled={unwatch.isPending}
        aria-label={removeLabel}
        title={removeLabel}
        className="shrink-0 p-2 rounded-[var(--radius-sm)] text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--border)] transition-colors-fast disabled:opacity-50"
      >
        <X size={16} aria-hidden="true" />
      </button>
    </li>
  );
}

export function WatchHistory({ showClearButton = false }: Props) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();

  const [showClearModal, setShowClearModal] = useState(false);
  const qc = useQueryClient();
  const clearAll = usePlaybackServiceClearWatchHistory({
    mutation: {
      onSuccess: () => {
        // Clearing history un-watches everything, so the items return to the
        // watched-filtered home feeds ("Continue Watching" etc.).
        invalidateWatchedState(qc);
        toast.success(t("auth.watchHistory.cleared"));
      },
      onError: (err: Error) => toast.error(err?.message || t("auth.watchHistory.clearFailed")),
    },
  });

  // Pages by keyset cursor behind a scroll sentinel: the page used to ask for
  // the first 50 entries and stop, so a busy day hid everything before it.
  // The key keeps the generated prefix so the unwatch invalidation reaches it.
  const { data, isLoading, isError, error, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: [...getPlaybackServiceGetWatchHistoryQueryKey(), "pages"],
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) =>
      playbackServiceGetWatchHistory(
        { "pagination.pageSize": HISTORY_PAGE_SIZE, ...(pageParam ? { "pagination.cursor": pageParam } : {}) },
        signal,
      ),
    getNextPageParam: (last) => last.pagination?.nextCursor || undefined,
  });
  const sentinelRef = useInfiniteSentinel(hasNextPage, isFetchingNextPage, fetchNextPage);
  const items = useMemo(() => data?.pages.flatMap((p) => p.items ?? []) ?? [], [data?.pages]);

  // Same buckets as the mobile app (historyGrouping), newest first.
  const groups = useMemo(() => {
    const groupOf = (iso: string | undefined): string => {
      const age = daysAgo(iso);
      if (age === null || !iso) return t("common.unknown");
      if (age === 0) return t("auth.watchHistory.today");
      if (age === 1) return t("auth.watchHistory.yesterday");
      if (age > 0 && age < 7) return t("auth.watchHistory.thisWeek");
      if (age > 0 && age < 30) return t("auth.watchHistory.thisMonth");
      return new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "long" });
    };
    const out: { label: string; items: V1WatchProgress[] }[] = [];
    for (const item of items) {
      const label = groupOf(item.lastPlayedAt);
      const last = out[out.length - 1];
      if (last && last.label === label) last.items.push(item);
      else out.push({ label, items: [item] });
    }
    return out;
  }, [items, t]);

  const header = (
    <PageHeader
      title={t("common.watchHistory.title")}
      actions={showClearButton && items.length > 0 ? (
        <Button {...lock} variant="ghost" onClick={() => setShowClearModal(true)} data-testid="clear-history-btn">
          {t("auth.watchHistory.clearHistory")}
        </Button>
      ) : undefined}
    />
  );

  const handleClearAll = () => {
    setShowClearModal(false);
    clearAll.mutate({});
  };

  let body;
  if (isLoading) {
    body = <p className="text-sm text-[var(--text-muted)]">{t("common.loading")}</p>;
  } else if (isError) {
    // History that failed to load is not history that was cleared.
    body = <FetchErrorState error={error} onRetry={() => void refetch()} />;
  } else if (items.length === 0) {
    body = <div data-testid="history-empty"><EmptyState icon={<Clock size={48} strokeWidth={1.5} />} title={t("auth.watchHistory.noHistory")} description={t("auth.watchHistory.noHistoryDesc")} /></div>;
  } else {
    body = (
      <div className="flex flex-col gap-6" data-testid="history-list">
        {groups.map((group) => (
          <section key={group.label} className="flex flex-col gap-2">
            <h2 className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide">{group.label}</h2>
            <ul className="bg-[var(--surface-raised)] rounded-[var(--radius-md)] overflow-hidden">
              {group.items.map((item) => (
                <HistoryItem key={item.id ?? item.mediaItemId} item={item} />
              ))}
            </ul>
          </section>
        ))}
        <div ref={sentinelRef} className="flex justify-center py-2" aria-hidden={!isFetchingNextPage}>
          {isFetchingNextPage && <Spinner size="sm" />}
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      {header}
      {body}

      <Modal open={showClearModal} onClose={() => setShowClearModal(false)} title={t("auth.watchHistory.clearTitle")}>
        <p className="text-sm text-[var(--text-muted)] mb-6">
          {t("auth.watchHistory.clearConfirm")}
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setShowClearModal(false)}>
            {t("common.cancel")}
          </Button>
          <Button {...lock} variant="danger" onClick={handleClearAll}>
            {t("auth.watchHistory.clearAll")}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
