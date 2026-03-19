import { useState } from "react";
import { createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { useUserServiceRegisterWithInvite } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Card } from "@/components/ui/Card";

type RegisterSearch = {
  invite?: string;
};

export const Route = createFileRoute("/register")({
  validateSearch: (search: Record<string, unknown>): RegisterSearch => ({
    invite: typeof search.invite === "string" ? search.invite : undefined,
  }),
  beforeLoad: () => {
    const { accessToken } = useAuthStore.getState();
    if (accessToken) throw redirect({ to: "/home" });
  },
  component: RegisterPage,
});

function RegisterPage() {
  const { t } = useTranslation("auth");
  const { invite } = Route.useSearch();
  const navigate = useNavigate();
  const { setTokens } = useAuthStore();

  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [inviteCode, setInviteCode] = useState(invite ?? "");
  const [validationError, setValidationError] = useState("");

  const register = useUserServiceRegisterWithInvite({
    mutation: {
      onSuccess: (data) => {
        if (data.accessToken) {
          setTokens(data.accessToken);
          void navigate({ to: "/home" });
        }
      },
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setValidationError("");
    if (password !== confirm) {
      setValidationError(t("register.passwordMismatch"));
      return;
    }
    if (password.length < 8) {
      setValidationError(t("register.passwordMin"));
      return;
    }
    register.mutate({
      data: {
        inviteCode,
        username,
        email,
        password,
      },
    });
  }

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
          <h1 className="text-2xl font-bold text-[var(--text)]">{t("register.createAccount")}</h1>
          {invite ? (
            <p className="text-[var(--text-muted)] text-sm mt-1 text-center">
              {t("register.invitedMessage")}
            </p>
          ) : (
            <p className="text-[var(--text-muted)] text-sm mt-1">{t("register.signUpWithInvite")}</p>
          )}
        </div>

        {invite && (
          <div
            className="mb-4 px-3 py-2 rounded-[var(--radius-md)] text-sm"
            style={{ backgroundColor: "var(--surface-raised)", color: "var(--text-muted)" }}
          >
            {t("register.inviteCodeLabel")} <span className="font-mono text-[var(--text)]">{invite}</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <Input
            label={t("register.username")}
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoFocus
          />
          <Input
            label={t("register.email")}
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
          <Input
            label={t("register.password")}
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
          <Input
            label={t("register.confirmPassword")}
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            required
          />
          {!invite && (
            <Input
              label={t("register.inviteCode")}
              value={inviteCode}
              onChange={(e) => setInviteCode(e.target.value)}
              required
            />
          )}
          {(validationError || register.error) && (
            <p className="text-sm text-center" style={{ color: "var(--color-danger)" }}>
              {validationError || (register.error instanceof Error ? register.error.message : t("register.registrationFailed"))}
            </p>
          )}
          <Button
            type="submit"
            loading={register.isPending}
            className="w-full mt-2"
            disabled={!username || !password || !confirm || !inviteCode}
          >
            {t("register.createAccountBtn")}
          </Button>
        </form>

        <p className="text-center text-sm text-[var(--text-muted)] mt-6">
          {t("register.alreadyHaveAccount")}{" "}
          <a href="/login" className="text-[var(--accent)] hover:underline">
            {t("register.signIn")}
          </a>
        </p>
      </Card>
    </div>
  );
}
