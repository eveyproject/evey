import crypto from "node:crypto";
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
interface Unsigned {
  bundleDir: string;
  runtimeVersion: string;
  commit: string;
  commitSeconds: number;
}
interface SigningLib {
  checkUnsignedBundle(input: Unsigned): string;
  deriveRelease(input: { runtimeVersion: string; launchHash: string; assetHashes: string[]; commitSeconds: number }): { id: string; createdAt: string };
  checkSigningCertificate(input: Check): string;
  writeSignedManifest(file: string, parts: { signature: string; certificate: string; manifest: string }): void;
  readSignedManifest(file: string): { signature: string; certificate: string; manifest: string; createdAt: string } | null;
}
const libPath = path.resolve(__dirname, "../../../../scripts/client-update-signing.mjs");
const lib = (await import(/* @vite-ignore */ libPath)) as SigningLib;

const T = fixture.times;
const NOW = Date.parse(T.now);
const valid: Check = {
  leafPem: fixture.leaf,
  rootPem: fixture.root,
  manifest: fixture.manifest,
  signature: fixture.signature,
  createdAt: T.released,
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
    expect(lib.checkSigningCertificate({ ...valid, now: Date.parse(T.beforeCertificate) })).toContain("not valid yet");
    expect(lib.checkSigningCertificate({ ...valid, now: Date.parse(T.lastWeeksOfCertificate) })).toContain("expires in less than 30 days");
  });

  it("refuses a release dated after the certificate expires", () => {
    expect(lib.checkSigningCertificate({ ...valid, createdAt: T.afterCertificate })).toContain("after the expiry");
  });

  it("refuses to publish over a newer release", () => {
    expect(lib.checkSigningCertificate({ ...valid, previousCreatedAt: T.dayAfterRelease })).toContain("older than the one already in the output");
    expect(lib.checkSigningCertificate({ ...valid, previousCreatedAt: T.released })).toBe("");
    expect(lib.checkSigningCertificate({ ...valid, previousCreatedAt: T.dayBeforeRelease })).toBe("");
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
        createdAt: T.released,
      });
      expect(fs.readdirSync(dir)).toEqual(["manifest.signed"]);
      expect(lib.readSignedManifest(path.join(dir, "absent"))).toBeNull();
    } finally {
      fs.rmSync(dir, { recursive: true, force: true });
    }
  });
});

// What the signing step demands of a bundle built by a job it does not trust.
describe("checkUnsignedBundle", () => {
  const RUNTIME = "tv-1";
  const COMMIT = "a".repeat(40);
  const SECONDS = Math.floor(Date.parse(T.released) / 1000);

  function describeFile(dir: string, content: string, ext: string, launch: boolean) {
    const data = Buffer.from(content);
    const hex = crypto.createHash("sha256").update(data).digest("hex");
    fs.writeFileSync(path.join(dir, "assets", `${hex}.${ext}`), data);
    return {
      hash: crypto.createHash("sha256").update(data).digest("base64url"),
      key: crypto.createHash("md5").update(data).digest("hex"),
      contentType: launch ? "application/javascript" : "image/png",
      ...(launch ? {} : { fileExtension: `.${ext}` }),
      url: `assets/android/${RUNTIME}/${hex}.${ext}`,
    };
  }

  // Builds a bundle the way the publication tool leaves it, then lets the
  // test bend it before it is checked.
  function check(bend: (manifest: Record<string, unknown>, dir: string) => void = () => {}, input: Partial<Unsigned> = {}): string {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "client-update-unsigned-"));
    try {
      fs.mkdirSync(path.join(dir, "assets"));
      const launchAsset = describeFile(dir, "bytecode", "hbc", true);
      const assets = [describeFile(dir, "picture", "png", false)];
      const release = lib.deriveRelease({ runtimeVersion: RUNTIME, launchHash: launchAsset.hash, assetHashes: assets.map((a) => a.hash), commitSeconds: SECONDS });
      const manifest: Record<string, unknown> = { ...release, runtimeVersion: RUNTIME, launchAsset, assets, metadata: {}, extra: {} };
      fs.writeFileSync(path.join(dir, "commit"), `${COMMIT}\n`);
      bend(manifest, dir);
      fs.writeFileSync(path.join(dir, "manifest.json"), JSON.stringify(manifest));
      return lib.checkUnsignedBundle({ bundleDir: dir, runtimeVersion: RUNTIME, commit: COMMIT, commitSeconds: SECONDS, ...input });
    } finally {
      fs.rmSync(dir, { recursive: true, force: true });
    }
  }

  it("accepts the bundle the publication tool leaves", () => {
    expect(check()).toBe("");
  });

  it("dates a release after the second of its commit, never on it", () => {
    const { createdAt } = lib.deriveRelease({ runtimeVersion: RUNTIME, launchHash: "a", assetHashes: [], commitSeconds: SECONDS });
    const ms = Date.parse(createdAt) - SECONDS * 1000;
    expect(ms).toBeGreaterThanOrEqual(1);
    expect(ms).toBeLessThanOrEqual(999);
  });

  it("refuses a bundle of another commit, or of a tree with uncommitted changes", () => {
    expect(check(() => {}, { commit: "b".repeat(40) })).toMatch(/was built from/);
    expect(check((_, dir) => fs.writeFileSync(path.join(dir, "commit"), `${COMMIT}-dirty\n`))).toMatch(/was built from/);
  });

  it("refuses a bundle for a runtime the app does not declare", () => {
    expect(check(() => {}, { runtimeVersion: "tv-2" })).toMatch(/runtime/);
  });

  it("refuses a date that is not the commit's: a later one would raise every floor", () => {
    expect(check((m) => (m.createdAt = new Date(Date.parse(m.createdAt as string) + 86_400_000).toISOString()))).toMatch(/is dated/);
    expect(check(() => {}, { commitSeconds: SECONDS - 60 })).toMatch(/is dated/);
  });

  it("refuses an id that is not the content's", () => {
    expect(check((m) => (m.id = "00000000-0000-4000-8000-000000000000"))).toMatch(/manifest id/);
  });

  it("refuses a file that is not what the manifest says", () => {
    expect(check((_, dir) => fs.writeFileSync(path.join(dir, "assets", fs.readdirSync(path.join(dir, "assets"))[0]), "other"))).toMatch(/is not the file/);
    expect(check((_, dir) => fs.rmSync(path.join(dir, "assets", fs.readdirSync(path.join(dir, "assets"))[0])))).toMatch(/missing from the bundle/);
  });

  it("refuses files the manifest does not name", () => {
    expect(check((_, dir) => fs.writeFileSync(path.join(dir, "assets", `${"0".repeat(64)}.png`), "extra"))).toMatch(/does not name/);
    expect(check((_, dir) => fs.writeFileSync(path.join(dir, "manifest.signed"), "x"))).toMatch(/something else/);
  });

  it("refuses an asset address that leaves the bundle", () => {
    expect(check((m) => ((m.launchAsset as { url: string }).url = "https://elsewhere.example/app.hbc"))).toMatch(/may name/);
    expect(check((m) => ((m.launchAsset as { url: string }).url = `assets/android/${RUNTIME}/../x.hbc`))).toMatch(/may name/);
  });

  it("refuses manifest fields that steer the apps", () => {
    expect(check((m) => (m.metadata = { channel: "beta" }))).toMatch(/metadata/);
    expect(check((m) => (m.extensions = {}))).toMatch(/expected fields/);
  });
});
