import { createFileRoute } from "@tanstack/react-router";
import { RouteErrorComponent } from "@/components/ui/RouteErrorComponent";

// `restart` carries the "from the beginning" choice made on the media page.
// Resuming is the default; this is the only way to say otherwise.
type PlayerSearch = { room?: string; restart?: boolean };

export const Route = createFileRoute("/_auth/player/$mediaId")({
  validateSearch: (search: Record<string, unknown>): PlayerSearch => ({
    room: typeof search.room === "string" ? search.room : undefined,
    restart: search.restart === true || search.restart === "1" || search.restart === "true" ? true : undefined,
  }),
  errorComponent: RouteErrorComponent,
});
