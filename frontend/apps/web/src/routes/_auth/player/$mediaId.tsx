import { createFileRoute } from "@tanstack/react-router";
import { parseTrackPick, type TrackPick } from "@evey/api";
import { RouteErrorComponent } from "@/components/ui/RouteErrorComponent";

// `restart` carries the "from the beginning" choice made on the media page.
// Resuming is the default; this is the only way to say otherwise. `audio` and
// `subtitle` are the tracks picked there before playback (see trackPick.ts).
type PlayerSearch = { room?: string; restart?: boolean } & TrackPick;

export const Route = createFileRoute("/_auth/player/$mediaId")({
  validateSearch: (search: Record<string, unknown>): PlayerSearch => ({
    room: typeof search.room === "string" ? search.room : undefined,
    restart: search.restart === true || search.restart === "1" || search.restart === "true" ? true : undefined,
    ...parseTrackPick(search),
  }),
  errorComponent: RouteErrorComponent,
});
