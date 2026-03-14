import type { V1MediaItem } from "@evey/api/src/generated/library.schemas";
import { Button } from "@/components/ui/Button";
import type { LibrarySearch } from "@/routes/_auth/library/$libraryId";

// Resolution bands derived from stream height
export function getResolutionLabel(height: number): string {
  if (height >= 2160) return "4K";
  if (height >= 1080) return "1080p";
  if (height >= 720) return "720p";
  return "SD";
}

const RESOLUTION_OPTIONS = ["4K", "1080p", "720p", "SD"] as const;
type Resolution = (typeof RESOLUTION_OPTIONS)[number];

// Normalize codec strings for display
function normalizeCodec(codec: string): string {
  const c = codec.toLowerCase();
  if (c === "hevc" || c === "h265" || c === "h.265") return "H.265/HEVC";
  if (c === "h264" || c === "avc" || c === "h.264") return "H.264";
  if (c === "av1") return "AV1";
  if (c === "vp9") return "VP9";
  if (c === "vp8") return "VP8";
  if (c === "mpeg4" || c === "xvid" || c === "divx") return "MPEG-4";
  if (c === "mpeg2video") return "MPEG-2";
  return codec.toUpperCase();
}

export function collectFilterOptions(items: V1MediaItem[]) {
  const resolutions = new Set<string>();
  const codecs = new Set<string>();
  const audioLangs = new Set<string>();
  const subLangs = new Set<string>();

  for (const item of items) {
    for (const stream of item.streams ?? []) {
      if (stream.type === "MEDIA_STREAM_TYPE_VIDEO") {
        if (stream.height) resolutions.add(getResolutionLabel(stream.height));
        if (stream.codec) codecs.add(normalizeCodec(stream.codec));
      } else if (stream.type === "MEDIA_STREAM_TYPE_AUDIO") {
        if (stream.language && stream.language !== "und") audioLangs.add(stream.language);
      } else if (stream.type === "MEDIA_STREAM_TYPE_SUBTITLE") {
        if (stream.language && stream.language !== "und") subLangs.add(stream.language);
      }
    }
  }

  return {
    resolutions: RESOLUTION_OPTIONS.filter((r) => resolutions.has(r)),
    codecs: Array.from(codecs).sort(),
    audioLangs: Array.from(audioLangs).sort(),
    subLangs: Array.from(subLangs).sort(),
  };
}

export function applyLocalFilters(
  items: V1MediaItem[],
  filters: Pick<LibrarySearch, "resolutions" | "codecs" | "audioLangs" | "subLangs">,
): V1MediaItem[] {
  const { resolutions, codecs, audioLangs, subLangs } = filters;
  const hasRes = resolutions && resolutions.length > 0;
  const hasCodec = codecs && codecs.length > 0;
  const hasAudio = audioLangs && audioLangs.length > 0;
  const hasSub = subLangs && subLangs.length > 0;
  if (!hasRes && !hasCodec && !hasAudio && !hasSub) return items;

  return items.filter((item) => {
    const streams = item.streams ?? [];
    const videoStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
    const audioStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO");
    const subStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE");

    if (hasRes) {
      const itemRes = videoStreams.map((s) => (s.height ? getResolutionLabel(s.height) : "SD"));
      if (!resolutions!.some((r) => itemRes.includes(r as Resolution))) return false;
    }
    if (hasCodec) {
      const itemCodecs = videoStreams.map((s) => (s.codec ? normalizeCodec(s.codec) : ""));
      if (!codecs!.some((c) => itemCodecs.includes(c))) return false;
    }
    if (hasAudio) {
      const itemAudioLangs = audioStreams.map((s) => s.language ?? "");
      if (!audioLangs!.some((l) => itemAudioLangs.includes(l))) return false;
    }
    if (hasSub) {
      const itemSubLangs = subStreams.map((s) => s.language ?? "");
      if (!subLangs!.some((l) => itemSubLangs.includes(l))) return false;
    }
    return true;
  });
}

function countActiveFilters(search: LibrarySearch): number {
  let count = 0;
  if (search.genre) count++;
  if (search.resolutions?.length) count += search.resolutions.length;
  if (search.codecs?.length) count += search.codecs.length;
  if (search.audioLangs?.length) count += search.audioLangs.length;
  if (search.subLangs?.length) count += search.subLangs.length;
  return count;
}

function CheckboxGroup({
  label,
  options,
  selected,
  onChange,
}: {
  label: string;
  options: string[];
  selected: string[];
  onChange: (values: string[]) => void;
}) {
  if (options.length === 0) return null;
  return (
    <div className="mb-4">
      <p className="text-xs font-semibold mb-2 uppercase tracking-wide" style={{ color: "var(--text-muted)" }}>
        {label}
      </p>
      <div className="flex flex-col gap-1.5">
        {options.map((opt) => {
          const checked = selected.includes(opt);
          return (
            <label key={opt} className="flex items-center gap-2 cursor-pointer group">
              <span
                role="checkbox"
                aria-checked={checked}
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === " " || e.key === "Enter") {
                    e.preventDefault();
                    onChange(checked ? selected.filter((v) => v !== opt) : [...selected, opt]);
                  }
                }}
                onClick={() => onChange(checked ? selected.filter((v) => v !== opt) : [...selected, opt])}
                className="flex-shrink-0 w-4 h-4 rounded border flex items-center justify-center transition-colors"
                style={{
                  backgroundColor: checked ? "var(--accent)" : "var(--surface-raised)",
                  borderColor: checked ? "var(--accent)" : "var(--border)",
                }}
              >
                {checked && (
                  <svg className="w-2.5 h-2.5 text-white" fill="none" stroke="currentColor" strokeWidth={3} viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
                )}
              </span>
              <span className="text-sm group-hover:text-[var(--text)] transition-colors" style={{ color: "var(--text-muted)" }}>
                {opt}
              </span>
            </label>
          );
        })}
      </div>
    </div>
  );
}

function GenreSelect({
  genres,
  value,
  onChange,
}: {
  genres: string[];
  value: string;
  onChange: (v: string) => void;
}) {
  if (genres.length === 0) return null;
  return (
    <div className="mb-4">
      <p className="text-xs font-semibold mb-2 uppercase tracking-wide" style={{ color: "var(--text-muted)" }}>
        Genre
      </p>
      <div className="relative">
        <select
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="w-full appearance-none px-3 py-2 pr-8 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
        >
          <option value="">All genres</option>
          {genres.map((g) => (
            <option key={g} value={g}>{g}</option>
          ))}
        </select>
        <svg
          className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 w-4 h-4"
          style={{ color: "var(--text-muted)" }}
          fill="none"
          stroke="currentColor"
          strokeWidth={2}
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M6 9l6 6 6-6" />
        </svg>
      </div>
    </div>
  );
}

interface LibraryFiltersProps {
  search: LibrarySearch;
  genres: string[];
  availableOptions: {
    resolutions: string[];
    codecs: string[];
    audioLangs: string[];
    subLangs: string[];
  };
  onUpdate: (patch: Partial<LibrarySearch>) => void;
}

export function LibraryFilterToggle({
  search,
  onClick,
}: {
  search: LibrarySearch;
  onClick: () => void;
}) {
  const count = countActiveFilters(search);
  return (
    <button
      type="button"
      onClick={onClick}
      className="relative flex items-center gap-1.5 px-3 py-2 rounded-[var(--radius-md)] border transition-colors text-sm"
      style={{
        backgroundColor: count > 0 ? "var(--accent)" : "var(--surface-raised)",
        color: count > 0 ? "#fff" : "var(--text-muted)",
        borderColor: count > 0 ? "var(--accent)" : "var(--border)",
      }}
      title="Toggle filters"
      data-testid="filter-toggle"
    >
      <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 4.5h17.25M3 9h11.25m-11.25 4.5h8.25M3 18h5.25" />
      </svg>
      Filters
      {count > 0 && (
        <span
          className="inline-flex items-center justify-center w-4 h-4 rounded-full text-xs font-bold"
          style={{ backgroundColor: "#fff", color: "var(--accent)" }}
        >
          {count}
        </span>
      )}
    </button>
  );
}

export function LibraryFiltersPanel({
  search,
  genres,
  availableOptions,
  onUpdate,
}: LibraryFiltersProps) {
  const activeCount = countActiveFilters(search);

  return (
    <div
      className="rounded-[var(--radius-md)] p-4"
      style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)", minWidth: "200px" }}
      data-testid="filter-panel"
    >
      <div className="flex items-center justify-between mb-4">
        <span className="text-sm font-semibold" style={{ color: "var(--text)" }}>Filters</span>
        {activeCount > 0 && (
          <Button
            variant="ghost"
            className="text-xs !px-2 !py-1"
            onClick={() =>
              onUpdate({
                genre: undefined,
                resolutions: undefined,
                codecs: undefined,
                audioLangs: undefined,
                subLangs: undefined,
              })
            }
          >
            Clear all
          </Button>
        )}
      </div>

      <GenreSelect
        genres={genres}
        value={search.genre ?? ""}
        onChange={(v) => onUpdate({ genre: v || undefined })}
      />

      <CheckboxGroup
        label="Resolution"
        options={availableOptions.resolutions}
        selected={search.resolutions ?? []}
        onChange={(v) => onUpdate({ resolutions: v.length ? v : undefined })}
      />

      <CheckboxGroup
        label="Video Codec"
        options={availableOptions.codecs}
        selected={search.codecs ?? []}
        onChange={(v) => onUpdate({ codecs: v.length ? v : undefined })}
      />

      <CheckboxGroup
        label="Audio Language"
        options={availableOptions.audioLangs}
        selected={search.audioLangs ?? []}
        onChange={(v) => onUpdate({ audioLangs: v.length ? v : undefined })}
      />

      <CheckboxGroup
        label="Subtitle Language"
        options={availableOptions.subLangs}
        selected={search.subLangs ?? []}
        onChange={(v) => onUpdate({ subLangs: v.length ? v : undefined })}
      />

      {availableOptions.resolutions.length === 0 &&
        availableOptions.codecs.length === 0 &&
        availableOptions.audioLangs.length === 0 &&
        availableOptions.subLangs.length === 0 &&
        genres.length === 0 && (
          <p className="text-xs" style={{ color: "var(--text-muted)" }}>
            No filter options available for this library yet.
          </p>
        )}
    </div>
  );
}

export { countActiveFilters };
