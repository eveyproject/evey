import { useMemo } from "react";
import type { V1MediaItem, LibraryServiceListMediaItemsType } from "@evey/api";
import { Check, Star, Filter } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { FilterChip } from "@/components/ui/FilterChip";
import { GenreDot } from "@/components/ui/GenreDot";
import type { LibrarySearch } from "@/routes/_auth/library/$libraryId";
import { ICON } from "@/constants/iconSizes";
import { normalizeLangCode } from "@/features/playback/hooks/hlsPlayerUtils";

// Resolution bands derived from stream height
function getResolutionLabel(height: number): string {
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
        // Normalize so 639-2/B and 639-2/T forms of the same language
        // ("fre"/"fra") collapse into one filter entry instead of two.
        if (stream.language && stream.language !== "und") {
          const norm = normalizeLangCode(stream.language);
          if (norm) audioLangs.add(norm);
        }
      } else if (stream.type === "MEDIA_STREAM_TYPE_SUBTITLE") {
        if (stream.language && stream.language !== "und") {
          const norm = normalizeLangCode(stream.language);
          if (norm) subLangs.add(norm);
        }
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
    // SearchMedia returns id/title stubs carrying no streams at all: no data
    // must read as "unknown, keep", not as a mismatch that would empty the
    // grid the moment a stream facet is combined with a text search.
    const hasStreamInfo = streams.length > 0;

    if (hasGenre) {
      const itemGenres = item.genres ?? [];
      if (!genres!.some((g) => itemGenres.includes(g))) return false;
    }
    if (hasRes && hasStreamInfo) {
      const itemRes = videoStreams.map((s) => (s.height ? getResolutionLabel(s.height) : "SD"));
      if (!resolutions!.some((r) => itemRes.includes(r as Resolution))) return false;
    }
    if (hasCodec && hasStreamInfo) {
      const itemCodecs = videoStreams.map((s) => (s.codec ? normalizeCodec(s.codec) : ""));
      if (!codecs!.some((c) => itemCodecs.includes(c))) return false;
    }
    if (hasAudio && hasStreamInfo) {
      // Normalize both sides so a "fr" filter (picked from a normalized
      // option) still matches a stream tagged "fra" - the raw code stays
      // untouched on the stream itself, only this comparison is normalized.
      const itemAudioLangs = audioStreams.map((s) => normalizeLangCode(s.language ?? ""));
      const wantAudioLangs = audioLangs!.map((l) => normalizeLangCode(l));
      if (!wantAudioLangs.some((l) => itemAudioLangs.includes(l))) return false;
    }
    if (hasSub && hasStreamInfo) {
      const itemSubLangs = subStreams.map((s) => normalizeLangCode(s.language ?? ""));
      const wantSubLangs = subLangs!.map((l) => normalizeLangCode(l));
      if (!wantSubLangs.some((l) => itemSubLangs.includes(l))) return false;
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
  labelFor,
}: {
  label: string;
  options: string[];
  selected: string[];
  onChange: (values: string[]) => void;
  // Renders a friendlier display label than the raw option value (e.g. the
  // localized language name for a normalized ISO 639-1 code) while the value
  // used for selection/matching stays the plain option string.
  labelFor?: (opt: string) => string;
}) {
  if (options.length === 0) return null;
  return (
    <div className="mb-4">
      <p className="text-xs font-semibold mb-2 uppercase tracking-wide text-[var(--text-muted)]">
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
                className="flex-shrink-0 w-4 h-4 rounded border flex items-center justify-center transition-colors-fast"
                style={{
                  backgroundColor: checked ? "var(--accent)" : "var(--surface-raised)",
                  borderColor: checked ? "var(--accent)" : "var(--border)",
                }}
              >
                {checked && (
                  <Check size={ICON.xs} className="text-[var(--filter-active-text)]" strokeWidth={3} />
                )}
              </span>
              <span className="text-sm group-hover:text-[var(--text)] transition-colors-fast text-[var(--text-muted)]">
                {labelFor ? labelFor(opt) : opt}
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
  const { t } = useTranslation();
  return (
    <div className="mb-4">
      <p className="text-xs font-semibold mb-2 uppercase tracking-wide text-[var(--text-muted)]">
        {t("media.library.yearLabel")}
      </p>
      <div className="flex items-center gap-2">
        <input
          type="number"
          placeholder={t("media.library.yearFrom")}
          aria-label={t("media.library.yearFrom")}
          value={yearFrom ?? ""}
          min={1900}
          max={new Date().getFullYear() + 1}
          onChange={(e) => {
            const val = e.target.value ? parseInt(e.target.value, 10) : undefined;
            onChange(val, yearTo);
          }}
          className="w-full px-2 py-1.5 rounded-[var(--radius-md)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--accent)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
        />
        <span className="flex-shrink-0 text-xs text-[var(--text-muted)]">-</span>
        <input
          type="number"
          placeholder={t("media.library.yearTo")}
          aria-label={t("media.library.yearTo")}
          value={yearTo ?? ""}
          min={1900}
          max={new Date().getFullYear() + 1}
          onChange={(e) => {
            const val = e.target.value ? parseInt(e.target.value, 10) : undefined;
            onChange(yearFrom, val);
          }}
          className="w-full px-2 py-1.5 rounded-[var(--radius-md)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--accent)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
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
  const { t } = useTranslation();
  return (
    <div className="mb-4">
      <p className="text-xs font-semibold mb-2 uppercase tracking-wide text-[var(--text-muted)]">
        {t("media.library.minRating")}
      </p>
      <div className="flex items-center gap-1">
        {RATING_OPTIONS.map((star) => {
          const active = minRating != null && star <= minRating;
          return (
            <button
              key={star}
              type="button"
              onClick={() => onChange(minRating === star ? undefined : star)}
              title={t("media.library.minStars", { count: star })}
              aria-label={t("media.library.minStars", { count: star })}
              className="p-0.5 transition-colors-fast"
              style={{ color: active ? "var(--accent)" : "var(--border)" }}
            >
              <Star size={ICON.lg} fill="currentColor"  aria-hidden="true" />
            </button>
          );
        })}
        {minRating != null && (
          <button
            type="button"
            onClick={() => onChange(undefined)}
            className="ml-1 text-xs hover:underline text-[var(--text-muted)]"
          >
            {t("media.library.clearRating")}
          </button>
        )}
      </div>
    </div>
  );
}

const TYPE_OPTIONS: Array<{ labelKey: string; value: LibraryServiceListMediaItemsType | "" }> = [
  { labelKey: "media.library.typeAll", value: "" },
  { labelKey: "media.library.typeMovie", value: "MEDIA_ITEM_TYPE_MOVIE" },
  { labelKey: "media.library.typeShow", value: "MEDIA_ITEM_TYPE_SERIES" },
  { labelKey: "media.library.typeEpisode", value: "MEDIA_ITEM_TYPE_EPISODE" },
  { labelKey: "media.library.typeTrack", value: "MEDIA_ITEM_TYPE_TRACK" },
  { labelKey: "media.library.typePhoto", value: "MEDIA_ITEM_TYPE_PHOTO" },
];

function TypeFilter({
  value,
  onChange,
}: {
  value?: LibraryServiceListMediaItemsType;
  onChange: (v: LibraryServiceListMediaItemsType | undefined) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="mb-4">
      <p className="text-xs font-semibold mb-2 uppercase tracking-wide text-[var(--text-muted)]">
        {t("media.library.typeLabel")}
      </p>
      <select
        value={value ?? ""}
        aria-label={t("media.library.typeLabel")}
        onChange={(e) => onChange((e.target.value as LibraryServiceListMediaItemsType) || undefined)}
        className="w-full px-2 py-1.5 rounded-[var(--radius-md)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--accent)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
      >
        {TYPE_OPTIONS.map((o) => (
          <option key={o.value} value={o.value}>{t(o.labelKey)}</option>
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
  const { t } = useTranslation();
  return (
    <div className="mb-4">
      <p className="text-xs font-semibold mb-2 uppercase tracking-wide text-[var(--text-muted)]">
        {t("media.library.genreText")}
      </p>
      <input
        type="text"
        placeholder={t("media.library.genrePlaceholder")}
        aria-label={t("media.library.genreText")}
        value={value ?? ""}
        onChange={(e) => onChange(e.target.value || undefined)}
        className="w-full px-2 py-1.5 rounded-[var(--radius-md)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--accent)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
      />
    </div>
  );
}

function GenreChips({
  availableGenres,
  selected,
  onChange,
}: {
  availableGenres: string[];
  selected: string[];
  onChange: (v: string[]) => void;
}) {
  // Only the genres the library actually contains (ListGenres), never a
  // hardcoded list.
  //
  // A hardcoded English list used to be merged in here, and metadata providers
  // return genres in the SERVER's language: a French library therefore showed
  // "Action, Adventure, Animation, Aventure, Comedy, Comédie, ..." - the same
  // genres twice, in two languages. Worse, the filter matches genre strings
  // exactly, so every English chip that was not also a real genre matched
  // nothing at all: half the chips were guaranteed empty results.
  const merged = Array.from(new Set(availableGenres)).sort((a, b) =>
    a.localeCompare(b),
  );
  const { t } = useTranslation();
  if (merged.length === 0) return null;
  return (
    <div className="mb-4">
      <p className="text-xs font-semibold mb-2 uppercase tracking-wide text-[var(--text-muted)]">
        {t("media.library.genreLabel")}
      </p>
      <div className="flex flex-wrap gap-1.5 max-h-36 overflow-y-auto pr-1">
        {merged.map((g) => {
          const active = selected.includes(g);
          return (
            <FilterChip
              key={g}
              active={active}
              onClick={() => onChange(active ? selected.filter((v) => v !== g) : [...selected, g])}
              icon={<GenreDot label={g} />}
            >
              {g}
            </FilterChip>
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
  const { t } = useTranslation();
  const count = countActiveFilters(search);
  return (
    <div className="flex items-center gap-2">
      <button
        type="button"
        onClick={onClick}
        className="relative flex items-center gap-1.5 px-3 py-2 rounded-[var(--radius-md)] border transition-colors-fast text-sm"
        style={{
          backgroundColor: count > 0 ? "var(--accent)" : "var(--surface-raised)",
          color: count > 0 ? "var(--filter-active-text)" : "var(--text-muted)",
          borderColor: count > 0 ? "var(--accent)" : "var(--border)",
        }}
        title={t("media.library.filters")}
        aria-label={t("media.library.filters")}
        data-testid="filter-toggle"
      >
        <Filter size={ICON.md}  aria-hidden="true" />
        {t("media.library.filters")}
        {count > 0 && (
          <span
            className="inline-flex items-center justify-center w-4 h-4 rounded-full text-xs font-bold bg-[var(--filter-active-text)] text-[var(--accent)]"
          >
            {count}
          </span>
        )}
      </button>
      {count > 0 && resultCount != null && (
        <span className="text-xs text-[var(--text-muted)]" data-testid="filter-result-count">
          {t("media.library.resultCount", { count: resultCount })}
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
  const { t, i18n } = useTranslation();
  const activeCount = countActiveFilters(search);

  // Localized display name for a normalized ISO 639-1 code (e.g. "fr" ->
  // "French"), so the audio/subtitle language filters read as language
  // names rather than raw container codes. Falls back to the code itself
  // when Intl.DisplayNames is unavailable or doesn't know the code.
  const langLabel = useMemo(() => {
    let names: Intl.DisplayNames | null = null;
    try {
      names = new Intl.DisplayNames([i18n.language], { type: "language" });
    } catch {
      /* unsupported locale/environment - fall back to the raw code */
    }
    return (code: string) => names?.of(code) ?? code;
  }, [i18n.language]);

  return (
    <div
      className="rounded-[var(--radius-md)] p-4 bg-[var(--surface)] border border-[var(--border)] min-w-[200px]"
      data-testid="filter-panel"
    >
      <div className="flex items-center justify-between mb-4">
        <span className="text-sm font-semibold text-[var(--text)]">{t("media.library.filters")}</span>
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
            {t("media.library.clearAll")}
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
        label={t("media.library.resolution")}
        options={availableOptions.resolutions}
        selected={search.resolutions ?? []}
        onChange={(v) => onUpdate({ resolutions: v.length ? v : undefined })}
      />

      <CheckboxGroup
        label={t("media.library.videoCodec")}
        options={availableOptions.codecs}
        selected={search.codecs ?? []}
        onChange={(v) => onUpdate({ codecs: v.length ? v : undefined })}
      />

      <CheckboxGroup
        label={t("media.library.audioLanguage")}
        options={availableOptions.audioLangs}
        selected={search.audioLangs ?? []}
        onChange={(v) => onUpdate({ audioLangs: v.length ? v : undefined })}
        labelFor={langLabel}
      />

      <CheckboxGroup
        label={t("media.library.subtitleLanguage")}
        options={availableOptions.subLangs}
        selected={search.subLangs ?? []}
        onChange={(v) => onUpdate({ subLangs: v.length ? v : undefined })}
        labelFor={langLabel}
      />

      {availableOptions.resolutions.length === 0 &&
        availableOptions.codecs.length === 0 &&
        availableOptions.audioLangs.length === 0 &&
        availableOptions.subLangs.length === 0 &&
        availableGenres.length === 0 && (
          <p className="text-xs text-[var(--text-muted)]">
            {t("media.library.noFilterOptions")}
          </p>
        )}
    </div>
  );
}
