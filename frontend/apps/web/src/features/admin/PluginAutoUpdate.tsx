import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  getAdminPluginServiceListPluginsQueryKey,
  useAdminPluginServiceCheckPluginUpdate,
  useAdminPluginServiceSetPluginUpdateFollow,
  type V1Plugin,
} from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Switch } from "@/components/ui/Switch";
import { useDemoLockFor } from "@/hooks/useDemoMode";

const OUTCOME_KEYS: Record<string, string> = {
  up_to_date: "admin.plugin.autoUpdate.outcome.upToDate",
  applied: "admin.plugin.autoUpdate.outcome.applied",
  held: "admin.plugin.autoUpdate.outcome.held",
  refused: "admin.plugin.autoUpdate.outcome.refused",
  failed: "admin.plugin.autoUpdate.outcome.failed",
};

const OUTCOME_TONE: Record<string, string> = {
  held: "text-[var(--badge-warning-text,#fde047)]",
  refused: "text-[var(--badge-error-text,#fca5a5)]",
  failed: "text-[var(--badge-error-text,#fca5a5)]",
};

interface Props {
  plugin: V1Plugin;
  // Opens the manual update dialog on the followed repository, for a release
  // that waits for the admin.
  onReview: (source: string) => void;
}

/**
 * The plugin's updates: the repository it follows, whether a release installs
 * on its own, and what the last look at the repository found.
 */
export function PluginAutoUpdate({ plugin, onReview }: Props) {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const { lock } = useDemoLockFor("/evey.v1.AdminPluginService/SetPluginUpdateFollow");
  const [editing, setEditing] = useState(false);
  const [repoInput, setRepoInput] = useState(plugin.updateSource ?? "");
  const refresh = () => queryClient.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() });

  const follow = useAdminPluginServiceSetPluginUpdateFollow({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.plugin.autoUpdate.saved"));
        setEditing(false);
        void refresh();
      },
      onError: (err: unknown) => {
        const msg = (err as { message?: string })?.message;
        toast.error(msg || t("admin.plugin.autoUpdate.saveFailed"));
      },
    },
  });
  const check = useAdminPluginServiceCheckPluginUpdate({
    mutation: {
      onSuccess: () => void refresh(),
      onError: (err: unknown) => {
        const msg = (err as { message?: string })?.message;
        toast.error(msg || t("admin.plugin.autoUpdate.checkFailed"));
      },
    },
  });

  if (!plugin.id) return null;
  const id = plugin.id;
  const source = plugin.updateSource ?? "";
  const auto = plugin.autoUpdate ?? true;
  const state = plugin.updateState;
  const outcomeKey = OUTCOME_KEYS[state?.outcome ?? ""];

  const save = (nextSource: string, nextAuto: boolean) =>
    follow.mutate({ pluginId: id, data: { updateSource: nextSource.trim(), autoUpdate: nextAuto } });

  return (
    <section className="border-t pt-3 mt-1 border-[var(--border)] flex flex-col gap-2" data-testid="plugin-auto-update">
      <p className="text-xs font-semibold text-[var(--text-muted)]">{t("admin.plugin.autoUpdate.title")}</p>

      {editing ? (
        <div className="flex gap-2 items-end flex-wrap">
          <div className="flex-1 min-w-[14rem]">
            <Input
              label={t("admin.plugin.autoUpdate.repository")}
              placeholder="https://github.com/owner/repo"
              value={repoInput}
              onChange={(e) => setRepoInput(e.target.value)}
              data-testid="plugin-auto-update-repo-input"
            />
          </div>
          <Button variant="primary" onClick={() => save(repoInput, auto)} disabled={follow.isPending} data-testid="plugin-auto-update-repo-save">
            {t("common.save")}
          </Button>
          <Button variant="ghost" onClick={() => { setEditing(false); setRepoInput(source); }}>
            {t("common.cancel")}
          </Button>
        </div>
      ) : (
        <div className="flex items-center gap-2 flex-wrap text-xs">
          <span className="text-[var(--text-muted)]">{t("admin.plugin.autoUpdate.repository")}</span>
          {source ? (
            <a href={source} target="_blank" rel="noopener noreferrer" className="font-mono break-all text-[var(--text)] hover:underline">
              {source.replace(/^https:\/\//, "")}
            </a>
          ) : (
            <span className="text-[var(--text-muted)]">{t("admin.plugin.autoUpdate.repositoryNone")}</span>
          )}
          <Button variant="ghost" onClick={() => setEditing(true)} data-testid="plugin-auto-update-repo-edit" {...lock}>
            {t("admin.plugin.autoUpdate.edit")}
          </Button>
        </div>
      )}

      <div className="flex items-start gap-3">
        <div className="shrink-0 pt-0.5">
          <Switch
            checked={auto}
            onChange={() => save(source, !auto)}
            label={t("admin.plugin.autoUpdate.toggle")}
            disabled={!source || follow.isPending || !!lock.disabled}
            data-testid="plugin-auto-update-switch"
          />
        </div>
        <div className="flex flex-col gap-0.5 min-w-0">
          <span className="text-sm text-[var(--text)]">{t("admin.plugin.autoUpdate.toggle")}</span>
          <span className="text-xs text-[var(--text-muted)]">{t("admin.plugin.autoUpdate.toggleHint")}</span>
        </div>
      </div>

      {source && (
        <div className="flex items-center gap-2 flex-wrap text-xs" data-testid="plugin-auto-update-state">
          {outcomeKey ? (
            <span className={OUTCOME_TONE[state?.outcome ?? ""] ?? "text-[var(--text-muted)]"}>
              {t(outcomeKey, { version: state?.latestVersion ?? "" })}
              {state?.reason ? ` - ${state.reason}` : ""}
            </span>
          ) : (
            <span className="text-[var(--text-muted)]">{t("admin.plugin.autoUpdate.neverChecked")}</span>
          )}
          {state?.checkedAt && (
            <span className="text-[var(--text-muted)]">
              ({t("admin.plugin.autoUpdate.checkedAt", { when: new Date(state.checkedAt).toLocaleString(i18n.language) })})
            </span>
          )}
          {state?.outcome === "held" && (
            <Button variant="secondary" onClick={() => onReview(source)} data-testid="plugin-auto-update-review" {...lock}>
              {t("admin.plugin.autoUpdate.review")}
            </Button>
          )}
          <Button
            variant="ghost"
            onClick={() => check.mutate({ pluginId: id, data: {} })}
            disabled={check.isPending}
            data-testid="plugin-auto-update-check"
            {...lock}
          >
            {check.isPending ? t("admin.plugin.autoUpdate.checking") : t("admin.plugin.autoUpdate.checkNow")}
          </Button>
        </div>
      )}
    </section>
  );
}
