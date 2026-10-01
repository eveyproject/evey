import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { eveyFetch, apiErrorMessage, useLibraryServiceListLibraries } from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Switch } from "@/components/ui/Switch";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";
import { useDemoLock } from "@/hooks/useDemoMode";

// SsoSettingsCard configures OpenID Connect sign-in. The server reads these
// settings live (GET/PUT /api/v1/admin/sso): saving applies without a restart,
// and the secret only ever comes back as the mask.

export type SsoStatus = {
  state: "disabled" | "incomplete" | "ok" | "error";
  error?: string;
  checkedAt?: string;
  endSessionEndpoint?: string;
};

export type SsoSettings = {
  enabled: boolean;
  issuer: string;
  clientId: string;
  clientSecret: string;
  allowedEmailDomains: string[];
  buttonLabel: string;
  linkByEmail: boolean;
  allowSignup: boolean;
  allowSignupDefault: boolean;
  defaultLibraryIds: string[];
  redirectUri: string;
  postLogoutRedirectUri: string;
  source: "panel" | "env" | "none";
  status: SsoStatus;
};

const SSO_URL = "/api/v1/admin/sso";
export const SSO_QUERY_KEY = ["admin", "sso"];

const STATUS_VARIANT: Record<SsoStatus["state"], "success" | "error" | "warning" | "default"> = {
  ok: "success",
  error: "error",
  incomplete: "warning",
  disabled: "default",
};

function CopyField({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs font-medium text-[var(--text-muted)]">{label}</span>
      <div className="flex items-center gap-2">
        <code className="flex-1 min-w-0 break-all rounded-[var(--radius-sm)] bg-[var(--surface-raised)] px-2 py-1 text-xs text-[var(--text)]">
          {value}
        </code>
        <Button
          type="button"
          variant="secondary"
          onClick={() => {
            void navigator.clipboard?.writeText(value).then(() => {
              setCopied(true);
              setTimeout(() => setCopied(false), 1500);
            });
          }}
        >
          {copied ? t("admin.sso.copied") : t("admin.sso.copy")}
        </Button>
      </div>
    </div>
  );
}

export function SsoSettingsCard() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const { lock } = useDemoLock();
  const { data, isLoading } = useQuery<SsoSettings>({
    queryKey: SSO_QUERY_KEY,
    queryFn: () => eveyFetch<SsoSettings>({ url: SSO_URL, method: "GET" }),
  });
  const { data: librariesData } = useLibraryServiceListLibraries();
  const libraries = librariesData?.libraries ?? [];

  const [enabled, setEnabled] = useState(false);
  const [issuer, setIssuer] = useState("");
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [domains, setDomains] = useState("");
  const [buttonLabel, setButtonLabel] = useState("");
  const [linkByEmail, setLinkByEmail] = useState(true);
  const [allowSignup, setAllowSignup] = useState(false);
  const [restrictLibraries, setRestrictLibraries] = useState(false);
  const [libraryIds, setLibraryIds] = useState<string[]>([]);
  const [testResult, setTestResult] = useState<{ ok: boolean; error?: string } | null>(null);

  // Seed the form from the server; a refetch after Save re-seeds it, which
  // is what we want (the server normalises the domains, masks the secret).
  useEffect(() => {
    if (!data) return;
    setEnabled(data.enabled);
    setIssuer(data.issuer);
    setClientId(data.clientId);
    setClientSecret(data.clientSecret);
    setDomains(data.allowedEmailDomains.join(", "));
    setButtonLabel(data.buttonLabel);
    setLinkByEmail(data.linkByEmail);
    setAllowSignup(data.allowSignup);
    setRestrictLibraries(data.defaultLibraryIds.length > 0);
    setLibraryIds(data.defaultLibraryIds);
  }, [data]);

  // The payload is built at click time and passed in: a mutationFn closing
  // over the form state could send the values of an earlier render.
  const save = useMutation({
    mutationFn: (body: Record<string, unknown>) => eveyFetch<SsoSettings>({ url: SSO_URL, method: "PUT", data: body }),
    onSuccess: (next) => {
      qc.setQueryData(SSO_QUERY_KEY, next);
      // The login page caches whether SSO is offered.
      void qc.invalidateQueries({ queryKey: ["oidc-config"] });
      setTestResult(null);
      toast.success(t("admin.sso.saved"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.sso.saveFailed"))),
  });

  const test = useMutation({
    mutationFn: (testIssuer: string) =>
      eveyFetch<{ ok: boolean; error?: string }>({ url: `${SSO_URL}/test`, method: "POST", data: { issuer: testIssuer } }),
    onSuccess: (res) => {
      setTestResult(res);
      void qc.invalidateQueries({ queryKey: SSO_QUERY_KEY });
    },
    onError: (err: Error) => setTestResult({ ok: false, error: apiErrorMessage(err, t, t("common.error")) }),
  });

  if (isLoading || !data) {
    return (
      <div className="flex justify-center py-6">
        <Spinner size="md" />
      </div>
    );
  }

  const selectionMissing = restrictLibraries && libraryIds.length === 0;
  const status = data.status;
  const checkedAt = status.checkedAt ? new Date(status.checkedAt).toLocaleString(i18n.language) : "";

  return (
    <div className="flex flex-col gap-5" data-testid="sso-settings">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium text-[var(--text)]">{t("admin.sso.status")}</span>
        <Badge variant={STATUS_VARIANT[status.state]} data-testid="sso-status">
          {t(`admin.sso.state.${status.state}`)}
        </Badge>
        {checkedAt && <span className="text-xs text-[var(--text-muted)]">{t("admin.sso.lastChecked", { time: checkedAt })}</span>}
      </div>
      {status.error && (
        <p className="text-xs text-[var(--color-danger)] break-words" role="alert" data-testid="sso-status-error">
          {status.error}
        </p>
      )}
      {data.source === "env" && <p className="text-xs text-[var(--text-muted)]">{t("admin.sso.sourceEnv")}</p>}

      <div className="flex items-center gap-3">
        <Switch checked={enabled} onChange={() => setEnabled((v) => !v)} label={t("admin.sso.enabled")} data-testid="sso-enabled" />
        <span className="text-sm text-[var(--text)]">{t("admin.sso.enabled")}</span>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-1 sm:col-span-2">
          <Input id="sso-issuer" label={t("admin.sso.issuer")} placeholder="https://auth.example.com/realms/home" value={issuer} onChange={(e) => setIssuer(e.target.value)} />
          <p className="text-xs text-[var(--text-muted)]">{t("admin.sso.issuerHint")}</p>
        </div>
        <Input id="sso-client-id" label={t("admin.sso.clientId")} value={clientId} onChange={(e) => setClientId(e.target.value)} autoComplete="off" />
        <div className="flex flex-col gap-1">
          <Input id="sso-client-secret" label={t("admin.sso.clientSecret")} type="password" value={clientSecret} onChange={(e) => setClientSecret(e.target.value)} autoComplete="new-password" />
          <p className="text-xs text-[var(--text-muted)]">{t("admin.sso.clientSecretHint")}</p>
        </div>
        <div className="flex flex-col gap-1">
          <Input id="sso-domains" label={t("admin.sso.allowedDomains")} placeholder="example.com" value={domains} onChange={(e) => setDomains(e.target.value)} />
          <p className="text-xs text-[var(--text-muted)]">{t("admin.sso.allowedDomainsHint")}</p>
        </div>
        <div className="flex flex-col gap-1">
          <Input id="sso-label" label={t("admin.sso.buttonLabel")} placeholder={t("auth.login.signInWithSSO")} value={buttonLabel} maxLength={64} onChange={(e) => setButtonLabel(e.target.value)} />
          <p className="text-xs text-[var(--text-muted)]">{t("admin.sso.buttonLabelHint")}</p>
        </div>
      </div>

      <div className="flex flex-col gap-4 border-t border-[var(--border)] pt-4">
        <div className="flex items-start gap-3">
          <Switch checked={linkByEmail} onChange={() => setLinkByEmail((v) => !v)} label={t("admin.sso.linkByEmail")} data-testid="sso-link" />
          <div className="flex flex-col gap-0.5">
            <span className="text-sm text-[var(--text)]">{t("admin.sso.linkByEmail")}</span>
            <span className="text-xs text-[var(--text-muted)]">{t("admin.sso.linkByEmailHint")}</span>
          </div>
        </div>
        <div className="flex items-start gap-3">
          <Switch checked={allowSignup} onChange={() => setAllowSignup((v) => !v)} label={t("admin.sso.allowSignup")} data-testid="sso-signup" />
          <div className="flex flex-col gap-0.5">
            <span className="text-sm text-[var(--text)]">{t("admin.sso.allowSignup")}</span>
            <span className="text-xs text-[var(--text-muted)]">{t("admin.sso.allowSignupHint")}</span>
          </div>
        </div>
        {allowSignup && (
          <fieldset className="flex flex-col gap-2">
            <legend className="text-sm font-medium text-[var(--text)] mb-1">{t("admin.sso.defaultAccess")}</legend>
            <label className="flex items-center gap-2 text-sm text-[var(--text)] cursor-pointer">
              <input type="radio" name="sso-access" checked={!restrictLibraries} onChange={() => setRestrictLibraries(false)} />
              {t("admin.sso.defaultAccessAll")}
            </label>
            <label className="flex items-center gap-2 text-sm text-[var(--text)] cursor-pointer">
              <input type="radio" name="sso-access" checked={restrictLibraries} onChange={() => setRestrictLibraries(true)} data-testid="sso-access-selected" />
              {t("admin.sso.defaultAccessSelected")}
            </label>
            {restrictLibraries && (
              <div className="flex flex-col gap-1 max-h-48 overflow-y-auto rounded-[var(--radius-sm)] border border-[var(--border)] p-2 bg-[var(--surface)]">
                {libraries.map((lib) => {
                  const id = lib.id ?? "";
                  return (
                    <label key={id} className="flex items-center gap-2 text-sm text-[var(--text)] cursor-pointer px-2 py-1 rounded hover:bg-[var(--surface-raised)]">
                      <input
                        type="checkbox"
                        checked={libraryIds.includes(id)}
                        onChange={() => setLibraryIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))}
                      />
                      <span className="truncate">{lib.name ?? id}</span>
                    </label>
                  );
                })}
              </div>
            )}
            {selectionMissing && <p className="text-xs text-[var(--color-danger)]">{t("admin.sso.defaultAccessNone")}</p>}
          </fieldset>
        )}
      </div>

      <div className="flex flex-col gap-3 border-t border-[var(--border)] pt-4">
        <p className="text-sm font-medium text-[var(--text)]">{t("admin.sso.registerTitle")}</p>
        <CopyField label={t("admin.sso.redirectUri")} value={data.redirectUri} />
        <CopyField label={t("admin.sso.postLogoutUri")} value={data.postLogoutRedirectUri} />
        <p className="text-xs text-[var(--text-muted)]">{t("admin.sso.registerHint")}</p>
        {status.state === "ok" && (
          <p className="text-xs text-[var(--text-muted)]">
            {status.endSessionEndpoint ? t("admin.sso.logoutSupported") : t("admin.sso.logoutUnsupported")}
          </p>
        )}
      </div>

      {testResult && (
        <p className={`text-xs break-words ${testResult.ok ? "text-[var(--text-muted)]" : "text-[var(--color-danger)]"}`} role="status" data-testid="sso-test-result">
          {testResult.ok ? t("admin.sso.testOk") : t("admin.sso.testFailed", { error: testResult.error ?? "" })}
        </p>
      )}

      <div className="flex flex-wrap justify-end gap-2">
        <Button type="button" variant="secondary" onClick={() => test.mutate(issuer)} loading={test.isPending} disabled={!issuer.trim() || lock.disabled} title={lock.title}>
          {t("admin.sso.test")}
        </Button>
        <Button type="button" onClick={() =>
            save.mutate({
              enabled,
              issuer,
              clientId,
              clientSecret,
              allowedEmailDomains: domains.split(",").map((d) => d.trim()).filter(Boolean),
              buttonLabel,
              linkByEmail,
              allowSignup,
              defaultLibraryIds: restrictLibraries ? libraryIds : [],
            })
          } loading={save.isPending} disabled={selectionMissing || lock.disabled} title={lock.title} data-testid="sso-save">
          {t("admin.sso.save")}
        </Button>
      </div>
    </div>
  );
}
