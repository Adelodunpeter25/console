import { requireNativeModule, type EventSubscription } from "expo-modules-core";
import { Platform } from "react-native";

/**
 * Native module that runs a tiny HTTP server on the device's localhost to
 * catch the OAuth loopback redirect. Android-only — on iOS the app gets
 * suspended when the user switches to the browser and the server dies, so
 * the manual paste flow is used there instead.
 */

let LocalAuthServerModule: any = null;
let emitter: {
  addListener(eventName: string, listener: (event: never) => void): EventSubscription;
} | null = null;

try {
  LocalAuthServerModule = requireNativeModule("LocalAuthServerModule");
  if (LocalAuthServerModule) {
    // In Expo SDK 52+ the native module object is already an EventEmitter.
    emitter = LocalAuthServerModule;
  }
} catch {
  // Not available on web / test / iOS builds without the native module.
}

export interface AuthCallbackResult {
  code: string;
  state?: string;
}

export interface AuthServerError {
  error: string;
}

/** True when the native localhost auth server is available (Android + module loaded). */
export function isLocalAuthServerAvailable(): boolean {
  return Boolean(LocalAuthServerModule && emitter && Platform.OS === "android");
}

/**
 * Start a localhost HTTP server that listens for a single OAuth redirect on
 * `http://127.0.0.1:{port}{callbackPath}`. When the browser redirects there
 * after authentication, the server extracts `code`/`state` from the query
 * string and emits an event. Resolves true when the socket is bound.
 *
 * Only one server runs at a time — starting a new one stops the previous.
 */
export async function startAuthServer(
  port: number,
  callbackPath: string,
): Promise<boolean> {
  if (!LocalAuthServerModule) {
    throw new Error("LocalAuthServer native module is not available.");
  }
  return LocalAuthServerModule.startServer(port, callbackPath);
}

/** Stop the localhost auth server if one is running. */
export async function stopAuthServer(): Promise<boolean> {
  if (!LocalAuthServerModule) return false;
  return LocalAuthServerModule.stopServer();
}

/** Whether a server is currently listening. */
export async function isAuthServerRunning(): Promise<boolean> {
  if (!LocalAuthServerModule) return false;
  return LocalAuthServerModule.isRunning();
}

/**
 * Subscribe to the auth-callback event. Returns an unsubscribe function.
 * The listener fires once with the `code`/`state` extracted from the
 * redirect URL, then the server auto-stops.
 */
export function addAuthCallbackListener(
  onCallback: (result: AuthCallbackResult) => void,
  onError?: (error: AuthServerError) => void,
): () => void {
  if (!emitter) return () => {};

  const subs: EventSubscription[] = [];
  subs.push(emitter.addListener("onAuthCallback", onCallback));
  if (onError) {
    subs.push(emitter.addListener("onAuthError", onError));
  }
  return () => subs.forEach((s) => s.remove());
}

/**
 * Subscribe to the auth-complete event (fires after onAuthCallback, once the
 * server has handled the redirect). Useful for tearing down UI state.
 */
export function addAuthCompleteListener(onComplete: () => void): () => void {
  if (!emitter) return () => {};
  const sub = emitter.addListener("onAuthComplete", onComplete);
  return () => sub.remove();
}
