import { describe, it, expect, beforeEach } from "vitest";
import { migrateFromSingleServer } from "./server-migration";
import { useServerListStore } from "./servers";
import { useAuthStore } from "./auth";

function resetStores() {
  useServerListStore.setState({
    servers: {},
    activeServerId: null,
    _syncActiveToAuth: null,
  });
  useAuthStore.setState({
    tokensByServer: {},
    accessToken: null,
    profileId: null,
    isAdmin: false,
  });
}

beforeEach(resetStores);

describe("migrateFromSingleServer", () => {
  it("no-ops when there is no legacy server URL", async () => {
    const result = await migrateFromSingleServer({
      readLegacyServerUrl: async () => null,
    });
    expect(result.migrated).toBe(false);
    expect(result.serverId).toBeNull();
    expect(Object.keys(useServerListStore.getState().servers)).toHaveLength(0);
  });

  it("migrates when a legacy URL is found", async () => {
    const result = await migrateFromSingleServer({
      readLegacyServerUrl: async () => "https://evey.home.net/",
    });
    expect(result.migrated).toBe(true);
    expect(typeof result.serverId).toBe("string");

    const servers = useServerListStore.getState().servers;
    expect(Object.keys(servers)).toHaveLength(1);
    const server = Object.values(servers)[0];
    expect(server.url).toBe("https://evey.home.net");
    expect(useServerListStore.getState().activeServerId).toBe(server.id);
  });

  it("copies existing auth tokens to the new server slot", async () => {
    // Pre-populate auth store with flat tokens (simulates existing single-server user)
    useAuthStore.setState({
      accessToken: "tok-access",
      profileId: "p-1",
      isAdmin: false,
    });

    const result = await migrateFromSingleServer({
      readLegacyServerUrl: async () => "https://evey.home.net",
    });

    expect(result.migrated).toBe(true);
    const serverId = result.serverId!;
    const byServer = useAuthStore.getState().tokensByServer[serverId];
    expect(byServer.accessToken).toBe("tok-access");
    expect(byServer.profileId).toBe("p-1");
  });

  it("is idempotent - second call is a no-op", async () => {
    await migrateFromSingleServer({
      readLegacyServerUrl: async () => "https://evey.home.net",
    });
    const serversBefore = { ...useServerListStore.getState().servers };

    const result2 = await migrateFromSingleServer({
      readLegacyServerUrl: async () => "https://evey.home.net",
    });
    expect(result2.migrated).toBe(false);
    // No new servers added
    expect(Object.keys(useServerListStore.getState().servers)).toHaveLength(
      Object.keys(serversBefore).length
    );
  });

  it("no-ops when servers store already has entries", async () => {
    // Pre-populate servers store
    useServerListStore.getState().addServer("https://existing.server.com");

    const result = await migrateFromSingleServer({
      readLegacyServerUrl: async () => "https://new.server.com",
    });
    expect(result.migrated).toBe(false);
    // Only the pre-existing server should be there
    expect(Object.keys(useServerListStore.getState().servers)).toHaveLength(1);
  });

  it("survives a storage read failure without throwing", async () => {
    const result = await migrateFromSingleServer({
      readLegacyServerUrl: async () => {
        throw new Error("secure store unavailable");
      },
    });
    expect(result.migrated).toBe(false);
    expect(result.serverId).toBeNull();
  });
});
