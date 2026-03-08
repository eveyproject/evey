import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";

export const Route = createFileRoute("/_auth/admin")({
  beforeLoad: () => {
    const { isAdmin } = useAuthStore.getState();
    if (!isAdmin) throw redirect({ to: "/home" });
  },
  component: () => <Outlet />,
});
