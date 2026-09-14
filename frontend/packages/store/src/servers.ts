import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { lazyZustandStorage, registerPersistedStore } from "./storage";

export interface SavedServer {
  id: string;
  url: string;        // normalized: trailing slash stripped
  label?: string;     // the user's own rename, always wins
  // The name the server advertises on /healthz (admin "Server name"),
  // refreshed on every successful probe. Kept apart from label so a server
  // rename shows up without erasing what the user typed.
  serverName?: string;
  lastUsedAt: number; // epoch ms
}

// DEFAULT_SERVER_NAME is what an unnamed instance advertises (server side
// config.DefaultServerName). It carries no information, so the lists fall
// back to the URL rather than show "Evey" under an Evey logo.
export const DEFAULT_SERVER_NAME = "Evey";

export function isDefaultServerName(name: string | undefined | null): boolean {
  const n = name?.trim() ?? "";
  return n === "" || n.toLowerCase() === DEFAULT_SERVER_NAME.toLowerCase();
}

// serverDisplayName is the one line a server list shows for a saved server:
// the user's label, else the advertised name, else the URL.
export function serverDisplayName(server: Pick<SavedServer, "url" | "label" | "serverName">): string {
  if (server.label) return server.label;
  if (!isDefaultServerName(server.serverName)) return server.serverName!.trim();
  return server.url;
}

interface ServerListState {
  servers: Record<string, SavedServer>;
  activeServerId: string | null;
  addServer: (url: string, label?: string) => SavedServer;
  removeServer: (id: string) => void;
  setActiveServer: (id: string | null) => void;
  renameServer: (id: string, label: string) => void;
  // setServerName records the name a probe just read; unknown id = no-op.
  setServerName: (id: string, serverName: string | undefined) => void;
  getActiveServer: () => SavedServer | null;
  /** Internal: assign tokens for a server from the auth store migration path */
  _syncActiveToAuth: (() => void) | null;
}

function generateId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return Math.random().toString(36).slice(2) + Date.now().toString(36);
}

/** Strip trailing slash and lowercase scheme+host, same as server-connect.tsx does */
export function normalizeServerUrl(raw: string): string {
  const trimmed = raw.trim().replace(/\/+$/, "");
  try {
    const u = new URL(trimmed);
    // lowercase scheme and host only; preserve path casing
    return `${u.protocol.toLowerCase()}//${u.hostname.toLowerCase()}${u.port ? `:${u.port}` : ""}${u.pathname === "/" ? "" : u.pathname}`;
  } catch {
    // not a valid URL - return as-is after trailing-slash strip
    return trimmed;
  }
}

export const useServerListStore = create<ServerListState>()(
  persist(
    (set, get) => ({
      servers: {},
      activeServerId: null,
      _syncActiveToAuth: null,

      addServer: (url: string, label?: string): SavedServer => {
        const normalized = normalizeServerUrl(url);
        const existing = Object.values(get().servers).find((s) => s.url === normalized);
        if (existing) return existing;

        const id = generateId();
        const server: SavedServer = {
          id,
          url: normalized,
          label,
          lastUsedAt: Date.now(),
        };
        set((state) => ({ servers: { ...state.servers, [id]: server } }));
        return server;
      },

      removeServer: (id: string) => {
        const { servers, activeServerId } = get();
        const remaining = Object.keys(servers).filter((k) => k !== id);
        // Refuse to remove the only server unless there is no active server
        if (remaining.length === 0 && activeServerId !== null) return;
        const next = { ...servers };
        delete next[id];
        set({ servers: next, activeServerId: activeServerId === id ? null : activeServerId });
      },

      setActiveServer: (id: string | null) => {
        set((state) => {
          if (id === null) return { activeServerId: null };
          const server = state.servers[id];
          if (!server) return {};
          const updated: SavedServer = { ...server, lastUsedAt: Date.now() };
          return {
            activeServerId: id,
            servers: { ...state.servers, [id]: updated },
          };
        });
        // Notify auth store to sync shadow fields
        get()._syncActiveToAuth?.();
      },

      renameServer: (id: string, label: string) => {
        set((state) => {
          const server = state.servers[id];
          if (!server) return {};
          return { servers: { ...state.servers, [id]: { ...server, label } } };
        });
      },

      setServerName: (id: string, serverName: string | undefined) => {
        const trimmed = serverName?.trim();
        set((state) => {
          const server = state.servers[id];
          if (!server || server.serverName === trimmed) return {};
          return { servers: { ...state.servers, [id]: { ...server, serverName: trimmed } } };
        });
      },

      getActiveServer: (): SavedServer | null => {
        const { servers, activeServerId } = get();
        if (!activeServerId) return null;
        return servers[activeServerId] ?? null;
      },
    }),
    {
      name: "evey-servers",
      storage: createJSONStorage(() => lazyZustandStorage),
      partialize: (state) => ({
        servers: state.servers,
        activeServerId: state.activeServerId,
      }),
    }
  )
);

// Late-storage rehydration (React Native): see storage.ts.
registerPersistedStore(useServerListStore);
