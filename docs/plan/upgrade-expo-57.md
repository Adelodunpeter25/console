# Expo SDK 57 Upgrade Plan

## 1. Overview & Goals
- **Objective**: Upgrade `@console/mobile` and root workspace from Expo SDK 54 to **Expo SDK 57** (React Native 0.86, React 19.2) in one coordinated update without breaking existing mobile functionality.
- **Key Benefits**:
  - Significant performance improvements and faster cold boot / startup times.
  - React Native 0.86 runtime and Hermes engine optimizations (including memory stability fixes for `react-native-worklets` and `react-native-reanimated`).
  - Updated native platform dependencies and modern build tooling.

---

## 2. Current State & Dependency Inventory

### Core Versions
| Package | Current Version (SDK 54) | Target Version (SDK 57) |
| :--- | :--- | :--- |
| `expo` | `~54.0.0` | `~57.0.0` (or `^57.0.17+`) |
| `react` | `19.1.0` | `19.2.0` |
| `react-dom` | `19.1.0` | `19.2.0` |
| `react-native` | `0.81.5` | `0.86.x` |
| `babel-preset-expo` | `~54.0.10` | `~57.0.0` |
| `@expo/metro-runtime` | `~6.1.2` | Matching SDK 57 |

### Expo Core Modules (`apps/mobile/package.json`)
- `expo-blur`, `expo-clipboard`, `expo-constants`, `expo-dev-client`, `expo-font`, `expo-image-picker`, `expo-linking`, `expo-notifications`, `expo-status-bar`

### Native & Community Libraries
- `react-native-reanimated` (`~4.1.0`) & `react-native-worklets` (`~0.8.0`)
- `react-native-nitro-modules` (`^0.37.0`) & `react-native-nitro-fetch` (`^1.6.2`)
- `react-native-mmkv` (`^4.3.2`)
- `react-native-safe-area-context` (`~5.6.0`)
- `react-native-screens` (`~4.16.0`)
- `react-native-gesture-handler` (`~2.28.0`)
- `react-native-keyboard-controller` (`1.18.5`)
- `react-native-svg` (`15.12.1`)
- `@gorhom/bottom-sheet` (`^5.2.14`)
- `@legendapp/state` (`^3.0.0-beta.48`) & `@legendapp/list` (`^3.3.8`)
- `uniwind` (`^1.10.0`) & `tailwindcss` (`^4.3.0`)
- Custom modules: `@console/mobile-terminal-native`, `native-stream`, `local-auth-server`

---

## 3. High-Risk Areas & Pre-Upgrade Checks

1. **React 19.2 & Monorepo Root Overrides**:
   - The root `package.json` pins `react`, `react-dom`, `@types/react`, and `react-native-safe-area-context` via `overrides`. These must be updated in tandem to prevent duplicate React instances across workspaces.
2. **Reanimated 4.x / Worklets Compatibility**:
   - SDK 57.0.17+ includes specific Hermes V1 fixes for `react-native-worklets`. Ensure `react-native-worklets/plugin` in `babel.config.js` resolves cleanly.
3. **Uniwind & Metro Resolver**:
   - `apps/mobile/metro.config.js` configures monorepo `watchFolders`, `extraNodeModules`, and `withUniwindConfig`. Check that Metro resolver hooks remain compatible with the updated `@expo/metro-config`.
4. **Android Native Configuration (`apps/mobile/android`)**:
   - Check Kotlin version, Gradle distribution, Android Gradle Plugin (AGP), and NDK versions against React Native 0.86 requirements.
   - Verify that custom local Expo modules (`modules/console-terminal`) compile cleanly under the new Android runtime.

---

## 4. Phased Implementation Plan

### Phase 1: Dependency & Manifest Alignment
- Update `package.json` in root:
  - Update `overrides` for `react`, `react-dom`, and `@types/react` to `19.2.0`.
  - Update root `dependencies` / `devDependencies` (`expo`, `react-native-safe-area-context`, `react-native-worklets`).
- Update `apps/mobile/package.json`:
  - Bump `expo` to `~57.0.0` (or latest SDK 57 release).
  - Bump `react` to `19.2.0`, `react-native` to `0.86.x`.
  - Run `npx expo install --fix` within `apps/mobile` to automatically align all `expo-*` modules and community dependencies (`react-native-screens`, `react-native-gesture-handler`, `react-native-svg`, `react-native-safe-area-context`) with SDK 57 supported versions.
  - Run `bun install` at the workspace root to regenerate `bun.lock`.

### Phase 2: Bundler & Configuration Verification
- **Metro (`apps/mobile/metro.config.js`)**:
  - Validate `getDefaultConfig(projectRoot)` behavior with SDK 57.
  - Ensure `extraNodeModules` mapping for `@console/api`, `@console/types`, and singletons (`react`, `react-native`, `@tanstack/react-query`) functions properly.
- **Babel (`apps/mobile/babel.config.js`)**:
  - Verify `babel-preset-expo` and `react-native-worklets/plugin` ordering.
- **TypeScript (`tsconfig.json`)**:
  - Run `bun run typecheck` across root and mobile app to resolve any typing discrepancies with React 19.2.

### Phase 3: Android Native Prebuild & Modules Validation
- Inspect `apps/mobile/android/gradle.properties`, `build.gradle`, and `app/build.gradle`.
- If using Expo CNG (Continuous Native Generation), run `bunx expo prebuild --platform android --clean` or reconcile Gradle / AGP settings with RN 0.86 standards.
- Test compilation of custom local module `modules/console-terminal` to ensure C++/Kotlin bindings conform to RN 0.86 TurboModule/Expo module APIs.

### Phase 4: Runtime & UI Sanity Check
- Verify navigation stack and gesture handling (`react-native-gesture-handler`, `react-native-screens`, `@gorhom/bottom-sheet`).
- Check Uniwind/Tailwind style rendering across dark/light modes.
- Verify Markdown parsing (`react-native-markdown-display`), Shiki code highlighting, and terminal emulation views.
- Test WebSocket/SSE streaming connection from mobile to `apps/server`.

### Phase 5: Verification & Automated Bundle Export
- Run mobile export verification:
  ```bash
  cd apps/mobile && bunx expo export --platform android
  ```
- Run doctor check:
  ```bash
  cd apps/mobile && bunx expo-doctor
  ```
- Verify TypeScript check passes:
  ```bash
  bun run typecheck
  ```

---

## 5. Verification Checklist
- [ ] `bun.lock` resolved with zero peer dependency conflicts.
- [ ] `bun run typecheck` passes with no errors.
- [ ] `cd apps/mobile && bunx expo export --platform android` bundles successfully.
- [ ] Local Android build (`bun run --cwd apps/mobile android`) compiles and starts without crash.
- [ ] Custom native module `@console/mobile-terminal-native` loads properly.
- [ ] Chat streaming, session switches, and diff views operate smoothly.
