import { useEffect, useState } from "react";
import { clientLogger } from "../log";
import { getServerUrl } from "./server-config";

const log = clientLogger("server-url");

export function useServerUrl(): string {
  const [url, setUrl] = useState("");

  useEffect(() => {
    getServerUrl()
      .then((u) => setUrl(u ?? ""))
      .catch((err) => log.warn("server URL read failed", err));
  }, []);

  return url;
}

export function useServerUrlNullable(): string | null | undefined {
  const [url, setUrl] = useState<string | null | undefined>(undefined);

  useEffect(() => {
    // A failed keystore read settles to null ("no server"), or the caller
    // stays on its "still loading" placeholder for good.
    getServerUrl()
      .then(setUrl)
      .catch((err) => {
        log.warn("server URL read failed", err);
        setUrl(null);
      });
  }, []);

  return url;
}
