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
