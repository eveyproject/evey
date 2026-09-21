import { useEffect, useState } from "react";
import { getServerUrl } from "./server-config";

export function useServerUrl(): string {
  const [url, setUrl] = useState("");

  useEffect(() => {
    getServerUrl().then((u) => setUrl(u ?? ""));
  }, []);

  return url;
}

export function useServerUrlNullable(): string | null | undefined {
  const [url, setUrl] = useState<string | null | undefined>(undefined);

  useEffect(() => {
    getServerUrl().then(setUrl);
  }, []);

  return url;
}
