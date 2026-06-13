import { Lock } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { MediaSegment } from "@/hooks/useAdminSegments";

export type { MediaSegment };

// Raw Tailwind colors are deliberate here: admin-only segment visualisation
// uses fixed semantic hues (intro=blue, recap=purple, credits=orange, studio
// logo=yellow) so admins can recognise the type at a glance regardless of the
// active theme — same rationale as error reds or success greens.
const SEGMENT_COLORS: Record<string, string> = {
  intro: "bg-blue-500",
  recap: "bg-purple-500",
  end_credits: "bg-orange-500",
  studio_logo: "bg-yellow-500",
};

const SEGMENT_BORDER_COLORS: Record<string, string> = {
  intro: "border-blue-500",
  recap: "border-purple-500",
  end_credits: "border-orange-500",
  studio_logo: "border-yellow-500",
};

function formatMs(ms: number): string {
  const totalSec = Math.floor(ms / 1000);
  const m = Math.floor(totalSec / 60);
  const s = totalSec % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

function isManualSource(source: string): boolean {
  return source === "manual" || source === "manual_propagated";
}

interface SegmentBlockProps {
  segment: MediaSegment;
  durationMs: number;
  isSelected: boolean;
  onSelect: () => void;
}

function SegmentBlock({
  segment,
  durationMs,
  isSelected,
  onSelect,
}: SegmentBlockProps) {
  const { t } = useTranslation();
  const leftPct = durationMs > 0 ? (segment.startMs / durationMs) * 100 : 0;
  const widthPct = durationMs > 0 ? ((segment.endMs - segment.startMs) / durationMs) * 100 : 0;
  const color = SEGMENT_COLORS[segment.type] ?? "bg-gray-500";
  const borderColor = SEGMENT_BORDER_COLORS[segment.type] ?? "border-gray-500";
  const manual = isManualSource(segment.source);

  return (
    <div
      role="button"
      tabIndex={0}
      data-testid={`segment-block-${segment.type}`}
      className={`absolute top-1 h-8 rounded cursor-pointer transition-all ${color} bg-opacity-70 border-2 ${isSelected ? `${borderColor} opacity-100` : "border-transparent opacity-60"}`}
      style={{ left: `${leftPct}%`, width: `${Math.max(widthPct, 0.5)}%` }}
      onClick={onSelect}
      onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onSelect(); } }}
      aria-label={`${segment.type} segment`}
      aria-pressed={isSelected}
    >
      {manual && (
        <Lock
          size={10}
          className="absolute top-0.5 right-0.5 text-white"
          aria-label={t("admin.segments.locked")}
          data-testid="lock-icon"
        />
      )}
    </div>
  );
}

interface SegmentEditorProps {
  segment: MediaSegment;
  durationMs: number;
  onStartChange: (ms: number) => void;
  onEndChange: (ms: number) => void;
}

function SegmentEditor({ segment, durationMs, onStartChange, onEndChange }: SegmentEditorProps) {
  const manual = isManualSource(segment.source);

  return (
    <div className="flex flex-col gap-2 p-3 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface-raised)]">
      <div className="flex items-center gap-2 flex-wrap">
        <span className="text-sm font-medium text-[var(--text)] capitalize">
          {segment.type.replace(/_/g, " ")}
        </span>
        <span className="text-xs text-[var(--text-muted)]">
          {formatMs(segment.startMs)} / {formatMs(segment.endMs)}
        </span>
        <span className="text-xs px-1.5 py-0.5 rounded bg-[var(--surface)] text-[var(--text-muted)]">
          {Math.round((segment.confidence ?? 0) * 100)}% — {segment.source}
        </span>
        {manual && (
          <span
            className="flex items-center gap-1 text-xs px-1.5 py-0.5 rounded bg-[var(--accent-subtle)] text-[var(--accent)]"
            title="Ce segment est verrouillé manuellement et ne sera pas écrasé par la détection auto."
          >
            <Lock size={10} />
            verrouillé
          </span>
        )}
      </div>

      <div className="flex flex-col gap-1">
        <label className="text-xs text-[var(--text-muted)]">
          Début : {formatMs(segment.startMs)}
        </label>
        <input
          type="range"
          min={0}
          max={durationMs}
          step={500}
          value={segment.startMs}
          data-testid={`start-slider-${segment.type}`}
          className="w-full accent-[var(--accent)]"
          onChange={(e) => {
            const v = Number(e.target.value);
            if (v < segment.endMs) onStartChange(v);
          }}
        />
      </div>

      <div className="flex flex-col gap-1">
        <label className="text-xs text-[var(--text-muted)]">
          Fin : {formatMs(segment.endMs)}
        </label>
        <input
          type="range"
          min={0}
          max={durationMs}
          step={500}
          value={segment.endMs}
          data-testid={`end-slider-${segment.type}`}
          className="w-full accent-[var(--accent)]"
          onChange={(e) => {
            const v = Number(e.target.value);
            if (v > segment.startMs) onEndChange(v);
          }}
        />
      </div>
    </div>
  );
}

interface SegmentTimelineProps {
  segments: MediaSegment[];
  durationMs: number;
  selectedType: string | null;
  onSelectSegment: (type: string) => void;
  onSegmentChange: (type: string, startMs: number, endMs: number) => void;
  currentMs?: number;
}

export function SegmentTimeline({
  segments,
  durationMs,
  selectedType,
  onSelectSegment,
  onSegmentChange,
  currentMs,
}: SegmentTimelineProps) {
  const selected = segments.find((s) => s.type === selectedType) ?? null;

  return (
    <div className="flex flex-col gap-3">
      {/* Timeline bar */}
      <div
        className="relative w-full h-10 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] overflow-visible"
        data-testid="timeline-bar"
      >
        {/* Playhead */}
        {durationMs > 0 && currentMs !== undefined && (
          <div
            className="absolute top-0 bottom-0 w-0.5 bg-white opacity-80 pointer-events-none z-10"
            style={{ left: `${(currentMs / durationMs) * 100}%` }}
            aria-hidden="true"
          />
        )}

        {segments.map((seg) => (
          <SegmentBlock
            key={seg.type}
            segment={seg}
            durationMs={durationMs}
            isSelected={selectedType === seg.type}
            onSelect={() => onSelectSegment(seg.type)}
          />
        ))}
      </div>

      {/* Legend */}
      <div className="flex flex-wrap gap-3">
        {segments.map((seg) => {
          const color = SEGMENT_COLORS[seg.type] ?? "bg-gray-500";
          const manual = isManualSource(seg.source);
          return (
            <button
              key={seg.type}
              type="button"
              onClick={() => onSelectSegment(seg.type)}
              className={`flex items-center gap-1.5 text-xs px-2 py-1 rounded border transition-colors ${selectedType === seg.type ? "border-[var(--accent)] bg-[var(--accent-subtle)]" : "border-[var(--border)] bg-[var(--surface)]"} text-[var(--text)]`}
            >
              <span className={`w-2.5 h-2.5 rounded-sm inline-block ${color}`} />
              <span className="capitalize">{seg.type.replace(/_/g, " ")}</span>
              {manual && <Lock size={10} className="text-[var(--accent)]" />}
            </button>
          );
        })}
      </div>

      {/* Selected segment editor */}
      {selected && (
        <SegmentEditor
          segment={selected}
          durationMs={durationMs}
          onStartChange={(ms) => onSegmentChange(selected.type, ms, selected.endMs)}
          onEndChange={(ms) => onSegmentChange(selected.type, selected.startMs, ms)}
        />
      )}
    </div>
  );
}
