import { describe, expect, it, vi } from "vitest";
import { servingServer } from "./servedServer";

const evey = vi.fn(async () => ({ probe: "ok" }));

describe("the server a served page belongs to", () => {
  // Measured in prod on 2026-09-23: /tv opened from the server itself still
  // showed an empty "Enter the URL of your self-hosted Evey instance" field.
  it("is the origin when the origin answers as Evey", async () => {
    expect(await servingServer({ protocol: "https:", origin: "https://evey.example.com" }, evey)).toBe("https://evey.example.com");
    expect(evey).toHaveBeenCalledWith("https://evey.example.com", 3000);
  });

  it("is nothing when the origin is not Evey (a development host serving Vite)", async () => {
    const vite = async () => ({ probe: "not-evey" });
    expect(await servingServer({ protocol: "http:", origin: "http://localhost:5174" }, vite)).toBeNull();
  });

  it("is nothing when the origin does not answer", async () => {
    const down = async () => { throw new Error("offline"); };
    expect(await servingServer({ protocol: "https:", origin: "https://evey.example.com" }, down)).toBeNull();
  });

  it("is never probed from the packaged application", async () => {
    const probe = vi.fn(async () => ({ probe: "ok" }));
    expect(await servingServer({ protocol: "file:", origin: "null" }, probe)).toBeNull();
    expect(probe).not.toHaveBeenCalled();
  });
});
