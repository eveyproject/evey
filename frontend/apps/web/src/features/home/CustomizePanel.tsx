import { useState, useEffect, useRef, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useFeedServiceGetFeedConfig,
  useFeedServiceUpdateProfileFeedConfig,
  useFeedServiceResetProfileFeedConfig,
  useLibraryServiceListLibraries,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { X, GripVertical, RotateCcw } from "lucide-react";

const FEED_LABEL_KEYS: Record<string, string> = {
  resume_playback: "common.feeds.labels.resume_playback",
  next_up: "common.feeds.labels.next_up",
  recommended: "common.feeds.labels.recommended",
  most_popular: "common.feeds.labels.most_popular",
  collections: "common.feeds.labels.collections",
};

function feedLabel(t: (k: string) => string, feedType: string, libraryNames: Map<string, string>): string {
  if (feedType.startsWith("library:")) {
    const libId = feedType.slice("library:".length);
    return libraryNames.get(libId) ?? feedType;
  }
  return FEED_LABEL_KEYS[feedType] ? t(FEED_LABEL_KEYS[feedType]) : feedType;
}

interface Row {
  feedType: string;
  enabled: boolean;
  displayOrder: number;
}

export function CustomizePanel({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const profileId = useAuthStore((s) => s.profileId);
  const queryClient = useQueryClient();
  const panelRef = useRef<HTMLDivElement>(null);
  const { data } = useFeedServiceGetFeedConfig({ profileId: profileId ?? undefined });
  const { data: librariesData } = useLibraryServiceListLibraries();
  const libraryNames = new Map(
    (librariesData?.libraries ?? []).map((l) => [l.id!, l.name ?? l.id!]),
  );
  const updateMutation = useFeedServiceUpdateProfileFeedConfig();
  const resetMutation = useFeedServiceResetProfileFeedConfig();

  const [rows, setRows] = useState<Row[]>([]);
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
          }))
          .sort((a, b) => a.displayOrder - b.displayOrder),
      );
    }
  }, [data]);

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open, onClose]);

  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [open, onClose]);

  const toggle = (i: number) => {
    setRows((prev) => {
      const next = [...prev];
      next[i] = { ...next[i], enabled: !next[i].enabled };
      return next;
    });
    const row = rows[i];
    updateMutation.mutate(
      { feedType: row.feedType, data: { enabled: !row.enabled, displayOrder: row.displayOrder } },
      { onSuccess: () => queryClient.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] }) },
    );
  };

  const commitReorder = useCallback((from: number, to: number) => {
    if (from === to) return;
    setRows((prev) => {
      const next = [...prev];
      const [moved] = next.splice(from, 1);
      next.splice(to, 0, moved);
      return next.map((r, i) => ({ ...r, displayOrder: i }));
    });
    // Persist both swapped items
    const reordered = [...rows];
    const [moved] = reordered.splice(from, 1);
    reordered.splice(to, 0, moved);
    reordered.forEach((r, i) => {
      if (r.displayOrder !== i) {
        updateMutation.mutate(
          { feedType: r.feedType, data: { enabled: r.enabled, displayOrder: i } },
          { onSuccess: () => queryClient.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] }) },
        );
      }
    });
  }, [rows, updateMutation, queryClient]);

  const handleDragEnd = useCallback(() => {
    if (dragIdx !== null && overIdx !== null && dragIdx !== overIdx) {
      commitReorder(dragIdx, overIdx);
    }
    setDragIdx(null);
    setOverIdx(null);
  }, [dragIdx, overIdx, commitReorder]);

  const resetAll = () => {
    rows.forEach((r) => {
      resetMutation.mutate(
        { feedType: r.feedType },
        { onSuccess: () => queryClient.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] }) },
      );
    });
  };

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-overlay flex justify-end bg-[var(--overlay-chip-bg)]">
      <div
        ref={panelRef}
        className="w-80 h-full overflow-y-auto animate-fade-in bg-[var(--surface)] border-l border-[var(--border)]"
      >
        <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border)]">
          <h2 className="text-base font-semibold text-[var(--text)]">
            {t("common.feeds.customizeTitle")}
          </h2>
          <button onClick={onClose} className="p-1 rounded hover:opacity-80 text-[var(--text-muted)]">
            <X size={18} />
          </button>
        </div>

        <div className="p-4 space-y-1">
          {rows.map((row, i) => (
            <div
              key={row.feedType}
              draggable
              onDragStart={() => setDragIdx(i)}
              onDragOver={(e) => { e.preventDefault(); setOverIdx(i); }}
              onDragEnd={handleDragEnd}
              className="flex items-center gap-2 px-3 py-2.5 rounded-[var(--radius-md)] transition-all"
              style={{
                backgroundColor: overIdx === i && dragIdx !== null && dragIdx !== i ? "var(--accent-subtle)" : "var(--bg)",
                opacity: dragIdx === i ? 0.5 : 1,
              }}
            >
              <GripVertical size={14} className="flex-shrink-0 cursor-grab active:cursor-grabbing text-[var(--text-muted)]" />

              <span
                className="flex-1 text-sm select-none"
                style={{ color: row.enabled ? "var(--text)" : "var(--text-muted)" }}
              >
                {feedLabel(t, row.feedType, libraryNames)}
              </span>

              <button
                role="switch"
                aria-checked={row.enabled}
                onClick={() => toggle(i)}
                className="w-9 h-5 rounded-full transition-colors-fast relative flex-shrink-0"
                style={{ backgroundColor: row.enabled ? "var(--accent)" : "var(--surface-raised)" }}
              >
                <span
                  className="absolute top-0.5 w-4 h-4 rounded-full bg-white transition-transform-fast"
                  style={{ left: row.enabled ? "calc(100% - 1.125rem)" : "0.125rem" }}
                />
              </button>
            </div>
          ))}
        </div>

        <div className="px-4 pb-4">
          <button
            onClick={resetAll}
            className="flex items-center gap-2 text-sm hover:opacity-80 transition-opacity-fast text-[var(--text-muted)]"
          >
            <RotateCcw size={14} />
            {t("common.feeds.resetDefaults")}
          </button>
        </div>
      </div>
    </div>
  );
}
