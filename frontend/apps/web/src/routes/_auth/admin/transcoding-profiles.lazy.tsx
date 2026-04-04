import { createLazyFileRoute, Navigate } from "@tanstack/react-router";

export const Route = createLazyFileRoute("/_auth/admin/transcoding-profiles")({
  component: () => <Navigate to="/admin" />,
});
