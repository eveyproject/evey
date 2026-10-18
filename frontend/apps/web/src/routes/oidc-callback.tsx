import { useEffect } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@evey/store";
import { eveyFetch } from "@evey/api";

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
  const { t } = useTranslation();
  const { oidc_error } = Route.useSearch();
  const { setTokens } = useAuthStore();
  const navigate = useNavigate();

  useEffect(() => {
    if (oidc_error) {
      void navigate({ to: "/login", search: { oidc_error } });
      return;
    }
    void (async () => {
      try {
        const { accessToken } = await eveyFetch<{ accessToken: string }>({ url: "/api/v1/auth/oidc/session", method: "GET" });
        setTokens(accessToken);
        void navigate({ to: "/profile-select" });
      } catch {
        void navigate({ to: "/login" });
      }
    })();
  }, [oidc_error, setTokens, navigate]);

  return (
    <div className="min-h-screen flex items-center justify-center">
      <p className="text-[var(--text-muted)]">{t("auth.signingIn")}</p>
    </div>
  );
}
