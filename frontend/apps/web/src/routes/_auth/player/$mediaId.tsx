import { createFileRoute } from "@tanstack/react-router";

type PlayerSearch = { room?: string };

export const Route = createFileRoute("/_auth/player/$mediaId")({
  validateSearch: (search: Record<string, unknown>): PlayerSearch => ({
    room: typeof search.room === "string" ? search.room : undefined,
  }),
});
