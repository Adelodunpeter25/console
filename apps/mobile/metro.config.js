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

// 3. Force single instance of react & react-native across all monorepo dependencies
config.resolver.extraNodeModules = {
  react: path.resolve(projectRoot, "node_modules/react"),
  "react/jsx-runtime": path.resolve(projectRoot, "node_modules/react/jsx-runtime"),
  "react-native": path.resolve(projectRoot, "node_modules/react-native"),
  "@tanstack/react-query": path.resolve(
    projectRoot,
    "node_modules/@tanstack/react-query"
  ),
  "react-native-safe-area-context": path.resolve(
    projectRoot,
    "node_modules/react-native-safe-area-context"
  ),
  "@console/api": path.resolve(workspaceRoot, "packages/api/src"),
  "@console/types": path.resolve(workspaceRoot, "packages/types/src"),
};

// 4. Force resolution of 'react', 'react-native', and '@tanstack/react-query' to apps/mobile
const defaultResolver = config.resolver.resolveRequest;
config.resolver.resolveRequest = (context, moduleName, platform) => {
  if (moduleName === "react") {
    return {
      filePath: path.resolve(projectRoot, "node_modules/react/index.js"),
      type: "sourceFile",
    };
  }
  if (moduleName === "react/jsx-runtime") {
    return {
      filePath: path.resolve(projectRoot, "node_modules/react/jsx-runtime.js"),
      type: "sourceFile",
    };
  }
  if (moduleName === "@tanstack/react-query") {
    return {
      filePath: path.resolve(
        projectRoot,
        "node_modules/@tanstack/react-query/build/legacy/index.js"
      ),
      type: "sourceFile",
    };
  }
  if (defaultResolver) {
    return defaultResolver(context, moduleName, platform);
  }
  return context.resolveRequest(context, moduleName, platform);
};

module.exports = withUniwindConfig(config, {
  cssEntryFile: "./global.css",
  dtsFile: "./uniwind-types.d.ts",
});
