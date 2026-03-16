import type { V1MediaItem, LibraryServiceListMediaItemsType } from "@evey/api/src/generated/library.schemas";
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
  filters: Pick<LibrarySearch, "genres" | "resolutions" | "codecs" | "audioLangs" | "subLangs" | "yearFrom" | "yearTo" | "minRating">,
): V1MediaItem[] {
  const { genres, resolutions, codecs, audioLangs, subLangs, yearFrom, yearTo, minRating } = filters;
  const hasGenre = genres && genres.length > 0;
  const hasRes = resolutions && resolutions.length > 0;
  const hasCodec = codecs && codecs.length > 0;
  const hasAudio = audioLangs && audioLangs.length > 0;
  const hasSub = subLangs && subLangs.length > 0;
  const hasYearFrom = yearFrom != null;
  const hasYearTo = yearTo != null;
  const hasMinRating = minRating != null && minRating > 0;
  if (!hasGenre && !hasRes && !hasCodec && !hasAudio && !hasSub && !hasYearFrom && !hasYearTo && !hasMinRating) return items;

  return items.filter((item) => {
    const streams = item.streams ?? [];
    const videoStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
    const audioStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO");
    const subStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE");

    if (hasGenre) {
      const itemGenres = item.genres ?? [];
      if (!genres!.some((g) => itemGenres.includes(g))) return false;
    }
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
    if (hasYearFrom && (item.year == null || item.year < yearFrom!)) return false;
    if (hasYearTo && (item.year == null || item.year > yearTo!)) return false;
    if (hasMinRating && (item.communityRating == null || item.communityRating < minRating!)) return false;
    return true;
  });
}

function countActiveFilters(search: LibrarySearch): number {
  let count = 0;
  if (search.type) count++;
  if (search.genre) count++;
  if (search.genres?.length) count += search.genres.length;
  if (search.resolutions?.length) count += search.resolutions.length;
  if (search.codecs?.length) count += search.codecs.length;
  if (search.audioLangs?.length) count += search.audioLangs.length;
  if (search.subLangs?.length) count += search.subLangs.length;
  if (search.yearFrom != null) count++;
  if (search.yearTo != null) count++;
  if (search.minRating != null && search.minRating > 0) count++;
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

function YearRangeFilter({
  yearFrom,
  yearTo,
  onChange,
}: {
  yearFrom?: number;
  yearTo?: number;
  onChange: (from: number | undefined, to: number | undefined) => void;
}) {
  return (
    <div className="mb-4">
      <p className="text-xs font-semibold mb-2 uppercase tracking-wide" style={{ color: "var(--text-muted)" }}>
        Year
      </p>
      <div className="flex items-center gap-2">
        <input
          type="number"
          placeholder="From"
          value={yearFrom ?? ""}
          min={1900}
          max={new Date().getFullYear() + 1}
          onChange={(e) => {
            const val = e.target.value ? parseInt(e.target.value, 10) : undefined;
            onChange(val, yearTo);
          }}
          className="w-full px-2 py-1.5 rounded-[var(--radius-md)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
          style={{
            backgroundColor: "var(--surface-raised)",
            border: "1px solid var(--border)",
            color: "var(--text)",
          }}
        />
        <span className="flex-shrink-0 text-xs" style={{ color: "var(--text-muted)" }}>—</span>
        <input
          type="number"
          placeholder="To"
          value={yearTo ?? ""}
          min={1900}
          max={new Date().getFullYear() + 1}
          onChange={(e) => {
            const val = e.target.value ? parseInt(e.target.value, 10) : undefined;
            onChange(yearFrom, val);
          }}
          className="w-full px-2 py-1.5 rounded-[var(--radius-md)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
          style={{
            backgroundColor: "var(--surface-raised)",
            border: "1px solid var(--border)",
            color: "var(--text)",
          }}
        />
      </div>
    </div>
  );
}

const RATING_OPTIONS = [1, 2, 3, 4, 5] as const;

function RatingFilter({
  minRating,
  onChange,
}: {
  minRating?: number;
  onChange: (v: number | undefined) => void;
}) {
  return (
    <div className="mb-4">
      <p className="text-xs font-semibold mb-2 uppercase tracking-wide" style={{ color: "var(--text-muted)" }}>
        Min Rating
      </p>
      <div className="flex items-center gap-1">
        {RATING_OPTIONS.map((star) => {
          const active = minRating != null && star <= minRating;
          return (
            <button
              key={star}
              type="button"
              onClick={() => onChange(minRating === star ? undefined : star)}
              title={`Min ${star} star${star > 1 ? "s" : ""}`}
              className="p-0.5 transition-colors"
              style={{ color: active ? "var(--accent)" : "var(--border)" }}
            >
              <svg className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
                <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
              </svg>
            </button>
          );
        })}
        {minRating != null && (
          <button
            type="button"
            onClick={() => onChange(undefined)}
            className="ml-1 text-xs hover:underline"
            style={{ color: "var(--text-muted)" }}
          >
            Clear
          </button>
        )}
      </div>
    </div>
  );
}

const TYPE_OPTIONS: Array<{ label: string; value: LibraryServiceListMediaItemsType | "" }> = [
  { label: "All", value: "" },
  { label: "Movie", value: "MEDIA_ITEM_TYPE_MOVIE" },
  { label: "Show", value: "MEDIA_ITEM_TYPE_SERIES" },
  { label: "Episode", value: "MEDIA_ITEM_TYPE_EPISODE" },
  { label: "Track", value: "MEDIA_ITEM_TYPE_TRACK" },
  { label: "Photo", value: "MEDIA_ITEM_TYPE_PHOTO" },
];

function TypeFilter({
  value,
  onChange,
}: {
  value?: LibraryServiceListMediaItemsType;
  onChange: (v: LibraryServiceListMediaItemsType | undefined) => void;
}) {
  return (
    <div className="mb-4">
      <p className="text-xs font-semibold mb-2 uppercase tracking-wide" style={{ color: "var(--text-muted)" }}>
        Type
      </p>
      <select
        value={value ?? ""}
        onChange={(e) => onChange((e.target.value as LibraryServiceListMediaItemsType) || undefined)}
        className="w-full px-2 py-1.5 rounded-[var(--radius-md)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
        style={{
          backgroundColor: "var(--surface-raised)",
          border: "1px solid var(--border)",
          color: "var(--text)",
        }}
      >
        {TYPE_OPTIONS.map((o) => (
          <option key={o.value} value={o.value}>{o.label}</option>
        ))}
      </select>
    </div>
  );
}

function GenreTextFilter({
  value,
  onChange,
}: {
  value?: string;
  onChange: (v: string | undefined) => void;
}) {
  return (
    <div className="mb-4">
      <p className="text-xs font-semibold mb-2 uppercase tracking-wide" style={{ color: "var(--text-muted)" }}>
        Genre (text)
      </p>
      <input
        type="text"
        placeholder="e.g. Action"
        value={value ?? ""}
        onChange={(e) => onChange(e.target.value || undefined)}
        className="w-full px-2 py-1.5 rounded-[var(--radius-md)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
        style={{
          backgroundColor: "var(--surface-raised)",
          border: "1px solid var(--border)",
          color: "var(--text)",
        }}
      />
    </div>
  );
}

const PREDEFINED_GENRES = [
  "Action", "Adventure", "Animation", "Comedy", "Crime",
  "Documentary", "Drama", "Fantasy", "Horror", "Mystery",
  "Romance", "Sci-Fi", "Thriller", "Western",
] as const;

function GenreChips({
  availableGenres,
  selected,
  onChange,
}: {
  availableGenres: string[];
  selected: string[];
  onChange: (v: string[]) => void;
}) {
  // Merge predefined list with any genres found in library data
  const merged = Array.from(new Set([...PREDEFINED_GENRES, ...availableGenres])).sort();
  if (merged.length === 0) return null;
  return (
    <div className="mb-4">
      <p className="text-xs font-semibold mb-2 uppercase tracking-wide" style={{ color: "var(--text-muted)" }}>
        Genre
      </p>
      <div className="flex flex-wrap gap-1.5 max-h-36 overflow-y-auto pr-1">
        {merged.map((g) => {
          const active = selected.includes(g);
          return (
            <button
              key={g}
              type="button"
              onClick={() => onChange(active ? selected.filter((v) => v !== g) : [...selected, g])}
              className="px-2 py-0.5 rounded-full text-xs font-medium transition-colors"
              style={{
                backgroundColor: active ? "var(--accent)" : "var(--surface-raised)",
                color: active ? "#fff" : "var(--text-muted)",
                border: `1px solid ${active ? "var(--accent)" : "var(--border)"}`,
              }}
            >
              {g}
            </button>
          );
        })}
      </div>
    </div>
  );
}

interface LibraryFiltersProps {
  search: LibrarySearch;
  availableGenres: string[];
  availableOptions: {
    resolutions: string[];
    codecs: string[];
    audioLangs: string[];
    subLangs: string[];
  };
  resultCount?: number;
  onUpdate: (patch: Partial<LibrarySearch>) => void;
}

export function LibraryFilterToggle({
  search,
  resultCount,
  onClick,
}: {
  search: LibrarySearch;
  resultCount?: number;
  onClick: () => void;
}) {
  const count = countActiveFilters(search);
  return (
    <div className="flex items-center gap-2">
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
            style={{ backgroundColor: "var(--filter-active-text)", color: "var(--accent)" }}
          >
            {count}
          </span>
        )}
      </button>
      {count > 0 && resultCount != null && (
        <span className="text-xs" style={{ color: "var(--text-muted)" }} data-testid="filter-result-count">
          {resultCount} result{resultCount !== 1 ? "s" : ""}
        </span>
      )}
    </div>
  );
}

export function LibraryFiltersPanel({
  search,
  availableGenres,
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
                type: undefined,
                genre: undefined,
                genres: undefined,
                resolutions: undefined,
                codecs: undefined,
                audioLangs: undefined,
                subLangs: undefined,
                yearFrom: undefined,
                yearTo: undefined,
                minRating: undefined,
              })
            }
          >
            Clear all
          </Button>
        )}
      </div>

      <TypeFilter
        value={search.type}
        onChange={(v) => onUpdate({ type: v })}
      />

      <GenreTextFilter
        value={search.genre}
        onChange={(v) => onUpdate({ genre: v })}
      />

      <GenreChips
        availableGenres={availableGenres}
        selected={search.genres ?? []}
        onChange={(v) => onUpdate({ genres: v.length ? v : undefined })}
      />

      <YearRangeFilter
        yearFrom={search.yearFrom}
        yearTo={search.yearTo}
        onChange={(from, to) => onUpdate({ yearFrom: from, yearTo: to })}
      />

      <RatingFilter
        minRating={search.minRating}
        onChange={(v) => onUpdate({ minRating: v })}
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
        availableGenres.length === 0 && (
          <p className="text-xs" style={{ color: "var(--text-muted)" }}>
            No filter options available for this library yet.
          </p>
        )}
    </div>
  );
}

export { countActiveFilters };
