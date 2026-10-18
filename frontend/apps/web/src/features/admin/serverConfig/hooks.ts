import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useServerConfigServiceListServerConfigEntries,
  serverConfigServiceSetServerConfigEntry,
  serverConfigServiceDeleteServerConfigEntry,
  getServerConfigServiceListServerConfigEntriesQueryKey,
} from "@evey/api";
import type { V1ServerConfigEntry } from "@evey/api";
import { toast } from "@evey/store";

export type ConfigEntry = V1ServerConfigEntry;

export const QUERY_KEY = getServerConfigServiceListServerConfigEntriesQueryKey();

export function useServerConfig() {
  return useServerConfigServiceListServerConfigEntries({
    query: { select: (d) => d.entries ?? [] },
  });
}

// ownToast: the caller writes several keys in one Save and toasts the outcome
// once, so each write stays quiet, the global mutation error net included.
export function useSetConfigEntry(opts: { ownToast?: boolean } = {}) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  return useMutation({
    meta: opts.ownToast ? { silentError: true } : undefined,
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      serverConfigServiceSetServerConfigEntry(key, { value }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      if (!opts.ownToast) toast.success(t("admin.serverConfig.configUpdated"));
    },
    onError: opts.ownToast ? undefined : () => toast.error(t("admin.serverConfig.failedUpdate")),
  });
}

export function useDeleteConfigEntry(opts: { ownToast?: boolean } = {}) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  return useMutation({
    meta: opts.ownToast ? { silentError: true } : undefined,
    mutationFn: (key: string) => serverConfigServiceDeleteServerConfigEntry(key),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      if (!opts.ownToast) toast.success(t("admin.serverConfig.configDeleted"));
    },
    onError: opts.ownToast ? undefined : () => toast.error(t("admin.serverConfig.failedDeleteEntry")),
  });
}
