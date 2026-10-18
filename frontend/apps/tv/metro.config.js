const { getDefaultConfig } = require("expo/metro-config");
const path = require("path");

const projectRoot = __dirname;
const monorepoRoot = path.resolve(projectRoot, "../..");

const config = getDefaultConfig(projectRoot);

config.watchFolders = [monorepoRoot];
config.resolver.nodeModulesPaths = [
  path.resolve(projectRoot, "node_modules"),
  path.resolve(monorepoRoot, "node_modules"),
];
config.resolver.disableHierarchicalLookup = true;
config.resolver.unstable_enableSymlinks = true;

// Tests never belong in the app bundle. This app colocates screen tests inside
// app/, where expo-router treats every file as a route: each *.test.tsx gets
// bundled, drags vitest -> vite into the graph and Metro dies with an opaque
// `import.meta` SyntaxError in vite/module-runner (same failure mobile hit on
// 2026-07-25). The blockList keeps vitest visible to the test runner only.
config.resolver.blockList = [/\.test\.(ts|tsx|js|jsx)$/];

// expo-router's native tabs (unused here) pull expo-symbols, whose Android
// build requires the 7 Material Symbols font weights (~6.6 MB in the bundle).
// Resolving that font package to an empty module drops them, as on mobile.
const upstreamResolveRequest = config.resolver.resolveRequest;
config.resolver.resolveRequest = (context, moduleName, platform) => {
  if (moduleName.startsWith("@expo-google-fonts/material-symbols")) {
    return { type: "empty" };
  }
  return (upstreamResolveRequest ?? context.resolveRequest)(context, moduleName, platform);
};

module.exports = config;
