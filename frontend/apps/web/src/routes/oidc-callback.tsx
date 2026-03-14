import { useEffect } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";

type OIDCCallbackSearch = {
  oidc_error?: string;
};

export const Route = createFileRoute("/oidc-callback")({
  validateSearch: (search: Record<string, unknown>): OIDCCallbackSearch => ({
    oidc_error: typeof search.oidc_error === "string" ? search.oidc_error : undefined,
  }),
  component: OIDCCallbackPage,
});

function OIDCCallbackPage() {
  const { oidc_error } = Route.useSearch();
  const { setTokens } = useAuthStore();
  const navigate = useNavigate();

  useEffect(() => {
    if (oidc_error) {
      void navigate({ to: "/login", search: { oidc_error: "1" } });
      return;
    }
    // Exchange the short-lived HttpOnly session cookie for the access token.
    fetch("/api/v1/auth/oidc/session")
      .then((res) => {
        if (!res.ok) throw new Error("no session");
        return res.json() as Promise<{ accessToken: string }>;
      })
      .then(({ accessToken }) => {
        setTokens(accessToken);
        void navigate({ to: "/home" });
      })
      .catch(() => {
        void navigate({ to: "/login" });
      });
  }, [oidc_error, setTokens, navigate]);

  return (
    <div className="min-h-screen flex items-center justify-center">
      <p className="text-[var(--text-muted)]">Signing in...</p>
    </div>
  );
}
