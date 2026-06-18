import { createFileRoute } from "@tanstack/react-router";

type SearchParams = { q?: string; kind?: "music" };

export const Route = createFileRoute("/_auth/search")({
  validateSearch: (search: Record<string, unknown>): SearchParams => ({
    q: typeof search.q === "string" ? search.q : undefined,
    kind: search.kind === "music" ? "music" : undefined,
  }),
});
