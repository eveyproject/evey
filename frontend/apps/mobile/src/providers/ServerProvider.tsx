import React, { createContext, useContext, useState, useEffect } from "react";
import { getServerUrl } from "../lib/server-config";

const ServerContext = createContext<string>("");

export function useServer(): string {
  return useContext(ServerContext);
}

export function ServerProvider({ children }: { children: React.ReactNode }) {
  const [url, setUrl] = useState("");

  useEffect(() => {
    getServerUrl().then((u) => setUrl(u ?? ""));
  }, []);

  return (
    <ServerContext.Provider value={url}>{children}</ServerContext.Provider>
  );
}
