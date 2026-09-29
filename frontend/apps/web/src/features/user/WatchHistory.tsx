import { useState, useRef, useMemo, useCallback } from "react";
import { formatDurationMs } from "@evey/api";
import { Clock, Film } from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  usePlaybackServiceGetWatchHistory,
  usePlaybackServiceMarkAsUnwatched,
  usePlaybackServiceClearWatchHistory,
  watchHistoryLabel,
} from "@evey/api";
import { toast } from "@evey/store";
import { invalidateWatchedState } from "@evey/client";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { MS_PER_DAY } from "@/lib/time";
import { useDemoLock } from "@/hooks/useDemoMode";

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
  title?: string;
  mediaType?: string;
  indexNumber?: number;
  parentIndexNumber?: number;
  seriesTitle?: string;
};

interface Props {
  limit?: number;
  showClearButton?: boolean;
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

  const pct = item.percentComplete ?? 0;
  const posterUrl = item.mediaItemId
    ? `/api/v1/media/${item.mediaItemId}/artwork/poster`
    : undefined;

  // The formatting itself lives in @evey/api so web, mobile and TV cannot drift
  // apart again; only the localized fallback stays here.
  const label = watchHistoryLabel(item) ?? t("auth.watchHistory.unknownMedia", "Unknown media");

  return (
    <div className="flex items-center gap-3 py-3 border-b border-[var(--border)] last:border-0" data-testid="history-item">
      <Link
        to="/media/$mediaId"
        params={{ mediaId: item.mediaItemId ?? "" }}
        aria-label={t("auth.watchHistory.openMedia", "Open media details")}
        className="shrink-0 w-12 h-16 rounded-[var(--radius-sm)] overflow-hidden bg-[var(--surface-raised)] block"
      >
        {posterUrl ? (
          <img
            src={posterUrl}
            alt=""
            aria-hidden="true"
            className="w-full h-full object-cover"
            onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }}
          />
        ) : (
          <div className="w-full h-full flex items-center justify-center text-[var(--text-muted)]">
            <Film size={20} strokeWidth={1.5} />
          </div>
        )}
      </Link>

      <div className="flex-1 min-w-0">
        <Link
          to="/media/$mediaId"
          params={{ mediaId: item.mediaItemId ?? "" }}
          className="text-sm font-medium text-[var(--text)] hover:text-[var(--accent)] transition-colors-fast truncate block"
        >
          {label}
          {item.playCount && item.playCount > 1 ? ` (${t("auth.watchHistory.plays", { count: item.playCount })})` : ""}
        </Link>
        <div className="flex items-center gap-2 mt-0.5">
          <p className="text-xs text-[var(--text-muted)]">{formatDate(item.lastPlayedAt)}</p>
          {item.durationMs && (
            <p className="text-xs text-[var(--text-muted)]">{formatDurationMs(item.positionMs)} / {formatDurationMs(item.durationMs)}</p>
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
        {item.played && <Badge variant="success">{t("auth.watchHistory.watched")}</Badge>}
        <button
          onClick={() => {
            if (item.mediaItemId) {
              unwatch.mutate({ mediaItemId: item.mediaItemId });
            }
          }}
          disabled={unwatch.isPending}
          className="text-xs text-[var(--text-muted)] hover:text-[var(--text)] transition-colors-fast disabled:opacity-50"
        >
          {t("common.remove")}
        </button>
      </div>
    </div>
  );
}

type VirtualRow =
  | { type: "header"; label: string }
  | { type: "item"; item: WatchProgress; isFirst: boolean; isLast: boolean };

const ITEM_HEIGHT = 76;
const HEADER_HEIGHT = 36;

export function WatchHistory({ limit, showClearButton = false }: Props) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const parentRef = useRef<HTMLDivElement>(null);

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
  const { data, isLoading, isError, error, refetch } = usePlaybackServiceGetWatchHistory(limit ? { limit } : undefined);
  const items = useMemo(() => data?.items ?? [], [data?.items]);

  const getDateGroup = useCallback((iso: string | undefined): string => {
    if (!iso) return t("common.unknown");
    const date = new Date(iso);
    const now = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const itemDate = new Date(date.getFullYear(), date.getMonth(), date.getDate());
    const diffDays = Math.floor((today.getTime() - itemDate.getTime()) / MS_PER_DAY);

    if (diffDays === 0) return t("auth.watchHistory.today");
    if (diffDays === 1) return t("auth.watchHistory.yesterday");
    if (diffDays < 7) return t("auth.watchHistory.thisWeek");
    if (diffDays < 30) return t("auth.watchHistory.thisMonth");
    return date.toLocaleDateString(undefined, { year: "numeric", month: "long" });
  }, [t]);

  const virtualRows = useMemo(() => {
    const grouped = items.reduce<Record<string, WatchProgress[]>>((acc, item) => {
      const group = getDateGroup(item.lastPlayedAt);
      (acc[group] ??= []).push(item);
      return acc;
    }, {});

    const rows: VirtualRow[] = [];
    for (const [label, groupItems] of Object.entries(grouped)) {
      rows.push({ type: "header", label });
      groupItems.forEach((item, i) =>
        rows.push({ type: "item", item, isFirst: i === 0, isLast: i === groupItems.length - 1 }),
      );
    }
    return rows;
  }, [items, getDateGroup]);

  const virtualizer = useVirtualizer({
    count: virtualRows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: (i) => virtualRows[i].type === "header" ? HEADER_HEIGHT : ITEM_HEIGHT,
    overscan: 10,
  });

  const handleClearAll = () => {
    setShowClearModal(false);
    clearAll.mutate({});
  };

  if (isLoading) {
    return <p className="text-sm text-[var(--text-muted)]">{t("common.loading")}</p>;
  }

  // History that failed to load is not history that was cleared.
  if (isError) {
    return <FetchErrorState error={error} onRetry={() => void refetch()} />;
  }

  if (items.length === 0) {
    return <div data-testid="history-empty"><EmptyState icon={<Clock size={48} strokeWidth={1.5} />} title={t("auth.watchHistory.noHistory")} description={t("auth.watchHistory.noHistoryDesc")} /></div>;
  }

  return (
    <div className="flex flex-col gap-4" data-testid="history-list">
      {showClearButton && items.length > 0 && (
        <div className="flex justify-end">
          <Button {...lock} variant="danger" onClick={() => setShowClearModal(true)} data-testid="clear-history-btn">
            {t("auth.watchHistory.clearHistory")}
          </Button>
        </div>
      )}

      <div ref={parentRef} className="max-h-[70vh] overflow-auto">
        <div style={{ height: virtualizer.getTotalSize(), width: "100%", position: "relative" }}>
          {virtualizer.getVirtualItems().map((vItem) => {
            const row = virtualRows[vItem.index];
            if (row.type === "header") {
              return (
                <div
                  key={vItem.key}
                  style={{ position: "absolute", top: vItem.start, left: 0, width: "100%", height: vItem.size }}
                >
                  <h3 className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide mb-2">{row.label}</h3>
                </div>
              );
            }
            return (
              <div
                key={vItem.key}
                style={{ position: "absolute", top: vItem.start, left: 0, width: "100%" }}
                className={`bg-[var(--surface-raised)] px-4 ${row.isFirst ? "rounded-t-[var(--radius-md)]" : ""} ${row.isLast ? "rounded-b-[var(--radius-md)]" : ""}`}
              >
                <HistoryItem item={row.item} />
              </div>
            );
          })}
        </div>
      </div>

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
