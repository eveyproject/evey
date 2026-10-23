import { describe, it, expect } from "vitest";
import { signatureKind, shortFingerprint } from "./pluginSignature";

describe("signatureKind", () => {
  it("maps the four card states", () => {
    expect(signatureKind({ verified: true, trusted: true, publisher: "Acme" })).toBe("trusted");
    expect(signatureKind({ verified: true, trusted: false })).toBe("untrusted");
    expect(signatureKind(undefined)).toBe("unsigned");
    expect(signatureKind({ verified: false, error: "no signature" })).toBe("unsigned");
    expect(signatureKind({ verified: false, keyFingerprint: "sha256:aa", error: "signature mismatch" })).toBe("invalid");
  });

  it("shortens a fingerprint to 8 hex digits", () => {
    expect(shortFingerprint("sha256:abcd1234ef567890")).toBe("sha256:abcd1234...");
    expect(shortFingerprint("")).toBe("");
  });
});
