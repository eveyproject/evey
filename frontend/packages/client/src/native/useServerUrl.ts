import { useEffect, useState } from "react";
import { useServerListStore } from "@evey/store";
import { clientLogger } from "../log";
import { getServerUrl } from "./server-config";

const log = clientLogger("server-url");

// The active server comes from the store, synchronously: read from the
// keystore on every mount, the url was "" on a screen's first render and the
// posters landed as relative paths the image loader could not fetch. The
// keystore is only consulted for a legacy single-server install that never migrated.

function useLegacyServerUrl(enabled: boolean): string | null | undefined {
  const [url, setUrl] = useState<string | null | undefined>(undefined);
  useEffect(() => {
    if (!enabled) return;
    getServerUrl()
      .then(setUrl)
      // A failed keystore read settles to null ("no server"), or the caller
      // stays on its "still loading" placeholder for good.
      .catch((err) => {
        log.warn("server URL read failed", err);
        setUrl(null);
      });
  }, [enabled]);
  return url;
}

export function useServerUrl(): string {
  const active = useServerListStore((s) => s.getActiveServer()?.url ?? "");
  const legacy = useLegacyServerUrl(!active);
  return active || legacy || "";
}

export function useServerUrlNullable(): string | null | undefined {
  const active = useServerListStore((s) => s.getActiveServer()?.url ?? "");
  const legacy = useLegacyServerUrl(!active);
  return active || legacy;
}
