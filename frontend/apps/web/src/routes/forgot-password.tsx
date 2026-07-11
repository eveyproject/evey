import { useState } from "react";
import { createFileRoute, redirect, Link } from "@tanstack/react-router";
import { useUserServiceRequestPasswordReset, useAdminSystemServiceGetSetupStatus } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Card } from "@/components/ui/Card";

export const Route = createFileRoute("/forgot-password")({
  beforeLoad: () => {
    const { accessToken } = useAuthStore.getState();
    if (accessToken) throw redirect({ to: "/home" });
  },
  component: ForgotPasswordPage,
});

function ForgotPasswordPage() {
  const { t } = useTranslation();
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);

  // Public setup status exposes whether recovery is configured (SMTP). When it
  // is off, the backend returns ErrPasswordRecoveryDisabled, but the anti-leak
  // onError below would still show "check your email" - promising a mail that is
  // never sent. So short-circuit to an honest message instead.
  const setupStatus = useAdminSystemServiceGetSetupStatus({ query: { staleTime: 60 * 1000 } });

  const requestReset = useUserServiceRequestPasswordReset({
    mutation: {
      onSuccess: () => setSent(true),
      onError: () => setSent(true), // no leak - always show success
    },
  });

  if (setupStatus.data && !setupStatus.data.passwordRecoveryEnabled) {
    return (
      <div className="min-h-screen flex items-center justify-center px-4 gradient-auth-page">
        <Card className="w-full max-w-sm p-8 shadow-2xl text-center">
          <h1 className="text-xl font-bold text-[var(--text)] mb-4">{t("auth.forgotPassword.unavailableTitle")}</h1>
          <p className="text-sm text-[var(--text-muted)] mb-6">{t("auth.forgotPassword.unavailableDescription")}</p>
          <Link to="/login" className="text-sm text-[var(--accent)] hover:underline">
            {t("auth.forgotPassword.backToLogin")}
          </Link>
        </Card>
      </div>
    );
  }

  if (sent) {
    return (
      <div className="min-h-screen flex items-center justify-center px-4 gradient-auth-page">
        <Card className="w-full max-w-sm p-8 shadow-2xl text-center">
          <h1 className="text-xl font-bold text-[var(--text)] mb-4">{t("auth.forgotPassword.checkEmail")}</h1>
          <p className="text-sm text-[var(--text-muted)] mb-6">{t("auth.forgotPassword.emailSent")}</p>
          <Link to="/login" className="text-sm text-[var(--accent)] hover:underline">
            {t("auth.forgotPassword.backToLogin")}
          </Link>
        </Card>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center px-4 gradient-auth-page">
      <Card className="w-full max-w-sm p-8 shadow-2xl">
        <h1 className="text-xl font-bold text-[var(--text)] mb-2">{t("auth.forgotPassword.title")}</h1>
        <p className="text-sm text-[var(--text-muted)] mb-6">{t("auth.forgotPassword.description")}</p>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            requestReset.mutate({ data: { email } });
          }}
          className="flex flex-col gap-4"
        >
          <Input
            label={t("auth.forgotPassword.email")}
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            autoFocus
          />
          <Button type="submit" loading={requestReset.isPending} className="w-full">
            {t("auth.forgotPassword.submit")}
          </Button>
        </form>
        <div className="mt-4 text-center">
          <Link to="/login" className="text-sm text-[var(--accent)] hover:underline">
            {t("auth.forgotPassword.backToLogin")}
          </Link>
        </div>
      </Card>
    </div>
  );
}
