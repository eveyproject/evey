import type { V1MediaItem } from "@evey/api";

// Mirrors apps/web/src/features/library/MediaDetailEdit.tsx's EditForm/FIELD_MAP so
// mobile saves the exact same override fields under the exact same server key names.
export interface MetadataFormValues {
  title: string;
  originalTitle: string;
  year: string;
  overview: string;
  genres: string;
  communityRating: string;
  imdbId: string;
  tmdbId: string;
}

export const METADATA_FIELD_MAP: Array<{ formKey: keyof MetadataOverrideFormKey; overrideKey: string }> = [
  { formKey: "title", overrideKey: "title" },
  { formKey: "originalTitle", overrideKey: "original_title" },
  { formKey: "year", overrideKey: "year" },
  { formKey: "overview", overrideKey: "overview" },
  { formKey: "genres", overrideKey: "genres" },
  { formKey: "communityRating", overrideKey: "community_rating" },
];

// Only the fields that can become a metadata override (imdbId/tmdbId trigger a
// re-enrich instead, same as web - see MediaDetailEdit's tmdbChanged/imdbChanged).
type MetadataOverrideFormKey = Pick<
  MetadataFormValues,
  "title" | "originalTitle" | "year" | "overview" | "genres" | "communityRating"
>;

export function buildInitialMetadataForm(item: V1MediaItem): MetadataFormValues {
  return {
    title: item.title ?? "",
    originalTitle: item.originalTitle ?? "",
    year: item.year ? String(item.year) : "",
    overview: item.overview ?? "",
    genres: item.genres?.join(", ") ?? "",
    communityRating: item.communityRating ? String(Math.round(item.communityRating * 100) / 100) : "",
    imdbId: item.providerIds?.imdb ?? "",
    tmdbId: item.providerIds?.tmdb ?? "",
  };
}

// True once at least one field carries a value - mirrors web's canSave, which
// blocks Save on a fully blank form (nothing to persist, nothing to re-enrich).
export function canSaveMetadataForm(form: MetadataFormValues): boolean {
  return Object.values(form).some((v) => v.trim() !== "");
}

export interface DirtyOverrideField {
  overrideKey: string;
  value: string;
}

// Only fields the user actually edited become overrides - a pre-filled field left
// untouched is not user intent and would shadow a fresh enrichment triggered by an
// IMDB/TMDB ID change in the same save. Mirrors web's dirtyFields filter exactly.
export function getDirtyOverrideFields(
  form: MetadataFormValues,
  initialForm: MetadataFormValues,
): DirtyOverrideField[] {
  return METADATA_FIELD_MAP
    .map(({ overrideKey, formKey }) => ({ overrideKey, value: form[formKey].trim(), formKey }))
    .filter(({ formKey, value }) => value !== "" && value !== initialForm[formKey].trim())
    .map(({ overrideKey, value }) => ({ overrideKey, value }));
}

export type MetadataFieldErrorCode = "tooLong" | "invalidYear" | "invalidRating";

export type MetadataFormErrors = Partial<Record<keyof MetadataFormValues, MetadataFieldErrorCode>> & {
  // Set when every field is blank: nothing to save and nothing to re-enrich.
  form?: "allEmpty";
};

// DB-column-driven caps (varchar limits on the metadata_overrides table); kept
// generous since these mirror user-authored replacements, not scraped metadata.
const MAX_LENGTHS: Partial<Record<keyof MetadataFormValues, number>> = {
  title: 500,
  originalTitle: 500,
  overview: 5000,
  genres: 500,
  imdbId: 20,
  tmdbId: 20,
};

const CURRENT_YEAR = new Date().getFullYear();
// 1870: earliest attributed film prints predate 1888, but a few catalogued
// works claim 1870s dates - wide enough to never reject a real title.
const MIN_YEAR = 1870;
const MAX_YEAR = CURRENT_YEAR + 5;

export function validateMetadataForm(form: MetadataFormValues): MetadataFormErrors {
  const errors: MetadataFormErrors = {};

  for (const key of Object.keys(MAX_LENGTHS) as Array<keyof MetadataFormValues>) {
    const max = MAX_LENGTHS[key]!;
    if (form[key].trim().length > max) errors[key] = "tooLong";
  }

  const year = form.year.trim();
  if (year !== "") {
    const n = Number(year);
    if (!Number.isInteger(n) || n < MIN_YEAR || n > MAX_YEAR) errors.year = "invalidYear";
  }

  const rating = form.communityRating.trim();
  if (rating !== "") {
    const n = Number(rating);
    if (Number.isNaN(n) || n < 0 || n > 10) errors.communityRating = "invalidRating";
  }

  if (!canSaveMetadataForm(form)) errors.form = "allEmpty";

  return errors;
}

export function hasMetadataFormErrors(errors: MetadataFormErrors): boolean {
  return Object.keys(errors).length > 0;
}
