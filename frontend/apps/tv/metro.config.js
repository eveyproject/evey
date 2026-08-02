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

module.exports = config;
