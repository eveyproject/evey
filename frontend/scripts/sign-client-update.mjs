#!/usr/bin/env node
// Signs the bundles build-client-update.mjs left unsigned.
//
//   node scripts/sign-client-update.mjs --in <dir> --out <dir> --key-file <leaf-key.pem> --cert-file <leaf-cert.pem>
//
// The release pipeline builds the bundles in one job, with every dependency
// of the apps and no key, and signs them here in another, with the key and
// nothing but this checkout: Node built-ins only, no install, so no code the
// project did not write runs next to the key.
//
// Both apps must be there, built from the commit checked out here, for the
// runtime each app declares at that commit (checkUnsignedBundle has the rest).
// Anything else in <in> is refused. <out> gets the layout the image copies.

import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { checkUnsignedBundle, PLATFORM, signManifest } from "./client-update-signing.mjs";

const FRONTEND = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const APPS = ["mobile", "tv"];

function fail(message) {
  console.error(`sign-client-update: ${message}`);
  process.exit(1);
}

const args = {};
const argv = process.argv.slice(2);
for (let i = 0; i < argv.length; i += 2) {
  // The argument is not echoed: a key pasted on the command line stays out of the output.
  if (!argv[i].startsWith("--") || argv[i + 1] === undefined) fail(`bad argument at position ${i + 1}`);
  args[argv[i].slice(2)] = argv[i + 1];
}
for (const name of ["in", "out", "key-file", "cert-file"]) {
  if (!args[name]) fail(`--${name} is required`);
}
if (args["key-file"].includes("-----BEGIN")) fail("the signing key must be given as a file path, not as the key itself");
const read = (file, what) => {
  try {
    return fs.readFileSync(file, "utf8");
  } catch {
    return fail(`cannot read ${what}`);
  }
};
const keyPem = read(args["key-file"], "the signing key file (--key-file)");
const certPem = read(args["cert-file"], "the signing certificate file (--cert-file)");
const rootPem = read(path.join(FRONTEND, "client-updates", "certificate.pem"), "frontend/client-updates/certificate.pem");

const git = (gitArgs) => execFileSync("git", gitArgs, { cwd: FRONTEND, encoding: "utf8" }).trim();
const commit = git(["rev-parse", "HEAD"]);
if (git(["status", "--porcelain", "--untracked-files=no"]) !== "") fail("the working tree has uncommitted changes");
const commitSeconds = Number(git(["log", "-1", "--format=%ct"]));

const input = path.resolve(args.in);
const output = path.resolve(args.out);
const list = (dir) => {
  try {
    return fs.readdirSync(dir).sort();
  } catch {
    return [];
  }
};
if (list(input).join() !== APPS.join()) fail(`--in must hold exactly ${APPS.join(" and ")}`);

// Every bundle is checked before the first signature is made.
const bundles = APPS.map((app) => {
  const runtimeVersion = JSON.parse(read(path.join(FRONTEND, "apps", app, "app.json"), `apps/${app}/app.json`)).expo?.runtimeVersion;
  if (typeof runtimeVersion !== "string" || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(runtimeVersion)) fail(`apps/${app}/app.json needs a plain string runtimeVersion`);
  if (list(path.join(input, app)).join() !== PLATFORM || list(path.join(input, app, PLATFORM)).join() !== runtimeVersion) {
    fail(`${app}: expected one bundle, for ${PLATFORM} and runtime ${runtimeVersion}`);
  }
  const bundleDir = path.join(input, app, PLATFORM, runtimeVersion);
  const problem = checkUnsignedBundle({ bundleDir, runtimeVersion, commit, commitSeconds });
  if (problem) fail(`${app}: ${problem}`);
  return { app, runtimeVersion, bundleDir };
});

for (const { app, runtimeVersion, bundleDir } of bundles) {
  const target = path.join(output, app, PLATFORM, runtimeVersion);
  fs.rmSync(target, { recursive: true, force: true });
  fs.mkdirSync(target, { recursive: true });
  fs.cpSync(path.join(bundleDir, "assets"), path.join(target, "assets"), { recursive: true });
  fs.copyFileSync(path.join(bundleDir, "commit"), path.join(target, "commit"));
  const manifest = fs.readFileSync(path.join(bundleDir, "manifest.json"), "utf8");
  const problem = signManifest({ bundleDir: target, manifest, keyPem, certPem, rootPem });
  if (problem) fail(`${app}: ${problem}`);
  console.log(`${app} ${PLATFORM} ${runtimeVersion}: update ${JSON.parse(manifest).id} signed -> ${target}`);
}
