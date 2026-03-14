import { useState } from "react";
import { createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useUserServiceLogin, eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Card } from "@/components/ui/Card";
import { QuickConnect } from "@/components/QuickConnect";

type LoginSearch = {
  oidc_error?: string;
};

export const Route = createFileRoute("/login")({
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    oidc_error: typeof search.oidc_error === "string" ? search.oidc_error : undefined,
  }),
  beforeLoad: () => {
    const { accessToken } = useAuthStore.getState();
    if (accessToken) throw redirect({ to: "/home" });
  },
  component: LoginPage,
});

function friendlyLoginError(err: unknown): string {
  const msg = err instanceof Error ? err.message.toLowerCase() : "";
  if (msg.includes("invalid credentials") || msg.includes("not found"))
    return "Invalid username or password";
  if (msg.includes("password") && (msg.includes("short") || msg.includes("length")))
    return "Password must be at least 8 characters";
  return "Login failed. Please try again.";
}

function LoginPage() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const { setTokens } = useAuthStore();
  const navigate = useNavigate();
  const { oidc_error } = Route.useSearch();

  const oidcConfig = useQuery<{ enabled: boolean }>({
    queryKey: ["oidc-config"],
    queryFn: () => eveyFetch<{ enabled: boolean }>({ url: "/api/v1/auth/oidc/config", method: "GET" }),
    staleTime: 60 * 1000,
  });

  const login = useUserServiceLogin({
    mutation: {
      onSuccess: (data) => {
        if (data.accessToken) {
          setTokens(data.accessToken);
          void navigate({ to: "/home" });
        }
      },
    },
  });

  return (
    <div
      className="min-h-screen flex items-center justify-center px-4"
      style={{
        background: "linear-gradient(135deg, var(--bg) 0%, #1a1a2e 50%, var(--bg) 100%)",
      }}
    >
      <Card className="w-full max-w-sm p-8 shadow-2xl">
        <div className="flex flex-col items-center mb-8">
          <div
            className="w-12 h-12 rounded-[var(--radius-md)] flex items-center justify-center mb-4"
            style={{ backgroundColor: "var(--accent)" }}
          >
            <svg className="w-6 h-6 text-white" fill="currentColor" viewBox="0 0 20 20">
              <path d="M6.3 2.8L17 10 6.3 17.2V2.8z" />
            </svg>
          </div>
          <h1 className="text-2xl font-bold text-[var(--text)]">Evey</h1>
          <p className="text-[var(--text-muted)] text-sm mt-1">Sign in to your account</p>
        </div>
        {oidc_error && (
          <p className="text-sm text-red-400 text-center mb-4">SSO sign-in failed. Please try again.</p>
        )}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            login.mutate({ data: { username, password } });
          }}
          className="flex flex-col gap-4"
        >
          <Input
            label="Username"
            type="text"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoFocus
          />
          <Input
            label="Password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
          {login.error && (
            <p className="text-sm text-red-400 text-center" data-testid="login-error">
              {friendlyLoginError(login.error)}
            </p>
          )}
          <Button type="submit" loading={login.isPending} className="w-full mt-2" data-testid="login-submit">
            Sign in
          </Button>
        </form>
        {oidcConfig.data?.enabled && (
          <>
            <div className="flex items-center gap-2 my-4">
              <div className="flex-1 h-px bg-[var(--border)]" />
              <span className="text-xs text-[var(--text-muted)]">or</span>
              <div className="flex-1 h-px bg-[var(--border)]" />
            </div>
            <Button
              type="button"
              variant="secondary"
              className="w-full"
              onClick={() => { window.location.href = "/api/v1/auth/oidc/login"; }}
            >
              Sign in with SSO
            </Button>
          </>
        )}
        <QuickConnect />
      </Card>
    </div>
  );
}
