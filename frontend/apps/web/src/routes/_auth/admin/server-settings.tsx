import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/_auth/admin/server-settings")({
  beforeLoad: () => { throw redirect({ to: "/admin/server-config" }); },
});
