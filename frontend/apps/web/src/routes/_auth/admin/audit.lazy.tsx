import { createLazyFileRoute, Navigate } from "@tanstack/react-router";

export const Route = createLazyFileRoute("/_auth/admin/audit")({
  component: () => <Navigate to="/admin/activity" />,
});
