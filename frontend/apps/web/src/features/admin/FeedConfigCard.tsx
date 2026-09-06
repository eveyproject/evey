import { useState, useEffect, useCallback, useRef, type PointerEvent as ReactPointerEvent } from "react";
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
import { GripVertical, ChevronUp, ChevronDown } from "lucide-react";
import { Switch } from "@/components/ui/Switch";

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
  // to appear on the home page - others (admin.nav-only, etc.) don't make
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
  // dirty tracks whether the admin has made unsaved local edits. While true,
  // background refetches must NOT overwrite in-progress changes.
  const [dirty, setDirty] = useState(false);
  const dragIdxRef = useRef<number | null>(null);
  const overIdxRef = useRef<number | null>(null);

  useEffect(() => {
    // Don't overwrite unsaved local edits when a background refetch arrives.
    if (dirty) return;
    if (data?.configs) {
      const persisted = data.configs.map((c) => ({
        feedType: c.feedType ?? "",
        enabled: c.enabled ?? true,
        displayOrder: c.displayOrder ?? 0,
        maxItems: c.maxItems ?? 20,
      }));
      // Synthesise rows for plugins that don't have an admin row yet so
      // they show up immediately after install - default enabled, parked
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
    // pluginsData is stable enough - re-run only when configs or plugin
    // set actually change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, pluginsData, dirty]);

  const commitReorder = useCallback((from: number, to: number) => {
    if (from === to) return;
    setRows((prev) => {
      // Plugin rows: admin can disable but not reorder for users - drop
      // a drag attempt rather than persist a bogus admin order that we
      // wouldn't honour at merge time anyway.
      if (isPluginRow(prev[from].feedType) || isPluginRow(prev[to].feedType)) {
        return prev;
      }
      const next = [...prev];
      const [moved] = next.splice(from, 1);
      next.splice(to, 0, moved);
      return next.map((r, i) => ({ ...r, displayOrder: i }));
    });
    setDirty(true);
  }, []);

  const startPointerDrag = useCallback((i: number, e: ReactPointerEvent) => {
    e.preventDefault();
    dragIdxRef.current = i;
    overIdxRef.current = i;
    setDragIdx(i);
    setOverIdx(i);
  }, []);

  // Pointer-based reordering: works on touch AND mouse. The previous native
  // HTML5 drag&drop (draggable/onDragStart) has NO touch support, so the drag
  // handle did nothing on phones/tablets.
  useEffect(() => {
    if (dragIdx === null) return;
    const onMove = (e: PointerEvent) => {
      const el = document.elementFromPoint(e.clientX, e.clientY);
      const row = el?.closest("[data-dragidx]");
      if (row) {
        const j = Number(row.getAttribute("data-dragidx"));
        if (!Number.isNaN(j)) { overIdxRef.current = j; setOverIdx(j); }
      }
    };
    const onUp = () => {
      const from = dragIdxRef.current;
      const to = overIdxRef.current;
      if (from !== null && to !== null && from !== to) commitReorder(from, to);
      dragIdxRef.current = null;
      overIdxRef.current = null;
      setDragIdx(null);
      setOverIdx(null);
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    window.addEventListener("pointercancel", onUp);
    return () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      window.removeEventListener("pointercancel", onUp);
    };
  }, [dragIdx, commitReorder]);

  const toggle = (i: number) => {
    setRows((prev) =>
      prev.map((r, idx) => (idx === i ? { ...r, enabled: !r.enabled } : r)),
    );
    setDirty(true);
  };

  const setMaxItems = (i: number, v: number) => {
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, maxItems: v } : r)));
    setDirty(true);
  };

  const save = async () => {
    setSaving(true);
    try {
      await feedServiceBatchUpdateAdminFeedConfigs({ configs: rows });
      toast.success(t("common.feeds.saved"));
      // Await the refetch of THIS card's source query before clearing dirty:
      // the rows-sync effect is gated on !dirty, so clearing it while the cache
      // still holds the pre-save config would momentarily rebuild rows from the
      // stale data and flash the old order/maxItems until the refetch lands.
      await queryClient.invalidateQueries({
        queryKey: getFeedServiceGetAdminFeedConfigsQueryKey(),
      });
      setDirty(false);
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
            data-dragidx={i}
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
              onPointerDown={(e) => startPointerDrag(i, e)}
              style={{ touchAction: "none" }}
              className="cursor-grab active:cursor-grabbing text-[var(--text-muted)]"
              aria-hidden="true"
            />
            {/* The grip carries a pointer handler and nothing else, so an admin
                on a keyboard or a screen reader could enable and disable feeds
                but never reorder them. Same two buttons the user-facing
                CustomizePanel already ships for this exact list. */}
            <button
              type="button"
              onClick={() => commitReorder(i, i - 1)}
              disabled={i === 0}
              aria-label={t("common.feeds.moveUp")}
              title={t("common.feeds.moveUp")}
              className="p-0.5 text-[var(--text-muted)] hover:text-[var(--text)] disabled:opacity-30 cursor-pointer disabled:cursor-default"
            >
              <ChevronUp size={14} />
            </button>
            <button
              type="button"
              onClick={() => commitReorder(i, i + 1)}
              disabled={i === rows.length - 1}
              aria-label={t("common.feeds.moveDown")}
              title={t("common.feeds.moveDown")}
              className="p-0.5 text-[var(--text-muted)] hover:text-[var(--text)] disabled:opacity-30 cursor-pointer disabled:cursor-default"
            >
              <ChevronDown size={14} />
            </button>
            {/* The Puzzle icon used to sit here for plugins, but it
                doubled as a "non-draggable" cue. Now that admin can
                reorder plugins too, the grip is shown everywhere - the
                Plugin badge in the label column still signals the kind. */}

            <span
              className="text-sm font-medium select-none flex items-center gap-2"
              style={{
                color: row.enabled ? "var(--text)" : "var(--text-muted)",
              }}
            >
              {label}
              {plugin && (
                <span className="text-xs uppercase tracking-wider px-1.5 py-0.5 rounded-[var(--radius-sm)] bg-[var(--accent-subtle)] text-[var(--accent)]">
                  {t("admin.serverConfig.feedConfig.pluginBadge", "Plugin")}
                </span>
              )}
            </span>

            <div className="flex justify-center">
              <Switch
                checked={row.enabled}
                onChange={() => toggle(i)}
                label={row.enabled ? t("common.disable") : t("common.enable")}
              />
            </div>

            <div className="flex justify-center">
              {plugin ? (
                <span className="text-xs text-[var(--text-muted)]">-</span>
              ) : (
                <input
                  type="number"
                  min={1}
                  max={100}
                  aria-label={`${t("common.feeds.maxItems")}: ${label}`}
                  value={row.maxItems}
                  onChange={(e) => {
                    // Guard NaN (empty / non-numeric paste): keep the old value
                    // rather than persisting NaN to the backend on Save.
                    const v = Number(e.target.value);
                    setMaxItems(i, Number.isFinite(v) ? Math.max(1, v) : row.maxItems);
                  }}
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
