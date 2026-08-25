/**
 * Multi-call binary entry point.
 *
 * One compiled `console` executable serves two roles:
 *   - default:        the management CLI (start/stop/status/logs/restart)
 *   - CONSOLE_SERVE=1: the agent server itself
 *
 * `console start` re-executes this same binary detached with CONSOLE_SERVE=1
 * so a single installed file can both manage and BE the daemon.
 */
if (process.env.CONSOLE_SERVE === "1") {
  await import("../server/index.js");
} else {
  await import("./index.js");
}
export {};
