import type { ComponentType } from "react";
import type { NativeSyntheticEvent, ViewProps } from "react-native";
import { requireNativeView } from "expo";

const NATIVE_TERMINAL_MODULE_NAME = "ConsoleTerminalSurface";

interface ExpoGlobalWithViewConfig {
  readonly expo?: {
    getViewConfig?: (moduleName: string, viewName?: string) => unknown;
  };
}

interface TerminalInputEvent {
  readonly data: string;
}

interface TerminalResizeEvent {
  readonly cols: number;
  readonly rows: number;
}

export interface NativeTerminalSurfaceProps extends ViewProps {
  readonly appearanceScheme?: "light" | "dark";
  readonly autoFocus?: boolean;
  readonly focusRequest?: number;
  readonly dismissKeyboard?: number;
  readonly themeConfig?: string;
  readonly backgroundColor?: string;
  readonly foregroundColor?: string;
  readonly mutedForegroundColor?: string;
  readonly terminalKey: string;
  readonly initialBuffer: string;
  readonly fontSize: number;
  readonly onInput?: (event: NativeSyntheticEvent<TerminalInputEvent>) => void;
  readonly onResize?: (event: NativeSyntheticEvent<TerminalResizeEvent>) => void;
}

let cachedNativeTerminalSurfaceView: ComponentType<NativeTerminalSurfaceProps> | undefined;
let nativeTerminalSurfaceViewResolutionFailed = false;

function getExpoViewConfig(moduleName: string) {
  return (globalThis as typeof globalThis & ExpoGlobalWithViewConfig).expo?.getViewConfig?.(
    moduleName,
  );
}

/**
 * Resolve the native `ConsoleTerminalSurface` view. Returns null when the native
 * module isn't present (e.g. dev-client binary built before the module landed);
 * callers fall back to the JS terminal surface.
 */
export function resolveNativeTerminalSurfaceView(): ComponentType<NativeTerminalSurfaceProps> | null {
  if (cachedNativeTerminalSurfaceView) {
    return cachedNativeTerminalSurfaceView;
  }

  if (nativeTerminalSurfaceViewResolutionFailed) {
    return null;
  }

  if (getExpoViewConfig(NATIVE_TERMINAL_MODULE_NAME) == null) {
    return null;
  }

  try {
    cachedNativeTerminalSurfaceView = requireNativeView<NativeTerminalSurfaceProps>(
      NATIVE_TERMINAL_MODULE_NAME,
    );
  } catch (cause) {
    nativeTerminalSurfaceViewResolutionFailed = true;
    console.error(`Failed to resolve native view "${NATIVE_TERMINAL_MODULE_NAME}"`, cause);
    return null;
  }

  return cachedNativeTerminalSurfaceView ?? null;
}

export function hasNativeTerminalSurface() {
  return resolveNativeTerminalSurfaceView() !== null;
}
