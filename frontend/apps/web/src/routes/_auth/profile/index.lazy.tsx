import { createLazyFileRoute, Navigate } from "@tanstack/react-router";

export const Route = createLazyFileRoute("/_auth/profile/")({
  component: () => <Navigate to="/profile/settings" />,
});
