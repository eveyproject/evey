import { createFileRoute, redirect } from "@tanstack/react-router";

// Old standalone /admin/feeds page is folded into /admin/server-config as a
// collapsible "Home feeds" card. Keep this redirect so existing bookmarks
// don't 404.
export const Route = createFileRoute("/_auth/admin/feeds")({
  beforeLoad: () => {
    throw redirect({ to: "/admin/server-config" });
  },
});
