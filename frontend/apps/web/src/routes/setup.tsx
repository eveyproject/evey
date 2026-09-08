import { useState, useEffect, useMemo } from "react";
import { EveyWordmark } from "@/components/ui/EveyLogo";
import { createFileRoute, isRedirect, redirect, useNavigate } from "@tanstack/react-router";
import {
  eveyFetch,
  useAdminSystemServiceSetupWizard,
  useLibraryServiceCreateLibrary,
  adminSystemServiceGetSetupStatus,
  defaultRegionForLocale,
} from "@evey/api";
import type { V1LibraryType } from "@evey/api";
import { useAuthStore, useLocaleStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { supportedLanguages } from "@/i18n";
import { ISO_REGIONS } from "@/features/user/regionList";
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
        queryFn: () => adminSystemServiceGetSetupStatus(),
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
  type: V1LibraryType;
  paths: string[];
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
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { setTokens } = useAuthStore();
  const setLocaleStore = useLocaleStore((s) => s.setLocale);
  const [step, setStep] = useState(0);
  const [serverLang, setServerLang] = useState<string>(() => {
    const code = (i18n.language || "en").split("-")[0];
    return supportedLanguages.some((l) => l.code === code) ? code : "en";
  });
  // Admin can still pick any region; this only pre-fills to the natural
  // default for the initial language. Once the user overrides, we keep their
  // choice even if they change the language afterwards (tracked via userPicked).
  const [serverRegion, setServerRegion] = useState<string>(() => {
    const code = (i18n.language || "en").split("-")[0];
    return defaultRegionForLocale(code);
  });
  const [regionUserPicked, setRegionUserPicked] = useState(false);
  const [adminForm, setAdminForm] = useState<AdminForm>({ username: "", password: "", confirmPassword: "" });
  const [adminErrors, setAdminErrors] = useState<AdminErrors>({});
  const [libraryForm, setLibraryForm] = useState<LibraryForm>({ name: "", type: "LIBRARY_TYPE_MOVIES", paths: [""] });
  const [libraryError, setLibraryError] = useState<string | undefined>();
  const [mediaDirs, setMediaDirs] = useState<string[]>([]);

  // Region options in the currently-selected language. Falls back to hardcoded
  // ISO list when Intl.supportedValuesOf("region") is unavailable.
  const regionOptions = useMemo(() => {
    try {
      const sv = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf;
      const dynamic: string[] = typeof sv === "function" ? sv("region") : [];
      const codes = dynamic.length > 0 ? dynamic : Array.from(ISO_REGIONS);
      const names = new Intl.DisplayNames([serverLang || "en"], { type: "region" });
      return codes
        .filter((c) => /^[A-Z]{2}$/.test(c))
        .map((c) => ({ value: c, label: names.of(c) ?? c }))
        .sort((a, b) => a.label.localeCompare(b.label, serverLang || "en"));
    } catch {
      return Array.from(ISO_REGIONS).map((c) => ({ value: c, label: c }));
    }
  }, [serverLang]);

  // Apply language immediately so the rest of the wizard is already localized.
  // If the admin hasn't explicitly picked a region, suggest the natural default
  // for the new language (FR for fr, DE for de, etc.).
  const handlePickLanguage = (code: string) => {
    setServerLang(code);
    setLocaleStore(code);
    void i18n.changeLanguage(code);
    if (!regionUserPicked) {
      setServerRegion(defaultRegionForLocale(code));
    }
  };

  const handlePickRegion = (code: string) => {
    setServerRegion(code);
    setRegionUserPicked(true);
  };

  useEffect(() => {
    if (step !== 3) return;
    (async () => {
      try {
        const dirs = await eveyFetch<{ name: string; path: string }[]>({
          url: "/api/v1/admin/fs/browse",
          method: "GET",
        });
        setMediaDirs(dirs.map((d) => d.path));
      } catch (err) {
        console.warn("fs browse failed", err);
        setMediaDirs([]);
      }
    })();
  }, [step]);
  const [setupError, setSetupError] = useState<string | undefined>();

  const STEPS = [
    t("auth.setup.steps.language"),
    t("auth.setup.steps.welcome"),
    t("auth.setup.steps.admin"),
    t("auth.setup.steps.library"),
  ] as const;

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

  const setup = useAdminSystemServiceSetupWizard({
    mutation: {
      onSuccess: (data) => {
        if (data.accessToken) setTokens(data.accessToken);
        setStep(3);
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
        serverLanguage: serverLang,
        serverRegion: serverRegion || undefined,
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
    const validPaths = libraryForm.paths.filter((p) => p.trim() !== "" && p !== "__custom__");
    if (validPaths.length === 0) {
      setLibraryError(t("admin.libraries.pathRequired") || "At least one path is required");
      return;
    }
    createLibrary.mutate({ data: { name: libraryForm.name, type: libraryForm.type, paths: validPaths } });
  };

  const logoIcon = <EveyWordmark height={36} className="mb-4 text-[var(--text)]" />;

  return (
    <div
      className="min-h-screen flex items-center justify-center px-4 gradient-auth-page"
    >
      <Card className="w-full max-w-md p-8 shadow-2xl">
        <div className="flex flex-col items-center mb-2">{logoIcon}</div>

        <p className="text-center text-xs text-[var(--text-muted)] mb-1">
          {t("auth.setup.stepOf", { current: step + 1, total: STEPS.length, name: STEPS[step] })}
        </p>
        <StepDots current={step} total={STEPS.length} />

        {/* Step 0: Language + Region */}
        {step === 0 && (
          <div className="flex flex-col gap-4">
            <h2 className="text-xl font-bold text-[var(--text)] text-center">{t("auth.setup.languageTitle")}</h2>
            <p className="text-sm text-center text-[var(--text-muted)]">
              {t("auth.setup.languageHint")}
            </p>
            <Select
              label={t("common.language")}
              value={serverLang}
              onChange={handlePickLanguage}
              options={supportedLanguages.map((l) => ({ value: l.code, label: l.label }))}
            />
            <Select
              label={t("common.region.title")}
              value={serverRegion}
              onChange={handlePickRegion}
              options={[{ value: "", label: t("common.region.auto") }, ...regionOptions]}
            />
            <Button className="w-full mt-2" onClick={() => setStep(1)}>
              {t("common.continue")}
            </Button>
          </div>
        )}

        {/* Step 1: Welcome */}
        {step === 1 && (
          <div className="flex flex-col items-center gap-6 text-center">
            <div>
              <h1 className="text-2xl font-bold text-[var(--text)]">{t("auth.setup.welcome")}</h1>
              <p className="text-[var(--text-muted)] text-sm mt-2">
                {t("auth.setup.description")}
              </p>
            </div>
            <div className="flex gap-2 w-full">
              <Button variant="secondary" className="flex-1" onClick={() => setStep(0)}>
                {t("common.back")}
              </Button>
              <Button className="flex-1" onClick={() => setStep(2)}>
                {t("auth.setup.getStarted")}
              </Button>
            </div>
          </div>
        )}

        {/* Step 2: Admin account */}
        {step === 2 && (
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
                onClick={() => setStep(1)}
              >
                {t("common.back")}
              </Button>
              <Button type="submit" loading={setup.isPending} className="flex-1">
                {t("common.continue")}
              </Button>
            </div>
          </form>
        )}

        {/* Step 3: First library */}
        {step === 3 && (
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
              onChange={(value) => setLibraryForm((prev) => ({ ...prev, type: value as V1LibraryType }))}
            />
            <div className="flex flex-col gap-2">
              <span className="text-sm font-medium text-[var(--text-muted)]">
                {t("admin.libraries.paths")}
              </span>
              {libraryForm.paths.map((p, i) => (
                <div key={i} className="flex gap-2 items-center">
                  {mediaDirs.length > 0 && p !== "__custom__" ? (
                    <Select
                      value={p}
                      onChange={(v) => {
                        setLibraryForm((prev) => ({
                          ...prev,
                          paths: prev.paths.map((pp, idx) => (idx === i ? v : pp)),
                        }));
                        setLibraryError(undefined);
                      }}
                      placeholder={t("admin.libraries.selectDirectory")}
                      options={[
                        ...mediaDirs.map((d) => ({ value: d, label: d })),
                        { value: "__custom__", label: t("admin.libraries.enterCustomPath") },
                      ]}
                      className="flex-1"
                    />
                  ) : (
                    <Input
                      placeholder="/media/movies"
                      value={p === "__custom__" ? "" : p}
                      onChange={(e) => {
                        const val = e.target.value;
                        setLibraryForm((prev) => ({
                          ...prev,
                          paths: prev.paths.map((pp, idx) => (idx === i ? val : pp)),
                        }));
                        setLibraryError(undefined);
                      }}
                      className="flex-1"
                    />
                  )}
                </div>
              ))}
            </div>
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
