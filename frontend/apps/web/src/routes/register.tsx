import { useState } from "react";
import { XCircle } from "lucide-react";
import { EveyWordmark } from "@/components/ui/EveyLogo";
import { createFileRoute, redirect, useNavigate, Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useUserServiceRegisterWithInvite, eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Card } from "@/components/ui/Card";
import { Spinner } from "@/components/ui/Spinner";

interface InviteValidation {
  valid?: boolean;
  isAdmin?: boolean;
}

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
  const { t } = useTranslation();
  const { invite } = Route.useSearch();
  const navigate = useNavigate();
  const { setTokens } = useAuthStore();

  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [inviteCode, setInviteCode] = useState(invite ?? "");
  const [validationError, setValidationError] = useState("");
  // Set when a submit is rejected because the invite is no longer usable (the
  // page may have been open since before the link was disabled).
  const [inviteRejected, setInviteRejected] = useState(false);

  // Validate the link from the URL up front, so a disabled/expired/used invite
  // shows a clear message instead of a full form that only fails on submit.
  const { data: validation, isLoading: validating } = useQuery<InviteValidation>({
    queryKey: ["invite-validate", invite],
    queryFn: () =>
      eveyFetch<InviteValidation>({
        url: `/api/v1/invites/${encodeURIComponent(invite ?? "")}/validate`,
        method: "GET",
      }),
    enabled: !!invite,
    retry: false,
    staleTime: 30_000,
  });
  const inviteUnusable = !!invite && (inviteRejected || (!validating && validation?.valid === false));

  const register = useUserServiceRegisterWithInvite({
    mutation: {
      onSuccess: (data) => {
        if (data.accessToken) {
          setTokens(data.accessToken);
          void navigate({ to: "/home" });
        }
      },
      onError: () => {
        // The server rejects any unusable invite at submit time; surface the
        // dedicated "link no longer valid" state rather than a raw error.
        setInviteRejected(true);
      },
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setValidationError("");
    if (password !== confirm) {
      setValidationError(t("auth.register.passwordMismatch"));
      return;
    }
    if (password.length < 8) {
      setValidationError(t("auth.register.passwordMin"));
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

  // Validating the link from the URL - hold the form until we know it's usable.
  if (invite && validating && !inviteRejected) {
    return (
      <div className="min-h-screen flex items-center justify-center px-4 gradient-auth-page">
        <Card className="w-full max-w-sm p-8 shadow-2xl flex flex-col items-center gap-4" data-testid="invite-checking">
          <Spinner size="md" />
          <p className="text-[var(--text-muted)] text-sm">{t("auth.register.inviteChecking")}</p>
        </Card>
      </div>
    );
  }

  // Disabled / expired / already-used invite - no form, just a clear message.
  if (inviteUnusable) {
    return (
      <div className="min-h-screen flex items-center justify-center px-4 gradient-auth-page">
        <Card className="w-full max-w-sm p-8 shadow-2xl flex flex-col items-center text-center gap-4" data-testid="invite-invalid">
          <div className="w-12 h-12 rounded-[var(--radius-md)] flex items-center justify-center bg-[var(--badge-error-bg,#7f1d1d)]">
            <XCircle size={24} className="text-[var(--badge-error-text,#fca5a5)]" />
          </div>
          <h1 className="text-xl font-bold text-[var(--text)]">{t("auth.register.inviteInvalidTitle")}</h1>
          <p className="text-[var(--text-muted)] text-sm">{t("auth.register.inviteInvalidMessage")}</p>
          <Link to="/login" className="text-sm underline text-[var(--accent)] hover:opacity-80">
            {t("auth.register.backToLogin")}
          </Link>
        </Card>
      </div>
    );
  }

  return (
    <div
      className="min-h-screen flex items-center justify-center px-4 gradient-auth-page"
    >
      <Card className="w-full max-w-sm p-8 shadow-2xl">
        <div className="flex flex-col items-center mb-8">
          <EveyWordmark height={36} className="mb-4 text-[var(--text)]" />
          <h1 className="text-2xl font-bold text-[var(--text)]">{t("auth.register.createAccount")}</h1>
          {invite ? (
            <p className="text-[var(--text-muted)] text-sm mt-1 text-center">
              {t("auth.register.invitedMessage")}
            </p>
          ) : (
            <p className="text-[var(--text-muted)] text-sm mt-1">{t("auth.register.signUpWithInvite")}</p>
          )}
        </div>

        {invite && (
          <div
            className="mb-4 px-3 py-2 rounded-[var(--radius-md)] text-sm bg-[var(--surface-raised)] text-[var(--text-muted)]"
          >
            {t("auth.register.inviteCodeLabel")} <span className="font-mono text-[var(--text)]">{invite}</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <Input
            label={t("auth.register.username")}
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoFocus
          />
          <Input
            label={t("auth.register.email")}
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
          <Input
            label={t("auth.register.password")}
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
          <Input
            label={t("auth.register.confirmPassword")}
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            required
          />
          {!invite && (
            <Input
              label={t("auth.register.inviteCode")}
              value={inviteCode}
              onChange={(e) => setInviteCode(e.target.value)}
              required
            />
          )}
          {(validationError || register.error) && (
            <p className="text-sm text-center text-[var(--color-danger)]">
              {validationError || (register.error instanceof Error ? register.error.message : t("auth.register.registrationFailed"))}
            </p>
          )}
          <Button
            type="submit"
            loading={register.isPending}
            className="w-full mt-2"
            disabled={!username || !password || !confirm || !inviteCode}
          >
            {t("auth.register.createAccountBtn")}
          </Button>
        </form>

        <p className="text-center text-sm text-[var(--text-muted)] mt-6">
          {t("auth.register.alreadyHaveAccount")}{" "}
          <Link to="/login" className="text-[var(--accent)] hover:underline">
            {t("auth.register.signIn")}
          </Link>
        </p>
      </Card>
    </div>
  );
}
