module.exports = function (api) {
  api.cache(true);
  return {
    presets: ["babel-preset-expo"],
    // Required by react-native-reanimated 4.x — worklets are compiled via
    // react-native-worklets' babel plugin.
    plugins: ["react-native-worklets/plugin"],
  };
};
