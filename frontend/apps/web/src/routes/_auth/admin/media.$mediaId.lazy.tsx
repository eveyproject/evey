import { useState, useRef, useEffect } from "react";
import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceGetMediaItem, itemLabel } from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { toast } from "@evey/store";
import { useTranslation } from "react-i18next";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { ChevronLeft, Lock, Plus, RefreshCw, Save, Trash2 } from "lucide-react";
import { PageHeader } from "@/components/ui/PageHeader";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { ICON } from "@/constants/iconSizes";
import { SegmentTimeline } from "@/features/admin/SegmentTimeline";
import {
  useAdminMediaSegments,
  useSetSegment,
  useDeleteSegment,
  usePropagateSegment,
  useTriggerAutoDetect,
} from "@/hooks/useAdminSegments";
import type { MediaSegment } from "@/hooks/useAdminSegments";

export const Route = createLazyFileRoute("/_auth/admin/media/$mediaId")({
  component: AdminMediaSegmentsPage,
});

const SEGMENT_TYPES = ["intro", "recap", "end_credits", "studio_logo"] as const;
type SegmentType = (typeof SEGMENT_TYPES)[number];

interface LocalSegment extends MediaSegment {
  isDirty: boolean;
}

function msToMmSs(ms: number): string {
  const totalSec = Math.floor(ms / 1000);
  const m = Math.floor(totalSec / 60);
  const s = totalSec % 60;
  return `${m.toString().padStart(2, "0")}:${s.toString().padStart(2, "0")}`;
}

function mmSsToMs(value: string): number | null {
  const parts = value.split(":");
  if (parts.length !== 2) return null;
  const m = parseInt(parts[0], 10);
  const s = parseInt(parts[1], 10);
  if (isNaN(m) || isNaN(s)) return null;
  return (m * 60 + s) * 1000;
}

function AdminMediaSegmentsPage() {
  const { t } = useTranslation();
  const { mediaId } = Route.useParams();
  const videoRef = useRef<HTMLVideoElement>(null);
  const [durationMs, setDurationMs] = useState(0);
  const [currentMs, setCurrentMs] = useState(0);
  const [selectedType, setSelectedType] = useState<string | null>(null);
  const [localSegments, setLocalSegments] = useState<LocalSegment[]>([]);
  const [streamToken, setStreamToken] = useState<string | null>(null);

  const { data: mediaItem, isLoading: itemLoading } = useQuery<V1MediaItem>({
    queryKey: [`/api/v1/media/${mediaId}`],
    queryFn: ({ signal }) => libraryServiceGetMediaItem(mediaId, signal),
    enabled: !!mediaId,
  });

  const { data: serverSegments, isLoading: segLoading, isError: segError, error: segErr, refetch: refetchSegments } = useAdminMediaSegments(mediaId);

  const setSegment = useSetSegment(mediaId);
  const deleteSegment = useDeleteSegment(mediaId);
  const propagate = usePropagateSegment(mediaId);
  const triggerDetect = useTriggerAutoDetect(mediaId);

  const isEpisode = mediaItem?.type === "MEDIA_ITEM_TYPE_EPISODE";

  // Fetch stream token on mount so video can play with auth
  useEffect(() => {
    let cancelled = false;
    async function fetchToken() {
      try {
        const res = await import("@evey/api").then((m) =>
          m.eveyFetch<{ token: string }>({
            url: `/api/v1/media/${mediaId}/stream-token`,
            method: "POST",
          }),
        );
        if (!cancelled) setStreamToken(res.token);
      } catch {
        // stream-token endpoint may not exist; fall back to Bearer-only streaming
        if (!cancelled) setStreamToken(null);
      }
    }
    void fetchToken();
    return () => { cancelled = true; };
  }, [mediaId]);

  // Sync server segments → local state (don't overwrite dirty entries)
  useEffect(() => {
    if (!serverSegments) return;
    setLocalSegments((prev) => {
      const dirtyTypes = new Set(prev.filter((s) => s.isDirty).map((s) => s.type));
      const updated: LocalSegment[] = serverSegments.map((ss) => {
        if (dirtyTypes.has(ss.type)) {
          return prev.find((p) => p.type === ss.type)!;
        }
        return { ...ss, isDirty: false };
      });
      // Keep dirty local-only segments (added but not yet on server)
      for (const p of prev) {
        if (p.isDirty && !updated.find((u) => u.type === p.type)) {
          updated.push(p);
        }
      }
      return updated;
    });
  }, [serverSegments]);

  const streamUrl = streamToken
    ? `/api/v1/media/${mediaId}/stream?token=${encodeURIComponent(streamToken)}`
    : null;

  function handleSegmentChange(type: string, startMs: number, endMs: number) {
    setLocalSegments((prev) =>
      prev.map((s) => (s.type === type ? { ...s, startMs, endMs, isDirty: true } : s)),
    );
  }

  function markStart() {
    if (!selectedType || !videoRef.current) return;
    const ms = Math.round(videoRef.current.currentTime * 1000);
    setLocalSegments((prev) =>
      prev.map((s) =>
        s.type === selectedType
          ? { ...s, startMs: ms, isDirty: true }
          : s,
      ),
    );
  }

  function markEnd() {
    if (!selectedType || !videoRef.current) return;
    const ms = Math.round(videoRef.current.currentTime * 1000);
    setLocalSegments((prev) =>
      prev.map((s) =>
        s.type === selectedType
          ? { ...s, endMs: ms, isDirty: true }
          : s,
      ),
    );
  }

  function addSegment(type: SegmentType) {
    if (localSegments.find((s) => s.type === type)) return;
    const startMs = Math.round(currentMs);
    const endMs = startMs + 5000;
    setLocalSegments((prev) => [
      ...prev,
      { type, startMs, endMs, confidence: 1, source: "manual", isDirty: true },
    ]);
    setSelectedType(type);
  }

  async function handleSaveAll() {
    const dirty = localSegments.filter((s) => s.isDirty);
    if (dirty.length === 0) {
      toast.info(t("admin.segments.noChanges"));
      return;
    }
    try {
      await Promise.all(
        dirty.map((seg) =>
          setSegment.mutateAsync({ type: seg.type, startMs: seg.startMs, endMs: seg.endMs }),
        ),
      );
      toast.success(t("admin.segments.save") + " - OK");
    } catch (err) {
      if (import.meta.env.DEV) console.error("[AdminSegments] save error", err);
      toast.error(t("admin.segments.saveError"));
    }
  }

  async function handleDelete(type: string) {
    try {
      await deleteSegment.mutateAsync(type);
      setLocalSegments((prev) => prev.filter((s) => s.type !== type));
      if (selectedType === type) setSelectedType(null);
      toast.success(t("admin.segments.delete") + " - OK");
    } catch (err) {
      if (import.meta.env.DEV) console.error("[AdminSegments] delete error", err);
      toast.error(t("admin.segments.deleteError"));
    }
  }

  async function handleRedetect() {
    try {
      await triggerDetect.mutateAsync();
      toast.success(t("admin.segments.redetectSuccess"));
    } catch (err) {
      if (import.meta.env.DEV) console.error("[AdminSegments] redetect error", err);
      toast.error(t("admin.segments.redetectError"));
    }
  }

  async function handlePropagate(type: string) {
    try {
      const res = await propagate.mutateAsync(type);
      toast.success(
        t("admin.segments.propagateSuccess", { count: res.count }),
      );
    } catch (err) {
      if (import.meta.env.DEV) console.error("[AdminSegments] propagate error", err);
      toast.error(t("admin.segments.propagateError"));
    }
  }

  const manualSegmentTypes = localSegments
    .filter((s) => s.source === "manual" || s.source === "manual_propagated")
    .map((s) => s.type);

  const availableToAdd = SEGMENT_TYPES.filter(
    (t) => !localSegments.find((s) => s.type === t),
  );

  const isLoading = itemLoading || segLoading;
  const isDirtyAny = localSegments.some((s) => s.isDirty);

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={
          mediaItem
            ? `${t("admin.segments.title")} - ${itemLabel(mediaItem, t) || mediaId}`
            : t("admin.segments.title")
        }
        actions={
          <Link
            to="/admin"
            className="flex items-center gap-1 text-sm text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
          >
            <ChevronLeft size={ICON.sm} />
            {t("common.nav.admin")}
          </Link>
        }
      />

      {isLoading && (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      )}

      {!isLoading && (
        <>
          {/* Section A: Mini video player */}
          <Card>
            <h2 className="text-sm font-semibold uppercase tracking-wide mb-3 text-[var(--text-muted)]">
              {t("admin.segments.preview")}
            </h2>
            {streamUrl ? (
              <video
                ref={videoRef}
                src={streamUrl}
                controls
                preload="metadata"
                className="w-full max-w-2xl rounded-md bg-black"
                onLoadedMetadata={() =>
                  setDurationMs((videoRef.current?.duration ?? 0) * 1000)
                }
                onTimeUpdate={() =>
                  setCurrentMs((videoRef.current?.currentTime ?? 0) * 1000)
                }
              />
            ) : (
              <div className="w-full max-w-2xl h-40 flex items-center justify-center rounded-md bg-black text-[var(--text-muted)] text-sm">
                {t("admin.segments.playerLoading")}
              </div>
            )}

            <div className="flex gap-2 mt-3 flex-wrap">
              <Button
                variant="secondary"
                onClick={markStart}
                disabled={!selectedType}
                className="text-xs"
              >
                {t("admin.segments.markStart")}
              </Button>
              <Button
                variant="secondary"
                onClick={markEnd}
                disabled={!selectedType}
                className="text-xs"
              >
                {t("admin.segments.markEnd")}
              </Button>
              {!selectedType && (
                <span className="text-xs text-[var(--text-muted)] self-center">
                  {t("admin.segments.selectHint")}
                </span>
              )}
            </div>
          </Card>

          {/* Section B: Timeline + editor */}
          <Card>
            <h2 className="text-sm font-semibold uppercase tracking-wide mb-3 text-[var(--text-muted)]">
              {t("admin.segments.timeline")}
            </h2>

            {segError && localSegments.length === 0 ? (
              // "No segments" would send an admin re-running detection on a
              // title whose segments simply could not be read.
              <FetchErrorState error={segErr} onRetry={() => void refetchSegments()} />
            ) : localSegments.length === 0 && !isLoading ? (
              <div className="py-6 text-center text-sm text-[var(--text-muted)]">
                {t("admin.segments.emptyState")}
              </div>
            ) : (
              <SegmentTimeline
                segments={localSegments}
                durationMs={durationMs}
                selectedType={selectedType}
                onSelectSegment={setSelectedType}
                onSegmentChange={handleSegmentChange}
                currentMs={currentMs}
              />
            )}

            {/* mm:ss input override for selected segment */}
            {selectedType && localSegments.find((s) => s.type === selectedType) && (
              <MmSsInputs
                segment={localSegments.find((s) => s.type === selectedType)!}
                onChange={(startMs, endMs) => handleSegmentChange(selectedType, startMs, endMs)}
              />
            )}

            {/* Add segment dropdown */}
            {availableToAdd.length > 0 && (
              <div className="mt-4 flex items-center gap-2 flex-wrap">
                <span className="text-xs text-[var(--text-muted)]">{t("admin.segments.addLabel")}</span>
                {availableToAdd.map((type) => (
                  <button
                    key={type}
                    type="button"
                    onClick={() => addSegment(type)}
                    className="flex items-center gap-1 text-xs px-2 py-1 rounded border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] hover:bg-[var(--border)] transition-colors"
                  >
                    <Plus size={ICON.xs} />
                    {type.replace(/_/g, " ")}
                  </button>
                ))}
              </div>
            )}
          </Card>

          {/* Section C: Action buttons */}
          <Card>
            <h2 className="text-sm font-semibold uppercase tracking-wide mb-3 text-[var(--text-muted)]">
              {t("common.actions")}
            </h2>
            <div className="flex flex-wrap gap-3">
              <Button
                onClick={handleSaveAll}
                loading={setSegment.isPending}
                disabled={!isDirtyAny}
              >
                <Save size={ICON.sm} />
                {t("admin.segments.save")}
              </Button>

              <Button
                variant="secondary"
                onClick={handleRedetect}
                loading={triggerDetect.isPending}
              >
                <RefreshCw size={ICON.sm} />
                {t("admin.segments.redetect")}
              </Button>

              {selectedType && (
                <Button
                  variant="danger"
                  onClick={() => handleDelete(selectedType)}
                  loading={deleteSegment.isPending}
                >
                  <Trash2 size={ICON.sm} />
                  {t("admin.segments.delete")}
                </Button>
              )}

              {isEpisode && manualSegmentTypes.length > 0 && (
                <PropagateButton
                  types={manualSegmentTypes}
                  isPending={propagate.isPending}
                  onPropagate={handlePropagate}
                  label={t("admin.segments.propagate")}
                />
              )}
            </div>

            <p className="mt-3 flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
              <Lock size={ICON.xs} />
              {t("admin.segments.lockedHint")}
            </p>
          </Card>
        </>
      )}
    </div>
  );
}

// Inline mm:ss text inputs for precise control
function MmSsInputs({
  segment,
  onChange,
}: {
  segment: MediaSegment;
  onChange: (startMs: number, endMs: number) => void;
}) {
  const { t } = useTranslation();
  const [startVal, setStartVal] = useState(msToMmSs(segment.startMs));
  const [endVal, setEndVal] = useState(msToMmSs(segment.endMs));

  // Keep in sync when segment changes (e.g. via slider)
  useEffect(() => { setStartVal(msToMmSs(segment.startMs)); }, [segment.startMs]);
  useEffect(() => { setEndVal(msToMmSs(segment.endMs)); }, [segment.endMs]);

  function commitStart() {
    const ms = mmSsToMs(startVal);
    if (ms !== null && ms < segment.endMs) {
      onChange(ms, segment.endMs);
    } else {
      setStartVal(msToMmSs(segment.startMs));
    }
  }

  function commitEnd() {
    const ms = mmSsToMs(endVal);
    if (ms !== null && ms > segment.startMs) {
      onChange(segment.startMs, ms);
    } else {
      setEndVal(msToMmSs(segment.endMs));
    }
  }

  return (
    <div className="mt-3 flex items-center gap-3 flex-wrap">
      <div className="flex items-center gap-1.5">
        <label htmlFor={`seg-start-${segment.type}`} className="text-xs text-[var(--text-muted)]">{t("admin.segments.startLabel")}</label>
        <input
          id={`seg-start-${segment.type}`}
          className="w-20 px-2 py-1 rounded text-xs font-mono bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
          value={startVal}
          onChange={(e) => setStartVal(e.target.value)}
          onBlur={commitStart}
          onKeyDown={(e) => { if (e.key === "Enter") commitStart(); }}
          placeholder="00:00"
        />
      </div>
      <div className="flex items-center gap-1.5">
        <label htmlFor={`seg-end-${segment.type}`} className="text-xs text-[var(--text-muted)]">{t("admin.segments.endLabel")}</label>
        <input
          id={`seg-end-${segment.type}`}
          className="w-20 px-2 py-1 rounded text-xs font-mono bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
          value={endVal}
          onChange={(e) => setEndVal(e.target.value)}
          onBlur={commitEnd}
          onKeyDown={(e) => { if (e.key === "Enter") commitEnd(); }}
          placeholder="00:30"
        />
      </div>
    </div>
  );
}

// Propagate button - shows dropdown if multiple types, direct click if one
function PropagateButton({
  types,
  isPending,
  onPropagate,
  label,
}: {
  types: string[];
  isPending: boolean;
  onPropagate: (type: string) => void;
  label: string;
}) {
  const [open, setOpen] = useState(false);

  if (types.length === 1) {
    return (
      <Button
        variant="secondary"
        onClick={() => onPropagate(types[0])}
        loading={isPending}
      >
        {label}
      </Button>
    );
  }

  return (
    <div className="relative">
      <Button
        variant="secondary"
        onClick={() => setOpen((v) => !v)}
        loading={isPending}
      >
        {label}
      </Button>
      {open && (
        <div className="absolute top-full mt-1 left-0 z-20 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface)] shadow-md min-w-[140px]">
          {types.map((type) => (
            <button
              key={type}
              type="button"
              className="w-full text-left px-3 py-2 text-sm text-[var(--text)] hover:bg-[var(--surface-raised)] capitalize"
              onClick={() => { setOpen(false); onPropagate(type); }}
            >
              {type.replace(/_/g, " ")}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
