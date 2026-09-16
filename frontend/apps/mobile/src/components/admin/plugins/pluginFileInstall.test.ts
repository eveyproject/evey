import { describe, it, expect, vi, beforeEach } from "vitest";

const uploadAsync = vi.fn();
vi.mock("expo-file-system/legacy", () => ({
  uploadAsync: (...args: unknown[]) => uploadAsync(...args),
  FileSystemUploadType: { MULTIPART: "multipart" },
}));
vi.mock("@evey/api", () => ({ getBaseURL: () => "http://evey.local" }));

import { installPluginFile, previewPluginFile } from "./pluginFileInstall";

beforeEach(() => {
  uploadAsync.mockReset();
});

// The upload route answers a refused archive with a JSON message: the
// operator reads the reason, never the raw JSON. 428 and 403 stay structured
// results the screen branches on; the token rides as a bearer header and the
// acknowledgment as a form field.
describe("installPluginFile", () => {
  it("passes the bearer and the acknowledgment, reports success", async () => {
    uploadAsync.mockResolvedValue({ status: 201, body: `{"id":"p1"}` });
    const res = await installPluginFile("file:///tmp/p.zip", { accept: true }, "tok");
    expect(res).toEqual({ ok: true });
    expect(uploadAsync).toHaveBeenCalledWith(
      "http://evey.local/api/v1/admin/plugins/upload",
      "file:///tmp/p.zip",
      expect.objectContaining({ fieldName: "plugin", parameters: { accept_permission_changes: "true" }, headers: { Authorization: "Bearer tok" } }),
    );
  });

  it("returns escalation entries on 428 and the findings on 403", async () => {
    uploadAsync.mockResolvedValueOnce({ status: 428, body: `{"permission_changes":true,"new_entries":["network"]}` });
    expect(await installPluginFile("f", {}, null)).toEqual({ ok: false, escalation: ["network"] });
    uploadAsync.mockResolvedValueOnce({ status: 403, body: `{"blocked":true,"issues":["[CRITICAL] eval"]}` });
    expect(await installPluginFile("f", {}, null)).toEqual({ ok: false, blocked: ["[CRITICAL] eval"] });
    uploadAsync.mockResolvedValueOnce({ status: 403, body: `{"message":"forbidden"}` });
    expect(await installPluginFile("f", {}, null)).toEqual({ ok: false, blocked: ["forbidden"] });
  });

  it("reads the reason out of a JSON refusal and keeps a plain body as is", async () => {
    uploadAsync.mockResolvedValueOnce({ status: 400, body: `{"message":"invalid manifest: name is required"}` });
    expect(await installPluginFile("f", {}, null)).toEqual({ ok: false, error: "invalid manifest: name is required" });
    uploadAsync.mockResolvedValueOnce({ status: 500, body: "" });
    expect(await installPluginFile("f", {}, null)).toEqual({ ok: false, error: "install failed (500)" });
    uploadAsync.mockResolvedValueOnce({ status: 502, body: "bad gateway" });
    expect(await installPluginFile("f", {}, null)).toEqual({ ok: false, error: "bad gateway" });
  });
});

describe("previewPluginFile", () => {
  it("parses the preview and throws the reason on a refusal", async () => {
    uploadAsync.mockResolvedValueOnce({ status: 200, body: `{"name":"p","permissions":["network"]}` });
    await expect(previewPluginFile("f", "tok")).resolves.toEqual({ name: "p", permissions: ["network"] });
    uploadAsync.mockResolvedValueOnce({ status: 400, body: `{"message":"invalid manifest: name is required"}` });
    await expect(previewPluginFile("f", "tok")).rejects.toThrow("invalid manifest: name is required");
  });
});
