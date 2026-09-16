import { describe, it, expect } from "vitest";
import { uploadRefusalMessage } from "./uploadRefusal";

function res(ok: boolean, body: unknown, broken = false) {
  return { ok, json: () => (broken ? Promise.reject(new Error("not json")) : Promise.resolve(body)) };
}

describe("uploadRefusalMessage", () => {
  it("surfaces the server's reason and falls back otherwise", async () => {
    expect(await uploadRefusalMessage(res(false, { message: "invalid manifest: name is required" }), "Install failed")).toBe("invalid manifest: name is required");
    expect(await uploadRefusalMessage(res(false, { message: "   " }), "Install failed")).toBe("Install failed");
    expect(await uploadRefusalMessage(res(false, { message: 42 }), "Install failed")).toBe("Install failed");
    expect(await uploadRefusalMessage(res(false, null, true), "Install failed")).toBe("Install failed");
    expect(await uploadRefusalMessage(res(true, { message: "ignored" }), "Install failed")).toBe("Install failed");
  });
});
