const { getDefaultConfig } = require("expo/metro-config");
const { withNativeWind } = require("nativewind/metro");
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

// Tests never belong in the app bundle. A single *.test.tsx dropped inside
// app/ becomes an expo-router route, drags vitest -> vite into the bundle and
// kills the release build at 92% with an opaque SyntaxError in
// vite/module-runner. The blockList makes that class of mistake
// fail fast at resolution instead.
config.resolver.blockList = [/\.test\.(ts|tsx|js|jsx)$/];

// expo-router's native tabs (unused here) pull expo-symbols, whose Android
// build requires the 7 Material Symbols font weights (~6.6 MB in the APK).
// Resolving that font package to an empty module drops them.
const upstreamResolveRequest = config.resolver.resolveRequest;
config.resolver.resolveRequest = (context, moduleName, platform) => {
  if (moduleName.startsWith("@expo-google-fonts/material-symbols")) {
    return { type: "empty" };
  }
  // query-string's ESM entry (index.js) only re-exports base.js as `default`, so
  // expo-router and react-navigation, which read the module through __importStar,
  // see no parse/stringify and crash at runtime. base.js carries those as named
  // exports. Resolve the package normally, then point Metro at base.js. We stay on
  // this line because it is the one that pulls the patched decode-uri-component
  // (ESM-only), so no node_modules patch and no open advisory.
  if (moduleName === "query-string") {
    const resolved = (upstreamResolveRequest ?? context.resolveRequest)(context, moduleName, platform);
    if (resolved && resolved.type === "sourceFile" && typeof resolved.filePath === "string") {
      return { ...resolved, filePath: path.join(path.dirname(resolved.filePath), "base.js") };
    }
    return resolved;
  }
  return (upstreamResolveRequest ?? context.resolveRequest)(context, moduleName, platform);
};

module.exports = withNativeWind(config, { input: "./global.css" });
