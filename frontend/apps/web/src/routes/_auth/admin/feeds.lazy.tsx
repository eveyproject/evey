import { useState, useEffect, useCallback } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useFeedServiceGetAdminFeedConfigs } from "@evey/api";
import { toast } from "@evey/store";
import { eveyFetch } from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import { GripVertical } from "lucide-react";
import { PageHeader } from "@/components/ui/PageHeader";

export const Route = createLazyFileRoute("/_auth/admin/feeds")({
  component: AdminFeedsPage,
});

interface FeedRow {
  feedType: string;
  enabled: boolean;
  displayOrder: number;
  maxItems: number;
}

const FEED_LABEL_KEYS: Record<string, string> = {
  resume_playback: "common.feeds.labels.resume_playback",
  recommended: "common.feeds.labels.recommended",
  most_popular: "common.feeds.labels.most_popular",
  collections: "common.feeds.labels.collections",
};

function feedLabel(t: (k: string) => string, feedType: string): string {
  if (feedType.startsWith("library:")) return feedType.replace("library:", "Library: ");
  return FEED_LABEL_KEYS[feedType] ? t(FEED_LABEL_KEYS[feedType]) : feedType;
}

function AdminFeedsPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data, isLoading } = useFeedServiceGetAdminFeedConfigs();
  const [rows, setRows] = useState<FeedRow[]>([]);
  const [saving, setSaving] = useState(false);
  const [dragIdx, setDragIdx] = useState<number | null>(null);
  const [overIdx, setOverIdx] = useState<number | null>(null);

  useEffect(() => {
    if (data?.configs) {
      setRows(
        data.configs
          .map((c) => ({
            feedType: c.feedType ?? "",
            enabled: c.enabled ?? true,
            displayOrder: c.displayOrder ?? 0,
            maxItems: c.maxItems ?? 20,
          }))
          .sort((a, b) => a.displayOrder - b.displayOrder),
      );
    }
  }, [data]);

  const handleDragEnd = useCallback(() => {
    if (dragIdx !== null && overIdx !== null && dragIdx !== overIdx) {
      setRows((prev) => {
        const next = [...prev];
        const [moved] = next.splice(dragIdx, 1);
        next.splice(overIdx, 0, moved);
        return next.map((r, i) => ({ ...r, displayOrder: i }));
      });
    }
    setDragIdx(null);
    setOverIdx(null);
  }, [dragIdx, overIdx]);

  const toggle = (i: number) => {
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, enabled: !r.enabled } : r)));
  };

  const setMaxItems = (i: number, v: number) => {
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, maxItems: v } : r)));
  };

  const save = async () => {
    setSaving(true);
    try {
      await eveyFetch<void>({
        url: "/api/v1/admin/feeds/config",
        method: "PUT",
        data: rows,
      });
      toast.success(t("common.feeds.saved"));
      queryClient.invalidateQueries({ queryKey: ["/api/v1/admin/feeds/config"] });
    } catch {
      toast.error(t("common.error"));
    } finally {
      setSaving(false);
    }
  };

  if (isLoading) {
    return (
      <div className="p-6 animate-fade-in">
        <PageHeader title={t("common.feeds.title")} />
        <div className="animate-shimmer h-64 rounded-[var(--radius-md)]" />
      </div>
    );
  }

  return (
    <div className="p-6 animate-fade-in max-w-3xl">
      <div className="mb-6">
        <PageHeader title={t("common.feeds.title")} actions={
          <button
            onClick={save}
            disabled={saving}
            className="px-4 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-opacity-fast hover:opacity-90 disabled:opacity-50 bg-[var(--accent)] text-[var(--filter-active-text)]"
          >
            {saving ? t("common.loading") : t("common.save")}
          </button>
        } />
      </div>

      <div
        className="rounded-[var(--radius-lg)] overflow-hidden border border-[var(--border)] bg-[var(--surface)]"
      >
        <div
          className="grid grid-cols-[24px_1fr_80px_80px] gap-2 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)] border-b border-[var(--border)]"
        >
          <span />
          <span>{t("common.feeds.section")}</span>
          <span className="text-center">{t("common.enabled")}</span>
          <span className="text-center">{t("common.feeds.maxItems")}</span>
        </div>

        {rows.map((row, i) => (
          <div
            key={row.feedType}
            draggable
            onDragStart={() => setDragIdx(i)}
            onDragOver={(e) => { e.preventDefault(); setOverIdx(i); }}
            onDragEnd={handleDragEnd}
            className="grid grid-cols-[24px_1fr_80px_80px] gap-2 items-center px-4 py-3 transition-all"
            style={{
              borderBottom: i < rows.length - 1 ? "1px solid var(--border)" : "none",
              backgroundColor: overIdx === i && dragIdx !== null && dragIdx !== i ? "var(--accent-subtle)" : "transparent",
              opacity: dragIdx === i ? 0.5 : 1,
            }}
          >
            <GripVertical size={16} className="cursor-grab active:cursor-grabbing text-[var(--text-muted)]" />

            <span className="text-sm font-medium select-none" style={{ color: row.enabled ? "var(--text)" : "var(--text-muted)" }}>
              {feedLabel(t, row.feedType)}
            </span>

            <div className="flex justify-center">
              <button
                onClick={() => toggle(i)}
                className="w-10 h-5 rounded-full transition-colors-fast relative"
                style={{ backgroundColor: row.enabled ? "var(--accent)" : "var(--surface-raised)" }}
                aria-label={row.enabled ? t("common.disable") : t("common.enable")}
              >
                <span
                  className="absolute top-0.5 w-4 h-4 rounded-full bg-white transition-transform-fast"
                  style={{ left: row.enabled ? "calc(100% - 1.125rem)" : "0.125rem" }}
                />
              </button>
            </div>

            <div className="flex justify-center">
              <input
                type="number"
                min={1}
                max={100}
                value={row.maxItems}
                onChange={(e) => setMaxItems(i, Math.max(1, Number(e.target.value)))}
                className="w-14 text-center text-sm py-0.5 rounded-[var(--radius-sm)] border bg-[var(--bg)] border-[var(--border)] text-[var(--text)]"
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
