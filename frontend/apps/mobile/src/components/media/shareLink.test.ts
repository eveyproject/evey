import { describe, it, expect } from "vitest";
import { buildPublicShareUrl } from "./shareLink";

describe("buildPublicShareUrl", () => {
  it("joins the server origin with the server-relative path", () => {
    expect(buildPublicShareUrl("https://evey.example.com", "/dl/abc123")).toBe(
      "https://evey.example.com/dl/abc123",
    );
  });

  it("returns an empty string when the path is missing", () => {
    expect(buildPublicShareUrl("https://evey.example.com", undefined)).toBe("");
  });

  it("returns an empty string when the path is an empty string", () => {
    expect(buildPublicShareUrl("https://evey.example.com", "")).toBe("");
  });
});
