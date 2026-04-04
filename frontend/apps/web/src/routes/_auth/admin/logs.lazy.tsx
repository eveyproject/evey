import { createLazyFileRoute, Navigate } from "@tanstack/react-router";

export const Route = createLazyFileRoute("/_auth/admin/logs")({
  component: () => <Navigate to="/admin/activity" />,
});
