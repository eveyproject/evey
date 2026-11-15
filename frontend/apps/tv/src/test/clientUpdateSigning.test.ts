import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, it, expect } from "vitest";
import fixture from "./fixtures/clientUpdateSigning.json";

// The publication tool refuses to write a bundle the apps would refuse, or
// one that would silently never be taken. The certificates are throwaway
// ones made for this test; their keys were discarded, the signature below is
// the only thing they ever signed.
interface Check {
  leafPem: string;
  rootPem: string;
  manifest: string;
  signature: string;
  createdAt: string;
  previousCreatedAt?: string;
  now?: number;
}
interface SigningLib {
  checkSigningCertificate(input: Check): string;
  writeSignedManifest(file: string, parts: { signature: string; certificate: string; manifest: string }): void;
  readSignedManifest(file: string): { signature: string; certificate: string; manifest: string; createdAt: string } | null;
}
const libPath = path.resolve(__dirname, "../../../../scripts/client-update-signing.mjs");
const lib = (await import(/* @vite-ignore */ libPath)) as SigningLib;

const NOW = Date.parse("2027-01-01T00:00:00.000Z");
const valid: Check = {
  leafPem: fixture.leaf,
  rootPem: fixture.root,
  manifest: fixture.manifest,
  signature: fixture.signature,
  createdAt: "2026-12-01T00:00:00.000Z",
  now: NOW,
};

describe("checkSigningCertificate", () => {
  it("accepts a release signed by a certificate the root issued", () => {
    expect(lib.checkSigningCertificate(valid)).toBe("");
  });

  it("refuses a manifest other than the signed one", () => {
    expect(lib.checkSigningCertificate({ ...valid, manifest: `${fixture.manifest} ` })).toContain("does not match");
  });

  it("refuses a certificate the root did not issue", () => {
    expect(lib.checkSigningCertificate({ ...valid, leafPem: fixture.root })).toContain("must not be a certificate authority");
    expect(lib.checkSigningCertificate({ ...valid, rootPem: fixture.leaf })).toContain("not a self-signed certificate authority");
  });

  it("refuses a certificate without the code signing usage", () => {
    expect(lib.checkSigningCertificate({ ...valid, leafPem: fixture.leafWithoutCodeSigning })).toContain("code signing");
  });

  it("refuses a certificate that is not valid yet or about to expire", () => {
    expect(lib.checkSigningCertificate({ ...valid, now: Date.parse("2020-01-01T00:00:00.000Z") })).toContain("not valid yet");
    expect(lib.checkSigningCertificate({ ...valid, now: Date.parse("2126-10-01T00:00:00.000Z") })).toContain("expires in less than 30 days");
  });

  it("refuses a release dated after the certificate expires", () => {
    expect(lib.checkSigningCertificate({ ...valid, createdAt: "2127-01-01T00:00:00.000Z" })).toContain("after the expiry");
  });

  it("refuses to publish over a newer release", () => {
    expect(lib.checkSigningCertificate({ ...valid, previousCreatedAt: "2026-12-02T00:00:00.000Z" })).toContain("older than the one already in the output");
    expect(lib.checkSigningCertificate({ ...valid, previousCreatedAt: "2026-12-01T00:00:00.000Z" })).toBe("");
    expect(lib.checkSigningCertificate({ ...valid, previousCreatedAt: "2026-11-30T00:00:00.000Z" })).toBe("");
  });
});

describe("the signed manifest file", () => {
  it("reads back what was written, byte for byte", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "client-update-signing-"));
    try {
      const file = path.join(dir, "manifest.signed");
      lib.writeSignedManifest(file, { signature: 'sig="abc", keyid="main"', certificate: fixture.leaf, manifest: fixture.manifest });

      expect(lib.readSignedManifest(file)).toEqual({
        signature: 'sig="abc", keyid="main"',
        certificate: fixture.leaf,
        manifest: fixture.manifest,
        createdAt: "2026-12-01T00:00:00.000Z",
      });
      expect(fs.readdirSync(dir)).toEqual(["manifest.signed"]);
      expect(lib.readSignedManifest(path.join(dir, "absent"))).toBeNull();
    } finally {
      fs.rmSync(dir, { recursive: true, force: true });
    }
  });
});
