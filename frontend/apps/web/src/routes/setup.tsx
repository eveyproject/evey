import { useState } from "react";
import { Play } from "lucide-react";
import { createFileRoute, isRedirect, redirect, useNavigate } from "@tanstack/react-router";
import { eveyFetch, useAdminServiceSetupWizard, useLibraryServiceCreateLibrary } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { Card } from "@/components/ui/Card";

type SetupStatus = { setupComplete?: boolean };

export const Route = createFileRoute("/setup")({
  beforeLoad: async ({ context }) => {
    try {
      const info = await context.queryClient.fetchQuery<SetupStatus>({
        queryKey: ["setup-status"],
        queryFn: () => eveyFetch<SetupStatus>({ url: "/api/v1/setup/status", method: "GET" }),
        staleTime: 30_000,
      });
      if (info.setupComplete) throw redirect({ to: "/login" });
    } catch (e) {
      if (isRedirect(e)) throw e;
    }
  },
  component: SetupPage,
});

interface AdminForm {
  username: string;
  password: string;
  confirmPassword: string;
}

interface LibraryForm {
  name: string;
  type: string;
}

interface AdminErrors {
  username?: string;
  password?: string;
  confirmPassword?: string;
}

function StepDots({ current, total }: { current: number; total: number }) {
  return (
    <div className="flex items-center justify-center gap-2 mb-6">
      {Array.from({ length: total }).map((_, i) => (
        <div
          key={i}
          className="rounded-full transition-all duration-300"
          style={{
            width: i === current ? "20px" : "8px",
            height: "8px",
            backgroundColor: i <= current ? "var(--accent)" : "var(--border)",
          }}
        />
      ))}
    </div>
  );
}

function SetupPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { setTokens } = useAuthStore();
  const [step, setStep] = useState(0);
  const [adminForm, setAdminForm] = useState<AdminForm>({ username: "", password: "", confirmPassword: "" });
  const [adminErrors, setAdminErrors] = useState<AdminErrors>({});
  const [libraryForm, setLibraryForm] = useState<LibraryForm>({ name: "", type: "LIBRARY_TYPE_MOVIES" });
  const [libraryError, setLibraryError] = useState<string | undefined>();
  const [setupError, setSetupError] = useState<string | undefined>();

  const STEPS = [t("auth.setup.steps.welcome"), t("auth.setup.steps.admin"), t("auth.setup.steps.library")] as const;

  const LIBRARY_TYPE_OPTIONS = [
    { value: "LIBRARY_TYPE_MOVIES", label: t("auth.setup.libraryTypes.movies") },
    { value: "LIBRARY_TYPE_SHOWS", label: t("auth.setup.libraryTypes.shows") },
    { value: "LIBRARY_TYPE_MUSIC", label: t("auth.setup.libraryTypes.music") },
    { value: "LIBRARY_TYPE_PHOTOS", label: t("auth.setup.libraryTypes.photos") },
  ];

  function validateAdmin(form: AdminForm): AdminErrors {
    const errors: AdminErrors = {};
    if (form.username.length < 3) errors.username = t("auth.setup.usernameMin");
    if (form.password.length < 8) errors.password = t("auth.setup.passwordMin");
    if (form.password !== form.confirmPassword) errors.confirmPassword = t("auth.setup.passwordMismatch");
    return errors;
  }

  const setup = useAdminServiceSetupWizard({
    mutation: {
      onSuccess: (data) => {
        if (data.accessToken) setTokens(data.accessToken);
        setStep(2);
      },
      onError: (err: Error & { httpStatus?: number }) => {
        if (err?.httpStatus === 409) {
          void navigate({ to: "/login" });
          return;
        }
        setSetupError(err instanceof Error ? err.message : t("auth.setup.setupFailed"));
      },
    },
  });

  const createLibrary = useLibraryServiceCreateLibrary({
    mutation: {
      onSuccess: () => void navigate({ to: "/home" }),
      onError: (err) => {
        setLibraryError(err instanceof Error ? err.message : t("auth.setup.setupFailed"));
      },
    },
  });

  const updateAdmin = (field: keyof AdminForm) => (e: React.ChangeEvent<HTMLInputElement>) => {
    setAdminForm((prev) => ({ ...prev, [field]: e.target.value }));
    if (adminErrors[field]) setAdminErrors((prev) => ({ ...prev, [field]: undefined }));
  };

  const handleAdminSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const errors = validateAdmin(adminForm);
    if (Object.keys(errors).length > 0) {
      setAdminErrors(errors);
      return;
    }
    setSetupError(undefined);
    setup.mutate({
      data: {
        adminUsername: adminForm.username,
        adminPassword: adminForm.password,
        adminEmail: "",
        serverName: "Evey",
        baseUrl: window.location.origin,
      },
    });
  };

  const handleCreateLibrary = (e: React.FormEvent) => {
    e.preventDefault();
    if (!libraryForm.name.trim()) {
      setLibraryError(t("auth.setup.libraryNameRequired"));
      return;
    }
    setLibraryError(undefined);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    createLibrary.mutate({ data: { name: libraryForm.name, type: libraryForm.type as any } });
  };

  const logoIcon = (
    <div
      className="w-12 h-12 rounded-[var(--radius-md)] flex items-center justify-center mb-4 bg-[var(--accent)]"
    >
      <Play size={24} className="text-[var(--filter-active-text)]" fill="currentColor" />
    </div>
  );

  return (
    <div
      className="min-h-screen flex items-center justify-center px-4"
      style={{ background: "linear-gradient(135deg, var(--bg) 0%, var(--bg-gradient-mid) 50%, var(--bg) 100%)" }}
    >
      <Card className="w-full max-w-md p-8 shadow-2xl">
        <div className="flex flex-col items-center mb-2">{logoIcon}</div>

        <p className="text-center text-xs text-[var(--text-muted)] mb-1">
          {t("auth.setup.stepOf", { current: step + 1, total: STEPS.length, name: STEPS[step] })}
        </p>
        <StepDots current={step} total={STEPS.length} />

        {/* Step 0: Welcome */}
        {step === 0 && (
          <div className="flex flex-col items-center gap-6 text-center">
            <div>
              <h1 className="text-2xl font-bold text-[var(--text)]">{t("auth.setup.welcome")}</h1>
              <p className="text-[var(--text-muted)] text-sm mt-2">
                {t("auth.setup.description")}
              </p>
            </div>
            <Button className="w-full" onClick={() => setStep(1)}>
              {t("auth.setup.getStarted")}
            </Button>
          </div>
        )}

        {/* Step 1: Admin account */}
        {step === 1 && (
          <form onSubmit={handleAdminSubmit} className="flex flex-col gap-4">
            <h2 className="text-xl font-bold text-[var(--text)] text-center">{t("auth.setup.createAdmin")}</h2>
            <div className="flex flex-col gap-1">
              <Input
                label={t("auth.setup.username")}
                value={adminForm.username}
                onChange={updateAdmin("username")}
                autoComplete="username"
                autoFocus
              />
              {adminErrors.username && (
                <p className="text-xs text-[var(--color-danger)]">
                  {adminErrors.username}
                </p>
              )}
            </div>
            <div className="flex flex-col gap-1">
              <Input
                label={t("auth.setup.password")}
                type="password"
                value={adminForm.password}
                onChange={updateAdmin("password")}
                autoComplete="new-password"
              />
              {adminErrors.password && (
                <p className="text-xs text-[var(--color-danger)]">
                  {adminErrors.password}
                </p>
              )}
            </div>
            <div className="flex flex-col gap-1">
              <Input
                label={t("auth.setup.confirmPassword")}
                type="password"
                value={adminForm.confirmPassword}
                onChange={updateAdmin("confirmPassword")}
                autoComplete="new-password"
              />
              {adminErrors.confirmPassword && (
                <p className="text-xs text-[var(--color-danger)]">
                  {adminErrors.confirmPassword}
                </p>
              )}
            </div>
            {setupError && (
              <p className="text-sm text-center text-[var(--color-danger)]">
                {setupError}
              </p>
            )}
            <div className="flex gap-2 mt-2">
              <Button
                type="button"
                variant="secondary"
                className="flex-1"
                onClick={() => setStep(0)}
              >
                {t("common.back")}
              </Button>
              <Button type="submit" loading={setup.isPending} className="flex-1">
                {t("common.continue")}
              </Button>
            </div>
          </form>
        )}

        {/* Step 2: First library */}
        {step === 2 && (
          <form onSubmit={handleCreateLibrary} className="flex flex-col gap-4">
            <h2 className="text-xl font-bold text-[var(--text)] text-center">{t("auth.setup.addLibrary")}</h2>
            <p className="text-sm text-center text-[var(--text-muted)]">
              {t("auth.setup.addLibraryHint")}
            </p>
            <div className="flex flex-col gap-1">
              <Input
                label={t("auth.setup.libraryName")}
                value={libraryForm.name}
                onChange={(e) => {
                  setLibraryForm((prev) => ({ ...prev, name: e.target.value }));
                  setLibraryError(undefined);
                }}
                autoFocus
              />
              {libraryError && (
                <p className="text-xs text-[var(--color-danger)]">
                  {libraryError}
                </p>
              )}
            </div>
            <Select
              label={t("auth.setup.libraryType")}
              options={LIBRARY_TYPE_OPTIONS}
              value={libraryForm.type}
              onChange={(value) => setLibraryForm((prev) => ({ ...prev, type: value }))}
            />
            <div className="flex gap-2 mt-2">
              <Button
                type="button"
                variant="secondary"
                className="flex-1"
                onClick={() => void navigate({ to: "/home" })}
              >
                {t("common.skip")}
              </Button>
              <Button type="submit" loading={createLibrary.isPending} className="flex-1">
                {t("common.create")}
              </Button>
            </div>
          </form>
        )}
      </Card>
    </div>
  );
}
