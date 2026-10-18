import { normalizeLangCode } from "@evey/api";
import { RESOLUTION_BANDS, resolutionBand, type ResolutionBand } from "./streamBadges";

// The library filter panel's codec, resolution and language chips, built from
// GetLibraryFilterFacets: the distinct values of the WHOLE library, where the
// web read them off a 192-item sample and the phone off that sample plus the
// pages it had scrolled, so a language the sample missed was never offered.
// Shared by both panels so the labels a chip shows and the value it matches
// on cannot drift between them.

// Structural shape of the generated V1GetLibraryFilterFacetsResponse, which
// the @evey/api barrel does not re-export.
export interface LibraryFilterFacets {
  videoCodecs?: string[];
  audioLanguages?: string[];
  subtitleLanguages?: string[];
  resolutionTiers?: number[];
}

export interface LibraryFilterOptions {
  resolutions: ResolutionBand[];
  codecs: string[];
  audioLangs: string[];
  subLangs: string[];
}

// Display label of a probed codec name, which is also the value a codec chip
// matches items on.
export function normalizeCodec(codec: string): string {
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

// Languages are folded with normalizeLangCode so the 639-2/B and 639-2/T forms
// of one language ("fre"/"fra") make one chip, the form the panels match on.
function languageOptions(tags: string[] | undefined): string[] {
  const out = new Set<string>();
  for (const tag of tags ?? []) {
    if (!tag || tag === "und") continue;
    const norm = normalizeLangCode(tag);
    if (norm) out.add(norm);
  }
  return Array.from(out).sort();
}

export function libraryFilterOptions(facets: LibraryFilterFacets | undefined): LibraryFilterOptions {
  // A tier is the height of its own band (2160 is 4K, 1080 is 1080p, 0 is
  // SD), so resolutionBand names it without a second copy of the tier rule.
  const bands = new Set<ResolutionBand>((facets?.resolutionTiers ?? []).map((tier) => resolutionBand(undefined, tier)));
  const codecs = new Set<string>();
  for (const codec of facets?.videoCodecs ?? []) {
    if (codec) codecs.add(normalizeCodec(codec));
  }
  return {
    resolutions: RESOLUTION_BANDS.filter((band) => bands.has(band)),
    codecs: Array.from(codecs).sort(),
    audioLangs: languageOptions(facets?.audioLanguages),
    subLangs: languageOptions(facets?.subtitleLanguages),
  };
}
