import { useState } from "react";
import { createFileRoute, redirect, Link, useNavigate } from "@tanstack/react-router";
import { useUserServiceResetPassword } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Card } from "@/components/ui/Card";

type ResetSearch = { token?: string };

export const Route = createFileRoute("/reset-password")({
  validateSearch: (search: Record<string, unknown>): ResetSearch => ({
    token: typeof search.token === "string" ? search.token : undefined,
  }),
  beforeLoad: () => {
    const { accessToken } = useAuthStore.getState();
    if (accessToken) throw redirect({ to: "/home" });
  },
  component: ResetPasswordPage,
});

function ResetPasswordPage() {
  const { t } = useTranslation();
  const { token } = Route.useSearch();
  const navigate = useNavigate();
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);

  const resetPassword = useUserServiceResetPassword({
    mutation: {
      onSuccess: () => setSuccess(true),
      onError: (err) => {
        const msg = err instanceof Error ? err.message.toLowerCase() : "";
        if (msg.includes("expired") || msg.includes("invalid"))
          setError(t("auth.resetPassword.tokenExpired"));
        else if (msg.includes("8 characters"))
          setError(t("auth.resetPassword.passwordTooShort"));
        else setError(t("auth.resetPassword.failed"));
      },
    },
  });

  if (!token) {
    return (
      <div className="min-h-screen flex items-center justify-center px-4 gradient-auth-page">
        <Card className="w-full max-w-sm p-8 shadow-2xl text-center">
          <h1 className="text-xl font-bold text-[var(--text)] mb-4">{t("auth.resetPassword.invalidLink")}</h1>
          <Link to="/login" className="text-sm text-[var(--accent)] hover:underline">
            {t("auth.resetPassword.backToLogin")}
          </Link>
        </Card>
      </div>
    );
  }

  if (success) {
    return (
      <div className="min-h-screen flex items-center justify-center px-4 gradient-auth-page">
        <Card className="w-full max-w-sm p-8 shadow-2xl text-center">
          <h1 className="text-xl font-bold text-[var(--text)] mb-4">{t("auth.resetPassword.success")}</h1>
          <p className="text-sm text-[var(--text-muted)] mb-6">{t("auth.resetPassword.successDescription")}</p>
          <Button onClick={() => void navigate({ to: "/login" })} className="w-full">
            {t("auth.resetPassword.goToLogin")}
          </Button>
        </Card>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center px-4 gradient-auth-page">
      <Card className="w-full max-w-sm p-8 shadow-2xl">
        <h1 className="text-xl font-bold text-[var(--text)] mb-2">{t("auth.resetPassword.title")}</h1>
        <p className="text-sm text-[var(--text-muted)] mb-6">{t("auth.resetPassword.description")}</p>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            setError("");
            if (password !== confirm) {
              setError(t("auth.resetPassword.mismatch"));
              return;
            }
            if (password.length < 8) {
              setError(t("auth.resetPassword.passwordTooShort"));
              return;
            }
            resetPassword.mutate({ data: { token, newPassword: password } });
          }}
          className="flex flex-col gap-4"
        >
          <Input
            label={t("auth.resetPassword.newPassword")}
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoFocus
          />
          <Input
            label={t("auth.resetPassword.confirmPassword")}
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            required
          />
          {error && (
            <p className="text-sm text-center text-[var(--color-danger)]" role="alert">{error}</p>
          )}
          <Button type="submit" loading={resetPassword.isPending} className="w-full">
            {t("auth.resetPassword.submit")}
          </Button>
        </form>
        <div className="mt-4 text-center">
          <Link to="/login" className="text-sm text-[var(--accent)] hover:underline">
            {t("auth.resetPassword.backToLogin")}
          </Link>
        </div>
      </Card>
    </div>
  );
}
