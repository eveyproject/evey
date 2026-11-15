// What makes a signed bundle one the apps accept, shared by the publication
// tool and its tests (docs/client-updates.md).

import crypto from "node:crypto";
import fs from "node:fs";

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
