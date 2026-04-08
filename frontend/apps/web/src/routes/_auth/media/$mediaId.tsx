import { createFileRoute } from "@tanstack/react-router";
import { RouteErrorComponent } from "@/components/ui/RouteErrorComponent";

type MediaSearch = { edit?: boolean };

export const Route = createFileRoute("/_auth/media/$mediaId")({
  validateSearch: (search: Record<string, unknown>): MediaSearch => ({
    edit: search.edit === true || search.edit === "true",
  }),
  errorComponent: RouteErrorComponent,
});
