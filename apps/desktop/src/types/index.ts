/**
 * Desktop-specific response types — barrel export.
 *
 * These types mirror the `data` payloads of the Console server routes that
 * are not already covered by the shared `@console/types` package (DTOs,
 * `AuthStatusResponse`, `FsTreeEntry`, `ProjectInfo`, `SessionHeader`, etc.
 * all come from there).
 */

export * from "./auth";
export * from "./fs";
export * from "./providers";
