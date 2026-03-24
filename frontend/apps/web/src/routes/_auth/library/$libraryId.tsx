import { createFileRoute } from "@tanstack/react-router";
import type { LibraryServiceListMediaItemsType, LibraryServiceListMediaItemsSortField, LibraryServiceListMediaItemsSortOrder } from "@evey/api";

export type LibrarySearch = {
  type?: LibraryServiceListMediaItemsType;
  genre?: string;
  genres?: string[];
  sortField?: LibraryServiceListMediaItemsSortField;
  sortOrder?: LibraryServiceListMediaItemsSortOrder;
  q?: string;
  resolutions?: string[];
  codecs?: string[];
  audioLangs?: string[];
  subLangs?: string[];
  yearFrom?: number;
  yearTo?: number;
  minRating?: number;
};

export const Route = createFileRoute("/_auth/library/$libraryId")({
  validateSearch: (search: Record<string, unknown>): LibrarySearch => ({
    type: typeof search.type === "string" ? (search.type as LibraryServiceListMediaItemsType) : undefined,
    genre: typeof search.genre === "string" ? search.genre : undefined,
    genres: Array.isArray(search.genres) ? (search.genres as string[]) : undefined,
    sortField: typeof search.sortField === "string" ? (search.sortField as LibraryServiceListMediaItemsSortField) : undefined,
    sortOrder: typeof search.sortOrder === "string" ? (search.sortOrder as LibraryServiceListMediaItemsSortOrder) : undefined,
    q: typeof search.q === "string" ? search.q : undefined,
    resolutions: Array.isArray(search.resolutions) ? (search.resolutions as string[]) : undefined,
    codecs: Array.isArray(search.codecs) ? (search.codecs as string[]) : undefined,
    audioLangs: Array.isArray(search.audioLangs) ? (search.audioLangs as string[]) : undefined,
    subLangs: Array.isArray(search.subLangs) ? (search.subLangs as string[]) : undefined,
    yearFrom: typeof search.yearFrom === "number" ? search.yearFrom : undefined,
    yearTo: typeof search.yearTo === "number" ? search.yearTo : undefined,
    minRating: typeof search.minRating === "number" ? search.minRating : undefined,
  }),
});
