import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  useFeedServiceGetAdminFeedConfigs,
  useLibraryServiceListLibraries,
  useAdminPluginServiceListPlugins,
  feedServiceBatchUpdateAdminFeedConfigs,
  getFeedServiceGetAdminFeedConfigsQueryKey,
  getFeedServiceGetFeedConfigQueryKey,
} from "@evey/api";
import { toast } from "@evey/store";
import { useQueryClient } from "@tanstack/react-query";
import { GripVertical } from "lucide-react";

// Mirrors AdminFeedsPage from routes/_auth/admin/feeds.lazy.tsx but as an
// embeddable card for the server-config page. The standalone /admin/feeds
// route now redirects to /admin/server-config so all server-level admin
// surfaces live in one page.

interface FeedRow {
  feedType: string;
  enabled: boolean;
  displayOrder: number;
  maxItems: number;
}

const FEED_LABEL_KEYS: Record<string, string> = {
  resume_playback: "common.feeds.labels.resume_playback",
  next_episode: "common.feeds.labels.next_up",
  recommended: "common.feeds.labels.recommended",
  most_popular: "common.feeds.labels.most_popular",
  collections: "common.feeds.labels.collections",
};

function feedLabel(
  t: (k: string) => string,
  feedType: string,
  libraryNames: Map<string, string>,
): string {
  if (feedType.startsWith("library:")) {
    const libId = feedType.slice("library:".length);
    return libraryNames.get(libId) ?? feedType;
  }
  return FEED_LABEL_KEYS[feedType] ? t(FEED_LABEL_KEYS[feedType]) : feedType;
}

// isPluginRow tells us if this feed entry represents a plugin home.section.
// Plugin rows render with a different style and have admin-restricted
// behaviour: admin can force-disable (the toggle propagates to every user
// as a hard override) but cannot force an order (each user keeps their own
// drag order in their Customize panel), so the drag handle is disabled.
function isPluginRow(feedType: string): boolean {
  return feedType.startsWith("plugin:");
}

function pluginLabel(
  feedType: string,
  pluginNames: Map<string, string>,
): string {
  const id = feedType.slice("plugin:".length);
  return pluginNames.get(id) ?? id;
}

export function FeedConfigCard() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data, isLoading } = useFeedServiceGetAdminFeedConfigs();
  const { data: librariesData } = useLibraryServiceListLibraries();
  const { data: pluginsData } = useAdminPluginServiceListPlugins();
  const libraryNames = new Map(
    (librariesData?.libraries ?? []).map((l) => [l.id!, l.name ?? l.id!]),
  );
  // Only plugins that expose a `home.section` frontend slot are eligible
  // to appear on the home page — others (admin.nav-only, etc.) don't make
  // sense to list here.
  const homePlugins = (pluginsData?.plugins ?? []).filter(
    (p) =>
      p.enabled &&
      (p.frontend ?? []).some((mod) => mod.slot === "home.section"),
  );
  const pluginNames = new Map(
    homePlugins.map((p) => [p.id ?? "", p.name ?? p.id ?? ""]),
  );
  const [rows, setRows] = useState<FeedRow[]>([]);
  const [saving, setSaving] = useState(false);
  const [dragIdx, setDragIdx] = useState<number | null>(null);
  const [overIdx, setOverIdx] = useState<number | null>(null);

  useEffect(() => {
    if (data?.configs) {
      const persisted = data.configs.map((c) => ({
        feedType: c.feedType ?? "",
        enabled: c.enabled ?? true,
        displayOrder: c.displayOrder ?? 0,
        maxItems: c.maxItems ?? 20,
      }));
      // Synthesise rows for plugins that don't have an admin row yet so
      // they show up immediately after install — default enabled, parked
      // at the end of the list.
      const persistedTypes = new Set(persisted.map((r) => r.feedType));
      const baseOrder = persisted.length;
      const virtualPluginRows = homePlugins
        .map((p) => `plugin:${p.id ?? ""}`)
        .filter((ft) => !persistedTypes.has(ft))
        .map((ft, i) => ({
          feedType: ft,
          enabled: true,
          displayOrder: baseOrder + i,
          maxItems: 20,
        }));
      setRows(
        [...persisted, ...virtualPluginRows].sort(
          (a, b) => a.displayOrder - b.displayOrder,
        ),
      );
    }
    // pluginsData is stable enough — re-run only when configs or plugin
    // set actually change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, pluginsData]);

  const handleDragEnd = useCallback(() => {
    if (dragIdx !== null && overIdx !== null && dragIdx !== overIdx) {
      setRows((prev) => {
        // Plugin rows: admin can disable but not reorder for users — drop
        // a drag attempt rather than persist a bogus admin order that we
        // wouldn't honour at merge time anyway.
        if (isPluginRow(prev[dragIdx].feedType) || isPluginRow(prev[overIdx].feedType)) {
          return prev;
        }
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
    setRows((prev) =>
      prev.map((r, idx) => (idx === i ? { ...r, enabled: !r.enabled } : r)),
    );
  };

  const setMaxItems = (i: number, v: number) => {
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, maxItems: v } : r)));
  };

  const save = async () => {
    setSaving(true);
    try {
      await feedServiceBatchUpdateAdminFeedConfigs({ configs: rows });
      toast.success(t("common.feeds.saved"));
      queryClient.invalidateQueries({
        queryKey: getFeedServiceGetAdminFeedConfigsQueryKey(),
      });
      // The home page reads merged config via GetFeedConfig (admin defaults
      // + profile overrides). Without this second invalidation the admin
      // save lands in DB but every open HP keeps showing the stale merged
      // result until the 60s staleTime expires.
      queryClient.invalidateQueries({
        queryKey: getFeedServiceGetFeedConfigQueryKey(),
      });
    } catch {
      toast.error(t("common.error"));
    } finally {
      setSaving(false);
    }
  };

  if (isLoading) {
    return <div className="animate-shimmer h-40 rounded-[var(--radius-md)]" />;
  }

  return (
    <div className="space-y-3">
      <div
        className="rounded-[var(--radius-lg)] overflow-hidden border border-[var(--border)] bg-[var(--surface)]"
      >
        <div className="grid grid-cols-[24px_1fr_80px_80px] gap-2 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)] border-b border-[var(--border)]">
          <span />
          <span>{t("common.feeds.section")}</span>
          <span className="text-center">{t("common.enabled")}</span>
          <span className="text-center">{t("common.feeds.maxItems")}</span>
        </div>

        {rows.map((row, i) => {
          const plugin = isPluginRow(row.feedType);
          const label = plugin
            ? pluginLabel(row.feedType, pluginNames)
            : feedLabel(t, row.feedType, libraryNames);
          return (
          <div
            key={row.feedType}
            draggable
            onDragStart={() => setDragIdx(i)}
            onDragOver={(e) => {
              e.preventDefault();
              setOverIdx(i);
            }}
            onDragEnd={handleDragEnd}
            className="grid grid-cols-[24px_1fr_80px_80px] gap-2 items-center px-4 py-3 transition-all"
            style={{
              borderBottom:
                i < rows.length - 1 ? "1px solid var(--border)" : "none",
              // Drop-target highlight wins over the plugin tint so the
              // admin still sees where the row will land when hovering one.
              backgroundColor:
                overIdx === i && dragIdx !== null && dragIdx !== i
                  ? "var(--accent-subtle)"
                  : plugin
                    ? "color-mix(in srgb, var(--accent) 6%, transparent)"
                    : "transparent",
              borderLeft: plugin
                ? "3px solid color-mix(in srgb, var(--accent) 70%, transparent)"
                : undefined,
              opacity: dragIdx === i ? 0.5 : 1,
            }}
          >
            <GripVertical
              size={16}
              className="cursor-grab active:cursor-grabbing text-[var(--text-muted)]"
            />
            {/* The Puzzle icon used to sit here for plugins, but it
                doubled as a "non-draggable" cue. Now that admin can
                reorder plugins too, the grip is shown everywhere — the
                Plugin badge in the label column still signals the kind. */}

            <span
              className="text-sm font-medium select-none flex items-center gap-2"
              style={{
                color: row.enabled ? "var(--text)" : "var(--text-muted)",
              }}
            >
              {label}
              {plugin && (
                <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded-[var(--radius-sm)] bg-[var(--accent-subtle)] text-[var(--accent)]">
                  {t("admin.serverConfig.feedConfig.pluginBadge", "Plugin")}
                </span>
              )}
            </span>

            <div className="flex justify-center">
              <button
                onClick={() => toggle(i)}
                className="w-10 h-5 rounded-full transition-colors-fast relative"
                style={{
                  backgroundColor: row.enabled
                    ? "var(--accent)"
                    : "var(--surface-raised)",
                }}
                role="switch"
                aria-checked={row.enabled}
                aria-label={row.enabled ? t("common.disable") : t("common.enable")}
              >
                <span
                  className="absolute top-0.5 w-4 h-4 rounded-full bg-white transition-transform-fast"
                  style={{
                    left: row.enabled ? "calc(100% - 1.125rem)" : "0.125rem",
                  }}
                />
              </button>
            </div>

            <div className="flex justify-center">
              {plugin ? (
                <span className="text-xs text-[var(--text-muted)]">—</span>
              ) : (
                <input
                  type="number"
                  min={1}
                  max={100}
                  value={row.maxItems}
                  onChange={(e) =>
                    setMaxItems(i, Math.max(1, Number(e.target.value)))
                  }
                  className="w-14 text-center text-sm py-0.5 rounded-[var(--radius-sm)] border bg-[var(--bg)] border-[var(--border)] text-[var(--text)]"
                />
              )}
            </div>
          </div>
          );
        })}
      </div>

      <div className="flex justify-end">
        <button
          onClick={save}
          disabled={saving}
          className="px-4 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-opacity-fast hover:opacity-90 disabled:opacity-50 bg-[var(--accent)] text-[var(--filter-active-text)]"
          aria-label={t("common.save")}
          data-testid="feeds-save-btn"
        >
          {saving ? t("common.loading") : t("common.save")}
        </button>
      </div>
    </div>
  );
}
