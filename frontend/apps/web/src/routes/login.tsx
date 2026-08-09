import { useState } from "react";
import { Play, Users, ChevronDown } from "lucide-react";
import { createFileRoute, redirect, useNavigate, Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useUserServiceLogin, useAdminSystemServiceGetSetupStatus, eveyFetch, normalizeRoomCode, ROOM_CODE_LENGTH } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Card } from "@/components/ui/Card";
import { QuickConnect } from "@/components/QuickConnect";
import { writeGuestSession } from "@/features/syncplay/guestSession";

type LoginSearch = {
  oidc_error?: string;
};

export const Route = createFileRoute("/login")({
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    oidc_error: typeof search.oidc_error === "string" ? search.oidc_error : undefined,
  }),
  beforeLoad: () => {
    const { accessToken } = useAuthStore.getState();
    if (accessToken) {
      try {
        const tokenParts = accessToken.split(".");
        if (tokenParts.length < 3 || !tokenParts[1]) return;
        const payload = JSON.parse(atob(tokenParts[1].replace(/-/g, "+").replace(/_/g, "/")));
        if (payload.exp * 1000 > Date.now()) throw redirect({ to: "/home" });
      } catch (e) {
        if (e && typeof e === "object" && "to" in e) throw e; // re-throw redirect
        // expired or malformed token - stay on login
      }
    }
  },
  component: LoginPage,
});

function friendlyLoginError(err: unknown, t: (key: string) => string): string {
  const msg = err instanceof Error ? err.message.toLowerCase() : "";
  if (msg.includes("invalid credentials") || msg.includes("not found"))
    return t("auth.login.invalidCredentials");
  if (msg.includes("password") && (msg.includes("short") || msg.includes("length")))
    return t("auth.login.passwordTooShort");
  return t("auth.login.loginFailed");
}

function LoginPage() {
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const { setTokens } = useAuthStore();
  const navigate = useNavigate();
  const [guestCode, setGuestCode] = useState("");
  const [guestJoining, setGuestJoining] = useState(false);
  const [guestError, setGuestError] = useState(false);
  const [guestOpen, setGuestOpen] = useState(false);

  async function handleGuestJoin(e: React.FormEvent) {
    e.preventDefault();
    const code = guestCode.trim();
    if (code.length < 4) return;
    setGuestJoining(true);
    setGuestError(false);
    try {
      const info = await eveyFetch<{ token: string; roomId: string; mediaId: string; mediaType: string }>({
        url: "/api/v1/syncplay/join-code",
        method: "POST",
        data: { code },
      });
      if (!info.token || !info.mediaId) throw new Error("invalid room");
      writeGuestSession({ token: info.token, roomId: info.roomId, mediaId: info.mediaId });
      void navigate({ to: "/watch/$mediaId", params: { mediaId: info.mediaId } });
    } catch {
      setGuestError(true);
    } finally {
      setGuestJoining(false);
    }
  }
  const { oidc_error } = Route.useSearch();

  const oidcConfig = useQuery<{ enabled: boolean }>({
    queryKey: ["oidc-config"],
    queryFn: () => eveyFetch<{ enabled: boolean }>({ url: "/api/v1/auth/oidc/config", method: "GET" }),
    staleTime: 60 * 1000,
  });

  // Public setup-status endpoint (no auth) exposes whether password recovery is
  // configured, so the "Forgot password?" link can show on the logged-out login
  // page. (Previously read from the admin-only SystemInfo, gated on accessToken,
  // so the link never appeared.)
  const setupStatus = useAdminSystemServiceGetSetupStatus({ query: { staleTime: 60 * 1000 } });

  const login = useUserServiceLogin({
    mutation: {
      onSuccess: (data) => {
        if (data.accessToken) {
          setTokens(data.accessToken);
          void navigate({ to: "/profile-select" });
        }
      },
    },
  });

  return (
    <div
      className="min-h-screen flex items-center justify-center px-4 relative gradient-auth-page"
    >
      <Card className="w-full max-w-sm p-8 shadow-2xl">
        <div className="flex flex-col items-center mb-8">
          <div
            className="w-12 h-12 rounded-[var(--radius-md)] flex items-center justify-center mb-4 bg-[var(--accent)]"
          >
            <Play size={24} className="text-[var(--filter-active-text)]" fill="currentColor" />
          </div>
          <h1 className="text-2xl font-bold text-[var(--text)]">Evey</h1>
          <p className="text-[var(--text-muted)] text-sm mt-1">{t("auth.login.signInToAccount")}</p>
        </div>
        {oidc_error && (
          <p className="text-sm text-center mb-4 text-[var(--color-danger)]">{t("auth.login.ssoFailed")}</p>
        )}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            login.mutate({ data: { username, password } });
          }}
          className="flex flex-col gap-4"
        >
          <Input
            id="username"
            label={t("auth.login.username")}
            type="text"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoFocus
          />
          <Input
            id="password"
            label={t("auth.login.password")}
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
          {login.error && (
            <p
              className="text-sm text-center text-[var(--color-danger)]"
              data-testid="login-error"
              role="alert"
            >
              {friendlyLoginError(login.error, t)}
            </p>
          )}
          <Button type="submit" loading={login.isPending} className="w-full mt-2" data-testid="login-submit">
            {t("auth.login.signIn")}
          </Button>
          {setupStatus.data?.passwordRecoveryEnabled && (
            <div className="text-center mt-2">
              <Link to="/forgot-password" className="text-sm text-[var(--accent)] hover:underline">
                {t("auth.login.forgotPassword")}
              </Link>
            </div>
          )}
        </form>
        {oidcConfig.data?.enabled && (
          <>
            <div className="flex items-center gap-2 my-4">
              <div className="flex-1 h-px bg-[var(--border)]" />
              <span className="text-xs text-[var(--text-muted)]">{t("auth.login.or")}</span>
              <div className="flex-1 h-px bg-[var(--border)]" />
            </div>
            <Button
              type="button"
              variant="secondary"
              className="w-full"
              onClick={() => { window.location.href = "/api/v1/auth/oidc/login"; }}
            >
              {t("auth.login.signInWithSSO")}
            </Button>
          </>
        )}
        <QuickConnect />
        <div className="mt-4 pt-4 border-t border-[var(--border)]">
          <button
            type="button"
            onClick={() => setGuestOpen((o) => !o)}
            aria-expanded={guestOpen}
            className="flex items-center justify-center gap-1.5 w-full text-sm text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
          >
            <Users className="w-4 h-4" />
            {t("auth.login.guestJoinTrigger", "Watch together")}
            <ChevronDown className={`w-4 h-4 transition-transform ${guestOpen ? "rotate-180" : ""}`} />
          </button>
          {guestOpen && (
            <form onSubmit={handleGuestJoin} className="flex flex-col gap-2 mt-3">
              <p className="text-xs text-center text-[var(--text-muted)]">
                {t("auth.login.guestJoinDesc", "Enter the code - no account needed.")}
              </p>
              <div className="flex gap-2">
                <Input
                  value={guestCode}
                  onChange={(e) => { setGuestCode(normalizeRoomCode(e.target.value).slice(0, ROOM_CODE_LENGTH)); setGuestError(false); }}
                  placeholder="ABC123"
                  autoCapitalize="characters"
                  autoComplete="off"
                  autoFocus
                  aria-label={t("media.syncplay.codeLabel", "Code")}
                  className="flex-1 font-mono tracking-widest text-center"
                />
                <Button type="submit" variant="secondary" loading={guestJoining} disabled={guestCode.trim().length < 4}>
                  {t("media.syncplay.joinRoom", "Join")}
                </Button>
              </div>
              {guestError && (
                <p className="text-xs text-center" style={{ color: "var(--color-danger)" }}>
                  {t("auth.login.guestJoinError", "Could not join - check the code.")}
                </p>
              )}
            </form>
          )}
        </div>
      </Card>
    </div>
  );
}
