import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowUpCircle, Copy, Check, ExternalLink } from "lucide-react";
import { UPDATE_COMMAND, changesUrl, latestLabel, useUpdateStatus } from "@evey/client";
import { ICON } from "@/constants/iconSizes";

// Shown on the dashboard only when the registry publishes a newer image than
// the one running. Evey never updates itself: the banner hands the admin the
// command to run next to their compose file.
export function DashboardUpdateBanner() {
  const { t } = useTranslation();
  const { data } = useUpdateStatus();
  const [copied, setCopied] = useState(false);
  if (!data || data.state !== "update_available") return null;

  const link = changesUrl(data);
  const copy = () => {
    void navigator.clipboard?.writeText(UPDATE_COMMAND).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <div
      className="flex flex-col gap-2 rounded-[var(--radius-lg)] border border-[var(--accent)] bg-[var(--surface)] p-4"
      role="status"
      data-testid="update-banner"
    >
      <p className="flex flex-wrap items-center gap-2 text-sm font-semibold text-[var(--text)]">
        <ArrowUpCircle size={ICON.md} className="text-[var(--accent)]" aria-hidden="true" />
        {t("admin.update.available", { latest: latestLabel(data) })}
        <span className="text-xs font-normal text-[var(--text-muted)]">
          {t("admin.update.current", { current: data.currentVersion })}
        </span>
      </p>
      <p className="text-xs text-[var(--text-muted)]">{t("admin.update.howTo")}</p>
      <div className="flex items-center gap-2">
        <code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap rounded bg-[var(--bg)] px-2 py-1 text-xs text-[var(--text)]">
          {UPDATE_COMMAND}
        </code>
        <button
          type="button"
          onClick={copy}
          className="rounded p-1.5 text-[var(--text-muted)] transition-colors hover:bg-[var(--border)] hover:text-[var(--text)]"
          aria-label={copied ? t("admin.update.copied") : t("admin.update.copy")}
          title={copied ? t("admin.update.copied") : t("admin.update.copy")}
        >
          {copied ? <Check size={ICON.sm} aria-hidden="true" /> : <Copy size={ICON.sm} aria-hidden="true" />}
        </button>
      </div>
      {link && (
        <a
          href={link}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex w-fit items-center gap-1 text-xs text-[var(--accent)] hover:underline"
        >
          {t("admin.update.changes")}
          <ExternalLink size={ICON.xs} aria-hidden="true" />
        </a>
      )}
    </div>
  );
}
