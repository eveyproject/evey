#!/usr/bin/env node
// Builds the signed JavaScript bundle an Evey instance serves to one app.
//
//   node scripts/build-client-update.mjs --app tv|mobile --out <dir> [--key-file <leaf-key.pem> --cert-file <leaf-cert.pem>] [--allow-dirty yes]
//
// Output, the layout internal/adapters/driving/http/client_update_handler.go reads:
//
//   <out>/<app>/android/<runtimeVersion>/manifest.signed   signature line, certificate line (base64), then the manifest
//   <out>/<app>/android/<runtimeVersion>/commit            the commit it was built from
//   <out>/<app>/android/<runtimeVersion>/assets/<sha256>.<ext>
//
// The manifest is the same on every instance that ships this build: assets
// are named by relative path, the id comes from the content and the date from
// the commit. The Hermes bytecode is not reproducible, so two builds of one
// commit are two updates: build once per release and ship that output.
//
// The signing key and its certificate come from client-update-certificate.sh
// leaf (--key-file and --cert-file, or EVEY_CLIENT_UPDATE_KEY_FILE and
// EVEY_CLIENT_UPDATE_CERT_FILE). Without them only manifest.json is written,
// which the server does not serve: enough to inspect the output, not to ship
// it. The key is always a path, never the key itself.

import { execFileSync } from "node:child_process";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { checkSigningCertificate, readSignedManifest, writeSignedManifest } from "./client-update-signing.mjs";

const FRONTEND = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const PLATFORM = "android";
const KEY_ID = "main";
const MIME = { png: "image/png", jpg: "image/jpeg", jpeg: "image/jpeg", gif: "image/gif", webp: "image/webp", svg: "image/svg+xml", ttf: "font/ttf", otf: "font/otf", json: "application/json" };

function fail(message) {
  console.error(`build-client-update: ${message}`);
  process.exit(1);
}

function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i += 2) {
    // The argument is not echoed: a key pasted on the command line stays out of the output.
    if (!argv[i].startsWith("--") || argv[i + 1] === undefined) fail(`bad argument at position ${i + 1}`);
    args[argv[i].slice(2)] = argv[i + 1];
  }
  return args;
}

const args = parseArgs(process.argv.slice(2));
const app = args.app;
if (app !== "tv" && app !== "mobile") fail("--app must be tv or mobile");
if (!args.out) fail("--out is required");
const keyPath = args["key-file"] ?? process.env.EVEY_CLIENT_UPDATE_KEY_FILE ?? "";
// A key pasted where a path is expected would end up in an error message,
// a process list or a CI log. Nothing below ever prints this value.
if (keyPath.includes("-----BEGIN")) fail("the signing key must be given as a file path, not as the key itself");
let signingKey = "";
if (keyPath) {
  try {
    signingKey = fs.readFileSync(keyPath, "utf8");
  } catch {
    fail("cannot read the signing key file (--key-file or EVEY_CLIENT_UPDATE_KEY_FILE)");
  }
}

const certPath = args["cert-file"] ?? process.env.EVEY_CLIENT_UPDATE_CERT_FILE ?? "";
let signingCertificate = "";
if (signingKey) {
  try {
    signingCertificate = fs.readFileSync(certPath, "utf8");
  } catch {
    fail("cannot read the signing certificate file (--cert-file or EVEY_CLIENT_UPDATE_CERT_FILE)");
  }
}
const rootCertificate = fs.readFileSync(path.join(FRONTEND, "client-updates", "certificate.pem"), "utf8");

const git = (gitArgs) => execFileSync("git", gitArgs, { cwd: FRONTEND, encoding: "utf8" }).trim();
const commit = git(["rev-parse", "HEAD"]);
// A signed bundle claims to be the code of a commit. Uncommitted changes
// would be signed under a commit that does not contain them; the image build
// refuses such a bundle anyway (its commit file says so).
const dirty = git(["status", "--porcelain", "--untracked-files=no"]) !== "";
if (signingKey && dirty && args["allow-dirty"] !== "yes") {
  fail("the working tree has uncommitted changes: commit them, or pass --allow-dirty yes for a bench bundle no image will accept");
}

const appDir = path.join(FRONTEND, "apps", app);
const env = { ...process.env, ...(app === "tv" ? { EXPO_TV: "1" } : {}) };
const expo = (cliArgs) => execFileSync("npx", ["expo", ...cliArgs], { cwd: appDir, env, encoding: "utf8", stdio: ["ignore", "pipe", "inherit"], maxBuffer: 64 << 20 });

const expoConfig = JSON.parse(expo(["config", "--json", "--type", "public"]));
const runtimeVersion = expoConfig.runtimeVersion;
if (typeof runtimeVersion !== "string" || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(runtimeVersion)) {
  fail(`apps/${app}/app.json needs a plain string runtimeVersion, got ${JSON.stringify(runtimeVersion)}`);
}

const exportDir = fs.mkdtempSync(path.join(os.tmpdir(), `evey-client-update-${app}-`));
try {
  expo(["export", "--platform", PLATFORM, "--output-dir", exportDir]);
  const exported = JSON.parse(fs.readFileSync(path.join(exportDir, "metadata.json"), "utf8")).fileMetadata[PLATFORM];

  const bundleDir = path.join(path.resolve(args.out), app, PLATFORM, runtimeVersion);
  const previous = readSignedManifest(path.join(bundleDir, "manifest.signed"));
  fs.rmSync(bundleDir, { recursive: true, force: true });
  fs.mkdirSync(path.join(bundleDir, "assets"), { recursive: true });

  const addAsset = (relPath, rawExt, isLaunch) => {
    const ext = String(rawExt).toLowerCase();
    // Same rule as the server, which answers 404 to any other name.
    if (!/^[a-z0-9]{1,8}$/.test(ext)) fail(`asset ${relPath} has an extension the server would not serve: ${JSON.stringify(rawExt)}`);
    const data = fs.readFileSync(path.join(exportDir, relPath));
    const sha = crypto.createHash("sha256").update(data);
    const name = `${sha.copy().digest("hex")}.${ext}`;
    fs.writeFileSync(path.join(bundleDir, "assets", name), data);
    return {
      hash: sha.digest("base64url"),
      key: crypto.createHash("md5").update(data).digest("hex"),
      contentType: isLaunch ? "application/javascript" : MIME[ext] ?? "application/octet-stream",
      ...(isLaunch ? {} : { fileExtension: `.${ext}` }),
      // Relative to the manifest URL (/api/v1/client-updates/<app>/manifest).
      url: `assets/${PLATFORM}/${runtimeVersion}/${name}`,
    };
  };
  const launchAsset = addAsset(exported.bundle, "hbc", true);
  const assets = exported.assets.map((asset) => addAsset(asset.path, asset.ext, false));

  // Same content, same id.
  const digest = crypto
    .createHash("sha256")
    .update([runtimeVersion, launchAsset.hash, ...assets.map((a) => a.hash).sort()].join("\n"))
    .digest("hex");
  const id = `${digest.slice(0, 8)}-${digest.slice(8, 12)}-4${digest.slice(13, 16)}-8${digest.slice(17, 20)}-${digest.slice(20, 32)}`;
  const commitSeconds = process.env.SOURCE_DATE_EPOCH
    ? Number(process.env.SOURCE_DATE_EPOCH)
    : Number(git(["log", "-1", "--format=%ct"]));
  // The apps store one update per date. The milliseconds come from the
  // content, so two builds of one commit do not claim the same slot.
  const createdAt = new Date(commitSeconds * 1000 + (parseInt(digest.slice(32, 40), 16) % 1000)).toISOString();

  const manifestObject = {
    id,
    createdAt,
    runtimeVersion,
    launchAsset,
    assets,
    // Must stay empty: the apps match it against filters sent in an unsigned
    // response header, which would let a server pick among signed bundles.
    metadata: {},
    extra: { expoClient: expoConfig },
  };
  if (Object.keys(manifestObject.metadata).length > 0) fail("the manifest metadata must stay empty");
  const manifest = JSON.stringify(manifestObject);

  if (signingKey) {
    let signature;
    try {
      signature = crypto.createSign("RSA-SHA256").update(manifest, "utf8").sign(signingKey, "base64");
    } catch {
      fail("the signing key file does not hold a usable private key");
    }
    // Everything a device will check, checked here first: a bundle the apps
    // refuse must not leave this machine.
    const problem = checkSigningCertificate({ leafPem: signingCertificate, rootPem: rootCertificate, manifest, signature, createdAt, previousCreatedAt: previous?.createdAt });
    if (problem) fail(problem);
    writeSignedManifest(path.join(bundleDir, "manifest.signed"), { signature: `sig="${signature}", keyid="${KEY_ID}"`, certificate: signingCertificate, manifest });
    fs.writeFileSync(path.join(bundleDir, "commit"), `${commit}${dirty ? "-dirty" : ""}\n`);
  } else {
    fs.writeFileSync(path.join(bundleDir, "manifest.json"), manifest);
    console.warn("build-client-update: no signing key, bundle left unsigned (the server will not serve it)");
  }

  console.log(`${app} ${PLATFORM} ${runtimeVersion}: update ${id} (${createdAt}), ${assets.length + 1} files, ${signingKey ? "signed" : "UNSIGNED"} -> ${bundleDir}`);
} finally {
  fs.rmSync(exportDir, { recursive: true, force: true });
}
