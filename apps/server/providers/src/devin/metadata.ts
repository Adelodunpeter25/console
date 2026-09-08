/**
 * Devin (Codeium Cascade) wire constants and IDE metadata.
 *
 * The `chisel`/`devin-cli` identity unlocks the native catalog
 * (`GetCliModelConfigs`) and `AssignModel` routing. Different metadata is
 * used for chat vs discovery — see oh-my-pi's `wire/devin.ts`.
 */

export const DEVIN_STREAMING_BASE_URL = "https://server.codeium.com";

const DEVIN_OS = process.platform === "darwin" ? "darwin" : process.platform === "win32" ? "windows" : "linux";
const DEVIN_LOCALE = "en";

/** Released-CLI identity for chat/auth/assignment calls. */
const DEVIN_CLI_METADATA = {
  ideName: "devin-cli",
  ideType: "chisel",
  ideVersion: "3000.6.2",
  extensionName: "chisel",
  extensionVersion: "3000.6.2",
  locale: DEVIN_LOCALE,
  os: DEVIN_OS,
} as const;

/** Dev-channel identity for `GetCliModelConfigs` discovery. */
const DEVIN_DISCOVERY_METADATA = {
  ideName: "chisel",
  ideVersion: "0.0.0-dev",
  extensionName: "chisel",
  extensionVersion: "0.0.0-dev",
  locale: DEVIN_LOCALE,
  os: DEVIN_OS,
} as const;

/** Session token as carried on the wire: prefix is required. */
export function normalizeDevinSessionToken(apiKey: string | undefined): string {
  if (!apiKey) return "";
  return apiKey.startsWith("devin-session-token$") ? apiKey : `devin-session-token$${apiKey}`;
}

/** Metadata for released-CLI calls. `userJwt` is left empty for auth calls. */
export function devinCliMetadata(apiKey: string | undefined, userJwt = ""): Record<string, string> {
  return {
    apiKey: normalizeDevinSessionToken(apiKey),
    userJwt,
    ...DEVIN_CLI_METADATA,
  };
}

/** Metadata for the dev-channel discovery call. */
export function devinDiscoveryMetadata(apiKey: string | undefined): Record<string, string> {
  return {
    apiKey: normalizeDevinSessionToken(apiKey),
    ...DEVIN_DISCOVERY_METADATA,
  };
}
