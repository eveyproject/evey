import { describe, it, expect } from "vitest";
import type { V1MediaItem } from "@evey/api";
import {
  buildInitialMetadataForm,
  canSaveMetadataForm,
  getDirtyOverrideFields,
  validateMetadataForm,
  hasMetadataFormErrors,
  type MetadataFormValues,
} from "./metadataFormValidation";

function emptyForm(): MetadataFormValues {
  return {
    title: "",
    originalTitle: "",
    year: "",
    overview: "",
    genres: "",
    communityRating: "",
    imdbId: "",
    tmdbId: "",
  };
}

describe("buildInitialMetadataForm", () => {
  it("maps a media item's fields to string form values", () => {
    const item: V1MediaItem = {
      title: "Arrival",
      originalTitle: "Arrival",
      year: 2016,
      overview: "A linguist works with the military.",
      genres: ["Drama", "Sci-Fi"],
      communityRating: 7.567,
      providerIds: { imdb: "tt2543164", tmdb: "329865" },
    };
    expect(buildInitialMetadataForm(item)).toEqual({
      title: "Arrival",
      originalTitle: "Arrival",
      year: "2016",
      overview: "A linguist works with the military.",
      genres: "Drama, Sci-Fi",
      communityRating: "7.57",
      imdbId: "tt2543164",
      tmdbId: "329865",
    });
  });

  it("defaults every field to an empty string when absent", () => {
    expect(buildInitialMetadataForm({})).toEqual(emptyForm());
  });
});

describe("canSaveMetadataForm", () => {
  it("is false when every field is blank", () => {
    expect(canSaveMetadataForm(emptyForm())).toBe(false);
  });

  it("is true once a single field carries a value", () => {
    expect(canSaveMetadataForm({ ...emptyForm(), title: "New Title" })).toBe(true);
  });

  it("treats whitespace-only values as blank", () => {
    expect(canSaveMetadataForm({ ...emptyForm(), title: "   " })).toBe(false);
  });
});

describe("getDirtyOverrideFields", () => {
  it("returns nothing when the form matches the initial values", () => {
    const initial = { ...emptyForm(), title: "Arrival" };
    expect(getDirtyOverrideFields(initial, initial)).toEqual([]);
  });

  it("returns only the fields the user actually changed", () => {
    const initial = { ...emptyForm(), title: "Arrival", year: "2016" };
    const edited = { ...initial, title: "Arrival (Director's Cut)" };
    expect(getDirtyOverrideFields(edited, initial)).toEqual([
      { overrideKey: "title", value: "Arrival (Director's Cut)" },
    ]);
  });

  it("does not re-save a field the user cleared back to blank", () => {
    const initial = { ...emptyForm(), overview: "Original overview" };
    const edited = { ...initial, overview: "" };
    expect(getDirtyOverrideFields(edited, initial)).toEqual([]);
  });

  it("never emits imdbId/tmdbId as overrides (they trigger re-enrichment instead)", () => {
    const initial = emptyForm();
    const edited = { ...initial, imdbId: "tt1234567", tmdbId: "555" };
    expect(getDirtyOverrideFields(edited, initial)).toEqual([]);
  });
});

describe("validateMetadataForm", () => {
  it("flags a fully empty form", () => {
    const errors = validateMetadataForm(emptyForm());
    expect(errors.form).toBe("allEmpty");
    expect(hasMetadataFormErrors(errors)).toBe(true);
  });

  it("accepts a well-formed form with no errors", () => {
    const errors = validateMetadataForm({ ...emptyForm(), title: "Arrival", year: "2016", communityRating: "7.5" });
    expect(hasMetadataFormErrors(errors)).toBe(false);
  });

  it.each(["18", "abc", "1869", "3100", "2016.5"])("rejects an invalid year %s", (year) => {
    const errors = validateMetadataForm({ ...emptyForm(), year });
    expect(errors.year).toBe("invalidYear");
  });

  it("accepts a year at the lower and upper bound", () => {
    expect(validateMetadataForm({ ...emptyForm(), year: "1870" }).year).toBeUndefined();
    const upper = String(new Date().getFullYear() + 5);
    expect(validateMetadataForm({ ...emptyForm(), year: upper }).year).toBeUndefined();
  });

  it.each(["-1", "10.1", "11", "abc"])("rejects an invalid community rating %s", (communityRating) => {
    const errors = validateMetadataForm({ ...emptyForm(), communityRating });
    expect(errors.communityRating).toBe("invalidRating");
  });

  it("accepts a community rating within 0-10", () => {
    expect(validateMetadataForm({ ...emptyForm(), communityRating: "10" }).communityRating).toBeUndefined();
    expect(validateMetadataForm({ ...emptyForm(), communityRating: "0" }).communityRating).toBeUndefined();
  });

  it("flags values that exceed the field's max length", () => {
    const errors = validateMetadataForm({ ...emptyForm(), title: "a".repeat(501) });
    expect(errors.title).toBe("tooLong");
  });

  it("does not flag a value right at the max length", () => {
    const errors = validateMetadataForm({ ...emptyForm(), title: "a".repeat(500) });
    expect(errors.title).toBeUndefined();
  });
});
