import { createFileRoute } from "@tanstack/react-router";
import { RouteErrorComponent } from "@/components/ui/RouteErrorComponent";

type PlayerSearch = { room?: string };

export const Route = createFileRoute("/_auth/player/$mediaId")({
  validateSearch: (search: Record<string, unknown>): PlayerSearch => ({
    room: typeof search.room === "string" ? search.room : undefined,
  }),
  errorComponent: RouteErrorComponent,
});
