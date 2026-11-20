import { describe, expect, it, vi } from "vitest";
import { createPluginRpc } from "./pluginRpc";

function okResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
}

describe("createPluginRpc token issue", () => {
  it("issues one token for concurrent calls, and reuses it afterwards", async () => {
    let resolveIssue: (t: string) => void = () => {};
    const issueToken = vi.fn(() => new Promise<string>((r) => { resolveIssue = r; }));
    const request = vi.fn(async () => okResponse({ ok: true }));
    const rpc = createPluginRpc("plg-1", { issueToken, request });
    const calls = Promise.all([
      rpc.call("prefs.get", { key: "a" }),
      rpc.call("prefs.get", { key: "b" }),
      rpc.call("prefs.get", { key: "c" }),
    ]);
    expect(issueToken).toHaveBeenCalledTimes(1);
    resolveIssue("tok-1");
    await calls;
    await rpc.call("prefs.get", { key: "d" });
    expect(issueToken).toHaveBeenCalledTimes(1);
    expect(request).toHaveBeenCalledTimes(4);
  });

  it("a failed issue does not stick: the next call tries again", async () => {
    const issueToken = vi.fn()
      .mockRejectedValueOnce(new Error("down"))
      .mockResolvedValueOnce("tok-2");
    const request = vi.fn(async () => okResponse({ ok: true }));
    const rpc = createPluginRpc("plg-1", { issueToken, request });
    await expect(rpc.call("prefs.get", { key: "a" })).rejects.toThrow("plugin token unavailable");
    await rpc.call("prefs.get", { key: "a" });
    expect(issueToken).toHaveBeenCalledTimes(2);
  });
});

describe("createPluginRpc config.save", () => {
  const configPath = "/api/v1/admin/plugins/plg-1/config";

  it("is refused outside the admin config slot, without touching the server", async () => {
    const request = vi.fn(async () => okResponse({}));
    const rpc = createPluginRpc("plg-1", { issueToken: async () => "tok", request });
    await expect(rpc.call("config.save", { values: { city: "Brest" } })).rejects.toThrow("method not permitted: config.save");
    expect(request).not.toHaveBeenCalled();
  });

  it("merges the given keys into the stored config, on the viewer's session", async () => {
    const issueToken = vi.fn(async () => "tok");
    const request = vi.fn(async (req: { method: string }) =>
      req.method === "GET"
        ? okResponse({ values: { city: "Paris", units: "celsius", apiKey: "********" } })
        : okResponse({ values: { city: "Brest", units: "celsius", lat: "48.39", apiKey: "********" } }),
    );
    const onConfigSaved = vi.fn();
    const rpc = createPluginRpc("plg-1", { issueToken, request }, { configWrite: true, onConfigSaved });
    const out = await rpc.call("config.save", { values: { city: "Brest", lat: 48.39 } });

    expect(request).toHaveBeenCalledTimes(2);
    expect(request.mock.calls[0][0]).toEqual({ path: configPath, method: "GET" });
    const put = request.mock.calls[1][0] as { path: string; method: string; body: string; authToken?: string };
    expect(put.path).toBe(configPath);
    expect(put.method).toBe("PUT");
    expect(put.authToken).toBeUndefined();
    expect(JSON.parse(put.body)).toEqual({ values: { city: "Brest", units: "celsius", lat: "48.39", apiKey: "********" } });
    expect(out).toEqual({ values: { city: "Brest", units: "celsius", lat: "48.39", apiKey: "********" } });
    expect(onConfigSaved).toHaveBeenCalledTimes(1);
    expect(issueToken).not.toHaveBeenCalled();
  });

  it("rejects with the server's message when the schema refuses the values", async () => {
    const request = vi.fn(async (req: { method: string }) =>
      req.method === "GET"
        ? okResponse({ values: { city: "Paris" } })
        : new Response(JSON.stringify({ message: "city is required" }), { status: 400 }),
    );
    const onConfigSaved = vi.fn();
    const rpc = createPluginRpc("plg-1", { issueToken: async () => "tok", request }, { configWrite: true, onConfigSaved });
    await expect(rpc.call("config.save", { values: { city: "" } })).rejects.toThrow("city is required");
    expect(onConfigSaved).not.toHaveBeenCalled();
  });

  it("refuses values that are not an object", async () => {
    const request = vi.fn(async () => okResponse({}));
    const rpc = createPluginRpc("plg-1", { issueToken: async () => "tok", request }, { configWrite: true });
    await expect(rpc.call("config.save", { values: ["city"] })).rejects.toThrow("config.save needs an object of values");
    expect(request).not.toHaveBeenCalled();
  });
});
