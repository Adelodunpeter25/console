/**
 * Terminal protocol types.
 *
 * The wire protocol (spawn params, server→client frames, client→server
 * frames) is defined once in `@console/types` and shared across server,
 * desktop, and mobile. This module re-exports them so mobile code imports
 * from the local types barrel like everything else.
 */
export type {
  TerminalId,
  TerminalSpawnParams,
  TerminalSpawnedEvent,
  TerminalOutputEvent,
  TerminalExitEvent,
  TerminalErrorEvent,
  TerminalServerMessage,
  TerminalInputMessage,
  TerminalResizeMessage,
  TerminalKillMessage,
  TerminalClientMessage,
} from "@console/types";
