import { createLazyFileRoute, Navigate } from "@tanstack/react-router";

export const Route = createLazyFileRoute("/_auth/admin/metadata")({
  component: () => <Navigate to="/admin" />,
});
