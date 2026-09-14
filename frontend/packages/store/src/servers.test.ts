import { describe, it, expect, beforeEach } from "vitest";
import { useServerListStore, normalizeServerUrl, serverDisplayName, isDefaultServerName } from "./servers";

function resetStore() {
  useServerListStore.setState({
    servers: {},
    activeServerId: null,
    _syncActiveToAuth: null,
  });
}

beforeEach(resetStore);

describe("normalizeServerUrl", () => {
  it("strips trailing slashes", () => {
    expect(normalizeServerUrl("https://evey.example.com/")).toBe("https://evey.example.com");
    expect(normalizeServerUrl("https://evey.example.com///")).toBe("https://evey.example.com");
  });

  it("lowercases scheme and host", () => {
    expect(normalizeServerUrl("HTTPS://EVEY.EXAMPLE.COM")).toBe("https://evey.example.com");
  });

  it("preserves port", () => {
    expect(normalizeServerUrl("http://192.168.1.1:8096")).toBe("http://192.168.1.1:8096");
  });

  it("preserves path beyond root", () => {
    expect(normalizeServerUrl("https://media.home.net/evey/")).toBe("https://media.home.net/evey");
  });

  it("returns trimmed string for invalid URLs", () => {
    expect(normalizeServerUrl("  not-a-url  ")).toBe("not-a-url");
  });
});

describe("useServerListStore.addServer", () => {
  it("adds a new server and returns it", () => {
    const s = useServerListStore.getState().addServer("https://evey.example.com/");
    expect(s.url).toBe("https://evey.example.com");
    expect(typeof s.id).toBe("string");
    expect(s.id.length).toBeGreaterThan(0);
    expect(typeof s.lastUsedAt).toBe("number");
  });

  it("de-duplicates by normalized URL - returns existing", () => {
    const s1 = useServerListStore.getState().addServer("https://evey.example.com/");
    const s2 = useServerListStore.getState().addServer("HTTPS://evey.EXAMPLE.COM");
    expect(s1.id).toBe(s2.id);
    expect(Object.keys(useServerListStore.getState().servers)).toHaveLength(1);
  });

  it("adds a label when provided", () => {
    const s = useServerListStore.getState().addServer("https://evey.example.com", "Home");
    expect(s.label).toBe("Home");
  });

  it("can add multiple distinct servers", () => {
    useServerListStore.getState().addServer("https://server-a.com");
    useServerListStore.getState().addServer("https://server-b.com");
    expect(Object.keys(useServerListStore.getState().servers)).toHaveLength(2);
  });
});

describe("useServerListStore.setActiveServer", () => {
  it("sets the active server id", () => {
    const s = useServerListStore.getState().addServer("https://evey.example.com");
    useServerListStore.getState().setActiveServer(s.id);
    expect(useServerListStore.getState().activeServerId).toBe(s.id);
  });

  it("bumps lastUsedAt when called", async () => {
    const before = Date.now();
    const s = useServerListStore.getState().addServer("https://evey.example.com");
    // Overwrite lastUsedAt with a known old value
    useServerListStore.setState((state) => ({
      servers: { ...state.servers, [s.id]: { ...s, lastUsedAt: before - 10000 } },
    }));
    useServerListStore.getState().setActiveServer(s.id);
    const updated = useServerListStore.getState().servers[s.id];
    expect(updated.lastUsedAt).toBeGreaterThanOrEqual(before);
  });

  it("accepts null to clear active server", () => {
    const s = useServerListStore.getState().addServer("https://evey.example.com");
    useServerListStore.getState().setActiveServer(s.id);
    useServerListStore.getState().setActiveServer(null);
    expect(useServerListStore.getState().activeServerId).toBeNull();
  });

  it("ignores unknown ids", () => {
    const s = useServerListStore.getState().addServer("https://evey.example.com");
    useServerListStore.getState().setActiveServer(s.id);
    useServerListStore.getState().setActiveServer("nonexistent-id");
    // State unchanged
    expect(useServerListStore.getState().activeServerId).toBe(s.id);
  });
});

describe("useServerListStore.getActiveServer", () => {
  it("returns null when no active server", () => {
    expect(useServerListStore.getState().getActiveServer()).toBeNull();
  });

  it("returns the active server", () => {
    const s = useServerListStore.getState().addServer("https://evey.example.com");
    useServerListStore.getState().setActiveServer(s.id);
    const active = useServerListStore.getState().getActiveServer();
    expect(active?.id).toBe(s.id);
    expect(active?.url).toBe("https://evey.example.com");
  });
});

describe("useServerListStore.removeServer", () => {
  it("removes a server by id", () => {
    const s1 = useServerListStore.getState().addServer("https://server-a.com");
    useServerListStore.getState().addServer("https://server-b.com");
    useServerListStore.getState().removeServer(s1.id);
    expect(useServerListStore.getState().servers[s1.id]).toBeUndefined();
    expect(Object.keys(useServerListStore.getState().servers)).toHaveLength(1);
  });

  it("refuses to remove the only server when activeServerId is set", () => {
    const s = useServerListStore.getState().addServer("https://evey.example.com");
    useServerListStore.getState().setActiveServer(s.id);
    useServerListStore.getState().removeServer(s.id);
    // Should still be there
    expect(useServerListStore.getState().servers[s.id]).toBeDefined();
  });

  it("allows removing the only server when activeServerId is null", () => {
    const s = useServerListStore.getState().addServer("https://evey.example.com");
    // Do NOT set active server
    useServerListStore.getState().removeServer(s.id);
    expect(Object.keys(useServerListStore.getState().servers)).toHaveLength(0);
  });

  it("clears activeServerId if the active server is removed", () => {
    const s1 = useServerListStore.getState().addServer("https://server-a.com");
    useServerListStore.getState().addServer("https://server-b.com");
    useServerListStore.getState().setActiveServer(s1.id);
    useServerListStore.getState().removeServer(s1.id);
    expect(useServerListStore.getState().activeServerId).toBeNull();
  });
});

describe("useServerListStore.renameServer", () => {
  it("updates the label", () => {
    const s = useServerListStore.getState().addServer("https://evey.example.com");
    useServerListStore.getState().renameServer(s.id, "My Server");
    expect(useServerListStore.getState().servers[s.id].label).toBe("My Server");
  });

  it("no-ops for unknown id", () => {
    useServerListStore.getState().addServer("https://evey.example.com");
    const before = { ...useServerListStore.getState().servers };
    useServerListStore.getState().renameServer("ghost-id", "Ghost");
    expect(useServerListStore.getState().servers).toEqual(before);
  });
});

describe("server name from /healthz", () => {
  it("records the advertised name and shows it when the user has no label", () => {
    const s = useServerListStore.getState().addServer("https://evey.example.com");
    useServerListStore.getState().setServerName(s.id, "  Salon ");
    const saved = useServerListStore.getState().servers[s.id];
    expect(saved.serverName).toBe("Salon");
    expect(serverDisplayName(saved)).toBe("Salon");
  });

  it("the user's label always wins over the advertised name", () => {
    const s = useServerListStore.getState().addServer("https://evey.example.com", "Home");
    useServerListStore.getState().setServerName(s.id, "Salon");
    expect(serverDisplayName(useServerListStore.getState().servers[s.id])).toBe("Home");
  });

  it("the default name carries no information: the URL is shown instead", () => {
    const s = useServerListStore.getState().addServer("https://evey.example.com");
    useServerListStore.getState().setServerName(s.id, "Evey");
    expect(serverDisplayName(useServerListStore.getState().servers[s.id])).toBe("https://evey.example.com");
    expect(isDefaultServerName("evey")).toBe(true);
    expect(isDefaultServerName("")).toBe(true);
    expect(isDefaultServerName(undefined)).toBe(true);
    expect(isDefaultServerName("Salon")).toBe(false);
  });

  it("ignores unknown ids and does not rewrite an unchanged name", () => {
    const s = useServerListStore.getState().addServer("https://evey.example.com");
    useServerListStore.getState().setServerName("nope", "Salon");
    useServerListStore.getState().setServerName(s.id, "Salon");
    const before = useServerListStore.getState().servers;
    useServerListStore.getState().setServerName(s.id, "Salon");
    expect(useServerListStore.getState().servers).toBe(before);
    expect(Object.keys(before)).toHaveLength(1);
  });
});
