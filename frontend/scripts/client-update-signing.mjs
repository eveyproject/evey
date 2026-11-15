// What makes a signed bundle one the apps accept, shared by the publication
// tool, the signing step and their tests (docs/client-updates.md).
//
// Node built-ins only: the signing step runs this file with the key at hand
// and must not load a single dependency.

import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";

export const PLATFORM = "android";
const KEY_ID = "main";

const CODE_SIGNING = "1.3.6.1.5.5.7.3.3";
const DAY_MS = 86_400_000;
// A release signed with a certificate about to expire stops being accepted
// while its image is still in use.
export const MIN_REMAINING_DAYS = 30;

// The file the server reads: signature header, certificate in base64, then
// the exact signed bytes.
export function writeSignedManifest(file, { signature, certificate, manifest }) {
  // One file, renamed into place: the server never sees a manifest next to
  // the signature of another one.
  fs.writeFileSync(`${file}.tmp`, `${signature}\n${Buffer.from(certificate, "utf8").toString("base64")}\n${manifest}`);
  fs.renameSync(`${file}.tmp`, file);
}

export function readSignedManifest(file) {
  let raw;
  try {
    raw = fs.readFileSync(file, "utf8");
  } catch {
    return null;
  }
  const [signature, certificate, ...rest] = raw.split("\n");
  try {
    const manifest = rest.join("\n");
    return { signature, certificate: Buffer.from(certificate ?? "", "base64").toString("utf8"), manifest, createdAt: JSON.parse(manifest).createdAt };
  } catch {
    return null;
  }
}

// Returns why the apps would refuse this bundle, or "" when they would not.
export function checkSigningCertificate({ leafPem, rootPem, manifest, signature, createdAt, previousCreatedAt, now = Date.now() }) {
  let leaf;
  let root;
  try {
    leaf = new crypto.X509Certificate(leafPem);
    root = new crypto.X509Certificate(rootPem);
  } catch {
    return "the signing certificate or client-updates/certificate.pem is not a certificate";
  }
  if (!root.ca || !root.checkIssued(root) || !root.verify(root.publicKey)) return "client-updates/certificate.pem is not a self-signed certificate authority";
  if (!leaf.checkIssued(root) || !leaf.verify(root.publicKey)) return "the signing certificate was not issued by client-updates/certificate.pem";
  if (leaf.ca) return "the signing certificate must not be a certificate authority";
  if (!(leaf.keyUsage ?? []).includes(CODE_SIGNING)) return "the signing certificate lacks the code signing usage";
  const from = Date.parse(leaf.validFrom);
  const to = Date.parse(leaf.validTo);
  if (now < from) return "the signing certificate is not valid yet";
  if (to - now < MIN_REMAINING_DAYS * DAY_MS) return `the signing certificate expires in less than ${MIN_REMAINING_DAYS} days: issue a new one (client-update-certificate.sh leaf)`;
  const released = Date.parse(createdAt);
  if (!(released <= to)) return "the release is dated after the expiry of the signing certificate";
  // The apps never go back to an older release: publishing one over a newer
  // one would ship an image whose bundle no device takes.
  if (previousCreatedAt && released < Date.parse(previousCreatedAt)) return `this release (${createdAt}) is older than the one already in the output (${previousCreatedAt})`;
  let verified = false;
  try {
    verified = crypto.createVerify("RSA-SHA256").update(manifest, "utf8").verify(leaf.publicKey, signature, "base64");
  } catch {
    verified = false;
  }
  return verified ? "" : "the signing key does not match the signing certificate";
}

// The id and the date of a release, from its content and its commit. Same
// content, same id. The apps store one update per date: the milliseconds come
// from the content, so two builds of one commit do not claim the same slot,
// and are never zero: the code built into an app is dated to the second of
// its commit, and the release of that commit must come after it.
export function deriveRelease({ runtimeVersion, launchHash, assetHashes, commitSeconds }) {
  const digest = crypto
    .createHash("sha256")
    .update([runtimeVersion, launchHash, ...[...assetHashes].sort()].join("\n"))
    .digest("hex");
  return {
    id: `${digest.slice(0, 8)}-${digest.slice(8, 12)}-4${digest.slice(13, 16)}-8${digest.slice(17, 20)}-${digest.slice(20, 32)}`,
    createdAt: new Date(commitSeconds * 1000 + 1 + (parseInt(digest.slice(32, 40), 16) % 999)).toISOString(),
  };
}

// Signs a manifest and writes the file the server reads. Returns why not, or "".
export function signManifest({ bundleDir, manifest, keyPem, certPem, rootPem, previousCreatedAt, now }) {
  let signature;
  try {
    signature = crypto.createSign("RSA-SHA256").update(manifest, "utf8").sign(keyPem, "base64");
  } catch {
    return "the signing key file does not hold a usable private key";
  }
  let createdAt;
  try {
    createdAt = JSON.parse(manifest).createdAt;
  } catch {
    return "the manifest is not JSON";
  }
  // Everything a device will check, checked here first: a bundle the apps
  // refuse must not leave this machine.
  const problem = checkSigningCertificate({ leafPem: certPem, rootPem, manifest, signature, createdAt, previousCreatedAt, now });
  if (problem) return problem;
  writeSignedManifest(path.join(bundleDir, "manifest.signed"), { signature: `sig="${signature}", keyid="${KEY_ID}"`, certificate: certPem, manifest });
  return "";
}

const MANIFEST_KEYS = ["assets", "createdAt", "extra", "id", "launchAsset", "metadata", "runtimeVersion"];

// What the signing step requires of a bundle built elsewhere, by code it does
// not trust, before it puts the project's signature on it: the commit and the
// runtime it claims, a date that is the commit's and not one that would raise
// every device's floor, and files that are what the manifest says they are.
// Returns why not, or "".
export function checkUnsignedBundle({ bundleDir, runtimeVersion, commit, commitSeconds }) {
  let built;
  let raw;
  try {
    built = fs.readFileSync(path.join(bundleDir, "commit"), "utf8").trim();
    raw = fs.readFileSync(path.join(bundleDir, "manifest.json"), "utf8");
  } catch {
    return "the bundle has no commit or manifest.json file";
  }
  if (built !== commit) return `the bundle was built from ${JSON.stringify(built)}, not from ${commit}`;
  let manifest;
  try {
    manifest = JSON.parse(raw);
  } catch {
    return "manifest.json is not JSON";
  }
  if (manifest === null || typeof manifest !== "object" || Object.keys(manifest).sort().join() !== MANIFEST_KEYS.join()) return "manifest.json does not have exactly the expected fields";
  if (manifest.runtimeVersion !== runtimeVersion) return `the manifest is for runtime ${JSON.stringify(manifest.runtimeVersion)}, the app declares ${JSON.stringify(runtimeVersion)}`;
  if (JSON.stringify(manifest.metadata) !== "{}") return "the manifest metadata must stay empty";
  if (!Array.isArray(manifest.assets) || manifest.launchAsset === null || typeof manifest.launchAsset !== "object") return "the manifest has no launch asset or no asset list";
  if (manifest.launchAsset.contentType !== "application/javascript") return "the launch asset is not JavaScript";

  const names = new Set();
  for (const asset of [manifest.launchAsset, ...manifest.assets]) {
    if (asset === null || typeof asset !== "object" || typeof asset.url !== "string" || typeof asset.hash !== "string" || typeof asset.key !== "string") return "an asset lacks its url, hash or key";
    const match = new RegExp(`^assets/${PLATFORM}/${runtimeVersion.replace(/[.]/g, "\\.")}/([0-9a-f]{64}\\.[a-z0-9]{1,8})$`).exec(asset.url);
    if (!match) return `asset address ${JSON.stringify(asset.url)} is not one this bundle may name`;
    const name = match[1];
    let data;
    try {
      data = fs.readFileSync(path.join(bundleDir, "assets", name));
    } catch {
      return `asset ${name} is named by the manifest and missing from the bundle`;
    }
    const sha = crypto.createHash("sha256").update(data);
    if (sha.copy().digest("hex") !== name.slice(0, 64) || sha.digest("base64url") !== asset.hash) return `asset ${name} is not the file its name and hash describe`;
    if (crypto.createHash("md5").update(data).digest("hex") !== asset.key) return `asset ${name} does not match its key`;
    names.add(name);
  }
  let present;
  try {
    present = fs.readdirSync(path.join(bundleDir, "assets"));
  } catch {
    return "the bundle has no assets directory";
  }
  if (present.length !== names.size || present.some((name) => !names.has(name))) return "the bundle holds files the manifest does not name";
  if (fs.readdirSync(bundleDir).sort().join() !== "assets,commit,manifest.json") return "the bundle holds something else than assets, commit and manifest.json";

  const expected = deriveRelease({ runtimeVersion, launchHash: manifest.launchAsset.hash, assetHashes: manifest.assets.map((asset) => asset.hash), commitSeconds });
  if (manifest.id !== expected.id) return "the manifest id is not the one its content gives";
  if (manifest.createdAt !== expected.createdAt) return `the release is dated ${JSON.stringify(manifest.createdAt)}, its commit and content give ${expected.createdAt}`;
  return "";
}
