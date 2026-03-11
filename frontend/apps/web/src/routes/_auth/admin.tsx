import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";

export const Route = createFileRoute("/_auth/admin")({
  beforeLoad: () => {
    const { isAdmin } = useAuthStore.getState();
    if (!isAdmin) throw redirect({ to: "/home" });
  },
  component: AdminLayout,
});

function AdminLayout() {
  return (
    <div className="flex-1 min-w-0 flex flex-col gap-8">
      <Outlet />
    </div>
  );
}
