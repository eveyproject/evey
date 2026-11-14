#!/usr/bin/env node
// Builds the signed JavaScript bundle an Evey instance serves to one app.
//
//   node scripts/build-client-update.mjs --app tv|mobile --out <dir> [--key <private-key.pem>]
//
// Output, the layout internal/adapters/driving/http/client_update_handler.go reads:
//
//   <out>/<app>/android/<runtimeVersion>/manifest.json
//   <out>/<app>/android/<runtimeVersion>/manifest.sig
//   <out>/<app>/android/<runtimeVersion>/assets/<sha256>.<ext>
//
// The manifest is the same on every instance that ships this build: assets
// are named by relative path, the id comes from the content and the date from
// the commit. The Hermes bytecode is not reproducible, so two builds of one
// commit are two updates: build once per release and ship that output.
//
// Without --key (or EVEY_CLIENT_UPDATE_KEY) the bundle is written unsigned and
// the server will not offer it: enough to inspect the output, not to ship it.

import { execFileSync } from "node:child_process";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

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
    if (!argv[i].startsWith("--") || argv[i + 1] === undefined) fail(`bad argument ${argv[i]}`);
    args[argv[i].slice(2)] = argv[i + 1];
  }
  return args;
}

const args = parseArgs(process.argv.slice(2));
const app = args.app;
if (app !== "tv" && app !== "mobile") fail("--app must be tv or mobile");
if (!args.out) fail("--out is required");
const keyPath = args.key ?? process.env.EVEY_CLIENT_UPDATE_KEY ?? "";

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
  fs.rmSync(bundleDir, { recursive: true, force: true });
  fs.mkdirSync(path.join(bundleDir, "assets"), { recursive: true });

  const addAsset = (relPath, ext, isLaunch) => {
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
    : Number(execFileSync("git", ["log", "-1", "--format=%ct"], { cwd: FRONTEND, encoding: "utf8" }).trim());
  // The apps store one update per date. The milliseconds come from the
  // content, so two builds of one commit do not claim the same slot.
  const createdAt = new Date(commitSeconds * 1000 + (parseInt(digest.slice(32, 40), 16) % 1000)).toISOString();

  const manifest = JSON.stringify({
    id,
    createdAt,
    runtimeVersion,
    launchAsset,
    assets,
    metadata: {},
    extra: { expoClient: expoConfig },
  });
  fs.writeFileSync(path.join(bundleDir, "manifest.json"), manifest);

  if (keyPath) {
    const signature = crypto.createSign("RSA-SHA256").update(manifest, "utf8").sign(fs.readFileSync(keyPath, "utf8"), "base64");
    // The apps verify against this certificate: a key that does not match it
    // would ship a bundle every app refuses.
    const certificate = new crypto.X509Certificate(fs.readFileSync(path.join(FRONTEND, "client-updates", "certificate.pem")));
    const verified = crypto.createVerify("RSA-SHA256").update(manifest, "utf8").verify(certificate.publicKey, signature, "base64");
    if (!verified) fail("the signing key does not match client-updates/certificate.pem");
    fs.writeFileSync(path.join(bundleDir, "manifest.sig"), `sig="${signature}", keyid="${KEY_ID}"\n`);
  } else {
    console.warn("build-client-update: no signing key, bundle left unsigned (the server will not offer it)");
  }

  console.log(`${app} ${PLATFORM} ${runtimeVersion}: update ${id} (${createdAt}), ${assets.length + 1} files, ${keyPath ? "signed" : "UNSIGNED"} -> ${bundleDir}`);
} finally {
  fs.rmSync(exportDir, { recursive: true, force: true });
}
