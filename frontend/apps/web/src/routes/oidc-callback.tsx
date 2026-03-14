import { useEffect } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";

type OIDCCallbackSearch = {
  token?: string;
  refresh?: string;
  oidc_error?: string;
};

export const Route = createFileRoute("/oidc-callback")({
  validateSearch: (search: Record<string, unknown>): OIDCCallbackSearch => ({
    token: typeof search.token === "string" ? search.token : undefined,
    refresh: typeof search.refresh === "string" ? search.refresh : undefined,
    oidc_error: typeof search.oidc_error === "string" ? search.oidc_error : undefined,
  }),
  component: OIDCCallbackPage,
});

function OIDCCallbackPage() {
  const { token, oidc_error } = Route.useSearch();
  const { setTokens } = useAuthStore();
  const navigate = useNavigate();

  useEffect(() => {
    if (oidc_error) {
      void navigate({ to: "/login", search: { oidc_error: "1" } });
      return;
    }
    if (token) {
      setTokens(token);
      void navigate({ to: "/home" });
    } else {
      void navigate({ to: "/login" });
    }
  }, [token, oidc_error, setTokens, navigate]);

  return (
    <div className="min-h-screen flex items-center justify-center">
      <p className="text-[var(--text-muted)]">Signing in...</p>
    </div>
  );
}
