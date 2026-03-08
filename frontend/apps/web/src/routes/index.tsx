import { createFileRoute, redirect } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";
import { eveyFetch } from "@evey/api";

type SystemInfo = { setupComplete?: boolean };

export const Route = createFileRoute("/")({
  beforeLoad: async ({ context }) => {
    let setupComplete = true;
    try {
      const info = await context.queryClient.fetchQuery<SystemInfo>({
        queryKey: ["system-info"],
        queryFn: () => eveyFetch<SystemInfo>({ url: "/api/v1/admin/system", method: "GET" }),
        staleTime: 0,
      });
      setupComplete = info.setupComplete ?? true;
    } catch {
      // backend unreachable — fall through to login
    }

    if (!setupComplete) throw redirect({ to: "/setup" });

    const { accessToken } = useAuthStore.getState();
    if (!accessToken) throw redirect({ to: "/login" });

    throw redirect({ to: "/home" });
  },
  component: () => null,
});
