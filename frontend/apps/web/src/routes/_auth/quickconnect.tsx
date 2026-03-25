import { createFileRoute } from "@tanstack/react-router";

type QuickConnectSearch = {
  code?: string;
};

export const Route = createFileRoute("/_auth/quickconnect")({
  validateSearch: (search: Record<string, unknown>): QuickConnectSearch => ({
    code: typeof search.code === "string" ? search.code : undefined,
  }),
});
