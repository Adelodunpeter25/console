const { getDefaultConfig } = require('expo/metro-config');
const { withMonorepoPaths } = require('@expo/metro-config');

const config = getDefaultConfig(__dirname);

// Add monorepo support for workspace packages
const monorepoConfig = withMonorepoPaths({
  projectRoot: __dirname,
  watchFolders: [
    // Add workspace packages to be resolved
    require('path').resolve(__dirname, '../../packages/api'),
    require('path').resolve(__dirname, '../../packages/types'),
  ],
});

module.exports = {
  ...config,
  ...monorepoConfig,
  resolver: {
    ...config.resolver,
    ...monorepoConfig.resolver,
    sourceExts: [...config.resolver.sourceExts, 'ts', 'tsx'],
  },
};
