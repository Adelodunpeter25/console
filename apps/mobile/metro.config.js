const { getDefaultConfig } = require("expo/metro-config");
const { withUniwindConfig } = require("uniwind/metro");
const path = require("path");

const projectRoot = __dirname;
const workspaceRoot = path.resolve(projectRoot, "../..");

const config = getDefaultConfig(projectRoot);

// 0. Honor tsconfig.json "paths" aliases (e.g. "@/*" -> app root)
config.experiments = { ...config.experiments, tsconfigPaths: true };

// 1. Watch all workspace folders within the monorepo
config.watchFolders = [workspaceRoot];

// 2. Let Metro resolve packages from local node_modules first, then workspace root
config.resolver.nodeModulesPaths = [
  path.resolve(projectRoot, "node_modules"),
  path.resolve(workspaceRoot, "node_modules"),
];

// 3. Map workspace packages and pin critical singletons to mobile's node_modules
config.resolver.extraNodeModules = {
  react: path.resolve(projectRoot, "node_modules/react"),
  "react-native": path.resolve(projectRoot, "node_modules/react-native"),
  "@tanstack/react-query": path.resolve(projectRoot, "node_modules/@tanstack/react-query"),
  "@console/api": path.resolve(workspaceRoot, "packages/api/src"),
  "@console/types": path.resolve(workspaceRoot, "packages/types/src"),
};

module.exports = withUniwindConfig(config, {
  cssEntryFile: "./global.css",
  dtsFile: "./uniwind-types.d.ts",
});
