import { createFileRoute } from "@tanstack/react-router";
import { RouteErrorComponent } from "@/components/ui/RouteErrorComponent";

export const Route = createFileRoute("/_auth/home")({
  errorComponent: RouteErrorComponent,
});
