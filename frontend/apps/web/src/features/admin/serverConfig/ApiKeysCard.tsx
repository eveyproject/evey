import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { SettingRow } from "@/components/ui/SettingRow";
import { useSetConfigEntry, useDeleteConfigEntry, type ConfigEntry } from "./hooks";

const API_KEY_FIELDS = [
  {
    dbKey: "api.tmdb_key",
    labelKey: "admin.serverConfig.apiKeys.tmdb",
    linkKey: "admin.serverConfig.apiKeys.tmdbLink",
    infoKey: "admin.settingsHelp.apiKeyTmdb",
    linkUrl: "https://www.themoviedb.org/settings/api",
    hasDefault: true,
  },
  {
    dbKey: "api.omdb_key",
    labelKey: "admin.serverConfig.apiKeys.omdb",
    linkKey: "admin.serverConfig.apiKeys.omdbLink",
    infoKey: "admin.settingsHelp.apiKeyOmdb",
    linkUrl: "https://www.omdbapi.com/apikey.aspx",
    hasDefault: false,
  },
  {
    dbKey: "api.opensubtitles_key",
    labelKey: "admin.serverConfig.apiKeys.opensubtitles",
    linkKey: "admin.serverConfig.apiKeys.opensubtitlesLink",
    infoKey: "admin.settingsHelp.apiKeyOpenSubtitles",
    linkUrl: "https://www.opensubtitles.com/en/consumers",
    hasDefault: false,
  },
  {
    dbKey: "api.tidb_key",
    labelKey: "admin.serverConfig.apiKeys.tidb",
    linkKey: "admin.serverConfig.apiKeys.tidbLink",
    infoKey: "admin.settingsHelp.apiKeyTidb",
    linkUrl: "https://theintrodb.org/account",
    hasDefault: false,
  },
] as const;

// Mirrors appconfig.SecretMask (internal/app/config/secrets.go). The server
// stopped sending API keys back at all: a configured key reads as this mask, so
// the browser genuinely does not hold the value any more and there is nothing
// left here to reveal or to retype by accident.
const SECRET_MASK = "••••••••";

function maskKey(value: string): string {
  if (value.length <= 8) return value;
  return value.slice(0, 4) + "•".repeat(6) + value.slice(-4);
}

export function ApiKeysCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();
  const deleteEntry = useDeleteConfigEntry();

  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [editing, setEditing] = useState<Record<string, boolean>>({});
  const [showValue, setShowValue] = useState<Record<string, boolean>>({});

  function val(dbKey: string) {
    return entries?.find((e) => e.key === dbKey)?.value ?? "";
  }

  function handleSave(dbKey: string) {
    const v = drafts[dbKey] ?? "";
    // An untouched editor on a configured key must not wipe it. The value is
    // masked, so the operator cannot read it back and retype it - saving ""
    // here would destroy a credential with no way to recover it. Removing a key
    // is what Reset is for, and echoing the mask back means "unchanged" to the
    // server anyway.
    if (v === "" || v === SECRET_MASK) {
      setEditing((p) => ({ ...p, [dbKey]: false }));
      return;
    }
    setEntry.mutate(
      { key: dbKey, value: v },
      { onSuccess: () => toast.success(t("admin.serverConfig.apiKeys.saved")) },
    );
    setEditing((p) => ({ ...p, [dbKey]: false }));
  }

  function handleReset(dbKey: string) {
    deleteEntry.mutate(dbKey, {
      onSuccess: () => toast.success(t("admin.serverConfig.apiKeys.usingDefault")),
    });
    setEditing((p) => ({ ...p, [dbKey]: false }));
    setDrafts((p) => ({ ...p, [dbKey]: "" }));
  }

  return (
    <div className="flex flex-col divide-y divide-[var(--border)]">
      <p className="pb-3 text-xs text-[var(--text-muted)]">
        {t("admin.serverConfig.apiKeys.description")}
      </p>

      {API_KEY_FIELDS.map((field) => {
        const current = val(field.dbKey);
        const isEditing = editing[field.dbKey] ?? false;
        const isShown = showValue[field.dbKey] ?? false;

        return (
          <SettingRow
            key={field.dbKey}
            label={t(field.labelKey)}
            htmlFor={isEditing ? field.dbKey : undefined}
            info={t(field.infoKey)}
            hint={
              <a href={field.linkUrl} target="_blank" rel="noopener noreferrer" className="underline text-[var(--accent)]">
                {t(field.linkKey)}
              </a>
            }
            kind="wide"
            data-testid={`api-key-row-${field.dbKey}`}
          >
            <div className="flex flex-col gap-1.5">
              <div className="flex flex-wrap items-center gap-2">
                {field.hasDefault && !current && (
                  <span className="text-xs px-1.5 py-0.5 rounded bg-[var(--surface-raised)] text-[var(--text-muted)]">
                    {t("admin.serverConfig.apiKeys.usingDefault")}
                  </span>
                )}
                {current && (
                  <span className="text-xs px-1.5 py-0.5 rounded bg-[var(--accent-subtle)] text-[var(--accent)]">
                    {t("admin.serverConfig.apiKeys.usingCustom")}
                  </span>
                )}
              </div>

              {isEditing ? (
                <div className="flex flex-wrap gap-2">
                  <input
                    id={field.dbKey}
                    className="min-w-0 flex-1 px-3 py-1.5 rounded-[var(--radius-md)] text-sm font-mono bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
                    type="text"
                    value={drafts[field.dbKey] ?? ""}
                    placeholder={current ? t("admin.serverConfig.apiKeys.keepCurrent") : ""}
                    onChange={(e) => setDrafts((p) => ({ ...p, [field.dbKey]: e.target.value }))}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") handleSave(field.dbKey);
                      if (e.key === "Escape") setEditing((p) => ({ ...p, [field.dbKey]: false }));
                    }}
                    autoFocus
                  />
                  <Button variant="secondary" onClick={() => handleSave(field.dbKey)} loading={setEntry.isPending} className="text-xs">
                    {t("common.save")}
                  </Button>
                  <Button variant="ghost" onClick={() => setEditing((p) => ({ ...p, [field.dbKey]: false }))} className="text-xs">
                    {t("common.cancel")}
                  </Button>
                </div>
              ) : (
                <div className="flex flex-wrap items-center gap-2">
                  {current ? (
                    <span className="font-mono text-xs text-[var(--text-muted)]">
                      {isShown ? current : maskKey(current)}
                    </span>
                  ) : (
                    <span className="text-xs text-[var(--text-muted)]">
                      {field.hasDefault ? maskKey("eyJhbGciOiJIUzI1NiJ9...V51Q") : "---"}
                    </span>
                  )}
                  {current && current !== SECRET_MASK && (
                    <button
                      type="button"
                      onClick={() => setShowValue((p) => ({ ...p, [field.dbKey]: !p[field.dbKey] }))}
                      className="px-1.5 rounded text-xs border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text-muted)]"
                      aria-label={isShown ? t("admin.serverConfig.hide") : t("admin.serverConfig.show")}
                    >
                      {isShown ? t("admin.serverConfig.hide") : t("admin.serverConfig.show")}
                    </button>
                  )}
                  <Button variant="secondary" onClick={() => { setDrafts((p) => ({ ...p, [field.dbKey]: current })); setEditing((p) => ({ ...p, [field.dbKey]: true })); }} className="text-xs">
                    {t("common.edit")}
                  </Button>
                  {field.hasDefault && current && (
                    <Button variant="ghost" onClick={() => handleReset(field.dbKey)} loading={deleteEntry.isPending} className="text-xs">
                      {t("admin.serverConfig.apiKeys.resetToDefault")}
                    </Button>
                  )}
                </div>
              )}
            </div>
          </SettingRow>
        );
      })}
    </div>
  );
}
