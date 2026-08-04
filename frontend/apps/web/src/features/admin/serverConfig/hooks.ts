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

export function useSetConfigEntry() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      serverConfigServiceSetServerConfigEntry(key, { value }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      toast.success(t("admin.serverConfig.configUpdated"));
    },
    onError: () => toast.error(t("admin.serverConfig.failedUpdate")),
  });
}

export function useDeleteConfigEntry() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (key: string) => serverConfigServiceDeleteServerConfigEntry(key),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      toast.success(t("admin.serverConfig.configDeleted"));
    },
    onError: () => toast.error(t("admin.serverConfig.failedDeleteEntry")),
  });
}
