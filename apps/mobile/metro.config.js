const { getDefaultConfig } = require("expo/metro-config");
const { withUniwindConfig } = require("uniwind/metro");
const path = require("path");

const projectRoot = __dirname;
const workspaceRoot = path.resolve(projectRoot, "../..");

const config = getDefaultConfig(projectRoot);

// 1. Watch all workspace folders within the monorepo
config.watchFolders = [workspaceRoot];

// 2. Let Metro resolve packages from local node_modules first, then workspace root
config.resolver.nodeModulesPaths = [
  path.resolve(projectRoot, "node_modules"),
  path.resolve(workspaceRoot, "node_modules"),
];

// 3. Map workspace packages
config.resolver.extraNodeModules = {
  "@console/api": path.resolve(workspaceRoot, "packages/api/src"),
  "@console/types": path.resolve(workspaceRoot, "packages/types/src"),
};

module.exports = withUniwindConfig(config, {
  cssEntryFile: "./global.css",
  dtsFile: "./uniwind-types.d.ts",
});
