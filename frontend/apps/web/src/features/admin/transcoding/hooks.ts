import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

export const SETTINGS_KEY = ["/api/v1/admin/server-settings"];

export type SettingValue<T> = { value: T; default: T; source: string; requiresRestart?: boolean };
export interface ServerSettings {
  maxConcurrentTranscodes: SettingValue<number>;
  transcodeMaxSessions?: SettingValue<number>;
  transcodeSingleVariant?: SettingValue<boolean>;
  transcodeHWPerVariant?: SettingValue<boolean>;
  softwareMaxHeight?: SettingValue<number>;
  softwareHDRMaxHeight?: SettingValue<number>;
}

// useTranscodingServerSettings is shared by ConcurrencyCard and
// AdvancedTranscodingCard: both read the same /admin/server-settings
// snapshot, just different fields of it.
export function useTranscodingServerSettings() {
  return useQuery<ServerSettings>({
    queryKey: SETTINGS_KEY,
    queryFn: () => eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "GET" }),
  });
}
